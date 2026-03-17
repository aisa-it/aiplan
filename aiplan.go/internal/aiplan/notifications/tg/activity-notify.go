package tg

import (
	"fmt"
	"net/url"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/go-telegram/bot"
	"github.com/gofrs/uuid"
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

type funcTgMsgFormat func(act *dao.ActivityEvent, af actField.ActivityField) TgMsg

//func (t *TelegramNotification) Handle(activity dao.ActivityEvent) error {
//	if t.Disabled {
//		return nil
//	}
//
//	var notify *ActivityTgNotification
//	var err error
//
//	switch activity.EntityType {
//	case types2.LayerIssue:
//		notify, err = notifyFromIssueActivity(t.db, &activity)
//	case types2.LayerProject:
//		notify, err = notifyFromProjectActivity(t.db, &activity)
//	case types2.LayerDoc:
//		notify, err = notifyFromDocActivity(t.db, &activity)
//	case types2.LayerForm:
//	//	fmt.Println("FormActivity", a.Comment)
//	case types2.LayerWorkspace:
//		notify, err = notifyFromWorkspaceActivity(t.db, &activity)
//	case types2.LayerRoot:
//	//	fmt.Println("RootActivity", a.Comment)
//	case types2.LayerSprint:
//		notify, err = notifyFromSprintActivity(t.db, &activity)
//	default:
//		slog.Warn("Unknown activity type for Telegram",
//			"type", fmt.Sprintf("%T", activity),
//			"entity", activity.EntityType.String(),
//			"verb", activity.Verb)
//		return nil
//	}
//
//	if err != nil && !errors.Is(err, ErrEmptyActivity) {
//		slog.Error("Telegram handle activity", "error", err)
//	}
//
//	if notify == nil || len(notify.Users) == 0 {
//		return nil
//	}
//
//	go func() {
//		var msgIds []int64
//		var invalidTgIds []int64
//
//		for _, u := range notify.Users {
//			if notify.AuthorTgID == u.id {
//				continue
//			}
//
//			if notify.Message.Skip != nil && notify.Message.Skip(u) {
//				continue
//			}
//
//			if id, err := t.Send(u.id, msgReplace(u, notify.Message)); err != nil {
//				if errors.Is(err, ErrInvalidTgId) {
//					invalidTgIds = append(invalidTgIds, u.id)
//					continue
//				}
//				slog.Error("Send telegram message", "error", err.Error(), "activityId", notify.ActID, "table", notify.TableName)
//				continue
//			} else {
//				msgIds = append(msgIds, id)
//			}
//		}
//
//		if len(msgIds) > 0 {
//			records := make([]dao.ActivityTelegramMessage, len(msgIds))
//
//			for i, msgID := range msgIds {
//				records[i] = dao.ActivityTelegramMessage{
//					ID:         dao.GenUUID(),
//					ActivityID: notify.ActID,
//					MessageID:  msgID,
//				}
//			}
//
//			if err := t.db.
//				CreateInBatches(records, 20).
//				Error; err != nil {
//				slog.Error("insert telegram activity messages", "err", err)
//			}
//
//		}
//
//		if len(invalidTgIds) > 0 {
//			if err := t.db.Model(&dao.User{}).Where("telegram_id IN (?)", invalidTgIds).Update("telegram_id", nil).Error; err != nil {
//				slog.Error("Remove dead telegram ids", "err", err)
//			}
//		}
//
//	}()
//
//	return nil
//}

//func (t *TelegramNotification) Handle(activity dao.ActivityI) error {
//	if t.Disabled {
//		return nil
//	}
//
//	var notify *ActivityTgNotification
//	var err error
//
//	switch a := activity.(type) {
//	case dao.IssueActivity:
//		notify, err = notifyFromIssueActivity(t.db, &a)
//	case dao.ProjectActivity:
//		notify, err = notifyFromProjectActivity(t.db, &a)
//	case dao.DocActivity:
//		notify, err = notifyFromDocActivity(t.db, &a)
//	case dao.FormActivity:
//	//	fmt.Println("FormActivity", a.Comment)
//	case dao.WorkspaceActivity:
//		notify, err = notifyFromWorkspaceActivity(t.db, &a)
//	case dao.RootActivity:
//	//	fmt.Println("RootActivity", a.Comment)
//	case dao.SprintActivity:
//		notify, err = notifyFromSprintActivity(t.db, &a)
//	default:
//		slog.Warn("Unknown activity type for Telegram",
//			"type", fmt.Sprintf("%T", activity),
//			"entity", activity.GetEntity(),
//			"verb", activity.GetVerb())
//		return nil
//	}
//
//	if err != nil && !errors.Is(err, ErrEmptyActivity) {
//		slog.Error("Telegram handle activity", "error", err)
//	}
//
//	if notify == nil || len(notify.Users) == 0 {
//		return nil
//	}
//
//	go func() {
//		var msgIds []int64
//		var invalidTgIds []int64
//
//		for _, u := range notify.Users {
//			if notify.AuthorTgID == u.id {
//				continue
//			}
//
//			if notify.Message.Skip != nil && notify.Message.Skip(u) {
//				continue
//			}
//
//			if id, err := t.Send(u.id, msgReplace(u, notify.Message)); err != nil {
//				if errors.Is(err, ErrInvalidTgId) {
//					invalidTgIds = append(invalidTgIds, u.id)
//					continue
//				}
//				slog.Error("Send telegram message", "error", err.Error(), "activityId", notify.ActID, "table", notify.TableName)
//				continue
//			} else {
//				msgIds = append(msgIds, id)
//			}
//		}
//		if len(msgIds) > 0 {
//			if err := t.db.Table(notify.TableName).
//				Where("id = ?", notify.ActID).
//				Select("telegram_msg_ids").
//				Update("telegram_msg_ids", pq.Int64Array(msgIds)).Error; err != nil {
//				slog.Error("Update activity tg msg ids", "err", err)
//			}
//		}
//
//		if len(invalidTgIds) > 0 {
//			if err := t.db.Model(&dao.User{}).Where("telegram_id IN (?)", invalidTgIds).Update("telegram_id", nil).Error; err != nil {
//				slog.Error("Remove dead telegram ids", "err", err)
//			}
//		}
//
//	}()
//
//	return nil
//}

type ActivityTgNotification struct {
	Message    TgMsg
	Users      []userTg
	TableName  string
	ActID      uuid.UUID
	AuthorTgID int64
}

//func NewActivityTgNotification(tx *gorm.DB, act dao.ActivityEvent, msg TgMsg, plan NotifyPlan) *ActivityTgNotification {
//	var notify ActivityTgNotification
//	notify.Message = msg
//	notify.Users = getUserTgActivity(tx, act, plan)
//	notify.TableName = plan.TableName
//	notify.ActID = act.ID
//	notify.AuthorTgID = plan.ActivitySender
//	return &notify
//}

type NotifyPlan struct {
	TableName string
	//settings       memberSettings
	ActivitySender int64
	Entity         actField.ActivityField
	AuthorRole     role

	Steps []UsersStep
}

//func getUserTgActivity(tx *gorm.DB, act dao.ActivityEvent, plan NotifyPlan) []userTg {
//	users := make(UserRegistry)
//	errs := make([]error, 0)
//
//	for _, step := range plan.Steps {
//		err := step(tx, act, users)
//		if err != nil {
//			errs = append(errs, err)
//		}
//	}
//
//	for _, err := range errs {
//		slog.Error("Get user tgActivity", "activityId", act.ID.String(), "entity", plan.Entity.String(), "err", err)
//	}
//
//	if err := plan.settings.Load(tx, plan.settings.EntityID, plan.AuthorRole, users); err != nil {
//		slog.Error("Get user tgActivity LoadSettings", "entityId", plan.settings.EntityID, "err", err)
//		return []userTg{}
//	}
//
//	return users.FilterActivity(act.Field.String(), act.Verb, plan.Entity, plan.settings.Notify, plan.AuthorRole)
//}

func finalizeActivityTitle(msg TgMsg, actor, entity string, url *url.URL) TgMsg {
	msg.Title = fmt.Sprintf(
		"*%s* %s [%s](%s)",
		bot.EscapeMarkdown(actor),
		bot.EscapeMarkdown(msg.Title),
		bot.EscapeMarkdown(entity),
		url.String(),
	)
	return msg
}

func formatByField(
	act *dao.ActivityEvent, m map[actField.ActivityField]funcTgMsgFormat, defaultFn funcTgMsgFormat,
) (TgMsg, error) {
	var res TgMsg

	if f, ok := m[act.Field]; ok {
		res = f(act, act.Field)
	} else if defaultFn != nil {
		res = defaultFn(act, act.Field)
	}

	if res.IsEmpty() {
		return res, fmt.Errorf("%s %w, verb: %s, field: %s, id: %s", act.EntityType.String(), ErrEmptyActivity, act.Verb, act.Field.String(), act.ID)
	}

	return res, nil
}
