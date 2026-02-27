package prompts

import (
	"context"
	"errors"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"gorm.io/gorm"
)

// PromptHandler определяет сигнатуру функции-обработчика MCP промпта.
type PromptHandler func(ctx context.Context, db *gorm.DB, user *dao.User, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error)

// Prompt представляет MCP промпт с его обработчиком.
type Prompt struct {
	Prompt  mcp.Prompt
	Handler PromptHandler
}

// WrapPrompt оборачивает обработчик промпта, извлекая пользователя из контекста.
func WrapPrompt(db *gorm.DB, handler PromptHandler) server.PromptHandlerFunc {
	return func(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		userRaw := ctx.Value("user")
		if userRaw == nil {
			return nil, errors.New("user not provided")
		}
		user := userRaw.(*dao.User)
		return handler(ctx, db, user, request)
	}
}
