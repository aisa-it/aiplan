package tools

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	tracker "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/activity-tracker"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/apierrors"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/business"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dto"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/mcp/logger"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// docCommentsTools содержит MCP инструменты для работы с комментариями документов.
// Все инструменты работают под DocPermissionsMiddleware: комментировать может любой,
// кто имеет доступ на чтение документа, — так же, как в HTTP (все ручки комментариев
// висят на docGroup с DocPermissionMiddleware).
var docCommentsTools = []Tool{
	{
		mcp.NewTool(
			"get_doc_comments",
			mcp.WithDescription("Получение списка комментариев документа с пагинацией"),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("doc_id",
				mcp.Required(),
				mcp.Description("ID документа (UUID)"),
			),
			mcp.WithNumber("offset",
				mcp.Description("Смещение для пагинации (по умолчанию 0)"),
			),
			mcp.WithNumber("limit",
				mcp.Description("Лимит записей (по умолчанию 100, максимум 200)"),
			),
		),
		DocPermissionsMiddleware(getDocComments),
	},
	{
		mcp.NewTool(
			"get_doc_comment",
			mcp.WithDescription("Получение комментария документа по его ID"),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("doc_id",
				mcp.Required(),
				mcp.Description("ID документа (UUID)"),
			),
			mcp.WithString("comment_id",
				mcp.Required(),
				mcp.Description("ID комментария (UUID)"),
			),
		),
		DocPermissionsMiddleware(getDocComment),
	},
	{
		mcp.NewTool(
			"create_doc_comment",
			mcp.WithDescription("Создание комментария к документу. Вложения через MCP не передаются — комментарий создается без файлов"),
			mcp.WithIdempotentHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("doc_id",
				mcp.Required(),
				mcp.Description("ID документа (UUID)"),
			),
			mcp.WithString("comment_html",
				mcp.Required(),
				mcp.Description("Текст комментария в HTML формате"),
			),
			mcp.WithString("reply_to_comment_id",
				mcp.Description("ID комментария этого же документа (UUID), на который дается ответ"),
			),
		),
		DocPermissionsMiddleware(createDocComment),
	},
	{
		mcp.NewTool(
			"update_doc_comment",
			mcp.WithDescription("Обновление комментария документа. Доступно только автору комментария. Вложения через MCP не передаются"),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(true),
			mcp.WithString("doc_id",
				mcp.Required(),
				mcp.Description("ID документа (UUID)"),
			),
			mcp.WithString("comment_id",
				mcp.Required(),
				mcp.Description("ID комментария (UUID)"),
			),
			mcp.WithString("comment_html",
				mcp.Required(),
				mcp.Description("Новый текст комментария в HTML формате"),
			),
		),
		DocPermissionsMiddleware(updateDocComment),
	},
	{
		mcp.NewTool(
			"delete_doc_comment",
			mcp.WithDescription("Удаление комментария документа. Доступно автору комментария и администратору пространства"),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(true),
			mcp.WithString("doc_id",
				mcp.Required(),
				mcp.Description("ID документа (UUID)"),
			),
			mcp.WithString("comment_id",
				mcp.Required(),
				mcp.Description("ID комментария (UUID)"),
			),
		),
		DocPermissionsMiddleware(deleteDocComment),
	},
	{
		mcp.NewTool(
			"add_doc_comment_reaction",
			mcp.WithDescription("Добавление реакции к комментарию документа"),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("doc_id",
				mcp.Required(),
				mcp.Description("ID документа (UUID)"),
			),
			mcp.WithString("comment_id",
				mcp.Required(),
				mcp.Description("ID комментария (UUID)"),
			),
			mcp.WithString("reaction",
				mcp.Required(),
				mcp.Description("Реакция (эмодзи из списка допустимых)"),
			),
		),
		DocPermissionsMiddleware(addDocCommentReaction),
	},
	{
		mcp.NewTool(
			"remove_doc_comment_reaction",
			mcp.WithDescription("Удаление своей реакции с комментария документа"),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(true),
			mcp.WithString("doc_id",
				mcp.Required(),
				mcp.Description("ID документа (UUID)"),
			),
			mcp.WithString("comment_id",
				mcp.Required(),
				mcp.Description("ID комментария (UUID)"),
			),
			mcp.WithString("reaction",
				mcp.Required(),
				mcp.Description("Реакция (эмодзи)"),
			),
		),
		DocPermissionsMiddleware(removeDocCommentReaction),
	},
	{
		mcp.NewTool(
			"get_doc_comment_history",
			mcp.WithDescription("История изменений комментария документа"),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("doc_id",
				mcp.Required(),
				mcp.Description("ID документа (UUID)"),
			),
			mcp.WithString("comment_id",
				mcp.Required(),
				mcp.Description("ID комментария (UUID)"),
			),
			mcp.WithNumber("offset",
				mcp.Description("Смещение для пагинации"),
			),
			mcp.WithNumber("limit",
				mcp.Description("Лимит записей (по умолчанию 100, максимум 200)"),
			),
		),
		DocPermissionsMiddleware(getDocCommentHistory),
	},
}

// docFromContext возвращает документ и участника пространства, загруженных в DocPermissionsMiddleware.
func docFromContext(ctx context.Context) docContext {
	return ctx.Value(docContextKey{}).(docContext)
}

// docCommentPagination разбирает offset/limit из аргументов инструмента.
func docCommentPagination(args map[string]any, defaultOffset int) (int, int) {
	offset, limit := defaultOffset, 100

	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
		if limit > 200 {
			limit = 200
		}
	}
	if o, ok := args["offset"].(float64); ok && o >= 0 {
		offset = int(o)
	}

	return offset, limit
}

// loadDocComment загружает комментарий документа со всеми связями, нужными для DTO.
func loadDocComment(db *gorm.DB, doc *dao.Doc, commentId uuid.UUID) (*dao.DocComment, *mcp.CallToolResult) {
	var comment dao.DocComment
	if err := db.
		Joins("Actor").
		Joins("OriginalComment").
		Joins("OriginalComment.Actor").
		Preload("Reactions").
		Preload("Attachments").
		Where("doc_comments.workspace_id = ?", doc.WorkspaceId).
		Where("doc_comments.doc_id = ?", doc.ID).
		Where("doc_comments.id = ?", commentId).
		First(&comment).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apierrors.ErrDocCommentNotFound.MCPError()
		}
		return nil, logger.Error(err)
	}

	return &comment, nil
}

// docCommentTarget возвращает документ из контекста и комментарий по аргументу comment_id.
func docCommentTarget(ctx context.Context, db *gorm.DB, args map[string]any) (docContext, *dao.DocComment, *mcp.CallToolResult) {
	docCtx := docFromContext(ctx)

	commentId, err := GetUUIDArg(args, "comment_id")
	if err != nil || commentId == uuid.Nil {
		return docCtx, nil, apierrors.ErrDocCommentNotFound.MCPError()
	}

	comment, errRes := loadDocComment(db, &docCtx.Doc, commentId)
	if errRes != nil {
		return docCtx, nil, errRes
	}

	return docCtx, comment, nil
}

// getDocComments возвращает список комментариев документа с пагинацией.
func getDocComments(ctx context.Context, db *gorm.DB, _ *business.Business, _ *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	doc := docFromContext(ctx).Doc
	offset, limit := docCommentPagination(request.GetArguments(), 0)

	query := db.
		Joins("Actor").
		Joins("OriginalComment").
		Joins("OriginalComment.Actor").
		Preload("Reactions").
		Where("doc_comments.workspace_id = ?", doc.WorkspaceId).
		Where("doc_comments.doc_id = ?", doc.ID).
		Order("doc_comments.created_at DESC")

	var comments []dao.DocComment
	resp, err := dao.PaginationRequest(offset, limit, query, &comments)
	if err != nil {
		return logger.Error(err), nil
	}

	resp.Result = utils.SliceToSlice(resp.Result.(*[]dao.DocComment), func(dc *dao.DocComment) dto.DocComment {
		return *dc.ToDTO()
	})

	return mcp.NewToolResultJSON(resp)
}

// getDocComment возвращает один комментарий документа.
func getDocComment(ctx context.Context, db *gorm.DB, _ *business.Business, _ *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	_, comment, errRes := docCommentTarget(ctx, db, request.GetArguments())
	if errRes != nil {
		return errRes, nil
	}

	return mcp.NewToolResultJSON(comment.ToDTO())
}

// checkDocCommentCooldown защищает от отправки нескольких комментариев подряд.
func checkDocCommentCooldown(db *gorm.DB, workspaceId, userId uuid.UUID) *mcp.CallToolResult {
	var lastCommentTime time.Time
	if err := db.Select("created_at").
		Where("workspace_id = ?", workspaceId).
		Where("actor_id = ?", userId).
		Order("created_at desc").
		Model(&dao.DocComment{}).
		First(&lastCommentTime).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return logger.Error(err)
	}

	if time.Since(lastCommentTime) <= types.CommentsCooldown {
		return apierrors.ErrTooManyComments.MCPError()
	}

	return nil
}

// docCommentReplyTo проверяет, что комментарий, на который дается ответ, существует в этом же документе.
func docCommentReplyTo(db *gorm.DB, doc *dao.Doc, args map[string]any) (uuid.NullUUID, *mcp.CallToolResult) {
	replyToStr, ok := args["reply_to_comment_id"].(string)
	if !ok || strings.TrimSpace(replyToStr) == "" {
		return uuid.NullUUID{}, nil
	}

	replyTo, err := uuid.FromString(replyToStr)
	if err != nil {
		return uuid.NullUUID{}, apierrors.ErrDocCommentNotFound.MCPError()
	}

	if _, errRes := loadDocComment(db, doc, replyTo); errRes != nil {
		return uuid.NullUUID{}, errRes
	}

	return uuid.NullUUID{UUID: replyTo, Valid: true}, nil
}

// docCommentHTML извлекает текст комментария из аргументов и проверяет, что он не пустой.
func docCommentHTML(args map[string]any) (types.RedactorHTML, *mcp.CallToolResult) {
	body, ok := args["comment_html"].(string)
	if !ok {
		return types.RedactorHTML{}, apierrors.ErrDocCommentEmpty.MCPError()
	}

	commentHtml := types.RedactorHTML{Body: body}
	if commentHtml.StripTags() == "" {
		return types.RedactorHTML{}, apierrors.ErrDocCommentEmpty.MCPError()
	}

	return commentHtml, nil
}

// createDocComment создает комментарий к документу.
func createDocComment(ctx context.Context, db *gorm.DB, bl *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	docCtx := docFromContext(ctx)
	doc := docCtx.Doc

	commentHtml, errRes := docCommentHTML(args)
	if errRes != nil {
		return errRes, nil
	}

	if errRes := checkDocCommentCooldown(db, doc.WorkspaceId, user.ID); errRes != nil {
		return errRes, nil
	}

	replyTo, errRes := docCommentReplyTo(db, &doc, args)
	if errRes != nil {
		return errRes, nil
	}

	userId := uuid.NullUUID{UUID: user.ID, Valid: true}
	comment := dao.DocComment{
		Id:               dao.GenUUID(),
		CreatedById:      userId,
		WorkspaceId:      doc.WorkspaceId,
		DocId:            doc.ID,
		ActorId:          userId,
		Actor:            user,
		CommentHtml:      commentHtml,
		CommentStripped:  commentHtml.StripTags(),
		ReplyToCommentId: replyTo,
		CommentType:      1,
		Attachments:      make([]dao.FileAsset, 0),
	}

	if err := db.Omit(clause.Associations).Create(&comment).Error; err != nil {
		return logger.Error(err), nil
	}

	trackDocCommentChanges(bl, &doc, user, nil, tracker.CommentToSnapshot(&comment))

	return mcp.NewToolResultJSON(comment.ToDTO())
}

// updateDocComment обновляет комментарий документа. Редактировать может только автор.
func updateDocComment(ctx context.Context, db *gorm.DB, bl *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	docCtx, comment, errRes := docCommentTarget(ctx, db, args)
	if errRes != nil {
		return errRes, nil
	}

	if !comment.ActorId.Valid || comment.ActorId.UUID != user.ID {
		return apierrors.ErrCommentEditForbidden.MCPError(), nil
	}

	commentHtml, errRes := docCommentHTML(args)
	if errRes != nil {
		return errRes, nil
	}

	oldSnapshot := tracker.CommentToSnapshot(comment)

	comment.CommentHtml = commentHtml
	comment.CommentStripped = commentHtml.StripTags()
	comment.UpdatedById = uuid.NullUUID{UUID: user.ID, Valid: true}

	if err := db.Omit(clause.Associations).
		Select("comment_html", "comment_stripped", "updated_by_id", "updated_at").
		Updates(comment).Error; err != nil {
		return logger.Error(err), nil
	}

	trackDocCommentChanges(bl, &docCtx.Doc, user, oldSnapshot, tracker.CommentToSnapshot(comment))

	return mcp.NewToolResultJSON(comment.ToDTO())
}

// deleteDocComment удаляет комментарий документа: автором или администратором пространства.
func deleteDocComment(ctx context.Context, db *gorm.DB, bl *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	docCtx, comment, errRes := docCommentTarget(ctx, db, request.GetArguments())
	if errRes != nil {
		return errRes, nil
	}

	isAuthor := comment.ActorId.Valid && comment.ActorId.UUID == user.ID
	if docCtx.WorkspaceMember.Role != types.AdminRole && !isAuthor {
		return apierrors.ErrCommentEditForbidden.MCPError(), nil
	}

	oldSnapshot := tracker.CommentToSnapshot(comment)

	if err := db.Delete(comment).Error; err != nil {
		return logger.Error(err), nil
	}

	trackDocCommentChanges(bl, &docCtx.Doc, user, oldSnapshot, nil)

	return mcp.NewToolResultJSON(map[string]any{"id": comment.Id, "deleted": true})
}

// trackDocCommentChanges пишет изменение комментария в ленту активности документа.
func trackDocCommentChanges(bl *business.Business, doc *dao.Doc, user *dao.User, oldSnapshot, newSnapshot tracker.SnapshotI) {
	if err := bl.GetSnapshotTracker().TrackChanges(types.LayerDoc, oldSnapshot, newSnapshot, doc, user); err != nil {
		slog.Error("MCP doc comment: track changes failed", "error", err)
	}
}

// addDocCommentReaction добавляет реакцию к комментарию документа.
func addDocCommentReaction(ctx context.Context, db *gorm.DB, _ *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	reaction, _ := args["reaction"].(string)
	if !validReactionsMCP[reaction] {
		return apierrors.ErrInvalidReaction.MCPError(), nil
	}

	_, comment, errRes := docCommentTarget(ctx, db, args)
	if errRes != nil {
		return errRes, nil
	}

	var existing dao.DocCommentReaction
	err := db.Where("user_id = ? AND comment_id = ? AND reaction = ?", user.ID, comment.Id, reaction).
		First(&existing).Error
	if err == nil {
		return mcp.NewToolResultJSON(existing.ToDTO())
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return logger.Error(err), nil
	}

	created := dao.DocCommentReaction{
		Id:        dao.GenUUID(),
		CreatedAt: time.Now(),
		UserId:    user.ID,
		CommentId: comment.Id,
		Reaction:  reaction,
	}
	if err := db.Create(&created).Error; err != nil {
		return logger.Error(err), nil
	}

	return mcp.NewToolResultJSON(created.ToDTO())
}

// removeDocCommentReaction удаляет реакцию пользователя с комментария документа.
func removeDocCommentReaction(ctx context.Context, db *gorm.DB, _ *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	reaction, _ := args["reaction"].(string)
	if reaction == "" {
		return mcp.NewToolResultError("reaction обязателен"), nil
	}

	_, comment, errRes := docCommentTarget(ctx, db, args)
	if errRes != nil {
		return errRes, nil
	}

	res := db.Where("user_id = ? AND comment_id = ? AND reaction = ?", user.ID, comment.Id, reaction).
		Delete(&dao.DocCommentReaction{})
	if res.Error != nil {
		return logger.Error(res.Error), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("удалено реакций: %d", res.RowsAffected)), nil
}

// getDocCommentHistory возвращает историю изменений комментария документа.
func getDocCommentHistory(ctx context.Context, db *gorm.DB, _ *business.Business, _ *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	docCtx, comment, errRes := docCommentTarget(ctx, db, args)
	if errRes != nil {
		return errRes, nil
	}
	offset, limit := docCommentPagination(args, -1)

	query := db.
		Joins("Actor").
		Where("activity_events.workspace_id = ?", docCtx.Doc.WorkspaceId).
		Where("activity_events.doc_id = ?", docCtx.Doc.ID).
		Where("activity_events.new_identifier = ?", comment.Id).
		Order("activity_events.created_at DESC")

	var activities []dao.ActivityEvent
	resp, err := dao.PaginationRequest(offset, limit, query, &activities)
	if err != nil {
		return logger.Error(err), nil
	}

	resp.Result = utils.SliceToSlice(resp.Result.(*[]dao.ActivityEvent), func(a *dao.ActivityEvent) dto.CommentHistory {
		body := types.RedactorHTML{Body: a.NewValue}
		var commentNullId uuid.NullUUID
		if a.NewDocComment != nil {
			commentNullId = uuid.NullUUID{UUID: a.NewDocComment.Id, Valid: true}
		}

		return dto.CommentHistory{
			CommentHtml:     body,
			CommentStripped: body.StripTags(),
			UpdatedById:     a.ActorID,
			ActorUpdate:     a.Actor.ToLightDTO(),
			CommentId:       commentNullId,
			CreatedAt:       a.CreatedAt,
		}
	})

	return mcp.NewToolResultJSON(resp)
}
