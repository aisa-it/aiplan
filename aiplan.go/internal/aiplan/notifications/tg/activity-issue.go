package tg

import (
	"fmt"
	"log/slog"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/go-telegram/bot"
)

type funcIssueMsgFormat func(act *dao.IssueActivity, af actField.ActivityField) issueFF

var (
	issueMap = map[actField.ActivityField]funcIssueMsgFormat{
		actField.StartDate.Field: issueSkiper,
		actField.EndDate.Field:   issueSkiper,

		actField.Comment.Field:     issueComment,
		actField.Description.Field: issueDescription,
		actField.Attachment.Field:  issueAttachment,
		actField.Link.Field:        issueLink,
		actField.LinkTitle.Field:   issueLink,
		actField.LinkUrl.Field:     issueLink,
		actField.Assignees.Field:   issueAssignees,
		actField.Watchers.Field:    issueWatchers,
		actField.Linked.Field:      issueLinked,
	}
)

func (t *TgService) preloadIssueActivity(act *dao.IssueActivity) error {
	if err := t.db.Unscoped().
		Joins("Author").
		Joins("Workspace").
		Joins("Project").
		Joins("Parent").
		Preload("Assignees").
		Preload("Watchers").
		Preload("Parent.Project").
		Where("issues.id = ?", act.IssueId).
		First(&act.Issue).Error; err != nil {
		slog.Error("Get issue for activity", "activityId", act.Id, "err", err)
		return fmt.Errorf("preloadIssueActivity: %v", err)
	}

	return nil
}

type issueFF struct {
	titleAction string
	body        string
}

func (t *TgService) msgt(act *dao.IssueActivity) (TgMsg, error) {
	var msg TgMsg

	var res issueFF

	if act.Field == nil {
		return msg, fmt.Errorf("IssueActivity field is nil")
	}

	af := actField.ActivityField(*act.Field)
	if f, ok := issueMap[af]; ok {
		res = f(act, af)

	} else {
		res = issueDefault(act, af)
	}

	msg.title = fmt.Sprintf(
		"*%s* %s [%s](%s)",
		act.Actor.GetName(),
		bot.EscapeMarkdown(res.titleAction),
		bot.EscapeMarkdown(act.Issue.FullIssueName()),
		act.Issue.URL,
	)
	msg.body = res.body
	return msg, nil
}

func issueDefault(act *dao.IssueActivity, af actField.ActivityField) issueFF {
	var msg issueFF
	oldValue := ""
	newValue := act.NewValue
	if act.OldValue != nil && *act.OldValue != "<nil>" {
		oldValue = *act.OldValue
	}

	if oldValue == "<p></p>" {
		oldValue = ""
	}
	msg.titleAction = "изменил(-a)"

	if af == actField.Priority.Field {
		oldValue = translateMap(priorityTranslation, act.OldValue)
		newValue = translateMap(priorityTranslation, &act.NewValue)
	}

	if oldValue != "" {
		msg.body += Stelegramf("*%s*: ~%s~ %s",
			fieldsTranslation[af],
			oldValue,
			newValue,
		)
	} else {
		msg.body += Stelegramf("*%s*: %s",
			fieldsTranslation[af],
			newValue,
		)
	}
	return msg

}

func issueDescription(act *dao.IssueActivity, af actField.ActivityField) issueFF {
	var msg issueFF

	msg.body = Stelegramf("```\n%s```",
		utils.HtmlToTg(act.NewValue),
	)

	switch act.Verb {
	case actField.VerbUpdated:
		msg.titleAction = "изменил(-а) описание"
	}
	return msg
}

func issueSkiper(act *dao.IssueActivity, af actField.ActivityField) issueFF {
	return issueFF{}
}

func issueComment(act *dao.IssueActivity, af actField.ActivityField) issueFF {
	var msg issueFF

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
		msg.titleAction = "изменил(-a) комментарий"
	case actField.VerbCreated:
		msg.titleAction = "прокомментировал(-a)"
	case actField.VerbDeleted:
		msg.titleAction = "удалил(-a) комментарий из"
	}
	return msg
}

func issueAttachment(act *dao.IssueActivity, af actField.ActivityField) issueFF {
	var msg issueFF

	if act.OldValue != nil {
		msg.body = Stelegramf("*файл*: %s", *act.OldValue)
	} else {
		msg.body = Stelegramf("*файл*: %s", act.NewValue)
	}

	switch act.Verb {
	case actField.VerbCreated:
		msg.titleAction = "добавил(-a) вложение в"
	case actField.VerbDeleted:
		msg.titleAction = "удалил(-a) вложение из"

	}
	return msg
}

func issueLinked(act *dao.IssueActivity, af actField.ActivityField) issueFF {
	var msg issueFF
	var targetIssue dao.Issue

	switch act.Verb {
	case actField.VerbUpdated:
		if !act.OldIdentifier.Valid && act.NewIdentifier.Valid {
			msg.titleAction = "добавил(-а) связь к"
			targetIssue = *act.NewIssueLinked
		}
		if !act.NewIdentifier.Valid && act.OldIdentifier.Valid {
			msg.titleAction = "убрал(-а) связь из"
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

func issueLink(act *dao.IssueActivity, af actField.ActivityField) issueFF {
	var msg issueFF

	format := "[%s](%s)"

	switch af {
	case actField.LinkTitle.Field:
		msg.titleAction = "изменил(-a) название ссылки"
	case actField.LinkUrl.Field:
		msg.titleAction = "изменил(-a) url ссылки"
	}

	var values []any

	link := act.NewLink
	if link != nil {
		values = append(values, link.Title, link.Url)
	}

	switch act.Verb {
	case actField.VerbCreated:
		msg.titleAction = "добавил(-a) ссылку в"
	case actField.VerbDeleted:
		msg.titleAction = "удалил(-a) ссылку из"
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

func issueAssignees(act *dao.IssueActivity, af actField.ActivityField) issueFF {
	var msg issueFF
	oldAssignee := act.OldAssignee
	newAssignee := act.NewAssignee

	switch act.Verb {
	case actField.VerbRemoved:
		msg.titleAction = "убрал(-а) исполнителя из"
		if oldAssignee != nil {
			msg.body = Stelegramf("%s",
				oldAssignee.GetName(),
			)
		}
	case actField.VerbAdded:
		msg.titleAction = "добавил(-a) нового исполнителя в"
		if newAssignee != nil {
			msg.body = Stelegramf("%s",
				newAssignee.GetName(),
			)
		}
	}
	return msg
}

func issueWatchers(act *dao.IssueActivity, af actField.ActivityField) issueFF {
	var msg issueFF
	oldWatcher := act.OldWatcher
	newWatcher := act.NewWatcher

	switch act.Verb {
	case actField.VerbRemoved:
		msg.titleAction = "убрал(-а) наблюдателя из"
		if oldWatcher != nil {
			msg.body = Stelegramf("%s",
				oldWatcher.GetName(),
			)
		}
	case actField.VerbAdded:
		msg.titleAction = "добавил(-a) нового наблюдателя в"
		if newWatcher != nil {
			msg.body = Stelegramf("%s",
				newWatcher.GetName(),
			)
		}
	}
	return msg
}
