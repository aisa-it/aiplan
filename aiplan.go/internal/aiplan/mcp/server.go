package mcp

import (
	"context"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/mcp/resources"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/mcp/tools"
	"github.com/labstack/echo/v4"
	"github.com/mark3labs/mcp-go/server"
	"gorm.io/gorm"
)

func NewMCPServer(db *gorm.DB) echo.HandlerFunc {
	srv := server.NewMCPServer(
		"aiplan-mcp",
		"version",
		server.WithInstructions("MCP сервер для получения информации из системы отслеживания задач АИПлан"),
	)

	srv.AddTools(tools.GetIssuesTools(db)...)
	srv.AddTools(tools.GetProjectsTools(db)...)
	srv.AddTools(tools.GetWorkspacesTools(db)...)

	srv.AddResources(resources.GetUsersResources(db)...)

	httpServer := server.NewStreamableHTTPServer(srv)
	return func(c echo.Context) error {
		sessionCtx := context.WithValue(c.Request().Context(), "user", c.Get("user"))
		httpServer.ServeHTTP(c.Response(), c.Request().WithContext(sessionCtx))
		return nil
	}
}
