package tools

import (
	"context"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/business"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dto"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
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
	{
		mcp.NewTool(
			"get_workspace_docs",
			mcp.WithDescription("Получение списка документов пространства с пагинацией"),
			mcp.WithIdempotentHintAnnotation(true),
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
			mcp.WithBoolean("include_nested",
				mcp.Description("Включить вложенные документы (по умолчанию false - только корневые)"),
			),
			mcp.WithBoolean("exclude_drafts",
				mcp.Description("Исключить черновики (по умолчанию false)"),
			),
		),
		getWorkspaceDocs,
	},
}

func GetWorkspacesTools(db *gorm.DB, bl *business.Business) []server.ServerTool {
	var result []server.ServerTool
	for _, t := range workspacesTools {
		result = append(result, server.ServerTool{
			Tool:    t.Tool,
			Handler: WrapTool(db, bl, t.Handler),
		})
	}
	return result
}

func getUserWorkspaces(ctx context.Context, db *gorm.DB, bl *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

// findWorkspaceByIdOrSlug ищет workspace по UUID или slug.
func findWorkspaceByIdOrSlug(db *gorm.DB, idOrSlug string) (*dao.Workspace, error) {
	var workspace dao.Workspace
	if _, err := uuid.FromString(idOrSlug); err == nil {
		return &workspace, db.Where("id = ?", idOrSlug).First(&workspace).Error
	}
	return &workspace, db.Where("slug = ?", idOrSlug).First(&workspace).Error
}

// getWorkspaceMemberOrSuperuser возвращает членство пользователя в workspace или виртуальное членство для суперпользователя.
func getWorkspaceMemberOrSuperuser(db *gorm.DB, workspace *dao.Workspace, user *dao.User) (*dao.WorkspaceMember, error) {
	var workspaceMember dao.WorkspaceMember
	err := db.Where("workspace_id = ? AND member_id = ?", workspace.ID, user.ID).First(&workspaceMember).Error
	if err == nil {
		return &workspaceMember, nil
	}
	if user.IsSuperuser {
		return &dao.WorkspaceMember{
			MemberId:    user.ID,
			WorkspaceId: workspace.ID,
			Role:        15, // AdminRole
		}, nil
	}
	return nil, err
}

// buildDocsQuery строит запрос документов с фильтрацией по правам доступа.
func buildDocsQuery(db *gorm.DB, workspace *dao.Workspace, member *dao.WorkspaceMember, includeNested, excludeDrafts bool) *gorm.DB {
	query := db.Model(&dao.Doc{}).
		Set("member_role", member.Role).
		Set("member_id", member.MemberId).
		Preload("Author").
		Where("docs.workspace_id = ?", workspace.ID).
		Where("docs.reader_role <= ? OR docs.editor_role <= ? OR EXISTS (SELECT 1 FROM doc_access_rules dar WHERE dar.doc_id = docs.id AND dar.member_id = ?) OR docs.created_by_id = ?",
			member.Role, member.Role, member.MemberId, member.MemberId)

	if !includeNested {
		query = query.Where("docs.parent_doc_id IS NULL")
	}
	if excludeDrafts {
		query = query.Where("docs.draft = false")
	}
	return query
}

// getWorkspaceDocs возвращает список документов пространства с пагинацией.
// Проверяет права доступа пользователя к каждому документу.
func getWorkspaceDocs(ctx context.Context, db *gorm.DB, bl *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	workspaceIdOrSlug, ok := args["workspace_id"].(string)
	if !ok {
		return mcp.NewToolResultError("workspace_id обязателен"), nil
	}

	offset := 0
	limit := 100
	if v, ok := args["offset"].(float64); ok {
		offset = int(v)
	}
	if v, ok := args["limit"].(float64); ok {
		limit = int(v)
	}

	includeNested, _ := args["include_nested"].(bool)
	excludeDrafts, _ := args["exclude_drafts"].(bool)

	workspace, err := findWorkspaceByIdOrSlug(db, workspaceIdOrSlug)
	if err != nil {
		return mcp.NewToolResultError("workspace не найден"), nil
	}

	workspaceMember, err := getWorkspaceMemberOrSuperuser(db, workspace, user)
	if err != nil {
		return mcp.NewToolResultError("нет доступа к workspace"), nil
	}

	query := buildDocsQuery(db, workspace, workspaceMember, includeNested, excludeDrafts)

	var count int64
	if err := query.Count(&count).Error; err != nil {
		return mcp.NewToolResultError("ошибка при подсчёте документов"), nil
	}

	var docs []dao.Doc
	if err := query.Order("seq_id ASC").Offset(offset).Limit(limit).Find(&docs).Error; err != nil {
		return mcp.NewToolResultError("ошибка при получении документов"), nil
	}

	return mcp.NewToolResultJSON(dao.PaginationResponse{
		Count:  count,
		Offset: offset,
		Limit:  limit,
		Result: utils.SliceToSlice(&docs, func(d *dao.Doc) dto.DocLight { return *d.ToLightDTO() }),
	})
}
