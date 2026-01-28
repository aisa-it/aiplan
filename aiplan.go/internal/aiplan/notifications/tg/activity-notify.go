package tg

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/go-telegram/bot"
	"github.com/gofrs/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

type TelegramNotification struct {
	TgService
}

func NewTelegramNotification(service *TgService) *TelegramNotification {
	if service == nil {
		return nil
	}

	return &TelegramNotification{
		TgService: *service,
	}
}

func (t *TelegramNotification) Handle(activity dao.ActivityI) error {
	if t.Disabled {
		return nil
	}

	var notify *ActivityTgNotification
	var err error

	switch a := activity.(type) {
	case dao.IssueActivity:
		notify, err = notifyFromIssueActivity(t.db, &a)
	case dao.ProjectActivity:
		notify, err = notifyFromProjectActivity(t.db, &a)
	case dao.DocActivity:
		notify, err = notifyFromDocActivity(t.db, &a)
	case dao.FormActivity:
	//	fmt.Println("FormActivity", a.Comment)
	case dao.WorkspaceActivity:
		notify, err = notifyFromWorkspaceActivity(t.db, &a)
	case dao.RootActivity:
	//	fmt.Println("RootActivity", a.Comment)
	case dao.SprintActivity:
		notify, err = notifyFromSprintActivity(t.db, &a)
	default:
		slog.Warn("Unknown activity type for Telegram",
			"type", fmt.Sprintf("%T", activity),
			"entity", activity.GetEntity(),
			"verb", activity.GetVerb())
		return nil
	}

	if err != nil && !errors.Is(err, ErrEmptyActivity) {
		slog.Error("Telegram handle activity", "error", err)
	}

	if notify == nil {
		return nil
	}

	go func() {
		var msgIds []int64

		for _, u := range notify.Users {
			if notify.AuthorTgID == u.id {
				continue
			}

			if id, err := t.Send(u.id, msgReplace(u, notify.Message)); err != nil {
				slog.Error("tg send message", "error", err.Error())
				continue
			} else {
				msgIds = append(msgIds, id)
			}
		}
		if len(msgIds) > 0 {
			if err := t.db.Table(notify.TableName).
				Where("id = ?", notify.ActID).
				Select("telegram_msg_ids").
				Update("telegram_msg_ids", pq.Int64Array(msgIds)).Error; err != nil {
				slog.Error("Update activity tg msg ids", "err", err)
			}
		}

	}()

	return nil
}

type ActivityTgNotification struct {
	Message    TgMsg
	Users      []userTg
	TableName  string
	ActID      uuid.UUID
	AuthorTgID int64
}

func NewActivityTgNotification(tx *gorm.DB, act dao.ActivityI, msg TgMsg, plan NotifyPlan) *ActivityTgNotification {
	var notify ActivityTgNotification
	notify.Message = msg
	notify.Users = getUserTgActivity(tx, act, plan)
	notify.TableName = plan.TableName
	notify.ActID = act.GetId()
	notify.AuthorTgID = plan.ActivitySender
	return &notify
}

type NotifyPlan struct {
	TableName      string
	settings       memberSettings
	ActivitySender int64
	Entity         actField.ActivityField
	AuthorRole     role

	Steps []UsersStep
}

func getUserTgActivity(tx *gorm.DB, act dao.ActivityI, plan NotifyPlan) []userTg {
	users := make(UserRegistry)
	errs := make([]error, 0)

	for _, step := range plan.Steps {
		err := step(tx, act, users)
		if err != nil {
			errs = append(errs, err)
		}
	}

	for _, err := range errs {
		slog.Error("Get user tgActivity", "activityId", act.GetId().String(), "entity", plan.Entity.String(), "err", err)
	}

	if err := plan.settings.Load(tx, plan.settings.EntityID, plan.AuthorRole, users); err != nil {
		slog.Error("Get user tgActivity LoadSettings", "entityId", plan.settings.EntityID, "err", err)
		return []userTg{}
	}

	return users.FilterActivity(act.GetField(), act.GetVerb(), plan.Entity, plan.settings.Notify, plan.AuthorRole)
}

func finalizeActivityTitle(msg TgMsg, actor, entity string, url *url.URL) TgMsg {
	msg.title = fmt.Sprintf(
		"*%s* %s [%s](%s)",
		bot.EscapeMarkdown(actor),
		bot.EscapeMarkdown(msg.title),
		bot.EscapeMarkdown(entity),
		url.String(),
	)
	return msg
}

func formatByField[T dao.ActivityI, F ~func(*T, actField.ActivityField) TgMsg](
	act *T,
	m map[actField.ActivityField]F,
	defaultFn F,
) (TgMsg, error) {
	var res TgMsg

	if (*act).GetField() == "" {
		return res, fmt.Errorf("%s field is nil", (*act).GetEntity())
	}

	af := actField.ActivityField((*act).GetField())

	if f, ok := m[af]; ok {
		res = f(act, af)
	} else if defaultFn != nil {
		res = defaultFn(act, af)
	}

	if res.IsEmpty() {
		return res, fmt.Errorf("%s %w, verb: %s, field: %s, id: %s", (*act).GetEntity(), ErrEmptyActivity, (*act).GetVerb(), (*act).GetField(), (*act).GetId())
	}

	return res, nil
}
