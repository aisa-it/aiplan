package tg

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gofrs/uuid"
)

type userTg struct {
	id  int64
	loc types.TimeZone

	projectDefaultWatcher  bool
	projectDefaultAssigner bool

	issueAuthor   bool
	issueWatcher  bool
	issueAssigner bool
}

type UserContext struct {
	context.Context
	User *dao.User
	//Update *models.Update
	//Bot    *bot.Bot
}

func (t *TgService) getUserMiddleware() bot.Middleware {
	return func(next bot.HandlerFunc) bot.HandlerFunc {
		return func(ctx context.Context, b *bot.Bot, update *models.Update) {
			if update.Message == nil {
				next(ctx, b, update)
				return
			}

			chatID := update.Message.Chat.ID

			var user dao.User
			err := t.db.Where("telegram_id = ?", chatID).First(&user).Error

			userCtx := &UserContext{
				Context: ctx,
				User:    nil,
			}

			if err == nil {
				userCtx.User = &user
			}
			next(userCtx, b, update)
		}
	}
}

func (t *TgService) startHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	userCtx := ctx.(*UserContext)

	if userCtx.User == nil {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text: bot.EscapeMarkdown(fmt.Sprintf(
				"Ваш Telegram ID: `%d`\nВнесите его в настройках профиля",
				update.Message.Chat.ID,
			)),
			ParseMode: models.ParseModeMarkdown,
		})
		return
	}

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		Text:      bot.EscapeMarkdown("Вы подключены к уведомлениям."),
		ParseMode: models.ParseModeMarkdown,
	})
}

func (t *TgService) commentActivityHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	userCtx := ctx.(*UserContext)

	if userCtx.User == nil {
		t.Send(update.Message.Chat.ID, TgMsg{
			title: "Telegram ID не связан с пользователем, добавьте в профиль\n/start",
		})
		return
	}

	user := userCtx.User

	var issue dao.IssueActivity
	issue.UnionCustomFields = "'issue' AS entity_type"
	var doc dao.DocActivity
	doc.UnionCustomFields = "'doc' AS entity_type"
	unionTable := dao.BuildUnionSubquery(t.db, "ua", dao.FullActivity{}, issue, doc)

	var act dao.FullActivity

	if err := unionTable.Unscoped().
		Joins("Issue").
		Joins("Doc").
		Where("? = any (telegram_msg_ids)", update.Message.ReplyToMessage.ID).First(&act).Error; err != nil {
		t.Send(update.Message.Chat.ID, TgMsg{
			title: "Не возможно оставлять комментарий",
		})
		return

	}

	if act.Issue != nil {
		var identifier uuid.UUID
		if act.Field != nil && *act.Field == actField.Comment.Field.String() && act.NewIdentifier.Valid {
			identifier = act.NewIdentifier.UUID
		}
		err := t.bl.CreateIssueComment(*act.Issue, *user, update.Message.Text, identifier, true)
		if err != nil {
			if err.Error() == "create comment forbidden" {
				t.Send(update.Message.Chat.ID, TgMsg{
					title: "У вас нет прав оставлять комментарии в данном проекте",
				})
				return
			}
			slog.Error("Create comment from tg reply", "err", err)
			return
		}
		t.Send(update.Message.Chat.ID, TgMsg{title: fmt.Sprintf("Комментарий к задаче '%s'\nотправлен", act.Issue.Name)})
		return
	}

	if act.Doc != nil {
		err := t.bl.CreateDocComment(*act.Doc, *user, update.Message.Text, act.NewIdentifier, true)
		if err != nil {
			if err.Error() == "create comment forbidden" {
				t.Send(update.Message.Chat.ID, TgMsg{
					title: "У вас нет прав оставлять комментарии в данном пространстве",
				})
				return
			}
			slog.Error("Create comment from tg reply", "err", err)
			return
		}
		t.Send(update.Message.Chat.ID, TgMsg{title: fmt.Sprintf("Комментарий в документ '%s'\nотправлен", act.Doc.Title)})
	}

	return

}
