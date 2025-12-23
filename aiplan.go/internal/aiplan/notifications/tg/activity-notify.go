package tg

import (
	"fmt"
	"log/slog"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/gofrs/uuid"
	"github.com/lib/pq"
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

type ActivityTgNotification struct {
	Message    TgMsg
	Users      []userTg
	TableName  string
	EntityID   uuid.UUID
	AuthorTgID int64
}

func (t *TelegramNotification) Handle(activity dao.ActivityI) error {
	if t.Disabled {
		return nil
	}

	var notify *ActivityTgNotification

	switch a := activity.(type) {
	case dao.IssueActivity:
		notify = notifyFromIssueActivity(t.db, &a)
	case dao.ProjectActivity:
		notify = notifyFromProjectActivity(t.db, &a)
	case dao.DocActivity:
		fmt.Println("DocActivity", a.Comment)

	case dao.FormActivity:
		fmt.Println("FormActivity", a.Comment)

	case dao.WorkspaceActivity:
		fmt.Println("WorkspaceActivity", a.Comment)

	case dao.RootActivity:
		fmt.Println("RootActivity", a.Comment)

	case dao.SprintActivity:
		fmt.Println("SprintActivity", a.Comment)

	default:
		slog.Warn("Unknown activity type for Telegram",
			"type", fmt.Sprintf("%T", activity),
			"entity", activity.GetEntity(),
			"verb", activity.GetVerb())
		return nil
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
				slog.Debug("tg send message", "error", err.Error())
				continue
			} else {
				msgIds = append(msgIds, id)
			}
		}
		if len(msgIds) > 0 {
			if err := t.db.Table(notify.TableName).
				Where("id = ?", notify.EntityID).
				Select("telegram_msg_ids").
				Update("telegram_msg_ids", pq.Int64Array(msgIds)).Error; err != nil {
				slog.Error("Update activity tg msg ids", "err", err)
			}
		}

	}()

	return nil
}
