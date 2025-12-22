package tools

import (
	"context"
	"errors"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/business"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"gorm.io/gorm"
)

// ToolHandler определяет сигнатуру функции-обработчика MCP инструмента.
// Получает контекст, соединение с БД, business слой, текущего пользователя и параметры запроса.
type ToolHandler func(ctx context.Context, db *gorm.DB, bl *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error)

// Tool представляет MCP инструмент с его обработчиком.
type Tool struct {
	Tool    mcp.Tool
	Handler ToolHandler
}

// WrapTool оборачивает обработчик инструмента, извлекая пользователя из контекста.
func WrapTool(db *gorm.DB, bl *business.Business, handler ToolHandler) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		userRaw := ctx.Value("user")
		if userRaw == nil {
			return nil, errors.New("user not provided")
		}
		user := userRaw.(*dao.User)
		return handler(ctx, db, bl, user, request)
	}
}
