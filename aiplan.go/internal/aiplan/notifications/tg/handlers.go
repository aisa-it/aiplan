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

type UserContext struct {
	context.Context
	User *dao.User
}

func (t *TgService) getUserMiddleware() bot.Middleware {
	return func(next bot.HandlerFunc) bot.HandlerFunc {
		return func(ctx context.Context, b *bot.Bot, update *models.Update) {
			if update.Message == nil {
				next(ctx, b, update)
				return
			}
			if update.Message.Chat.Type != "private" {
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

			next(userCtx, b, update)
		}
	}
}

func (t *TgService) startHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	userCtx, ok := ctx.(*UserContext)
	if !ok {
		return
	}
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
	user := ctx.(*UserContext).User
	var act dao.ActivityTelegramMessage

	if err := t.db.
		Joins("Activity").
		Joins("Activity.Issue").
		Joins("Activity.Doc").
		Where("message_id = ?", update.Message.ReplyToMessage.ID).First(&act).Error; err != nil {
		t.Send(update.Message.Chat.ID, TgMsg{
			title: "Не возможно оставить комментарий",
		})
		return
	}

	switch act.Activity.EntityType {
	case types.LayerIssue:
		var identifier uuid.UUID
		if act.Activity.Field == actField.Comment.Field && act.Activity.NewIdentifier.Valid {
			identifier = act.Activity.NewIdentifier.UUID
		}
		err := t.bl.CreateIssueComment(*act.Activity.Issue, *user, update.Message.Text, identifier, true)
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
		_, errSend := t.Send(update.Message.Chat.ID, TgMsg{title: fmt.Sprintf("Комментарий к задаче '%s'\nотправлен", bot.EscapeMarkdown(act.Activity.Issue.Name))})
		if errSend != nil {
			slog.Error("Send comment from tg reply", "err", errSend)
		}
		return
	case types.LayerDoc:
		err := t.bl.CreateDocComment(*act.Activity.Doc, *user, update.Message.Text, act.Activity.NewIdentifier, true)
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
		_, errSend := t.Send(update.Message.Chat.ID, TgMsg{title: fmt.Sprintf("Комментарий в документ '%s'\nотправлен", bot.EscapeMarkdown(act.Activity.Doc.Title))})
		if errSend != nil {
			slog.Error("Send comment from tg reply", "err", errSend)
		}
		return
	}
	t.Send(update.Message.Chat.ID, TgMsg{
		title: "Не возможно оставить комментарий",
	})
	return
}
