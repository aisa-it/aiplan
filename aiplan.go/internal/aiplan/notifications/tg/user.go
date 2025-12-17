package tg

import (
	"context"
	"fmt"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"gorm.io/gorm"
)

type UserContext struct {
	context.Context
	User *dao.User
	//Update *models.Update
	//Bot    *bot.Bot
}

func TelegramAuthMiddleware(db *gorm.DB) bot.Middleware {
	return func(next bot.HandlerFunc) bot.HandlerFunc {
		return func(ctx context.Context, b *bot.Bot, update *models.Update) {
			if update.Message == nil {
				next(ctx, b, update)
				return
			}

			chatID := update.Message.Chat.ID

			var user dao.User
			err := db.Where("telegram_id = ?", chatID).First(&user).Error

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
			Text: fmt.Sprintf(
				"Ваш Telegram ID: `%d`\nВнесите его в настройках профиля",
				update.Message.Chat.ID,
			),
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
