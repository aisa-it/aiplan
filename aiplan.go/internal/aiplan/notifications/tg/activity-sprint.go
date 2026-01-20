package tg

import (
	"fmt"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"gorm.io/gorm"
)

type funcSprintMsgFormat func(act *dao.SprintActivity, af actField.ActivityField) TgMsg

var (
	sprintMap = map[actField.ActivityField]funcSprintMsgFormat{
		actField.Watchers.Field:    sprintWatchers,
		actField.Issue.Field:       sprintIssues,
		actField.Description.Field: sprintDescription,

		actField.StartDate.Field: sprintDate,
		actField.EndDate.Field:   sprintDate,
	}
)

func notifyFromSprintActivity(tx *gorm.DB, act *dao.SprintActivity) (*ActivityTgNotification, error) {
	if act.Field == nil {
		return nil, nil
	}

	if err := preloadSprintActivity(tx, act); err != nil {
		return nil, err
	}

	msg, err := formatSprintActivity(act)
	if err != nil {
		return nil, fmt.Errorf("formatSprintActivity: %w", err)
	}

	steps := []UsersStep{
		addUserRole(act.Actor, actionAuthor),
		addUserRole(&act.Sprint.CreatedBy, sprintAuthor),
		addUsers(act.Sprint.Watchers, sprintWatcher),
	}

	plan := NotifyPlan{
		TableName:      act.TableName(),
		settings:       fromWorkspace(act.WorkspaceId),
		ActivitySender: act.ActivitySender.SenderTg,
		Entity:         actField.Sprint.Field,
		AuthorRole:     sprintAuthor,
		Steps:          steps,
	}

	return NewActivityTgNotification(tx, act, msg, plan), nil
}

func preloadSprintActivity(tx *gorm.DB, act *dao.SprintActivity) error {
	if err := tx.Unscoped().
		Joins("Workspace").
		Joins("CreatedBy").
		Preload("Watchers").
		Where("sprints.id = ?", act.SprintId).
		First(&act.Sprint).Error; err != nil {
		return fmt.Errorf("preloadSprintActivity: %v", err)
	}

	if act.NewSprintIssue != nil {
		act.NewSprintIssue = preloadIssueActivity(tx, act.NewSprintIssue.ID)
	}
	if act.OldSprintIssue != nil {
		act.OldSprintIssue = preloadIssueActivity(tx, act.OldSprintIssue.ID)
	}

	act.Workspace = act.Sprint.Workspace

	return nil
}

func formatSprintActivity(act *dao.SprintActivity) (TgMsg, error) {
	res, err := formatByField(act, sprintMap, sprintDefault)
	if err != nil {
		return res, err
	}

	entity := fmt.Sprintf("%s/%s", act.Workspace.Slug, act.Sprint.Name)

	return finalizeActivityTitle(res, act.Actor.GetName(), entity, act.Sprint.URL), nil
}

func sprintDefault(act *dao.SprintActivity, af actField.ActivityField) TgMsg {
	return genDefault(act.OldValue, act.NewValue, af, "изменил(-a) в спринте")
}

func sprintWatchers(act *dao.SprintActivity, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	switch act.Verb {
	case actField.VerbAdded:
		msg.title = "добавил(-a) наблюдателя в спринт"
		msg.body = Stelegramf("__%s__", getUserName(act.NewSprintWatcher))
	case actField.VerbRemoved:
		msg.title = "убрал(-a) наблюдателя из спринта"
		msg.body = Stelegramf("~%s~", getUserName(act.OldSprintWatcher))
	}
	return msg
}

func sprintIssues(act *dao.SprintActivity, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	switch act.Verb {
	case actField.VerbAdded:
		msg.title = "добавил(-a) задачу в спринт"
		msg.body = Stelegramf("[%s](%s)", act.NewSprintIssue.FullIssueName(), act.NewSprintIssue.URL.String())
	case actField.VerbRemoved:
		msg.title = "убрал(-a) задачу из спринта"
		msg.body = Stelegramf("[~%s~](%s)", act.OldSprintIssue.FullIssueName(), act.OldSprintIssue.URL.String())
	}
	return msg
}

func sprintDescription(act *dao.SprintActivity, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	msg.title = "изменил(-а) описание спринта"
	msg.body = Stelegramf("```\n%s```", utils.HtmlToTg(act.NewValue))
	return msg
}

func sprintDate(act *dao.SprintActivity, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	msg.title = "изменил(-a) в спринте"
	format := "*%s*: "
	values := []any{types.FieldsTranslation[af]}

	if act.OldValue != nil && *act.OldValue != "<nil>" {
		format += "~%s~ "
		str, err := utils.FormatDateStr(*act.OldValue, "02.01.2006", nil)
		if err != nil {
			return TgMsg{}
		}
		values = append(values, str)
	}

	if act.NewValue != "" && act.NewValue != "<nil>" {
		format += "%s "
		str, err := utils.FormatDateStr(act.NewValue, "02.01.2006", nil)
		if err != nil {
			return TgMsg{}
		}
		values = append(values, str)
	}

	msg.body = Stelegramf(format, values...)
	return msg
}
