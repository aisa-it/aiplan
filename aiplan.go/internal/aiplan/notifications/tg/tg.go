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
}

func NewTgMsg() TgMsg {
	return TgMsg{
		replace: make(map[string]any),
	}
}

func New(db *gorm.DB, cfg *config.Config, tracker *tracker.ActivitiesTracker, bl *business.Business) *TgService {
	if cfg.TelegramBotToken == "" {
		slog.Info("Telegram notifications disabled")
		return &TgService{Disabled: true}
	}

	opts := []bot.Option{
		bot.WithDefaultHandler(func(ctx context.Context, bot *bot.Bot, update *models.Update) {}),
		//bot.WithMiddlewares(TelegramAuthMiddleware(db)),
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

	serv := &TgService{
		db:          db,
		bot:         b,
		botUserName: res.Username,
		cfg:         cfg,
		tracker:     tracker,
		Disabled:    false,
		bl:          bl,
		ctx:         ctx,
		cancel:      cancel,
	}

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
	if update.Message.ReplyToMessage == nil {
		return false
	}
	return true
}

func (t *TgService) SendMessage(tgId int64, format string, anyStr []string) bool {
	fmt.Println("SendMessage")
	return false
}

func (t *TgService) SendFormAnswer(tgId int64, form dao.Form, answer *dao.FormAnswer, user *dao.User) {
	fmt.Println("SendFormAnswer")
}

func (t *TgService) UserMentionNotification(user dao.User, comment dao.IssueComment) {
	fmt.Println("UserMentionNotification")
	//if user.TelegramId == nil || ts.disabled {
	//  return
	//}
	//tmpComment := replaceTablesToText(comment.CommentHtml.Body)
	//tmpComment = replaceImageToText(tmpComment)
	//tmpComment = prepareHtmlBody(htmlStripPolicy, tmpComment)
	//msg := tgbotapi.NewMessage(*user.TelegramId, fmt.Sprintf("%s %s упомянул(-а) вас в комментарии [%s](%s):\n```\n%s```",
	//  comment.Actor.FirstName,
	//  comment.Actor.LastName,
	//  comment.Issue.FullIssueName(),
	//  comment.Issue.URL.String(),
	//  replaceImgToEmoj(tmpComment),
	//))
	//msg.ParseMode = "markdown"
	//msg.DisableWebPagePreview = true
	//ts.bot.Send(msg)
}

//func helloHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
//  b.SendMessage(ctx, &bot.SendMessageParams{
//    ChatID:    update.Message.Chat.ID,
//    Text:      "Hello, *" + bot.EscapeMarkdown(update.Message.From.FirstName) + "*",
//    ParseMode: models.ParseModeMarkdown,
//  })
//}

func (t *TgService) WorkspaceInvitation(workspaceMember dao.WorkspaceMember) {
	fmt.Println("WorkspaceInvitation")

	//if ts.disabled {
	//  return
	//}
	//
	//if workspaceMember.Member.TelegramId != nil {
	//  msg := tgbotapi.NewMessage(*workspaceMember.Member.TelegramId, Stelegramf("Вас добавили в пространство [%s](%s)",
	//    workspaceMember.Workspace.Slug,
	//    workspaceMember.Workspace.URL.String()))
	//  msg.ParseMode = "MarkdownV2"
	//  msg.DisableWebPagePreview = true
	//  ts.bot.Send(msg)
	//}
	//
	//if workspaceMember.CreatedBy.TelegramId != nil {
	//  msg := tgbotapi.NewMessage(*workspaceMember.CreatedBy.TelegramId, Stelegramf("Вы добавили пользователя *%s* в пространство [%s](%s)",
	//    workspaceMember.Member.GetName(),
	//    workspaceMember.Workspace.Slug,
	//    workspaceMember.Workspace.URL.String()))
	//  msg.ParseMode = "MarkdownV2"
	//  msg.DisableWebPagePreview = true
	//  ts.bot.Send(msg)
	//}
}

func (t *TgService) UserBlockedUntil(user dao.User, until time.Time) {
	fmt.Println("UserBlockedUntil")

	//if user.TelegramId == nil || ts.disabled {
	//  return
	//}
	//
	//msg := tgbotapi.NewMessage(*user.TelegramId, fmt.Sprintf("❗️ Ваша учетная запись была заблокирована из-за подозрительной активности до *%s*", until.Format("02.01.2006 15:04")))
	//msg.ParseMode = "markdown"
	//msg.DisableWebPagePreview = true
	//ts.bot.Send(msg)
}
