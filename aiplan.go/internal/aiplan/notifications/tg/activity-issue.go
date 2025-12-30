package tg

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/go-telegram/bot"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

type funcIssueMsgFormat func(act *dao.IssueActivity, af actField.ActivityField) TgMsg

var (
	issueMap = map[actField.ActivityField]funcIssueMsgFormat{
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
	}
)

func notifyFromIssueActivity(tx *gorm.DB, act *dao.IssueActivity) *ActivityTgNotification {
	notify := ActivityTgNotification{}
	if act.Field == nil {
		return nil
	}

	act.Issue = preloadIssueActivity(tx, act.IssueId)
	msg, err := formatIssueActivity(act)
	if err != nil {
		return nil
	}

	notify.Message = msg
	notify.Users = getUserTgIdIssueActivity(tx, act)
	notify.TableName = act.TableName()
	notify.EntityID = act.Id
	notify.AuthorTgID = act.ActivitySender.SenderTg

	return &notify
}

func preloadIssueActivity(tx *gorm.DB, id uuid.UUID) *dao.Issue {
	var issue dao.Issue
	if err := tx.Unscoped().
		Joins("Author").
		Joins("Workspace").
		Joins("Project").
		Joins("Parent").
		Preload("Assignees").
		Preload("Watchers").
		Preload("Parent.Project").
		Where("issues.id = ?", id).
		First(&issue).Error; err != nil {
		slog.Error("Get IssueActivity", "err", err)
		return nil
	}

	return &issue
}

func formatIssueActivity(act *dao.IssueActivity) (TgMsg, error) {
	var res TgMsg

	af := actField.ActivityField(*act.Field)
	if f, ok := issueMap[af]; ok {
		res = f(act, af)
	} else {
		res = issueDefault(act, af)
	}

	if res.IsEmpty() {
		return res, fmt.Errorf("issue activity is empty")
	}

	res.title = fmt.Sprintf(
		"*%s* %s [%s](%s)",
		bot.EscapeMarkdown(act.Actor.GetName()),
		bot.EscapeMarkdown(res.title),
		bot.EscapeMarkdown(act.Issue.FullIssueName()),
		act.Issue.URL.String(),
	)

	return res, nil
}

func issueSkipper(act *dao.IssueActivity, af actField.ActivityField) TgMsg {
	return NewTgMsg()
}

func issueDefault(act *dao.IssueActivity, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()

	oldValue := ""
	newValue := act.NewValue
	if act.OldValue != nil && *act.OldValue != "<nil>" {
		oldValue = *act.OldValue
	}

	if oldValue == "<p></p>" {
		oldValue = ""
	}
	msg.title = "изменил(-a)"

	if af == actField.Priority.Field {
		oldValue = types.TranslateMap(types.PriorityTranslation, act.OldValue)
		newValue = types.TranslateMap(types.PriorityTranslation, &act.NewValue)
	}

	if oldValue != "" {
		msg.body += Stelegramf("*%s*: ~%s~ %s",
			types.FieldsTranslation[af],
			oldValue,
			newValue,
		)
	} else {
		msg.body += Stelegramf("*%s*: %s",
			types.FieldsTranslation[af],
			newValue,
		)
	}
	return msg

}

func issueDescription(act *dao.IssueActivity, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	if act.Verb != actField.VerbUpdated {
		return msg
	}

	msg.title = "изменил(-а) описание"
	msg.body = Stelegramf("```\n%s```",
		utils.HtmlToTg(act.NewValue),
	)
	return msg
}

func issueComment(act *dao.IssueActivity, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()

	comment := act.NewIssueComment

	if comment != nil {
		msg.body = Stelegramf("```\n%s```",
			utils.HtmlToTg(comment.CommentHtml.Body),
		)
	} else {
		if act.OldValue != nil {
			msg.body = Stelegramf("```\n%s```",
				utils.HtmlToTg(*act.OldValue))
		}
	}

	switch act.Verb {
	case actField.VerbUpdated:
		msg.title = "изменил(-a) комментарий"
	case actField.VerbCreated:
		msg.title = "прокомментировал(-a)"
	case actField.VerbDeleted:
		msg.title = "удалил(-a) комментарий из"
	}
	return msg
}

func issueAttachment(act *dao.IssueActivity, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()

	if act.OldValue != nil {
		msg.body = Stelegramf("*файл*: %s", *act.OldValue)
	} else {
		msg.body = Stelegramf("*файл*: %s", act.NewValue)
	}

	switch act.Verb {
	case actField.VerbCreated:
		msg.title = "добавил(-a) вложение в"
	case actField.VerbDeleted:
		msg.title = "удалил(-a) вложение из"

	}
	return msg
}

func issueLinked(act *dao.IssueActivity, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	var targetIssue dao.Issue

	switch act.Verb {
	case actField.VerbUpdated:
		if !act.OldIdentifier.Valid && act.NewIdentifier.Valid {
			msg.title = "добавил(-а) связь к"
			targetIssue = *act.NewIssueLinked
		}
		if !act.NewIdentifier.Valid && act.OldIdentifier.Valid {
			msg.title = "убрал(-а) связь из"
			targetIssue = *act.OldIssueLinked
		}

		targetIssue.Project = act.Issue.Project

		msg.body = Stelegramf("*Задача*: [%s](%s)",
			targetIssue.FullIssueName(),
			targetIssue.URL,
		)
	}

	return msg
}

func issueSubIssue(act *dao.IssueActivity, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()

	msg.title = "изменил(-a)"
	format := "*Подзадача*: "

	switch act.Verb {
	case actField.VerbAdded:
		format += "[%s](%s)"
	case actField.VerbRemoved:
		format += " ~[%s](%s)~"
	default:
		return NewTgMsg()
	}

	msg.body += Stelegramf(format,
		act.NewSubIssue.FullIssueName(),
		act.NewSubIssue.URL,
	)
	return msg
}

func issueLink(act *dao.IssueActivity, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()

	format := "[%s](%s)"

	switch af {
	case actField.LinkTitle.Field:
		msg.title = "изменил(-a) название ссылки"
	case actField.LinkUrl.Field:
		msg.title = "изменил(-a) url ссылки"
	}

	var values []any

	link := act.NewLink
	if link != nil {
		values = append(values, link.Title, link.Url)
	}

	switch act.Verb {
	case actField.VerbCreated:
		msg.title = "добавил(-a) ссылку в"
	case actField.VerbDeleted:
		msg.title = "удалил(-a) ссылку из"
		format = ""
	case actField.VerbUpdated:
		if act.OldValue != nil {
			format = "~%s~ " + format
			values = append([]any{*act.OldValue}, values...)
		}
	}

	msg.body = Stelegramf(format, values...)
	return msg
}

func issueAssignees(act *dao.IssueActivity, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()

	switch act.Verb {
	case actField.VerbRemoved:
		msg.title = "убрал(-а) исполнителя из"
		if act.OldAssignee != nil {
			msg.body = Stelegramf("%s",
				act.OldAssignee.GetName(),
			)
		}
	case actField.VerbAdded:
		msg.title = "добавил(-a) нового исполнителя в"
		if act.NewAssignee != nil {
			msg.body = Stelegramf("%s",
				act.NewAssignee.GetName(),
			)
		}
	}
	return msg
}

func issueWatchers(act *dao.IssueActivity, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()

	switch act.Verb {
	case actField.VerbRemoved:
		msg.title = "убрал(-а) наблюдателя из"
		if act.OldWatcher != nil {
			msg.body = Stelegramf("%s",
				act.OldWatcher.GetName(),
			)
		}
	case actField.VerbAdded:
		msg.title = "добавил(-a) нового наблюдателя в"
		if act.NewWatcher != nil {
			msg.body = Stelegramf("%s",
				act.NewWatcher.GetName(),
			)
		}
	}
	return msg
}

func issueTag(act *dao.IssueActivity, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()

	switch act.Verb {
	case actField.VerbAdded:
		msg.title = "добавил(-a) тег в"
		if act.NewLabel != nil {
			msg.body = Stelegramf("%s", act.NewLabel.Name)
		}
	case actField.VerbRemoved:
		msg.title = "убрал(-a) тег из"
		if act.OldLabel != nil {
			msg.body = Stelegramf("%s", act.OldLabel.Name)
		}
	}
	return msg
}

func issueParent(act *dao.IssueActivity, af actField.ActivityField) TgMsg {
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
		msg.title = "изменил(-a)"
		msg.body = Stelegramf(strings.TrimSpace(format), values...)
	}

	return msg
}

func issueTargetDate(act *dao.IssueActivity, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	if act.Verb != actField.VerbUpdated {
		return msg
	}

	msg.title = "изменил(-a)"
	format := "*%s*: "
	values := []any{types.FieldsTranslation[af]}
	if act.OldValue != nil && *act.OldValue != "<nil>" {
		format += "~%s~ "
		key := targetDateTimeZ + "_old"
		msg.replace[key] = utils.FormatDateToSqlNullTime(*act.OldValue)
		values = append(values, strReplace(key))
	}

	if act.NewValue != "" && act.NewValue != "<nil>" {
		format += "%s "
		key := targetDateTimeZ + "_new"
		msg.replace[key] = utils.FormatDateToSqlNullTime(act.NewValue)
		values = append(values, strReplace(key))
	}

	msg.body = Stelegramf(format, values...)
	return msg
}

func issueProject(act *dao.IssueActivity, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	if act.Verb != actField.VerbMove || (act.NewProject == nil && act.OldProject == nil) {
		return msg
	}
	msg.title = "перенес(-лa)"
	msg.body += Stelegramf("из ~%s~ в %s ",
		fmt.Sprint(act.OldProject.Name),
		act.NewProject.Name,
	)
	return msg
}

func getUserTgIdIssueActivity(tx *gorm.DB, act *dao.IssueActivity) []userTg {
	users := make(UserRegistry)
	users.addUser(act.Actor, actionAuthor)

	addOriginalCommentAuthor(tx, act, users)
	addDefaultWatchers(tx, act.ProjectId, users)
	addIssueUsers(act.Issue, users)

	if err := users.LoadProjectSettings(tx, act.ProjectId, issueAuthor); err != nil {
		return []userTg{}
	}
	return users.FilterActivity(act.Field, act.Verb, actField.Issue.Field.String(), shouldProjectNotify, issueAuthor)
}
