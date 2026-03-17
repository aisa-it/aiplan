package tg

import (
	"fmt"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
)

var (
	sprintMap = map[actField.ActivityField]funcTgMsgFormat{
		actField.Watchers.Field:    sprintWatchers,
		actField.Issue.Field:       sprintIssues,
		actField.Description.Field: sprintDescription,

		actField.StartDate.Field: sprintDate,
		actField.EndDate.Field:   sprintDate,
	}
)

func FormatSprintActivity(act *dao.ActivityEvent) (TgMsg, error) {
	res, err := formatByField(act, sprintMap, sprintDefault)
	if err != nil {
		return res, err
	}

	entity := fmt.Sprintf("%s/%s", act.Workspace.Slug, act.Sprint.Name)

	return finalizeActivityTitle(res, act.Actor.GetName(), entity, act.Sprint.URL), nil
}

func sprintDefault(act *dao.ActivityEvent, af actField.ActivityField) TgMsg {
	return genDefault(act.OldValue, act.NewValue, af, "изменил(-a) в спринте")
}

func sprintWatchers(act *dao.ActivityEvent, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	switch act.Verb {
	case actField.VerbAdded:
		msg.Title = "добавил(-a) наблюдателя в спринт"
		msg.Body = Stelegramf("__%s__", getUserName(act.NewSprintWatcher))
	case actField.VerbRemoved:
		msg.Title = "убрал(-a) наблюдателя из спринта"
		msg.Body = Stelegramf("~%s~", getUserName(act.OldSprintWatcher))
	}
	return msg
}

func sprintIssues(act *dao.ActivityEvent, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	switch act.Verb {
	case actField.VerbAdded:
		msg.Title = "добавил(-a) задачу в спринт"
		msg.Body = Stelegramf("[%s](%s)", act.NewSprintIssue.FullIssueName(), act.NewSprintIssue.URL.String())
	case actField.VerbRemoved:
		msg.Title = "убрал(-a) задачу из спринта"
		msg.Body = Stelegramf("[~%s~](%s)", act.OldSprintIssue.FullIssueName(), act.OldSprintIssue.URL.String())
	}
	return msg
}

func sprintDescription(act *dao.ActivityEvent, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	msg.Title = "изменил(-а) описание спринта"
	msg.Body = Stelegramf("```\n%s```", utils.HtmlToTg(act.NewValue))
	return msg
}

func sprintDate(act *dao.ActivityEvent, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	msg.Title = "изменил(-a) в спринте"
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

	msg.Body = Stelegramf(format, values...)
	return msg
}
