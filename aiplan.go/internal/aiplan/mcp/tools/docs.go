// Пакет tools содержит MCP инструменты для работы с документами.
// Предоставляет функциональность для получения документов с проверкой прав доступа.
package tools

import (
	"context"
	"errors"
	"strings"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/apierrors"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/business"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/mcp/logger"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/gofrs/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"gorm.io/gorm"
)

// docContextKey - типизированный ключ для контекста документа.
type docContextKey struct{}

// docsTools содержит список MCP инструментов для работы с документами.
var docsTools = []Tool{
	{
		mcp.NewTool(
			"get_doc",
			mcp.WithDescription("Получение документа по его ID"),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("doc_id",
				mcp.Required(),
				mcp.Description("ID документа (UUID)"),
			),
		),
		DocPermissionsMiddleware(getDoc),
	},
	{
		mcp.NewTool(
			"create_doc",
			mcp.WithDescription("Создание документа в пространстве"),
			mcp.WithIdempotentHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(true),
			mcp.WithString("workspace_id",
				mcp.Required(),
				mcp.Description("ID пространства (UUID или slug)"),
			),
			mcp.WithString("title",
				mcp.Required(),
				mcp.Description("Название документа (макс. 150 символов)"),
			),
			mcp.WithString("content",
				mcp.Description("HTML-содержимое документа в формате TipTap (поддерживает <p>, <h1>-<h6>, <ul>, <ol>, <li>, <a>, <img>, <strong>, <em>, <code>, <pre>, <blockquote>, <table> и другие стандартные HTML-теги)"),
			),
			mcp.WithString("parent_doc_id",
				mcp.Description("ID родительского документа (UUID) для создания вложенного"),
			),
			mcp.WithNumber("reader_role",
				mcp.Description("Минимальная роль для чтения (5=Guest, 10=Member, 15=Admin)"),
			),
			mcp.WithNumber("editor_role",
				mcp.Description("Минимальная роль для редактирования (5=Guest, 10=Member, 15=Admin)"),
			),
			mcp.WithBoolean("draft",
				mcp.Description("Создать как черновик (по умолчанию false)"),
			),
		),
		createDoc,
	},
	{
		mcp.NewTool(
			"update_doc",
			mcp.WithDescription("Обновление документа по его ID"),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(true),
			mcp.WithString("doc_id",
				mcp.Required(),
				mcp.Description("ID документа (UUID)"),
			),
			mcp.WithString("title",
				mcp.Description("Новое название документа (макс. 150 символов)"),
			),
			mcp.WithString("content",
				mcp.Description("Новое HTML-содержимое документа в формате TipTap"),
			),
			mcp.WithBoolean("draft",
				mcp.Description("Статус черновика (true/false)"),
			),
		),
		DocEditPermissionsMiddleware(updateDocTool),
	},
}

// GetDocsTools возвращает список MCP инструментов для работы с документами.
func GetDocsTools(db *gorm.DB, bl *business.Business) []server.ServerTool {
	result := make([]server.ServerTool, 0, len(docsTools))
	for _, t := range docsTools {
		result = append(result, server.ServerTool{
			Tool:    t.Tool,
			Handler: WrapTool(db, bl, t.Handler),
		})
	}
	return result
}

// docContext хранит информацию о документе и правах пользователя.
// Используется для передачи данных между middleware и handler.
type docContext struct {
	Doc             dao.Doc
	WorkspaceMember dao.WorkspaceMember
}

// checkDocReadAccess проверяет права пользователя на чтение документа.
// Возвращает true, если пользователь имеет доступ.
func checkDocReadAccess(db *gorm.DB, doc *dao.Doc, user *dao.User, workspaceMember *dao.WorkspaceMember) (bool, error) {
	// Проверка по ролям в workspace
	if doc.ReaderRole <= workspaceMember.Role || doc.EditorRole <= workspaceMember.Role {
		return true, nil
	}

	// Проверка автора документа
	if doc.CreatedById == user.ID {
		return true, nil
	}

	// Проверка персональных правил доступа в doc_access_rules
	var accessRuleCount int64
	if err := db.Model(&dao.DocAccessRules{}).
		Where("doc_id = ?", doc.ID).
		Where("member_id = ?", user.ID).
		Count(&accessRuleCount).Error; err != nil {
		return false, err
	}

	return accessRuleCount > 0, nil
}

// checkDocEditAccess проверяет права пользователя на редактирование документа.
// Возвращает true, если пользователь имеет доступ на редактирование.
func checkDocEditAccess(db *gorm.DB, doc *dao.Doc, user *dao.User, workspaceMember *dao.WorkspaceMember) (bool, error) {
	// Автор документа всегда может редактировать
	if doc.CreatedById == user.ID {
		return true, nil
	}

	// Суперпользователь имеет полный доступ
	if user.IsSuperuser {
		return true, nil
	}

	// Проверка по роли редактора в workspace
	if workspaceMember.Role >= doc.EditorRole {
		return true, nil
	}

	// Проверка персональных правил доступа с правом редактирования
	var editAccessCount int64
	if err := db.Model(&dao.DocAccessRules{}).
		Where("doc_id = ?", doc.ID).
		Where("member_id = ?", user.ID).
		Where("edit = true").
		Count(&editAccessCount).Error; err != nil {
		return false, err
	}

	return editAccessCount > 0, nil
}

// DocPermissionsMiddleware проверяет права пользователя на доступ к документу.
// Логика проверки прав:
// 1. Находит документ по ID
// 2. Получает workspace документа
// 3. Проверяет членство пользователя в workspace
// 4. Проверяет права на чтение документа (reader_role, editor_role, access_rules, author)
func DocPermissionsMiddleware(handler ToolHandler) ToolHandler {
	return func(ctx context.Context, db *gorm.DB, bl *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// 1. Валидация и парсинг doc_id
		docId, errResult := parseDocId(request)
		if errResult != nil {
			return errResult, nil
		}

		// 2. Находим документ и проверяем членство в workspace
		docBasic, workspaceMember, errResult := findDocAndCheckMembership(db, docId, user)
		if errResult != nil {
			return errResult, nil
		}

		// 3. Проверяем права на чтение документа
		hasAccess, err := checkDocReadAccess(db, docBasic, user, workspaceMember)
		if err != nil {
			return logger.Error(err), nil
		}
		if !hasAccess {
			return apierrors.ErrDocForbidden.MCPError(), nil
		}

		// 4. Загружаем полный документ со всеми связями
		doc, errResult := loadFullDoc(db, docId, workspaceMember)
		if errResult != nil {
			return errResult, nil
		}

		// 5. Сохраняем контекст документа
		ctx = context.WithValue(ctx, docContextKey{}, docContext{
			Doc:             *doc,
			WorkspaceMember: *workspaceMember,
		})

		return handler(ctx, db, bl, user, request)
	}
}

// DocEditPermissionsMiddleware проверяет права пользователя на редактирование документа.
// Логика проверки прав:
// 1. Находит документ по ID
// 2. Получает workspace документа
// 3. Проверяет членство пользователя в workspace
// 4. Проверяет права на редактирование документа (editor_role, access_rules с edit=true, author)
func DocEditPermissionsMiddleware(handler ToolHandler) ToolHandler {
	return func(ctx context.Context, db *gorm.DB, bl *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// 1. Валидация и парсинг doc_id
		docId, errResult := parseDocId(request)
		if errResult != nil {
			return errResult, nil
		}

		// 2. Находим документ и проверяем членство в workspace
		docBasic, workspaceMember, errResult := findDocAndCheckMembership(db, docId, user)
		if errResult != nil {
			return errResult, nil
		}

		// 3. Проверяем права на редактирование документа
		hasAccess, err := checkDocEditAccess(db, docBasic, user, workspaceMember)
		if err != nil {
			return logger.Error(err), nil
		}
		if !hasAccess {
			return apierrors.ErrDocForbidden.MCPError(), nil
		}

		// 4. Загружаем полный документ со всеми связями
		doc, errResult := loadFullDoc(db, docId, workspaceMember)
		if errResult != nil {
			return errResult, nil
		}

		// 5. Сохраняем контекст документа
		ctx = context.WithValue(ctx, docContextKey{}, docContext{
			Doc:             *doc,
			WorkspaceMember: *workspaceMember,
		})

		return handler(ctx, db, bl, user, request)
	}
}

// parseDocId извлекает и валидирует doc_id из параметров запроса.
func parseDocId(request mcp.CallToolRequest) (uuid.UUID, *mcp.CallToolResult) {
	docIdRaw, ok := request.GetArguments()["doc_id"]
	if !ok {
		return uuid.Nil, mcp.NewToolResultError("doc_id обязателен")
	}

	docIdStr, ok := docIdRaw.(string)
	if !ok {
		return uuid.Nil, mcp.NewToolResultError("doc_id должен быть строкой")
	}

	docId, err := uuid.FromString(docIdStr)
	if err != nil {
		return uuid.Nil, mcp.NewToolResultError("некорректный формат doc_id (ожидается UUID)")
	}

	return docId, nil
}

// findDocAndCheckMembership находит документ и проверяет членство пользователя в workspace.
func findDocAndCheckMembership(db *gorm.DB, docId uuid.UUID, user *dao.User) (*dao.Doc, *dao.WorkspaceMember, *mcp.CallToolResult) {
	// Находим документ по ID (только базовые поля для проверки прав)
	var docBasic dao.Doc
	if err := db.
		Select("id", "workspace_id", "reader_role", "editor_role", "created_by_id").
		Where("id = ?", docId).
		First(&docBasic).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, apierrors.ErrDocNotFound.MCPError()
		}
		return nil, nil, logger.Error(err)
	}

	// Проверяем членство пользователя в workspace
	var workspaceMember dao.WorkspaceMember
	if err := db.
		Where("workspace_id = ?", docBasic.WorkspaceId).
		Where("member_id = ?", user.ID).
		First(&workspaceMember).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, apierrors.ErrDocForbidden.MCPError()
		}
		return nil, nil, logger.Error(err)
	}

	return &docBasic, &workspaceMember, nil
}

// loadFullDoc загружает полный документ со всеми связями.
func loadFullDoc(db *gorm.DB, docId uuid.UUID, workspaceMember *dao.WorkspaceMember) (*dao.Doc, *mcp.CallToolResult) {
	var doc dao.Doc
	if err := db.
		Set("member_id", workspaceMember.MemberId).
		Set("member_role", workspaceMember.Role).
		Set("breadcrumbs", true).
		Preload("AccessRules.Member").
		Preload("ParentDoc").
		Preload("Author").
		Preload("Workspace").
		Preload("InlineAttachments").
		Where("id = ?", docId).
		First(&doc).Error; err != nil {
		return nil, logger.Error(err)
	}
	return &doc, nil
}

// getDoc возвращает полную информацию о документе.
func getDoc(ctx context.Context, _ *gorm.DB, _ *business.Business, _ *dao.User, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Получаем документ из контекста (уже загружен в middleware)
	docCtx := ctx.Value(docContextKey{}).(docContext)

	// Преобразуем в DTO и возвращаем
	return mcp.NewToolResultJSON(docCtx.Doc.ToDTO())
}

// createDocParams содержит параметры для создания документа.
type createDocParams struct {
	workspaceIdOrSlug string
	title             string
	content           string
	parentDocIdStr    string
	readerRole        int
	editorRole        int
	draft             bool
}

// parseCreateDocParams извлекает и валидирует параметры из запроса.
func parseCreateDocParams(args map[string]interface{}) (*createDocParams, *mcp.CallToolResult) {
	workspaceIdOrSlug, ok := args["workspace_id"].(string)
	if !ok {
		return nil, mcp.NewToolResultError("workspace_id обязателен")
	}

	title, ok := args["title"].(string)
	if !ok {
		return nil, mcp.NewToolResultError("title обязателен")
	}

	title = strings.TrimSpace(title)
	if len(title) == 0 {
		return nil, mcp.NewToolResultError("title не может быть пустым")
	}
	if len(title) > 150 {
		return nil, mcp.NewToolResultError("title не должен превышать 150 символов")
	}

	params := &createDocParams{
		workspaceIdOrSlug: workspaceIdOrSlug,
		title:             title,
	}

	params.content, _ = args["content"].(string)
	params.draft, _ = args["draft"].(bool)
	params.parentDocIdStr, _ = args["parent_doc_id"].(string)

	if v, ok := args["reader_role"].(float64); ok {
		params.readerRole = int(v)
	}
	if v, ok := args["editor_role"].(float64); ok {
		params.editorRole = int(v)
	}

	return params, nil
}

// findParentDoc находит родительский документ и проверяет доступ.
func findParentDoc(db *gorm.DB, parentDocId uuid.UUID, workspace *dao.Workspace, user *dao.User,
	workspaceMember *dao.WorkspaceMember) (*dao.Doc, *mcp.CallToolResult) {

	var parentDoc dao.Doc
	if err := db.Where("id = ? AND workspace_id = ?", parentDocId, workspace.ID).First(&parentDoc).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apierrors.ErrDocNotFound.MCPError("родительский документ не найден")
		}
		return nil, logger.Error(err)
	}

	hasAccess, err := checkDocReadAccess(db, &parentDoc, user, workspaceMember)
	if err != nil {
		return nil, logger.Error(err)
	}
	if !hasAccess {
		return nil, apierrors.ErrDocForbidden.MCPError("нет доступа к родительскому документу")
	}

	return &parentDoc, nil
}

// handleParentDoc обрабатывает родительский документ и возвращает обновлённые роли.
func handleParentDoc(db *gorm.DB, parentDocIdStr string, workspace *dao.Workspace, user *dao.User,
	workspaceMember *dao.WorkspaceMember, readerRole, editorRole int) (uuid.NullUUID, int, int, *mcp.CallToolResult) {

	if parentDocIdStr == "" {
		return uuid.NullUUID{}, readerRole, editorRole, nil
	}

	parentDocId, err := uuid.FromString(parentDocIdStr)
	if err != nil {
		return uuid.NullUUID{}, 0, 0, mcp.NewToolResultError("некорректный формат parent_doc_id (ожидается UUID)")
	}

	parentDoc, errResult := findParentDoc(db, parentDocId, workspace, user, workspaceMember)
	if errResult != nil {
		return uuid.NullUUID{}, 0, 0, errResult
	}

	// Наследуем роли от родителя, если не указаны
	if readerRole == 0 {
		readerRole = parentDoc.ReaderRole
	}
	if editorRole == 0 {
		editorRole = parentDoc.EditorRole
	}

	// Проверка иерархии ролей
	if readerRole < parentDoc.ReaderRole || editorRole < parentDoc.EditorRole {
		return uuid.NullUUID{}, 0, 0, apierrors.ErrDocChildRoleTooLow.MCPError()
	}

	return uuid.NullUUID{UUID: parentDocId, Valid: true}, readerRole, editorRole, nil
}

// createDoc создаёт новый документ в пространстве.
func createDoc(ctx context.Context, db *gorm.DB, bl *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	params, errResult := parseCreateDocParams(request.GetArguments())
	if errResult != nil {
		return errResult, nil
	}

	workspace, err := findWorkspaceByIdOrSlug(db, params.workspaceIdOrSlug)
	if err != nil {
		return mcp.NewToolResultError("workspace не найден"), nil
	}

	workspaceMember, err := getWorkspaceMemberOrSuperuser(db, workspace, user)
	if err != nil {
		return mcp.NewToolResultError("нет доступа к workspace"), nil
	}

	if workspaceMember.Role <= types.GuestRole {
		return apierrors.ErrDocForbidden.MCPError(), nil
	}

	parentDocID, readerRole, editorRole, errResult := handleParentDoc(
		db, params.parentDocIdStr, workspace, user, workspaceMember, params.readerRole, params.editorRole)
	if errResult != nil {
		return errResult, nil
	}

	doc := dao.Doc{
		ID:          dao.GenUUID(),
		Title:       params.title,
		Content:     types.RedactorHTML{Body: params.content},
		LLMContent:  true,
		WorkspaceId: workspace.ID,
		Workspace:   workspace,
		CreatedById: user.ID,
		Author:      user,
		ReaderRole:  readerRole,
		EditorRole:  editorRole,
		Draft:       params.draft,
		ParentDocID: parentDocID,
	}

	if err := dao.CreateDoc(db, &doc, user); err != nil {
		return logger.Error(err), nil
	}

	createdDoc, errResult := loadFullDoc(db, doc.ID, workspaceMember)
	if errResult != nil {
		return errResult, nil
	}

	return mcp.NewToolResultJSON(createdDoc.ToDTO())
}

// updateDocParams содержит параметры для обновления документа.
type updateDocParams struct {
	title      string
	content    string
	draft      *bool
	hasTitle   bool
	hasContent bool
	hasDraft   bool
}

// parseUpdateDocParams извлекает и валидирует параметры обновления из запроса.
func parseUpdateDocParams(args map[string]interface{}) (*updateDocParams, *mcp.CallToolResult) {
	params := &updateDocParams{}

	if title, ok := args["title"].(string); ok {
		title = strings.TrimSpace(title)
		if len(title) == 0 {
			return nil, mcp.NewToolResultError("title не может быть пустым")
		}
		if len(title) > 150 {
			return nil, mcp.NewToolResultError("title не должен превышать 150 символов")
		}
		params.title = title
		params.hasTitle = true
	}

	if content, ok := args["content"].(string); ok {
		params.content = content
		params.hasContent = true
	}

	if draft, ok := args["draft"].(bool); ok {
		params.draft = &draft
		params.hasDraft = true
	}

	// Проверяем, что хотя бы одно поле указано для обновления
	if !params.hasTitle && !params.hasContent && !params.hasDraft {
		return nil, mcp.NewToolResultError("необходимо указать хотя бы одно поле для обновления (title, content или draft)")
	}

	return params, nil
}

// updateDocTool обновляет существующий документ.
func updateDocTool(ctx context.Context, db *gorm.DB, _ *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Получаем документ из контекста (уже загружен и проверен в middleware)
	docCtx := ctx.Value(docContextKey{}).(docContext)

	params, errResult := parseUpdateDocParams(request.GetArguments())
	if errResult != nil {
		return errResult, nil
	}

	// Собираем поля для обновления
	updates := make(map[string]interface{})
	if params.hasTitle {
		updates["title"] = params.title
	}
	if params.hasContent {
		updates["content"] = types.RedactorHTML{Body: params.content}
	}
	if params.hasDraft {
		updates["draft"] = *params.draft
	}
	updates["updated_by_id"] = user.ID
	updates["llm_content"] = true

	// Обновляем документ
	if err := db.Model(&dao.Doc{}).Where("id = ?", docCtx.Doc.ID).Updates(updates).Error; err != nil {
		return logger.Error(err), nil
	}

	// Загружаем обновлённый документ
	updatedDoc, errResult := loadFullDoc(db, docCtx.Doc.ID, &docCtx.WorkspaceMember)
	if errResult != nil {
		return errResult, nil
	}

	return mcp.NewToolResultJSON(updatedDoc.ToDTO())
}
