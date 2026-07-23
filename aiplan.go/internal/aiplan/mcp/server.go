package mcp

import (
	"context"
	"log/slog"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/business"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/mcp/prompts"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/mcp/resources"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/mcp/tools"
	"github.com/labstack/echo/v4"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"gorm.io/gorm"
)

// mcpInstructions содержит описание MCP сервера для LLM-моделей.
const mcpInstructions = `MCP сервер для работы с системой управления проектами АИПлан.
АИПлан — система с трекингом задач, документацией (AIDoc), спринтами, метками и ролевой моделью доступа.

## Структура данных

### Иерархия сущностей
- Workspace (пространство) → Project (проект) → Issue (задача)
- Workspace (пространство) → Doc (документ)
- У каждого Workspace есть Owner (владелец)
- У каждого Project есть ProjectLead (руководитель проекта)
- Задачи могут иметь подзадачи (parent_id), метки (labels), спринты (sprints), исполнителей (assignees) и наблюдателей (watchers)
- Документы могут быть вложенными (parent_doc_id), иметь черновик (draft) и настраиваемые права доступа

### Роли пользователей (role)
Роли используются в WorkspaceMember и ProjectMember для определения прав доступа:
- 5 (Guest) — гостевой доступ, только просмотр
- 10 (Member) — участник, может создавать и редактировать задачи и документы
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

## Доступные инструменты

### Задачи (Issues)
- get_issue — получение задачи по ID, sequence ID или ссылке
- search_issues — поиск задач с фильтрацией и сортировкой
- create_issue — создание новой задачи в проекте
- update_issue — обновление задачи (админ/автор — все поля, участник — только статус)
- get_issue_comments — комментарии к задаче
- get_issue_comment — один комментарий по UUID
- get_issue_activity — история изменений задачи (с фильтром по полю)
- get_issue_links — внешние ссылки задачи
- get_issue_attachments — вложения задачи
- create_issue_comment — создание комментария к задаче

### Проекты (Projects)
- get_project — полная информация о проекте
- get_project_stats — агрегированная статистика (по исполнителям, меткам, спринтам, временная шкала)
- get_state_list — список статусов проекта, сгруппированных по group
- get_state — получение статуса по ID
- get_project_member_list — участники проекта с пагинацией и поиском
- get_project_member — информация об участнике проекта
- get_project_labels — метки (теги) проекта
- get_issue_label — метка по ID

### Пространства (Workspaces)
- get_user_workspaces — список пространств текущего пользователя
- get_workspace_projects — проекты пространства
- get_workspace_docs — документы пространства (с фильтрами: вложенные, черновики)

### Спринты
- get_sprints — список спринтов пространства со статистикой

### Документы (AIDoc)
- get_doc — получение документа по UUID
- create_doc — создание документа (workspace_id, title, content в HTML/TipTap, parent_doc_id, роли доступа, draft)
- update_doc — обновление документа (title, content, draft)

## Ресурсы
- aiplan://users/current — информация о текущем пользователе (имя, email, права, настройки)

## Идентификаторы задач
Задачи можно получать по:
- UUID (например: 595aaa46-f5ec-423d-8272-eb29b602ee08)
- Sequence ID формата {workspace.slug}-{project.identifier}-{issue.sequence} (например: test-PORTAL-1960)
- Короткой ссылке: https://{host}/i/{workspace.slug}/{project.identifier}/{issue.sequence}

## Идентификаторы комментариев
Комментарии можно получать по:
- UUID (например: 595aaa46-f5ec-423d-8272-eb29b602ee08)
- Ссылке: https://{host}/{workspace.slug}/projects/{project.identifier}/issues/{issue.sequence}/{comment.id}

## Типичные сценарии работы

### Поиск задач
Для поиска задач через search_issues необходимо знать slug пространства или UUID проекта.
Если ты не знаешь эти идентификаторы, используй промпт search_issues_guide — он вернёт список всех доступных пространств и проектов с их ID, а также подробную инструкцию по поиску.

Пошаговый алгоритм:
1. Вызови промпт search_issues_guide для получения списка пространств и проектов
2. Найди нужное пространство (slug) и проект (UUID)
3. Используй search_issues с параметрами workspace_slugs и/или project_ids

### Создание задачи
1. Узнай project_id (через get_user_workspaces → get_workspace_projects)
2. Опционально: получи state_id через get_state_list, label_ids через get_project_labels
3. Вызови create_issue с project_id и name (обязательные), остальные параметры опциональны

### Обновление задачи
1. Получи задачу через get_issue для актуальных данных
2. Вызови update_issue с нужными полями (передавай только изменяемые поля)
3. Учитывай права: участник может менять только статус, админ/автор — все поля

### Работа с документами
1. Получи workspace_id через get_user_workspaces
2. Для просмотра: get_workspace_docs → get_doc
3. Для создания: create_doc с workspace_id, title и content (HTML формат TipTap)
4. Для обновления: update_doc с doc_id и изменяемыми полями

### Анализ проекта
1. get_project_stats с флагами include_assignee_stats, include_label_stats, include_sprint_stats, include_timeline
2. get_sprints для информации о спринтах
3. search_issues с фильтрами для детализации
`

// NewMCPServer создаёт MCP сервер с доступом к БД и business слою.
func NewMCPServer(db *gorm.DB, bl *business.Business, version string) echo.HandlerFunc {
	hooks := &server.Hooks{}
	hooks.AddOnError(ErrorLoggerHook)

	srv := server.NewMCPServer(
		"aiplan-mcp",
		version,
		server.WithInstructions(mcpInstructions),
		server.WithHooks(hooks),
	)
	srv.AddTools(tools.GetIssuesTools(db, bl)...)
	srv.AddTools(tools.GetProjectsTools(db, bl)...)
	srv.AddTools(tools.GetWorkspacesTools(db, bl)...)
	srv.AddTools(tools.GetDocsTools(db, bl)...)

	srv.AddResources(resources.GetUsersResources(db)...)

	srv.AddPrompts(prompts.GetSearchPrompts(db)...)

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
