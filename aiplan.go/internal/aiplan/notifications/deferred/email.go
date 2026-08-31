package deferred

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications/email"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications/shared"
	"gorm.io/gorm"
)

// dispatchCtx — общий контекст отправки (email и TG), загружается лениво один раз на расписание
type dispatchCtx struct {
	loaded    bool
	issue     *dao.Issue
	workspace *dao.Workspace
	author    *dao.User
}

func (c *dispatchCtx) load(db *gorm.DB, s *dao.NotificationSchedule) {
	if c.loaded {
		return
	}
	c.loaded = true

	switch s.NotificationType {
	case dao.NotificationTypeDeadline:
		if !s.IssueID.Valid {
			return
		}
		var issue dao.Issue
		if err := db.Preload("Project").Preload("Workspace").Where("id = ?", s.IssueID.UUID).First(&issue).Error; err != nil {
			slog.Error("dispatch ctx: issue not found", "id", s.IssueID.UUID, "err", err)
			return
		}
		issue.SetUrl()
		c.issue = &issue

	case dao.NotificationTypeWorkspaceMessage:
		if s.WorkspaceID.Valid {
			var ws dao.Workspace
			if err := db.Where("id = ?", s.WorkspaceID.UUID).First(&ws).Error; err != nil {
				slog.Error("dispatch ctx: workspace not found", "id", s.WorkspaceID.UUID, "err", err)
			} else {
				ws.SetUrl()
				c.workspace = &ws
			}
		}

		var p workspaceMessagePayload
		if len(s.Payload) > 0 {
			if err := json.Unmarshal(s.Payload, &p); err != nil {
				slog.Error("dispatch ctx: unmarshal payload", "id", s.ID, "err", err)
				return
			}
		}
		if p.AuthorID != "" {
			var u dao.User
			if err := db.Where("id = ?", p.AuthorID).First(&u).Error; err == nil {
				u.Avatar = "" // недоступный url аватара
				c.author = &u
			}
		}
	}
}

// sendEmailDeferred — dispatch email-отправки по типу уведомления.
func (np *NotificationProcessor) sendEmailDeferred(user *dao.User, s *dao.NotificationSchedule, dctx *dispatchCtx) error {
	if user.Email == "" {
		return nil
	}

	switch s.NotificationType {
	case dao.NotificationTypeDeadline:

		return np.sendDeadlineEmail(user, s, dctx)
	case dao.NotificationTypeWorkspaceMessage:
		return np.sendWorkspaceMsgEmail(user, s, dctx)
	case dao.NotificationTypeServiceMessage:

		return np.sendServiceMsgEmail(user, s)
	}
	return nil
}

func (np *NotificationProcessor) sendDeadlineEmail(user *dao.User, s *dao.NotificationSchedule, dctx *dispatchCtx) error {
	var p deadlinePayload
	if err := json.Unmarshal(s.Payload, &p); err != nil {
		return err
	}

	dctx.load(np.db, s)
	if dctx.issue == nil {
		return fmt.Errorf("deadline email: issue not loaded")
	}

	ctx := email.MessageDeadlineCtx{
		Msg:      p.Body,
		Deadline: p.Deadline,
		TimeSend: s.ScheduledAt,
	}

	return np.emailService.DeadlineMessageNotify(*user, dctx.issue, ctx)
}

func (np *NotificationProcessor) sendWorkspaceMsgEmail(user *dao.User, s *dao.NotificationSchedule, dctx *dispatchCtx) error {
	var p messagePayload
	if err := json.Unmarshal(s.Payload, &p); err != nil {
		return err
	}

	dctx.load(np.db, s)
	if dctx.workspace == nil {
		return fmt.Errorf("workspace message email: workspace not loaded")
	}
	workspace := *dctx.workspace

	msg := shared.PrepareHtmlBody(shared.HtmlStripPolicy, p.Msg)

	r := email.MessageNotifyCtx{
		WebUrl:     fmt.Sprintf("%s/", workspace.Slug), //todo понаблюдать
		Actor:      dctx.author,
		TitleMsg:   p.Title,
		Msg:        msg,
		TimeSend:   s.ScheduledAt,
		Workspace:  &workspace,
		TextButton: "Перейти в рабочее пространство",
	}

	return np.emailService.MessageNotify(user.Email, fmt.Sprintf("Сообщение для участников рабочего пространства: %s", workspace.Name), r)
}

func (np *NotificationProcessor) sendServiceMsgEmail(user *dao.User, s *dao.NotificationSchedule) error {
	var p messagePayload
	if err := json.Unmarshal(s.Payload, &p); err != nil {
		return err
	}

	msg := shared.PrepareHtmlBody(shared.HtmlStripPolicy, p.Msg)
	r := email.MessageNotifyCtx{
		TitleMsg:   p.Title,
		Msg:        msg,
		TimeSend:   s.ScheduledAt,
		TextButton: "Перейти на главную страницу",
	}

	return np.emailService.MessageNotify(user.Email, "Сервисное уведомление пользователям", r)
}
