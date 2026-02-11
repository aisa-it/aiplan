package tg

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	tracker "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/activity-tracker"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/business"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/config"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"gorm.io/gorm"
)

type TgService struct {
	db          *gorm.DB
	bot         *bot.Bot
	botUserName string
	cfg         *config.Config
	tracker     *tracker.ActivitiesTracker
	Disabled    bool
	bl          *business.Business

	ctx    context.Context
	cancel context.CancelFunc
}

type TgMsg struct {
	title string
	body  string

	replace map[string]any
	Skip    func(u userTg) bool
}

func NewTgMsg() TgMsg {
	return TgMsg{
		replace: make(map[string]any),
	}
}

func (m TgMsg) IsEmpty() bool {
	if m.title == "" && m.body == "" {
		return true
	}
	return false
}

func New(db *gorm.DB, cfg *config.Config, tracker *tracker.ActivitiesTracker, bl *business.Business) *TgService {
	if cfg.TelegramBotToken == "" {
		slog.Info("Telegram notifications disabled")
		return &TgService{Disabled: true}
	}

	serv := &TgService{
		db:       db,
		cfg:      cfg,
		tracker:  tracker,
		Disabled: false,
		bl:       bl,
	}

	opts := []bot.Option{
		bot.WithDefaultHandler(serv.getUserMiddleware()(serv.startHandler)),
	}

	b, err := bot.New(
		cfg.TelegramBotToken,
		opts...)

	if err != nil {
		slog.Error("Connect to TG bot", "err", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	res, _ := b.GetMe(ctx)

	serv.bot = b
	serv.ctx = ctx
	serv.cancel = cancel
	serv.botUserName = res.Username

	b.RegisterHandler(bot.HandlerTypeMessageText, "/start", bot.MatchTypeExact, serv.getUserMiddleware()(serv.startHandler))
	b.RegisterHandlerMatchFunc(isReplyMessage, serv.getUserMiddleware()(serv.commentActivityHandler))

	go serv.Start()

	return serv
}

func (t *TgService) Start() {
	if t.Disabled {
		return
	}
	slog.Info("TG bot connected", "username", t.botUserName)
	t.bot.Start(t.ctx)
	slog.Info("Telegram bot stopped")
}

func (s *TgService) Stop() {
	if s.Disabled {
		return
	}

	s.cancel()
}

func (t *TgService) GetBotLink() string {
	if t.Disabled {
		return ""
	}
	return "https://t.me/" + fmt.Sprint(t.botUserName)
}

func (t *TgService) Send(tgId int64, tgMsg TgMsg) (int64, error) {
	msg := strings.Join([]string{tgMsg.title, tgMsg.body}, "\n")
	if msg == "" {
		return 0, fmt.Errorf("message is empty")
	}

	smp := bot.SendMessageParams{
		ChatID:    tgId,
		Text:      msg,
		ParseMode: models.ParseModeMarkdown,
		LinkPreviewOptions: &models.LinkPreviewOptions{
			IsDisabled: bot.True(),
		},
	}

	message, err := t.bot.SendMessage(t.ctx, &smp)
	if err != nil {
		return 0, fmt.Errorf("send message error: %w", err)
	}

	return int64(message.ID), nil
}

func isReplyMessage(update *models.Update) bool {
	if update.Message == nil || update.Message.ReplyToMessage == nil {
		return false
	}
	return update.Message.Chat.Type == "private"
}

func (t *TgService) SendMessage(tgId int64, format string, anyStr []any) bool {
	msg := NewTgMsg()
	msg.title = Stelegramf(format, anyStr...)
	_, err := t.Send(tgId, msg)
	if err != nil {
		slog.Error("Sending message to Telegram:", "error", err)
		return false
	}
	return true
}

func (t *TgService) SendFormAnswer(tgId int64, form dao.Form, answer *dao.FormAnswer, user *dao.User) {
	var d strings.Builder
	var out []any

	msg := NewTgMsg()
	formName := fmt.Sprintf("%s/%s", form.Workspace.Name, form.Title)
	msg.title = fmt.Sprintf("*%s* прошел форму [%s](%s)\n", bot.EscapeMarkdown(user.GetName()), bot.EscapeMarkdown(formName), form.URL.String())

	fileName := make(map[string]string, len(answer.Attachments))
	for _, attachment := range answer.Attachments {
		fileName[attachment.Id.String()] = attachment.Asset.Name
	}

	count := 0
	for _, field := range answer.Fields {
		count++
		d.WriteString(fmt.Sprintf(" %d\\. *%s* ", count, bot.EscapeMarkdown(field.Label)))
		switch field.Type {
		case "checkbox":
			if field.Val == nil {
				d.WriteString(" ❌\n")
			} else {
				if v := field.Val.(bool); v {
					d.WriteString(" ☑️\n")
				} else {
					d.WriteString(" ❌\n")
				}
			}
		case "numeric":
			if field.Val == nil {
				d.WriteString("\n ✖️\n")
			} else {
				d.WriteString("```\n%s```\n")
				out = append(out, fmt.Sprint(field.Val))
			}
		case "input", "textarea":
			if field.Val == nil {
				d.WriteString("\n ✖️\n")
			} else {
				d.WriteString("```\n%s\n```\n")
				out = append(out, utils.Substr(fmt.Sprint(field.Val), 0, 4000))
			}
		case "multiselect":
			if field.Val == nil {
				d.WriteString("\n ✖️\n")
			} else {
				if values, ok := field.Val.([]interface{}); ok {
					for _, v := range values {
						d.WriteString("\n \\-  %s")
						out = append(out, fmt.Sprint(v))
					}
					d.WriteString("\n")
				}
			}
		case "date":
			if field.Val == nil {
				d.WriteString("\n ✖️\n")
			} else {
				d.WriteString("```\n%s\n```\n")
				out = append(out, fmt.Sprint(time.UnixMilli(int64(field.Val.(float64))).Format("02.01.2006")))
			}
		case "color":
			if field.Val == nil {
				d.WriteString("\n ✖️\n")
			} else {
				d.WriteString("```\n%s\n```\n")
				out = append(out, fmt.Sprint(field.Val))
			}
		case "attachment":
			if field.Val == nil {
				d.WriteString("\n ✖️\n")
			} else {
				if v, ok := fileName[fmt.Sprint(field.Val)]; ok {
					d.WriteString("```\n%s\n```\n")
					out = append(out, v)
				} else {
					d.WriteString("\n 🖼\n")
				}
			}
		}
	}
	msg.body = Stelegramf(d.String(), out...)

	t.Send(tgId, msg)
}

func (t *TgService) UserMentionNotification(user dao.User, comment dao.IssueComment) {
	if user.TelegramId == nil || t.Disabled {
		return
	}

	msg := NewTgMsg()
	msg.title = fmt.Sprintf(
		"*%s* %s [%s](%s)",
		bot.EscapeMarkdown(comment.Actor.GetName()),
		bot.EscapeMarkdown("упомянул(-а) вас в комментарии"),
		bot.EscapeMarkdown(comment.Issue.FullIssueName()),
		comment.Issue.URL.String(),
	)

	msg.body = Stelegramf("```\n%s```",
		utils.HtmlToTg(comment.CommentHtml.Body))
	t.Send(*user.TelegramId, msg)
}

func (t *TgService) WorkspaceInvitation(workspaceMember dao.WorkspaceMember) {
	if t.Disabled {
		return
	}

	if workspaceMember.Member.TelegramId != nil {
		msg := NewTgMsg()
		msg.title = fmt.Sprintf(
			"Вас добавили в пространство [%s](%s)",
			bot.EscapeMarkdown(workspaceMember.Workspace.Slug),
			workspaceMember.Workspace.URL.String(),
		)
		t.Send(*workspaceMember.Member.TelegramId, msg)
	}

	if workspaceMember.CreatedBy.TelegramId != nil {
		msg := NewTgMsg()
		msg.title = fmt.Sprintf(
			"Вы добавили пользователя *%s* в пространство [%s](%s)",
			bot.EscapeMarkdown(workspaceMember.Member.GetName()),
			bot.EscapeMarkdown(workspaceMember.Workspace.Slug),
			workspaceMember.Workspace.URL.String(),
		)

		t.Send(*workspaceMember.CreatedBy.TelegramId, msg)
	}
}

func (t *TgService) UserBlockedUntil(user dao.User, until time.Time) {
	if user.TelegramId == nil || t.Disabled {
		return
	}

	msg := NewTgMsg()
	msg.title = Stelegramf("❗️ Ваша учетная запись была заблокирована")
	msg.body = Stelegramf("из-за подозрительной активности до *%s*", until.Format("02.01.2006 15:04"))
	t.Send(*user.TelegramId, msg)
}
