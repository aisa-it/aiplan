package notifications

import (
	"fmt"
	"log/slog"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	member_role "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications/member-role"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications/tg"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

const batchSize = 30

var (
	handlerMap = map[types.EntityLayer]EventHandler{
		types.LayerIssue:     issueEvent{},
		types.LayerProject:   projectEvent{},
		types.LayerSprint:    sprintEvent{},
		types.LayerWorkspace: workspaceEvent{},
		types.LayerDoc:       docEvent{},
	}
)

type EventNotificationService struct {
	db        *gorm.DB
	tgService *tg.TgService
	wsService *WebsocketNotificationService
	emailSvc  *EmailService

	channels []NotificationChannel
}

func NewEventNotificationService(ns *Notification) *EventNotificationService {

	channels := []NotificationChannel{
		newTgChannel(ns.Db, ns.Tg),
		newWsChannel(ns.Db, ns.Ws),
	}

	return &EventNotificationService{
		db:        ns.Db,
		tgService: ns.Tg,
		wsService: ns.Ws,
		//emailSvc:  ns.,

		channels: channels,
	}
}

// EventHandler интерфейс для обработчиков конкретных типов событий.
type EventHandler interface {
	CanHandle(event *dao.ActivityEvent) bool
	Preload(tx *gorm.DB, event *dao.ActivityEvent) error
	GetRecipientsSteps(event *dao.ActivityEvent) []member_role.UsersStep
	GetSettingsFunc() member_role.IsNotifyFunc
	AuthorRole() member_role.Role
	FilterRecipients(event *dao.ActivityEvent, recipients []member_role.MemberNotify) []member_role.MemberNotify
}

type NotificationChannel interface {
	ChannelType() types.NotifyChannel
	NewDelivery(event *dao.ActivityEvent, userIds []uuid.UUID) Delivery
}

type Delivery interface {
	CanSend(recipient member_role.MemberNotify) bool
	Send(recipient *member_role.MemberNotify) error
	Commit(tx *gorm.DB) error
}

func (np *EventNotificationService) Handle(activity dao.ActivityEvent) error {
	event := &activity

	steps := []member_role.UsersStep{
		member_role.AddUserRole(event.Actor, member_role.ActionAuthor),
	}

	eh := getHandler(event.EntityType)
	if eh == nil {
		return fmt.Errorf("unknown event type: %s", event.EntityType)
	}

	if err := eh.Preload(np.db, event); err != nil {
		return err
	}

	recipients := BuildRecipientsFromActivity(np.db, event, append(steps, eh.GetRecipientsSteps(event)...))
	recipients = eh.FilterRecipients(event, recipients)
	if len(recipients) == 0 {
		slog.Debug("no recipients for event", "eventID", event.ID)
		return nil
	}

	np.sendNotifications(event, recipients, eh)

	return nil
}

func (np *EventNotificationService) sendNotifications(
	event *dao.ActivityEvent, recipients []member_role.MemberNotify, eh EventHandler) {
	for _, ch := range np.channels {
		if delivery := ch.NewDelivery(event, utils.SliceToSlice(&recipients, func(t *member_role.MemberNotify) uuid.UUID {
			return t.GetUser().ID
		})); delivery != nil {
			for _, r := range recipients {
				if !delivery.CanSend(r) {
					continue
				}
				// пользовательские настройки уведомлений для текущего слоя и текущего канала доставки
				if !eh.GetSettingsFunc()(r, event, r.Has(eh.AuthorRole()), ch.ChannelType()) {
					continue
				}
				_ = delivery.Send(&r)
			}
			_ = delivery.Commit(np.db) // batch insert||async send
		}
	}
}

func BuildRecipientsFromActivity(
	tx *gorm.DB, act *dao.ActivityEvent, steps []member_role.UsersStep,
) []member_role.MemberNotify {

	users := make(member_role.UserRegistry)

	for _, step := range steps {
		if err := step(tx, users); err != nil {
			slog.Error("recipient step failed", "err", err)
		}
	}

	if act.WorkspaceID.Valid {
		err := member_role.LoadWorkspaceSettings(tx, act.WorkspaceID.UUID, users)
		if err != nil {
			return []member_role.MemberNotify{}
		}
	}

	if act.ProjectID.Valid {
		err := member_role.LoadProjectSettings(tx, act.ProjectID.UUID, users)
		if err != nil {
			return []member_role.MemberNotify{}
		}
	}

	return utils.MapToSlice(users, func(k uuid.UUID, t *member_role.MemberNotify) member_role.MemberNotify {
		return *t
	})
}

func getHandler(t types.EntityLayer) EventHandler {
	if v, ok := handlerMap[t]; ok {
		return v
	}
	return nil
}
