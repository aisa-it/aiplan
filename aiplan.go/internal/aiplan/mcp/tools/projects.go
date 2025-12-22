package tools

import (
	"context"
	"strings"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/apierrors"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/business"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dto"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/mcp/logger"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var projectsTools = []Tool{
	{
		mcp.NewTool(
			"get_workspace_projects",
			mcp.WithDescription("Получение списка проектов пространства с пагинацией"),
			mcp.WithIdempotentHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("workspace_id",
				mcp.Required(),
				mcp.Description("ID пространства (UUID или slug)"),
			),
			mcp.WithNumber("offset",
				mcp.Description("Смещение для пагинации (по умолчанию 0)"),
			),
			mcp.WithNumber("limit",
				mcp.Description("Лимит записей (по умолчанию 100)"),
			),
		),
		getWorkspaceProjects,
	},
	{
		mcp.NewTool(
			"get_state_list",
			mcp.WithDescription("Получение списка статусов проекта"),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("project_id",
				mcp.Required(),
				mcp.Description("ID проекта (UUID)"),
			),
			mcp.WithString("search_query",
				mcp.Description("Поисковый запрос для фильтрации статусов по названию"),
			),
		),
		ProjectPermissionsMiddleware(getStateList),
	},
	{
		mcp.NewTool(
			"get_project_stats",
			mcp.WithDescription("Получение агрегированной статистики проекта"),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("project_id",
				mcp.Required(),
				mcp.Description("ID проекта (UUID)"),
			),
			mcp.WithBoolean("include_assignee_stats",
				mcp.Description("Включить статистику по исполнителям (топ-50)"),
			),
			mcp.WithBoolean("include_label_stats",
				mcp.Description("Включить статистику по меткам (топ-50)"),
			),
			mcp.WithBoolean("include_sprint_stats",
				mcp.Description("Включить статистику по спринтам (последние 50)"),
			),
			mcp.WithBoolean("include_timeline",
				mcp.Description("Включить временную статистику (создано/завершено по месяцам за 12 месяцев)"),
			),
		),
		ProjectPermissionsMiddleware(getProjectStats),
	},
	{
		mcp.NewTool(
			"get_project",
			mcp.WithDescription("Получение полной информации о проекте"),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("project_id",
				mcp.Required(),
				mcp.Description("ID проекта (UUID)"),
			),
		),
		ProjectPermissionsMiddleware(getProject),
	},
	{
		mcp.NewTool(
			"get_project_member_list",
			mcp.WithDescription("Получение списка участников проекта с пагинацией"),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("project_id",
				mcp.Required(),
				mcp.Description("ID проекта (UUID)"),
			),
			mcp.WithNumber("offset",
				mcp.Description("Смещение для пагинации (по умолчанию 0)"),
			),
			mcp.WithNumber("limit",
				mcp.Description("Лимит записей (по умолчанию 100)"),
			),
			mcp.WithString("search_query",
				mcp.Description("Поиск по имени или email участника"),
			),
		),
		ProjectPermissionsMiddleware(getProjectMemberList),
	},
	{
		mcp.NewTool(
			"get_project_member",
			mcp.WithDescription("Получение информации об участнике проекта"),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("project_id",
				mcp.Required(),
				mcp.Description("ID проекта (UUID)"),
			),
			mcp.WithString("member_id",
				mcp.Required(),
				mcp.Description("ID участника проекта (UUID)"),
			),
		),
		ProjectPermissionsMiddleware(getProjectMember),
	},
	{
		mcp.NewTool(
			"get_issue_label",
			mcp.WithDescription("Получение метки (тега) задачи по ID"),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("project_id",
				mcp.Required(),
				mcp.Description("ID проекта (UUID)"),
			),
			mcp.WithString("label_id",
				mcp.Required(),
				mcp.Description("ID метки (UUID)"),
			),
		),
		ProjectPermissionsMiddleware(getIssueLabel),
	},
	{
		mcp.NewTool(
			"get_state",
			mcp.WithDescription("Получение статуса по ID"),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("project_id",
				mcp.Required(),
				mcp.Description("ID проекта (UUID)"),
			),
			mcp.WithString("state_id",
				mcp.Required(),
				mcp.Description("ID статуса (UUID)"),
			),
		),
		ProjectPermissionsMiddleware(getState),
	},
}

func GetProjectsTools(db *gorm.DB, bl *business.Business) []server.ServerTool {
	var result []server.ServerTool
	for _, t := range projectsTools {
		result = append(result, server.ServerTool{
			Tool:    t.Tool,
			Handler: WrapTool(db, bl, t.Handler),
		})
	}
	return result
}

func ProjectPermissionsMiddleware(handler ToolHandler) ToolHandler {
	return func(ctx context.Context, db *gorm.DB, bl *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectId, ok := request.GetArguments()["project_id"]
		if !ok {
			return apierrors.ErrProjectIdentifierRequired.MCPError(), nil
		}

		var projectMember dao.ProjectMember
		if err := db.Where("member_id = ?", user.ID).Where("project_id = ?", projectId).First(&projectMember).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return apierrors.ErrProjectForbidden.MCPError(), nil
			}
			return logger.Error(err), nil
		}

		ctx = context.WithValue(ctx, "projectMember", projectMember)

		return handler(ctx, db, bl, user, request)
	}
}

func getWorkspaceProjects(ctx context.Context, db *gorm.DB, bl *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	// Получаем workspace_id (обязательный)
	workspaceIdOrSlug := args["workspace_id"].(string)

	// Получаем параметры пагинации
	offset := 0
	limit := 100
	if v, ok := args["offset"].(float64); ok {
		offset = int(v)
	}
	if v, ok := args["limit"].(float64); ok {
		limit = int(v)
	}

	// Находим workspace по UUID или slug
	var workspace dao.Workspace
	if _, err := uuid.FromString(workspaceIdOrSlug); err == nil {
		// UUID
		if err := db.Where("id = ?", workspaceIdOrSlug).First(&workspace).Error; err != nil {
			return mcp.NewToolResultError("workspace не найден"), nil
		}
	} else {
		// Slug
		if err := db.Where("slug = ?", workspaceIdOrSlug).First(&workspace).Error; err != nil {
			return mcp.NewToolResultError("workspace не найден"), nil
		}
	}

	// Проверяем доступ пользователя к workspace
	var workspaceMember dao.WorkspaceMember
	hasAccess := true
	if err := db.Where("workspace_id = ? AND member_id = ?", workspace.ID, user.ID).First(&workspaceMember).Error; err != nil {
		if !user.IsSuperuser {
			return mcp.NewToolResultError("нет доступа к workspace"), nil
		}
		hasAccess = false
	}

	// Запрос проектов
	var projects []dao.ProjectWithCount
	query := db.
		Preload("DefaultAssigneesDetails", "is_default_assignee = ?", true).
		Preload("DefaultWatchersDetails", "is_default_watcher = ?", true).
		Preload("Workspace").
		Preload("Workspace.Owner").
		Preload("ProjectLead").
		Select("*,(?) as total_members, (?) as is_favorite",
			db.Model(&dao.ProjectMember{}).Select("count(*)").Where("project_members.project_id = projects.id"),
			db.Raw("EXISTS(SELECT 1 FROM project_favorites WHERE project_favorites.project_id = projects.id AND user_id = ?)", user.ID)).
		Set("userId", user.ID).
		Where("workspace_id = ?", workspace.ID).
		Order("is_favorite desc, lower(name)")

	// Фильтрация по доступу (если пользователь не админ workspace и не суперпользователь)
	if hasAccess && workspaceMember.Role != types.AdminRole && !user.IsSuperuser {
		query = query.Where("id in (?) or public = true",
			db.Model(&dao.ProjectMember{}).Select("project_id").Where("member_id = ?", user.ID))
	}

	// Подсчет общего количества
	var count int64
	if err := query.Model(&dao.Project{}).Count(&count).Error; err != nil {
		return mcp.NewToolResultError("ошибка при подсчете проектов"), nil
	}

	// Получаем проекты с пагинацией
	if err := query.Offset(offset).Limit(limit).Find(&projects).Error; err != nil {
		return mcp.NewToolResultError("ошибка при получении проектов"), nil
	}

	// Формируем ответ
	result := dao.PaginationResponse{
		Count:  count,
		Offset: offset,
		Limit:  limit,
		Result: utils.SliceToSlice(&projects, func(p *dao.ProjectWithCount) dto.ProjectLight {
			return *p.ToLightDTO()
		}),
	}

	return mcp.NewToolResultJSON(result)
}

// getStateList возвращает список статусов проекта, сгруппированных по группам
func getStateList(ctx context.Context, db *gorm.DB, bl *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	projectId := args["project_id"].(string)

	// Получаем search_query (опциональный)
	searchQuery := ""
	if v, ok := args["search_query"].(string); ok {
		searchQuery = v
	}

	// Строим запрос
	query := db.
		Order("sequence").
		Where("project_id = ?", projectId)

	// Фильтрация по поисковому запросу
	if searchQuery != "" {
		escapedQuery := "%" + strings.ToLower(searchQuery) + "%"
		query = query.Where("lower(name) LIKE ?", escapedQuery)
	}

	var states []dao.State
	if err := query.Find(&states).Error; err != nil {
		return mcp.NewToolResultError("ошибка при получении статусов"), nil
	}

	// Группируем по Group (как в HTTP API)
	result := make(map[string][]dto.StateLight)
	for _, state := range states {
		arr, ok := result[state.Group]
		if !ok {
			arr = make([]dto.StateLight, 0)
		}
		arr = append(arr, *state.ToLightDTO())
		result[state.Group] = arr
	}

	return mcp.NewToolResultJSON(result)
}

// getProjectStats возвращает агрегированную статистику проекта.
// Использует business.GetProjectStats для получения данных.
func getProjectStats(ctx context.Context, db *gorm.DB, bl *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	projectIdStr := args["project_id"].(string)

	projectID, err := uuid.FromString(projectIdStr)
	if err != nil {
		return mcp.NewToolResultError("некорректный формат project_id"), nil
	}

	// Собираем опции
	opts := dto.ProjectStatsRequest{}

	if v, ok := args["include_assignee_stats"].(bool); ok {
		opts.IncludeAssigneeStats = v
	}
	if v, ok := args["include_label_stats"].(bool); ok {
		opts.IncludeLabelStats = v
	}
	if v, ok := args["include_sprint_stats"].(bool); ok {
		opts.IncludeSprintStats = v
	}
	if v, ok := args["include_timeline"].(bool); ok {
		opts.IncludeTimeline = v
	}

	// Вызываем business логику
	stats, err := bl.GetProjectStats(projectID, opts)
	if err != nil {
		return logger.Error(err), nil
	}

	return mcp.NewToolResultJSON(stats)
}

// getProject возвращает полную информацию о проекте.
func getProject(ctx context.Context, db *gorm.DB, bl *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	projectIdStr := args["project_id"].(string)

	projectID, err := uuid.FromString(projectIdStr)
	if err != nil {
		return mcp.NewToolResultError("некорректный формат project_id"), nil
	}

	var project dao.Project
	if err := db.
		Preload("ProjectLead").
		Preload("Workspace").
		Preload("Workspace.Owner").
		Where("id = ?", projectID).
		First(&project).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return apierrors.ErrProjectNotFound.MCPError(), nil
		}
		return logger.Error(err), nil
	}

	return mcp.NewToolResultJSON(project.ToDTO())
}

// getProjectMemberList возвращает список участников проекта с пагинацией и поиском.
func getProjectMemberList(ctx context.Context, db *gorm.DB, bl *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	projectIdStr := args["project_id"].(string)

	projectID, err := uuid.FromString(projectIdStr)
	if err != nil {
		return mcp.NewToolResultError("некорректный формат project_id"), nil
	}

	offset := 0
	limit := 100
	if v, ok := args["offset"].(float64); ok {
		offset = int(v)
	}
	if v, ok := args["limit"].(float64); ok {
		limit = int(v)
	}

	searchQuery := ""
	if v, ok := args["search_query"].(string); ok {
		searchQuery = v
	}

	query := db.
		Where("project_id = ?", projectID).
		Joins("Member").
		Preload(clause.Associations).
		Preload("Workspace.Owner").
		Order("\"Member\".last_name")

	// Поиск
	if searchQuery != "" {
		escapedQuery := "%" + strings.ToLower(searchQuery) + "%"
		query = query.Where(
			"lower(\"Member\".username) LIKE ? OR lower(\"Member\".email) LIKE ? OR lower(\"Member\".last_name) LIKE ? OR lower(\"Member\".first_name) LIKE ?",
			escapedQuery, escapedQuery, escapedQuery, escapedQuery,
		)
	}

	var count int64
	if err := query.Model(&dao.ProjectMember{}).Count(&count).Error; err != nil {
		return logger.Error(err), nil
	}

	var members []dao.ProjectMember
	if err := query.Offset(offset).Limit(limit).Find(&members).Error; err != nil {
		return logger.Error(err), nil
	}

	result := dao.PaginationResponse{
		Count:  count,
		Offset: offset,
		Limit:  limit,
		Result: utils.SliceToSlice(&members, func(m *dao.ProjectMember) dto.ProjectMemberLight {
			return *m.ToLightDTO()
		}),
	}

	return mcp.NewToolResultJSON(result)
}

// getProjectMember возвращает информацию об участнике проекта.
func getProjectMember(ctx context.Context, db *gorm.DB, bl *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	projectIdStr := args["project_id"].(string)
	memberIdStr := args["member_id"].(string)

	var member dao.ProjectMember
	if err := db.
		Where("project_id = ?", projectIdStr).
		Where("project_members.id = ?", memberIdStr).
		Joins("Workspace").
		Joins("Member").
		Preload(clause.Associations).
		Preload("Workspace.Owner").
		First(&member).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return mcp.NewToolResultError("участник проекта не найден"), nil
		}
		return logger.Error(err), nil
	}

	return mcp.NewToolResultJSON(member.ToLightDTO())
}

// getIssueLabel возвращает метку (тег) задачи по ID.
func getIssueLabel(ctx context.Context, db *gorm.DB, bl *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	projectIdStr := args["project_id"].(string)
	labelIdStr := args["label_id"].(string)

	var label dao.Label
	if err := db.
		Where("project_id = ?", projectIdStr).
		Where("id = ?", labelIdStr).
		Preload("Parent").
		First(&label).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return mcp.NewToolResultError("метка не найдена"), nil
		}
		return logger.Error(err), nil
	}

	return mcp.NewToolResultJSON(label.ToLightDTO())
}

// getState возвращает статус по ID.
func getState(ctx context.Context, db *gorm.DB, bl *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	projectIdStr := args["project_id"].(string)
	stateIdStr := args["state_id"].(string)

	stateID, err := uuid.FromString(stateIdStr)
	if err != nil {
		return mcp.NewToolResultError("некорректный формат state_id"), nil
	}

	var state dao.State
	if err := db.
		Preload(clause.Associations).
		Where("project_id = ?", projectIdStr).
		Where("id = ?", stateID).
		First(&state).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return apierrors.ErrProjectStateNotFound.MCPError(), nil
		}
		return logger.Error(err), nil
	}

	return mcp.NewToolResultJSON(state.ToLightDTO())
}
