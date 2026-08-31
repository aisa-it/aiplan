package deferred

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications/shared"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
)

func (np *NotificationProcessor) formatDeadlineTg(user *dao.User, s *dao.NotificationSchedule, dctx *dispatchCtx) (int64, string, []interface{}) {
	var p deadlinePayload
	if err := json.Unmarshal(s.Payload, &p); err != nil {
		return 0, "", nil
	}

	dctx.load(np.db, s)
	if dctx.issue == nil {
		return 0, "", nil
	}

	formatMsg := "❗Срок выполнения задачи\n[%s](%s)\nистекает *%s*"

	date, err := shared.FormatDate(p.Deadline.Format("02.01.2006 15:04 MST"), "02.01.2006 15:04 MST", &user.UserTimezone)
	if err != nil {
		return 0, "", nil
	}

	return *user.TelegramId, formatMsg, []interface{}{
		dctx.issue.FullIssueName(),
		dctx.issue.URL.String(),
		date,
	}
}

func (np *NotificationProcessor) formatWorkspaceMsgTg(user *dao.User, s *dao.NotificationSchedule, dctx *dispatchCtx) (int64, string, []interface{}) {
	var p messagePayload
	if err := json.Unmarshal(s.Payload, &p); err != nil {
		return 0, "", nil
	}

	dctx.load(np.db, s)
	if dctx.workspace == nil {
		return 0, "", nil
	}
	workspace := *dctx.workspace

	var firstName, lastName string
	if dctx.author != nil {
		firstName = dctx.author.FirstName
		lastName = dctx.author.LastName
	}

	formatMsg := "%s %s отправил сообщение пользователям\n[%s](%s)\n*%s*\n```\n%s```"

	return *user.TelegramId, formatMsg, []interface{}{
		firstName,
		lastName,
		workspace.Name,
		workspace.URL.String(),
		p.Title,
		prepareTgText(p.Msg),
	}
}

func (np *NotificationProcessor) formatServiceMsgTg(s *dao.NotificationSchedule) (int64, string, []interface{}) {
	var p messagePayload
	if err := json.Unmarshal(s.Payload, &p); err != nil {
		return 0, "", nil
	}

	formatMsg := "🔹Сервисное уведомление пользователям\n*%s*\n```\n%s```"

	return 0, formatMsg, []interface{}{
		p.Title,
		prepareTgText(p.Msg),
	}
}

// prepareTgText — приводит HTML-сообщение к текстовому виду для Telegram:
func prepareTgText(msg string) string {
	msg = shared.ReplaceTablesToText(msg)
	msg = shared.ReplaceImageToText(msg)
	msg = shared.PrepareHtmlBody(shared.HtmlStripPolicy, msg)
	return utils.Substr(utils.ReplaceImgToEmoj(msg), 0, 4000)
}

// formatAppContent формирует title/msg для app-уведомления (UserAppNotify).
func (np *NotificationProcessor) formatAppContent(user *dao.User, s *dao.NotificationSchedule, dctx *dispatchCtx) (string, string) {
	switch s.NotificationType {
	case dao.NotificationTypeDeadline:
		const title = "Уведомление об истечении срока выполнения задачи"

		var p deadlinePayload
		if err := json.Unmarshal(s.Payload, &p); err != nil {
			return "", ""
		}
		if user == nil {
			return title, p.Body
		}

		if dctx == nil {
			dctx = &dispatchCtx{}
		}
		dctx.load(np.db, s)
		if dctx.issue == nil || dctx.issue.Project == nil {
			return title, p.Body
		}

		date := p.Deadline.In((*time.Location)(&user.UserTimezone)).Format("02.01.2006 15:04 MST")
		body := fmt.Sprintf("Срок выполнения задачи %s-%d истекает %s", dctx.issue.Project.Identifier, dctx.issue.SequenceId, date)
		return title, body

	case dao.NotificationTypeWorkspaceMessage, dao.NotificationTypeServiceMessage:
		var p messagePayload
		if err := json.Unmarshal(s.Payload, &p); err == nil {
			return p.Title, p.Msg
		}
	}
	return "", ""
}

func typeForDeferredApp(notificationType string) string {
	switch notificationType {
	case dao.NotificationTypeServiceMessage:
		return "service_message"
	default:
		return "message"
	}
}
