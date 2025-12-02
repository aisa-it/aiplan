package notifications

import (
	"bytes"
	"encoding/base64"
	"fmt"
	htmlTemplate "html/template"
	"log/slog"
	"net/url"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
)

func (es *EmailService) getHTML(title string, body string) (string, error) {
	return es.getHTMLWithParams(title, body, nil, nil, 0, 0)
}

func (es *EmailService) getHTMLWithParams(title string, body string, issue *dao.Issue, project *dao.Project, commentCount int, activityCount int) (string, error) {
	var template dao.Template
	if err := es.db.Where("name = ?", "body").First(&template).Error; err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := template.ParsedTemplate.Execute(&buf, struct {
		Issue         *dao.Issue
		Title         string
		CreatedAt     time.Time
		Body          string
		CommentCount  int
		ActivityCount int
		Project       *dao.Project
	}{
		Title:         title,
		CreatedAt:     time.Now(), //TODO: timezone
		Body:          body,
		Issue:         issue,
		CommentCount:  commentCount,
		ActivityCount: activityCount,
		Project:       project,
	}); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (es *EmailService) WorkspaceInvitation(workspaceInvite dao.WorkspaceMember) {
	inviter := workspaceInvite.CreatedBy
	link := workspaceInvite.Workspace.URL.String()

	subject := fmt.Sprintf("Пользователь %s пригласил Вас в рабочее пространство %s системы AIPlan", inviter.Email, workspaceInvite.Workspace.Name)

	context := struct {
		Inviter       *dao.User
		WorkspaceName string
		InvitationUrl string
	}{
		Inviter:       workspaceInvite.CreatedBy,
		WorkspaceName: workspaceInvite.Workspace.Name,
		InvitationUrl: link,
	}

	var template dao.Template
	if err := es.db.Where("name = ?", "workspace_invitation").First(&template).Error; err != nil {
		slog.Error("Find workspace invitation email template", "err", err)
		return
	}

	var buf bytes.Buffer
	if err := template.ParsedTemplate.Execute(&buf, context); err != nil {
		slog.Error("Execute workspace invitation email template", "err", err)
		return
	}

	content, err := es.getHTML("Приглашение в пространство", buf.String())
	if err != nil {
		slog.Error("Execute main email template", "err", err)
		return
	}

	textContent := htmlStripPolicy.Sanitize(content)

	if err := es.Send(mail{
		To:          workspaceInvite.Member.Email,
		Subject:     subject,
		Content:     content,
		TextContent: textContent,
	}); err != nil {
		slog.Error("Send workspace invitation email", "err", err)
	}
}

func (es *EmailService) ProjectInvitation(projectMember dao.ProjectMember) {
	var workspace string
	if projectMember.Workspace != nil {
		workspace = projectMember.Workspace.Slug
	}
	link := fmt.Sprintf("%s/%s/projects/%s/issues", es.cfg.WebURL, workspace, projectMember.ProjectId)

	subject := fmt.Sprintf("Пользователь %s %s добавил Вас в проект %s системы AIPlan", projectMember.CreatedBy.FirstName, projectMember.CreatedBy.LastName, projectMember.Project.Name)

	context := struct {
		Inviter       *dao.User
		ProjectName   string
		InvitationUrl string
		WorkspaceName string
	}{
		Inviter:       projectMember.CreatedBy,
		ProjectName:   projectMember.Project.Name,
		InvitationUrl: link,
		WorkspaceName: projectMember.Project.Workspace.Name,
	}

	var template dao.Template
	if err := es.db.Where("name = ?", "project_invitation").First(&template).Error; err != nil {
		slog.Error("Find project invitation template", "err", err)
		return
	}

	var buf bytes.Buffer
	if err := template.ParsedTemplate.Execute(&buf, context); err != nil {
		slog.Error("Parse project invitation template", "err", err)
		return
	}

	content, err := es.getHTML("Приглашение в проект", buf.String())
	if err != nil {
		slog.Error("Execute main email template", "err", err)
		return
	}

	textContent := htmlStripPolicy.Sanitize(content)

	if err := es.Send(mail{
		To:          projectMember.Member.Email,
		Subject:     subject,
		Content:     content,
		TextContent: textContent,
	}); err != nil {
		slog.Error("Send project invitation email", "err", err)
	}
}

func (es *EmailService) NewUserPasswordNotify(user dao.User, password string) error {
	subject := "Пароль для входа в АИПЛАН"

	context := struct {
		WebUrl   *url.URL
		Password string
	}{
		WebUrl:   es.cfg.WebURL.ResolveReference(&url.URL{Path: "/signin/"}),
		Password: password,
	}

	var template dao.Template
	if err := es.db.Where("name = ?", "new_user_password_notify").First(&template).Error; err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := template.ParsedTemplate.Execute(&buf, context); err != nil {
		return err
	}

	content, err := es.getHTML("Добро пожаловать в АИПлан", buf.String())
	if err != nil {
		return err
	}

	textContent := htmlStripPolicy.Sanitize(content)

	return es.Send(mail{
		To:          user.Email,
		Subject:     subject,
		Content:     content,
		TextContent: textContent,
	})
}

func (es *EmailService) UserChangeEmailNotify(user dao.User, newEmail, code string) error {
	subject := "Подтверждение смены email"

	context := struct {
		Code string
	}{
		Code: code,
	}

	var template dao.Template
	if err := es.db.Where("name = ?", "user_change_email").First(&template).Error; err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := template.ParsedTemplate.Execute(&buf, context); err != nil {
		return err
	}

	content, err := es.getHTML("Ссылка для верификации email", buf.String())
	if err != nil {
		return err
	}

	textContent := htmlStripPolicy.Sanitize(content)

	return es.Send(mail{
		To:          newEmail,
		Subject:     subject,
		Content:     content,
		TextContent: textContent,
	})
}

func (es *EmailService) UserPasswordForgotNotify(user dao.User, token string) error {
	subject := "Сброс пароля для входа в АИПлан"
	link := fmt.Sprintf("%s/reset-password?uidb64=%s&token=%s", es.cfg.WebURL, base64.StdEncoding.EncodeToString([]byte(user.ID)), token)
	context := struct {
		URL string
	}{
		URL: link,
	}

	var template dao.Template
	if err := es.db.Where("name = ?", "user_password_reset_notify").First(&template).Error; err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := template.ParsedTemplate.Execute(&buf, context); err != nil {
		return err
	}

	content, err := es.getHTML(subject, buf.String())
	if err != nil {
		return err
	}

	textContent := htmlStripPolicy.Sanitize(content)

	return es.Send(mail{
		To:          user.Email,
		Subject:     subject,
		Content:     content,
		TextContent: textContent,
	})
}

func (es *EmailService) ChangePasswordNotify(user dao.User) error {
	subject := "Смена пароля учетной записи АИПлан"

	var template dao.Template
	if err := es.db.Where("name = ?", "password_change").First(&template).Error; err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := template.ParsedTemplate.Execute(&buf, struct{}{}); err != nil {
		return err
	}

	content, err := es.getHTML("Изменение пароля для входа", buf.String())
	if err != nil {
		return err
	}

	textContent := htmlStripPolicy.Sanitize(content)

	return es.Send(mail{
		To:          user.Email,
		Subject:     subject,
		Content:     content,
		TextContent: textContent,
	})
}

func (es *EmailService) JiraImportStartNotify(user dao.User, project *dao.Project) error {
	subject := "Импорт проекта из Jira начат"

	var template dao.Template
	if err := es.db.Where("name = ?", "jira_import_start").First(&template).Error; err != nil {
		return err
	}

	if project.Workspace == nil {
		if err := es.db.Where("id = ?", project.WorkspaceId).First(&project.Workspace).Error; err != nil {
			return err
		}
	}

	context := struct {
		Workspace string
		Project   string
		Actor     dao.User
		CreatedAt time.Time
	}{
		Workspace: project.Workspace.Name,
		Project:   project.Name,
		Actor:     user,
		CreatedAt: time.Now().In((*time.Location)(&user.UserTimezone)),
	}

	var buf bytes.Buffer
	if err := template.ParsedTemplate.Execute(&buf, context); err != nil {
		return err
	}

	content, err := es.getHTMLWithParams(subject, buf.String(), nil, nil, 0, 0)
	if err != nil {
		return err
	}

	textContent := htmlStripPolicy.Sanitize(content)

	return es.Send(mail{
		To:          user.Email,
		Subject:     subject,
		Content:     content,
		TextContent: textContent,
	})
}

func (es *EmailService) JiraImportEndNotify(user dao.User, project *dao.Project, tasks, attachments, users int) error {
	subject := "Импорт проекта из Jira закончен"

	getForm := func(n int, one, few, many string) string {
		mod10 := n % 10
		mod100 := n % 100

		if mod10 == 1 && mod100 != 11 {
			return one
		} else if mod10 >= 2 && mod10 <= 4 && (mod100 < 10 || mod100 >= 20) {
			return few
		} else {
			return many
		}
	}

	var template dao.Template
	template.Func =
		htmlTemplate.FuncMap{
			"getTaskForm": func(n int) string {
				return getForm(n, "задача", "задачи", "задач")
			},
			"getAttachmentForm": func(n int) string {
				return getForm(n, "вложение", "вложения", "вложений")
			},
			"getUserForm": func(n int) string {
				return getForm(n, "новый пользователь", "новых пользователя", "новых пользователей")
			},
		}
	if err := es.db.Where("name = ?", "jira_import_end").First(&template).Error; err != nil {
		return err
	}

	context := struct {
		Workspace   string
		Project     string
		Actor       dao.User
		Tasks       int
		Attachments int
		Users       int
		CreatedAt   time.Time
	}{
		Workspace:   project.Workspace.Name,
		Project:     project.Name,
		Actor:       user,
		Tasks:       tasks,
		Attachments: attachments,
		Users:       users,
		CreatedAt:   time.Now().In((*time.Location)(&user.UserTimezone)),
	}
	var buf bytes.Buffer
	if err := template.ParsedTemplate.Execute(&buf, context); err != nil {
		return err
	}

	content, err := es.getHTMLWithParams(subject, buf.String(), nil, project, 0, 0)
	if err != nil {
		return err
	}

	textContent := htmlStripPolicy.Sanitize(content)

	return es.Send(mail{
		To:          user.Email,
		Subject:     subject,
		Content:     content,
		TextContent: textContent,
	})
}

func (es *EmailService) MessageNotify(notification dao.DeferredNotifications, msg emailNotify) error {
	subject := msg.Subj
	mailContext := struct {
		WebUrl     string
		Actor      *dao.User
		TitleMsg   string
		Msg        string
		TimeSend   time.Time
		Workspace  *dao.Workspace
		TextButton string
	}{
		WebUrl:     fmt.Sprintf("%s/%s", es.cfg.WebURL, msg.AddRout),
		Actor:      msg.Author,
		TitleMsg:   msg.Title,
		Msg:        msg.Msg,
		TimeSend:   *notification.TimeSend,
		Workspace:  notification.Workspace,
		TextButton: msg.TextButton,
	}

	var template dao.Template
	if err := es.db.Where("name = ?", "admin_message").First(&template).Error; err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := template.ParsedTemplate.Execute(&buf, mailContext); err != nil {
		return err
	}

	content, err := es.getHTML(subject, buf.String())
	if err != nil {
		return err
	}

	textContent := htmlStripPolicy.Sanitize(content)
	mailSend := mail{
		To:          notification.User.Email,
		Subject:     subject,
		Content:     content,
		TextContent: textContent,
	}
	return es.Send(mailSend)
}

func (es *EmailService) DeadlineMessageNotify(user dao.User, notification dao.DeferredNotifications, nd notifyDeadline) error {
	subject := fmt.Sprintf("Уведомление об истечении срока выполнения задачи: %s-%d", notification.Issue.Project.Identifier, notification.Issue.SequenceId)

	loc := time.Location(user.UserTimezone)
	date := nd.Deadline.In(&loc)
	context := struct {
		WebUrl   *url.URL
		Msg      string
		Deadline time.Time
		TimeSend time.Time
	}{
		WebUrl:   notification.Issue.URL,
		Msg:      nd.Body,
		Deadline: date,
		TimeSend: *notification.TimeSend,
	}

	var template dao.Template
	if err := es.db.Where("name = ?", "deadline_notification").First(&template).Error; err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := template.ParsedTemplate.Execute(&buf, context); err != nil {
		return err
	}

	content, err := es.getHTMLWithParams(subject, buf.String(), notification.Issue, nil, 0, 0)
	if err != nil {
		return err
	}

	textContent := htmlStripPolicy.Sanitize(content)

	return es.Send(mail{
		To:          user.Email,
		Subject:     subject,
		Content:     content,
		TextContent: textContent,
	})
}
