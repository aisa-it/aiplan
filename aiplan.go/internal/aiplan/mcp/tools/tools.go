package tools

import (
	"context"
	"errors"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"gorm.io/gorm"
)

type ToolHandler func(db *gorm.DB, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error)

type Tool struct {
	Tool    mcp.Tool
	Handler ToolHandler
}

func WrapTool(db *gorm.DB, handler ToolHandler) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		userRaw := ctx.Value("user")
		if userRaw == nil {
			return nil, errors.New("user not provided")
		}
		user := userRaw.(*dao.User)
		return handler(db, user, request)
	}
}
