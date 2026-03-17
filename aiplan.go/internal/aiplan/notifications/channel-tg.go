package notifications

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	member_role "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications/member-role"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications/tg"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

type tgChannel struct {
	db        *gorm.DB
	tgService *tg.TgService
}

func newTgChannel(db *gorm.DB, service *tg.TgService) *tgChannel {
	return &tgChannel{
		db:        db,
		tgService: service,
	}
}

func (t *tgChannel) ChannelType() types.NotifyChannel {
	return types.TgCh
}

func (t *tgChannel) NewDelivery(event *dao.ActivityEvent, userIds []uuid.UUID) Delivery {

	msg, _ := formatMsg(event)
	if msg.IsEmpty() {
		return nil
	}

	return &tgDelivery{
		db:        t.db,
		tgService: t.tgService,
		event:     event,
		msg:       msg,
		authorTg:  event.SenderTg,
	}
}

type tgDelivery struct {
	db        *gorm.DB
	tgService *tg.TgService

	event *dao.ActivityEvent
	msg   tg.TgMsg

	authorTg int64

	users []member_role.MemberNotify
}

func (d *tgDelivery) CanSend(r member_role.MemberNotify) bool {

	tgID := r.GetUser().TelegramId
	if tgID == nil || r.GetUser().Settings.TgNotificationMute || d.authorTg == *tgID {
		return false
	}

	return true
}

func (d *tgDelivery) Send(r *member_role.MemberNotify) error {
	d.users = append(d.users, *r)
	return nil
}

func (d *tgDelivery) Commit(tx *gorm.DB) error {
	if len(d.users) == 0 {
		return nil
	}

	go d.asyncSend()

	return nil
}

func (d *tgDelivery) asyncSend() {
	msgIds := make([]int64, 0, len(d.users))
	var invalidTgIds []int64

	for _, u := range d.users {
		id, err := d.tgService.Send(*u.GetUser().TelegramId, tg.MsgReplace(u, d.msg))

		if err != nil {
			if errors.Is(err, tg.ErrInvalidTgId) {
				invalidTgIds = append(invalidTgIds, *u.GetUser().TelegramId)
				continue
			}

			slog.Error("Send telegram message", "error", err.Error(), "activityId", d.event.ID)
			continue
		}

		msgIds = append(msgIds, id)
	}

	d.persistResults(msgIds, invalidTgIds)
}

func (d *tgDelivery) persistResults(msgIds []int64, invalidTgIds []int64) {
	if len(msgIds) > 0 {
		records := make([]dao.ActivityTelegramMessage, len(msgIds))
		for i, msgID := range msgIds {
			records[i] = dao.ActivityTelegramMessage{
				ID:         dao.GenUUID(),
				ActivityID: d.event.ID,
				MessageID:  msgID,
			}
		}

		if err := d.db.CreateInBatches(records, 20).
			Error; err != nil {
			slog.Error("insert telegram activity messages", "err", err)
		}
	}

	if len(invalidTgIds) > 0 {

		if err := d.db.
			Model(&dao.User{}).Where("telegram_id IN ?", invalidTgIds).Update("telegram_id", nil).
			Error; err != nil {

			slog.Error("remove dead telegram ids", "err", err)
		}
	}
}

func formatMsg(event *dao.ActivityEvent) (tg.TgMsg, error) {
	switch event.EntityType {
	case types.LayerIssue:
		return tg.FormatIssueActivity(event)
	case types.LayerProject:
		return tg.FormatProjectActivity(event)
	case types.LayerWorkspace:
		return tg.FormatWorkspaceActivity(event)
	case types.LayerSprint:
		return tg.FormatSprintActivity(event)
	case types.LayerDoc:
		return tg.FormatDocActivity(event)
	case types.LayerForm:
		return tg.NewTgMsg(), fmt.Errorf("unknown event type: %s", event.EntityType)
	case types.LayerRoot:
		return tg.NewTgMsg(), fmt.Errorf("unknown event type: %s", event.EntityType)
	default:
		return tg.NewTgMsg(), fmt.Errorf("unknown event type: %s", event.EntityType)
	}
}
