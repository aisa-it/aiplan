package tg

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
)

var (
	projectMap = map[actField.ActivityField]funcTgMsgFormat{
		actField.Issue.Field:            projectIssue,
		actField.Template.Field:         projectTemplate,
		actField.TemplateTemplate.Field: projectTemplate,
		actField.TemplateName.Field:     projectTemplate,

		actField.Status.Field:            projectStatus,
		actField.StatusName.Field:        projectStatus,
		actField.StatusDescription.Field: projectStatus,
		actField.StatusColor.Field:       projectStatus,
		actField.StatusDefault.Field:     projectStatus,
		actField.StatusGroup.Field:       projectStatus,

		actField.Label.Field:      projectLabel,
		actField.LabelColor.Field: projectLabel,
		actField.LabelName.Field:  projectLabel,

		actField.Member.Field:      projectMember,
		actField.Role.Field:        projectRole,
		actField.ProjectLead.Field: projectLead,

		actField.DefaultAssignees.Field: projectDefaultMember,
		actField.DefaultWatchers.Field:  projectDefaultMember,

		actField.Public.Field: projectPublic,
		actField.Logo.Field:   projectLogo,
		actField.Emoj.Field:   projectEmoj,
	}
)

func FormatProjectActivity(act *dao.ActivityEvent) (TgMsg, error) {
	res, err := formatByField(act, projectMap, projectDefault)
	if err != nil {
		return res, err
	}

	entity := fmt.Sprintf("%s/%s", act.Workspace.Slug, act.Project.Identifier)

	return finalizeActivityTitle(res, act.Actor.GetName(), entity, act.Project.URL), nil
}

var issueRolesNotified = []role{actionAuthor, issueAuthor, projectDefaultWatcher, issueWatcher, issueAssigner}

func projectIssue(act *dao.ActivityEvent, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()

	switch act.Verb {
	case actField.VerbCreated:
		msg.Title = "создал(-a) задачу в проекте"
	case actField.VerbAdded:
		msg.Title = "добавил(-a) задачу в проект"
	case actField.VerbCopied:
		msg.Title = "создал(-a) копию задачи в проекте"
	case actField.VerbRemoved:
		msg.Title = "убрал(-a) задачу из"
		msg.Body = Stelegramf("*Задача:* %s", fmt.Sprint(*act.OldValue))
		return msg
	case actField.VerbDeleted:
		msg.Title = "удалил(-a) задачу из"
		msg.Body = Stelegramf("*Задача:* %s", fmt.Sprint(*act.OldValue))
		return msg
	}

	format := "[%s](%s)"
	values := []any{act.NewIssue.FullIssueName(), act.NewIssue.URL.String()}

	if act.NewIssue.Parent != nil {
		issue := act.NewIssue.Parent
		act.NewIssue.Parent.SetUrl()
		format += "\n*%s*: [%s](%s)"
		values = append(values, types.FieldsTranslation[actField.Parent.Field], issue.FullIssueName(), issue.URL.String())
	}

	if act.NewIssue.Priority != nil {
		format += "\n*%s*: %s"
		values = append(values, types.FieldsTranslation[actField.Priority.Field], types.PriorityTranslation[*act.NewIssue.Priority])
	}

	if act.NewIssue.Assignees != nil && len(*act.NewIssue.Assignees) > 0 {
		var assignees []string
		for _, assignee := range *act.NewIssue.Assignees {
			assignees = append(assignees, getUserName(&assignee))
		}
		format += "\n*Исполнители*: %s"
		values = append(values, strings.Join(assignees, ", "))
	}

	if act.NewIssue.Links != nil && len(*act.NewIssue.Links) > 0 {
		format += "\n*Ссылки*: "
		first := true
		for _, link := range *act.NewIssue.Links {
			if !first {
				format += ", "
			}
			format += "[%s](%s)"
			values = append(values, link.Title, link.Url)
			first = false
		}
	}

	msg.Body += Stelegramf(format, values...)
	return msg
}

func projectTemplate(act *dao.ActivityEvent, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	format := "*Название*: "
	var values []any

	switch act.Verb {
	case actField.VerbCreated:
		msg.Title = "создал(-a) шаблон задачи в"
		format += "%s\n```\n%s```"
		values = append(values, act.NewIssueTemplate.Name, utils.HtmlToTg(act.NewIssueTemplate.Template.Body))
	case actField.VerbUpdated:
		msg.Title = "изменил(-a) шаблон задачи в"
		switch af {
		case actField.TemplateTemplate.Field:
			format += "%s\n```\n%s```"
			values = append(values, act.NewIssueTemplate.Name, utils.HtmlToTg(act.NewIssueTemplate.Template.Body))
		case actField.TemplateName.Field:
			format += "~%s~ %s"
			values = append(values, fmt.Sprint(*act.OldValue), act.NewValue)
		default:
			if act.NewIssueTemplate != nil {
				format += "%s\n```\n%s```"
				values = append(values, act.NewIssueTemplate.Name, utils.HtmlToTg(act.NewIssueTemplate.Template.Body))
			}
		}
	case actField.VerbDeleted:
		if act.OldValue == nil {
			return msg
		}
		msg.Title = "удалил(-a) из"
		format = "*Шаблон*: ~%s~"
		values = []any{fmt.Sprintf(*act.OldValue)}
	}

	msg.Body += Stelegramf(format, values...)
	return msg
}

func projectStatus(act *dao.ActivityEvent, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()

	var format string
	var values []any
	sef := act.ProjectActivityExtendFields.StateExtendFields
	if sef.NewState != nil {
		format = "__Статус %s__"
		values = append(values, sef.NewState.Name)
	}

	switch af {
	case actField.StatusGroup.Field:
		format += "\n*Группу Статуса:* ~%s~ %s"
		values = append(values, types.TranslateMap(types.StatusTranslation, act.OldValue), types.TranslateMap(types.StatusTranslation, &act.NewValue))
	case actField.StatusDescription.Field:
		format += "\n```\n%s```"
		values = append(values, utils.HtmlToTg(act.NewValue))
	case actField.StatusColor.Field:
		format += "\nизменен цвет"
	case actField.StatusName.Field:
		format += "\n*Название:* ~%s~ %s"
		values = append(values, fmt.Sprint(*act.OldValue), act.NewValue)
	case actField.StatusDefault.Field:
		format = "*статус по умолчанию:* ~%s~ %s"
		values = []any{sef.OldState.Name, sef.NewState.Name}
	}

	switch act.Verb {
	case actField.VerbCreated:
		msg.Title = "создал(-a) статус в"
		format = "*Название*: %s\n*Группа*: %s"
		values = []any{sef.NewState.Name, types.TranslateMap(types.StatusTranslation, &sef.NewState.Group)}
	case actField.VerbUpdated:
		msg.Title = "изменил(-a) в"
	case actField.VerbDeleted:
		msg.Title = "удалил(-a) из"
		format = "*Статус*: ~%s~"
		values = []any{fmt.Sprint(*act.OldValue)}
	}

	msg.Body += Stelegramf(format, values...)
	return msg
}

func projectLabel(act *dao.ActivityEvent, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	var format string
	var values []any
	lef := act.ProjectActivityExtendFields.LabelExtendFields
	if lef.NewLabel != nil {
		format = "__Тег %s__"
		values = append(values, lef.NewLabel.Name)
	}

	switch af {
	case actField.LabelColor.Field:
		format += "\nизменен цвет"

	case actField.LabelName.Field:
		format += "\n*Название:* ~%s~ %s"
		values = append(values, fmt.Sprint(*act.OldValue), act.NewValue)
	}

	switch act.Verb {
	case actField.VerbCreated:
		msg.Title = "создал(-a) тег в"
		format = "*Название*: %s"
		values = []any{lef.NewLabel.Name}
	case actField.VerbUpdated:
		msg.Title = "изменил(-a) в"
	case actField.VerbDeleted:
		msg.Title = "удалил(-a) из"
		format = "*Тег*: ~%s~"
		values = []any{fmt.Sprint(*act.OldValue)}
	}

	msg.Body += Stelegramf(format, values...)
	return msg
}

func projectMember(act *dao.ActivityEvent, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	var format string
	var values []any

	switch act.Verb {
	case actField.VerbAdded:
		msg.Title = "добавил(-a) участника в"
		format = "__%s__\n*Роль:* %s"
		values = []any{getUserName(act.NewMember), types.TranslateMap(types.RoleTranslation, &act.NewValue)}
	case actField.VerbRemoved:
		msg.Title = "убрал(-a) участника из"
		format = "~%s~"
		values = []any{getUserName(act.OldMember)}
	}
	msg.Body = Stelegramf(format, values...)
	return msg
}

func projectRole(act *dao.ActivityEvent, af actField.ActivityField) TgMsg {
	if act.Verb != actField.VerbUpdated && act.NewRole == nil {
		return NewTgMsg()
	}
	msg := NewTgMsg()
	msg.Title = "изменил(-a) роль пользователя в"
	msg.Body = Stelegramf("__%s__\n*Роль*: ~%s~ %s", getUserName(act.NewRole), types.TranslateMap(types.RoleTranslation, act.OldValue), types.TranslateMap(types.RoleTranslation, &act.NewValue))
	return msg
}

func projectLead(act *dao.ActivityEvent, af actField.ActivityField) TgMsg {
	if act.Verb != actField.VerbUpdated && act.NewProjectLead == nil {
		return NewTgMsg()
	}
	msg := NewTgMsg()
	msg.Title = "изменил(-a) лидера проекта в"
	msg.Body = Stelegramf("~%s~ %s", getUserName(act.OldProjectLead), getUserName(act.NewProjectLead))
	return msg
}

func projectPublic(act *dao.ActivityEvent, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	if act.Verb != actField.VerbUpdated {
		return msg
	}
	if act.NewValue == "true" {
		msg.Title = "сделал(-a) публичным"
	} else {
		msg.Title = "сделал(-a) приватным"
	}
	return msg
}

func projectLogo(act *dao.ActivityEvent, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	if act.Verb != actField.VerbUpdated {
		return msg
	}
	msg.Title = "изменил(-a) логотип в проекте"
	return msg
}

func projectDefaultMember(act *dao.ActivityEvent, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	user := utils.GetFirstOrNil(act.NewDefaultWatcher, act.NewDefaultAssignee, act.OldDefaultWatcher, act.OldDefaultAssignee)
	if user == nil {
		return msg
	}
	var role string
	switch af {
	case actField.DefaultWatchers.Field:
		role = "наблюдателя"
	case actField.DefaultAssignees.Field:
		role = "исполнителя"
	}

	switch act.Verb {
	case actField.VerbAdded:
		msg.Title = fmt.Sprintf("добавил(-a) %s по умолчанию в", role)
		msg.Body = Stelegramf("%s", getUserName(user))
	case actField.VerbRemoved:
		msg.Title = fmt.Sprintf("убрал(-a) %s по умолчанию из", role)
		msg.Body = Stelegramf("~%s~", getUserName(user))

	}
	return msg
}

func projectDefault(act *dao.ActivityEvent, af actField.ActivityField) TgMsg {
	return genDefault(act.OldValue, act.NewValue, af, "изменил(-a) в")
}

func projectEmoj(act *dao.ActivityEvent, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	msg.Title = "изменил(-a) в"
	msg.Body = Stelegramf("*%s*: ~%s~ %s", "Emoji", emojiFromCode(fmt.Sprint(*act.OldValue)), emojiFromCode(act.NewValue))
	return msg
}

func emojiFromCode(code string) string {
	i, err := strconv.Atoi(code)
	if err != nil {
		return ""
	}
	return string(rune(i))
}
