package tools

import (
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dto"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"gorm.io/gorm"
)

var workspacesTools = []Tool{
	{
		mcp.NewTool(
			"get_user_workspaces",
			mcp.WithDescription("Получение списка пространств текущего пользователя с пагинацией"),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithNumber("offset",
				mcp.Description("Смещение для пагинации (по умолчанию 0)"),
			),
			mcp.WithNumber("limit",
				mcp.Description("Лимит записей (по умолчанию 100)"),
			),
		),
		getUserWorkspaces,
	},
}

func GetWorkspacesTools(db *gorm.DB) []server.ServerTool {
	var result []server.ServerTool
	for _, t := range workspacesTools {
		result = append(result, server.ServerTool{
			Tool:    t.Tool,
			Handler: WrapTool(db, t.Handler),
		})
	}
	return result
}

func getUserWorkspaces(db *gorm.DB, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	// Получаем параметры пагинации
	offset := 0
	limit := 100
	if v, ok := args["offset"].(float64); ok {
		offset = int(v)
	}
	if v, ok := args["limit"].(float64); ok {
		limit = int(v)
	}

	// Запрос пространств пользователя
	var workspaces []dao.WorkspaceWithCount
	query := db.Model(&dao.Workspace{}).
		Select("*,(?) as total_members,(?) as total_projects,(?) as is_favorite",
			db.Model(&dao.WorkspaceMember{}).Select("count(*)").Where("workspace_id = workspaces.id"),
			db.Model(&dao.Project{}).Select("count(*)").Where("workspace_id = workspaces.id"),
			db.Raw("EXISTS(select 1 from workspace_favorites WHERE workspace_favorites.workspace_id = workspaces.id AND user_id = ?)", user.ID),
		).
		Preload("Owner").
		Set("userID", user.ID).
		Where("workspaces.id in (?)", db.Model(&dao.WorkspaceMember{}).
			Select("workspace_id").
			Where("member_id = ?", user.ID)).
		Order("is_favorite desc, lower(name)")

	// Подсчет общего количества
	var count int64
	if err := query.Model(&dao.Workspace{}).Count(&count).Error; err != nil {
		return mcp.NewToolResultError("ошибка при подсчете пространств"), nil
	}

	// Получаем пространства с пагинацией
	if err := query.Offset(offset).Limit(limit).Find(&workspaces).Error; err != nil {
		return mcp.NewToolResultError("ошибка при получении пространств"), nil
	}

	// Формируем ответ
	result := dao.PaginationResponse{
		Count:  count,
		Offset: offset,
		Limit:  limit,
		Result: utils.SliceToSlice(&workspaces, func(w *dao.WorkspaceWithCount) dto.WorkspaceWithCount {
			return *w.ToDTO()
		}),
	}

	return mcp.NewToolResultJSON(result)
}
