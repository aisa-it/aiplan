package tools

import (
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dto"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"gorm.io/gorm"
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
}

func GetProjectsTools(db *gorm.DB) []server.ServerTool {
	var result []server.ServerTool
	for _, t := range projectsTools {
		result = append(result, server.ServerTool{
			Tool:    t.Tool,
			Handler: WrapTool(db, t.Handler),
		})
	}
	return result
}

func getWorkspaceProjects(db *gorm.DB, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
