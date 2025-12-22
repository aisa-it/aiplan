package mcp

import (
	"context"
	"log/slog"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/business"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/mcp/resources"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/mcp/tools"
	"github.com/labstack/echo/v4"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"gorm.io/gorm"
)

// mcpInstructions содержит описание MCP сервера для LLM-моделей.
const mcpInstructions = `MCP сервер для получения информации из системы отслеживания задач АИПлан

## Структура данных

### Иерархия сущностей
- Workspace (пространство) → Project (проект) → Issue (задача)
- У каждого Workspace есть Owner (владелец)
- У каждого Project есть ProjectLead (руководитель проекта)

### Роли пользователей (role)
Роли используются в WorkspaceMember и ProjectMember для определения прав доступа:
- 5 (Guest) — гостевой доступ, только просмотр
- 10 (Member) — участник, может создавать и редактировать задачи
- 15 (Admin) — администратор, полный доступ к настройкам проекта/пространства

### Группы статусов (state.group)
Статусы задач группируются по этапам workflow:
- backlog — задачи в бэклоге, ещё не взяты в работу
- unstarted — запланированные, но не начатые
- started — в процессе выполнения
- completed — успешно завершённые
- cancelled — отменённые

### Приоритеты задач (priority)
- urgent — срочный
- high — высокий
- medium — средний
- low — низкий
- none — без приоритета

## Идентификаторы задач
Задачи можно получать по:
- UUID (например: 595aaa46-f5ec-423d-8272-eb29b602ee08)
- Sequence ID формата {workspace.slug}-{project.identifier}-{issue.sequence} (например: test-PORTAL-1960)
- Короткой ссылке: https://{host}/i/{workspace.slug}/{project.identifier}/{issue.sequence}
`

// NewMCPServer создаёт MCP сервер с доступом к БД и business слою.
func NewMCPServer(db *gorm.DB, bl *business.Business) echo.HandlerFunc {
	hooks := &server.Hooks{}
	hooks.AddOnError(ErrorLoggerHook)

	srv := server.NewMCPServer(
		"aiplan-mcp",
		"version",
		server.WithInstructions(mcpInstructions),
		server.WithHooks(hooks),
	)
	srv.AddTools(tools.GetIssuesTools(db, bl)...)
	srv.AddTools(tools.GetProjectsTools(db, bl)...)
	srv.AddTools(tools.GetWorkspacesTools(db, bl)...)

	srv.AddResources(resources.GetUsersResources(db)...)

	httpServer := server.NewStreamableHTTPServer(srv)
	return func(c echo.Context) error {
		sessionCtx := context.WithValue(c.Request().Context(), "user", c.Get("user"))
		httpServer.ServeHTTP(c.Response(), c.Request().WithContext(sessionCtx))
		return nil
	}
}

func ErrorLoggerHook(ctx context.Context, id any, method mcp.MCPMethod, message any, err error) {
	user := ctx.Value("user")
	slog.Error("MCP Error", "user", user, "id", id, "method", method, "message", message, "err", err)
}
