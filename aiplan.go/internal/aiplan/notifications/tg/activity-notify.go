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

func (t *TelegramNotification) Handle(activity dao.ActivityI) error {
	if t.Disabled {
		return nil
	}
	var ms TgMsg
	var users []userTg

	var activityTable string
	var activityID uuid.UUID
	var activityAuthorTg int64

	switch a := activity.(type) {
	case dao.IssueActivity:
		if err := t.preloadIssueActivity(&a); err != nil {
			return err
		}
		ms, _ = t.msgt(&a)
		users = getUserTgIdIssueActivity(t.db, &a)
		activityTable = a.TableName()
		activityID = a.Id
		activityAuthorTg = a.ActivitySender.SenderTg

	case dao.ProjectActivity:
		fmt.Println("ProjectActivity", a.Comment)

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

	go func() {
		var msgIds []int64
		for _, u := range users {
			if activityAuthorTg == u.id {
				continue
			}

			if id, err := t.Send(u.id, ms); err != nil {
				slog.Debug("tg send message", "error", err.Error())
				continue
			} else {
				msgIds = append(msgIds, id)
			}
		}

		if err := t.db.Table(activityTable).
			Where("id = ?", activityID).
			Select("telegram_msg_ids").
			Update("telegram_msg_ids", pq.Int64Array(msgIds)).Error; err != nil {
			slog.Error("Update activity tg msg ids", "err", err)
		}
	}()

	return nil
}
