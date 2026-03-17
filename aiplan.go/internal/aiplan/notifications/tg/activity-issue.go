package tg

import (
	"fmt"
	"strings"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
)

var (
	issueMap = map[actField.ActivityField]funcTgMsgFormat{
		actField.StartDate.Field: issueSkipper,
		actField.EndDate.Field:   issueSkipper,

		actField.Comment.Field:     issueComment,
		actField.Description.Field: issueDescription,
		actField.Attachment.Field:  issueAttachment,
		actField.Link.Field:        issueLink,
		actField.LinkTitle.Field:   issueLink,
		actField.LinkUrl.Field:     issueLink,
		actField.Assignees.Field:   issueAssignees,
		actField.Watchers.Field:    issueWatchers,
		actField.Linked.Field:      issueLinked,
		actField.Label.Field:       issueTag,
		actField.SubIssue.Field:    issueSubIssue,
		actField.Parent.Field:      issueParent,
		actField.TargetDate.Field:  issueTargetDate,
		actField.Project.Field:     issueProject,
		actField.Sprint.Field:      issueSprint,
	}
)

func FormatIssueActivity(act *dao.ActivityEvent) (TgMsg, error) {

	res, err := formatByField(act, issueMap, issueDefault)
	if err != nil {
		return res, err
	}

	return finalizeActivityTitle(res, act.Actor.GetName(), act.Issue.FullIssueName(), act.Issue.URL), nil
}

func issueSkipper(act *dao.ActivityEvent, af actField.ActivityField) TgMsg {
	return NewTgMsg()
}

func issueDefault(act *dao.ActivityEvent, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()

	oldValue := ""
	newValue := act.NewValue
	if act.OldValue != nil && *act.OldValue != "<nil>" {
		oldValue = *act.OldValue
	}

	if oldValue == "<p></p>" {
		oldValue = ""
	}
	msg.Title = "изменил(-a)"

	if af == actField.Priority.Field {
		oldValue = types.TranslateMap(types.PriorityTranslation, act.OldValue)
		newValue = types.TranslateMap(types.PriorityTranslation, &act.NewValue)
	}

	if oldValue != "" {
		msg.Body += Stelegramf("*%s*: ~%s~ %s",
			types.FieldsTranslation[af],
			oldValue,
			newValue,
		)
	} else {
		msg.Body += Stelegramf("*%s*: %s",
			types.FieldsTranslation[af],
			newValue,
		)
	}
	return msg

}

func issueDescription(act *dao.ActivityEvent, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	if act.Verb != actField.VerbUpdated {
		return msg
	}

	msg.Title = "изменил(-а) описание"
	msg.Body = Stelegramf("```\n%s```",
		utils.HtmlToTg(act.NewValue),
	)
	return msg
}

func issueComment(act *dao.ActivityEvent, af actField.ActivityField) TgMsg {
	return genComment(act.NewIssueComment, act.OldValue, act.Verb,
		"изменил(-a) комментарий",
		"прокомментировал(-a)",
		"удалил(-a) комментарий из")
}

func issueAttachment(act *dao.ActivityEvent, af actField.ActivityField) TgMsg {
	return genAttachment(act.OldValue, act.NewValue, act.Verb,
		"добавил(-a) вложение в",
		"удалил(-a) вложение из",
	)
}

func issueLinked(act *dao.ActivityEvent, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	var targetIssue dao.Issue

	switch act.Verb {
	case actField.VerbUpdated:
		if !act.OldIdentifier.Valid && act.NewIdentifier.Valid {
			msg.Title = "добавил(-а) связь к"
			targetIssue = *act.NewIssueLinked
		}
		if !act.NewIdentifier.Valid && act.OldIdentifier.Valid {
			msg.Title = "убрал(-а) связь из"
			targetIssue = *act.OldIssueLinked
		}

		targetIssue.Project = act.Issue.Project

		msg.Body = Stelegramf("*Задача*: [%s](%s)",
			targetIssue.FullIssueName(),
			targetIssue.URL,
		)
	}

	return msg
}

func issueSubIssue(act *dao.ActivityEvent, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	subIssue := utils.GetFirstOrNil(act.NewSubIssue, act.OldSubIssue)
	if subIssue == nil {
		return msg
	}
	msg.Title = "изменил(-a)"
	format := "*Подзадача*: "

	switch act.Verb {
	case actField.VerbAdded:
		format += "[%s](%s)"
	case actField.VerbRemoved:
		format += " ~[%s](%s)~"
	default:
		return NewTgMsg()
	}

	msg.Body += Stelegramf(format, subIssue.FullIssueName(), subIssue.URL)
	return msg
}

func issueLink(act *dao.ActivityEvent, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()

	format := "[%s](%s)"

	switch af {
	case actField.LinkTitle.Field:
		msg.Title = "изменил(-a) название ссылки"
	case actField.LinkUrl.Field:
		msg.Title = "изменил(-a) url ссылки"
	}

	var values []any

	link := act.NewLink
	if link != nil {
		values = append(values, link.Title, link.Url)
	}

	switch act.Verb {
	case actField.VerbCreated:
		msg.Title = "добавил(-a) ссылку в"
	case actField.VerbDeleted:
		msg.Title = "удалил(-a) ссылку из"
		format = ""
	case actField.VerbUpdated:
		if act.OldValue != nil {
			format = "~%s~ " + format
			if af == actField.LinkUrl.Field {
				values = append([]any{*act.OldValue}, values[1], values[1])
			} else {
				values = append([]any{*act.OldValue}, values...)
			}
		}
	}

	msg.Body = Stelegramf(format, values...)
	return msg
}

func issueAssignees(act *dao.ActivityEvent, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()

	switch act.Verb {
	case actField.VerbRemoved:
		msg.Title = "убрал(-а) исполнителя из"
		if act.OldAssignee != nil {
			msg.Body = Stelegramf("%s",
				act.OldAssignee.GetName(),
			)
		}
	case actField.VerbAdded:
		msg.Title = "добавил(-a) нового исполнителя в"
		if act.NewAssignee != nil {
			msg.Body = Stelegramf("%s",
				act.NewAssignee.GetName(),
			)
		}
	}
	return msg
}

func issueWatchers(act *dao.ActivityEvent, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()

	switch act.Verb {
	case actField.VerbRemoved:
		msg.Title = "убрал(-а) наблюдателя из"
		if act.OldWatcher != nil {
			msg.Body = Stelegramf("%s",
				act.OldWatcher.GetName(),
			)
		}
	case actField.VerbAdded:
		msg.Title = "добавил(-a) нового наблюдателя в"
		if act.NewWatcher != nil {
			msg.Body = Stelegramf("%s",
				act.NewWatcher.GetName(),
			)
		}
	}
	return msg
}

func issueTag(act *dao.ActivityEvent, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	lef := act.IssueActivityExtendFields.LabelExtendFields
	switch act.Verb {
	case actField.VerbAdded:
		msg.Title = "добавил(-a) тег в"
		if lef.NewLabel != nil {
			msg.Body = Stelegramf("%s", lef.NewLabel.Name)
		}
	case actField.VerbRemoved:
		msg.Title = "убрал(-a) тег из"
		if lef.OldLabel != nil {
			msg.Body = Stelegramf("%s", lef.OldLabel.Name)
		}
	}
	return msg
}

func issueParent(act *dao.ActivityEvent, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()

	format := "*Родитель*: "
	var values []any

	if act.OldParentIssue != nil {
		act.OldParentIssue.Workspace = act.Issue.Workspace
		act.OldParentIssue.Project = act.Issue.Project
		act.OldParentIssue.SetUrl()

		format += "~[%s](%s)~ "
		values = append(values, act.OldParentIssue.FullIssueName(), act.OldParentIssue.URL.String())
	}

	if act.NewParentIssue != nil {
		act.NewParentIssue.Workspace = act.Issue.Workspace
		act.NewParentIssue.Project = act.Issue.Project
		act.NewParentIssue.SetUrl()
		format += "[%s](%s) "
		values = append(values, act.NewParentIssue.FullIssueName(), act.NewParentIssue.URL.String())
	}

	switch act.Verb {
	case actField.VerbUpdated:
		msg.Title = "изменил(-a)"
		msg.Body = Stelegramf(strings.TrimSpace(format), values...)
	}

	return msg
}

func issueTargetDate(act *dao.ActivityEvent, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	if act.Verb != actField.VerbUpdated {
		return msg
	}

	msg.Title = "изменил(-a)"
	format := "*%s*: "
	values := []any{types.FieldsTranslation[af]}
	if act.OldValue != nil && *act.OldValue != "<nil>" {
		format += "~%s~ "
		key := targetDateTimeZ + "_old"
		msg.Replace[key] = utils.FormatDateToSqlNullTime(*act.OldValue)
		values = append(values, strReplace(key))
	}

	if act.NewValue != "" && act.NewValue != "<nil>" {
		format += "%s "
		key := targetDateTimeZ + "_new"
		msg.Replace[key] = utils.FormatDateToSqlNullTime(act.NewValue)
		values = append(values, strReplace(key))
	}

	msg.Body = Stelegramf(format, values...)
	return msg
}

func issueProject(act *dao.ActivityEvent, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	pef := act.IssueActivityExtendFields.ProjectExtendFields
	if act.Verb != actField.VerbMove || (pef.NewProject == nil && pef.OldProject == nil) {
		return msg
	}
	msg.Title = "перенес(-лa)"
	msg.Body += Stelegramf("из ~%s~ в %s ",
		fmt.Sprint(pef.OldProject.Name),
		pef.NewProject.Name,
	)
	return msg
}

func issueSprint(act *dao.ActivityEvent, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()

	switch act.Verb {
	case actField.VerbAdded:
		msg.Title = "добавил(-a) в спринт задачу"
		if act.NewIssueSprint != nil {
			act.NewIssueSprint.SetUrl()
			msg.Body = Stelegramf("*спринт*: [%s](%s)", act.NewIssueSprint.GetFullName(), act.NewIssueSprint.URL)
		}
	case actField.VerbRemoved:
		msg.Title = "убрал(-a) из спринта задачу"
		if act.OldIssueSprint != nil {
			act.OldIssueSprint.SetUrl()
			msg.Body = Stelegramf("*спринт*: [~%s~](%s)", act.OldIssueSprint.GetFullName(), act.OldIssueSprint.URL)
		}
	}
	return msg
}
