package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	tracker "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/activity-tracker"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/apierrors"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/business"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dto"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/mcp/logger"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var validReactionsMCP = map[string]bool{
	"\xf0\x9f\x91\x8d":         true,
	"\xf0\x9f\x91\x8e":         true,
	"\xe2\x9d\xa4\xef\xb8\x8f": true,
	"\xf0\x9f\x98\x82":         true,
	"\xf0\x9f\x98\xae":         true,
	"\xf0\x9f\xa4\xa1":         true,
	"\xf0\x9f\x92\xa9":         true,
	"\xf0\x9f\xa4\xae":         true,
}

var issuesActionsTools = []Tool{
	{
		mcp.NewTool(
			"delete_issue",
			mcp.WithDescription("Удаление задачи. Доступно администратору или автору (если в проекте разрешено удаление автором)"),
			mcp.WithIdempotentHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(true),
			mcp.WithString("issue_id",
				mcp.Required(),
				mcp.Description("ID задачи (UUID или workspace-PROJECT-123)"),
			),
		),
		deleteIssue,
	},
	{
		mcp.NewTool(
			"get_available_states",
			mcp.WithDescription("Получение списка доступных статусов для перехода. Админу возвращаются все, остальным — только разрешённые из текущего статуса"),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("issue_id",
				mcp.Required(),
				mcp.Description("ID задачи (UUID или workspace-PROJECT-123)"),
			),
		),
		getAvailableStates,
	},
	{
		mcp.NewTool(
			"get_sub_issues",
			mcp.WithDescription("Получение списка подзадач задачи с распределением по группам статусов"),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("issue_id",
				mcp.Required(),
				mcp.Description("ID родительской задачи (UUID или workspace-PROJECT-123)"),
			),
		),
		getSubIssues,
	},
	{
		mcp.NewTool(
			"add_sub_issues",
			mcp.WithDescription("Прикрепление существующих задач как подзадач к указанной задаче. Не-админ может прикрепить только свои задачи"),
			mcp.WithIdempotentHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(true),
			mcp.WithString("issue_id",
				mcp.Required(),
				mcp.Description("ID родительской задачи (UUID или workspace-PROJECT-123)"),
			),
			mcp.WithArray("sub_issue_ids",
				mcp.Required(),
				mcp.Description("Список UUID задач, которые нужно сделать подзадачами"),
				mcp.Items(map[string]interface{}{"type": "string"}),
			),
		),
		addSubIssues,
	},
	{
		mcp.NewTool(
			"get_linked_issues",
			mcp.WithDescription("Получение списка связанных задач (linked issues)"),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("issue_id",
				mcp.Required(),
				mcp.Description("ID задачи (UUID или workspace-PROJECT-123)"),
			),
		),
		getLinkedIssues,
	},
	{
		mcp.NewTool(
			"set_linked_issues",
			mcp.WithDescription("Полная замена списка связанных задач. Старые связи удаляются, создаются переданные"),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(true),
			mcp.WithString("issue_id",
				mcp.Required(),
				mcp.Description("ID задачи (UUID или workspace-PROJECT-123)"),
			),
			mcp.WithArray("issue_ids",
				mcp.Required(),
				mcp.Description("Список UUID задач из того же проекта, которые должны быть связаны"),
				mcp.Items(map[string]interface{}{"type": "string"}),
			),
		),
		setLinkedIssues,
	},
	{
		mcp.NewTool(
			"create_issue_link",
			mcp.WithDescription("Добавление внешней ссылки (URL) к задаче"),
			mcp.WithIdempotentHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(true),
			mcp.WithString("issue_id",
				mcp.Required(),
				mcp.Description("ID задачи (UUID или workspace-PROJECT-123)"),
			),
			mcp.WithString("url",
				mcp.Required(),
				mcp.Description("URL ссылки"),
			),
			mcp.WithString("title",
				mcp.Required(),
				mcp.Description("Заголовок ссылки"),
			),
		),
		createIssueLink,
	},
	{
		mcp.NewTool(
			"update_issue_link",
			mcp.WithDescription("Обновление внешней ссылки задачи"),
			mcp.WithIdempotentHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(true),
			mcp.WithString("link_id",
				mcp.Required(),
				mcp.Description("UUID ссылки"),
			),
			mcp.WithString("url",
				mcp.Required(),
				mcp.Description("Новый URL"),
			),
			mcp.WithString("title",
				mcp.Required(),
				mcp.Description("Новый заголовок"),
			),
		),
		updateIssueLink,
	},
	{
		mcp.NewTool(
			"delete_issue_link",
			mcp.WithDescription("Удаление внешней ссылки задачи"),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(true),
			mcp.WithString("link_id",
				mcp.Required(),
				mcp.Description("UUID ссылки"),
			),
		),
		deleteIssueLink,
	},
	{
		mcp.NewTool(
			"update_issue_comment",
			mcp.WithDescription("Изменение текста комментария. Доступно только автору комментария"),
			mcp.WithIdempotentHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(true),
			mcp.WithString("comment_id",
				mcp.Required(),
				mcp.Description("UUID комментария"),
			),
			mcp.WithString("comment_html",
				mcp.Required(),
				mcp.Description("Новый текст комментария в HTML"),
			),
		),
		updateIssueComment,
	},
	{
		mcp.NewTool(
			"delete_issue_comment",
			mcp.WithDescription("Удаление комментария. Доступно администратору проекта или автору комментария"),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(true),
			mcp.WithString("comment_id",
				mcp.Required(),
				mcp.Description("UUID комментария"),
			),
		),
		deleteIssueComment,
	},
	{
		mcp.NewTool(
			"add_comment_reaction",
			mcp.WithDescription("Добавление эмодзи-реакции к комментарию задачи"),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("comment_id",
				mcp.Required(),
				mcp.Description("UUID комментария"),
			),
			mcp.WithString("reaction",
				mcp.Required(),
				mcp.Description("Эмодзи из разрешённого набора: 👍 👎 ❤️ 😂 😮 🤡 💩 🤮"),
			),
		),
		addCommentReaction,
	},
	{
		mcp.NewTool(
			"remove_comment_reaction",
			mcp.WithDescription("Удаление эмодзи-реакции пользователя с комментария"),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(true),
			mcp.WithString("comment_id",
				mcp.Required(),
				mcp.Description("UUID комментария"),
			),
			mcp.WithString("reaction",
				mcp.Required(),
				mcp.Description("Эмодзи реакции для удаления"),
			),
		),
		removeCommentReaction,
	},
	{
		mcp.NewTool(
			"get_issue_history",
			mcp.WithDescription("Объединённая лента изменений и комментариев задачи, отсортированная по времени"),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("issue_id",
				mcp.Required(),
				mcp.Description("ID задачи (UUID или workspace-PROJECT-123)"),
			),
		),
		getIssueHistory,
	},
	{
		mcp.NewTool(
			"get_comment_history",
			mcp.WithDescription("История правок (версии) комментария задачи"),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("issue_id",
				mcp.Required(),
				mcp.Description("ID задачи (UUID или workspace-PROJECT-123)"),
			),
			mcp.WithString("comment_id",
				mcp.Required(),
				mcp.Description("UUID комментария"),
			),
			mcp.WithNumber("limit",
				mcp.Description("Лимит записей (по умолчанию 100)"),
			),
			mcp.WithNumber("offset",
				mcp.Description("Смещение"),
			),
		),
		getCommentHistory,
	},
	{
		mcp.NewTool(
			"get_available_issues_for_relation",
			mcp.WithDescription("Поиск задач, доступных для прикрепления как подзадача/родитель/блокируемая/блокирующая/связанная. Учитывает права (не-админ видит только свои в parent/sub; для linked не-автору видны 0 записей)"),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("issue_id",
				mcp.Required(),
				mcp.Description("ID текущей задачи (UUID или workspace-PROJECT-123)"),
			),
			mcp.WithString("relation",
				mcp.Required(),
				mcp.Description("Тип отношения, для которого ищем кандидатов"),
				mcp.Enum("sub", "parent", "blocks", "blockers", "linked"),
			),
			mcp.WithString("search_query",
				mcp.Description("Полнотекстовый поиск по названию"),
			),
			mcp.WithString("order_by",
				mcp.Description("Поле сортировки (по умолчанию name)"),
			),
			mcp.WithBoolean("desc",
				mcp.Description("Сортировка по убыванию"),
			),
			mcp.WithNumber("limit",
				mcp.Description("Лимит (по умолчанию 100, максимум 100)"),
			),
			mcp.WithNumber("offset",
				mcp.Description("Смещение"),
			),
		),
		getAvailableIssuesForRelation,
	},
	{
		mcp.NewTool(
			"move_sub_issue",
			mcp.WithDescription("Перемещение подзадачи вверх или вниз при ручной сортировке"),
			mcp.WithIdempotentHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(true),
			mcp.WithString("issue_id",
				mcp.Required(),
				mcp.Description("ID родительской задачи (UUID или workspace-PROJECT-123)"),
			),
			mcp.WithString("sub_issue_id",
				mcp.Required(),
				mcp.Description("UUID подзадачи"),
			),
			mcp.WithString("direction",
				mcp.Required(),
				mcp.Description("Направление перемещения"),
				mcp.Enum("up", "down"),
			),
		),
		moveSubIssue,
	},
	{
		mcp.NewTool(
			"pin_issue",
			mcp.WithDescription("Закрепить задачу"),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(true),
			mcp.WithString("issue_id",
				mcp.Required(),
				mcp.Description("ID задачи (UUID или workspace-PROJECT-123)"),
			),
		),
		pinIssue,
	},
	{
		mcp.NewTool(
			"unpin_issue",
			mcp.WithDescription("Открепить задачу"),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(true),
			mcp.WithString("issue_id",
				mcp.Required(),
				mcp.Description("ID задачи (UUID или workspace-PROJECT-123)"),
			),
		),
		unpinIssue,
	},
	{
		mcp.NewTool(
			"get_issue_properties",
			mcp.WithDescription("Получение кастомных свойств задачи. Возвращает все шаблоны проекта со значениями или дефолтами. OnlyAdmin поля скрыты для не-админов"),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("issue_id",
				mcp.Required(),
				mcp.Description("ID задачи (UUID или workspace-PROJECT-123)"),
			),
		),
		getIssueProperties,
	},
	{
		mcp.NewTool(
			"set_issue_property",
			mcp.WithDescription("Установка значения кастомного свойства задачи. OnlyAdmin шаблоны может ставить только админ. Значение проходит JSON Schema валидацию"),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(true),
			mcp.WithString("issue_id",
				mcp.Required(),
				mcp.Description("ID задачи (UUID или workspace-PROJECT-123)"),
			),
			mcp.WithString("template_id",
				mcp.Required(),
				mcp.Description("UUID шаблона свойства проекта"),
			),
			mcp.WithObject("value",
				mcp.Required(),
				mcp.Description("Значение: строка для string/select, bool для boolean, объект {url,title} для link"),
			),
		),
		setIssueProperty,
	},
}

func loadIssueAndMember(db *gorm.DB, userID uuid.UUID, issueIdOrSeq string) (*dao.Issue, *dao.ProjectMember, *mcp.CallToolResult) {
	issue, err := findIssueByIdOrSeq(db, issueIdOrSeq)
	if err != nil {
		return nil, nil, logger.Error(err)
	}
	if issue == nil {
		return nil, nil, apierrors.ErrIssueNotFound.MCPError()
	}

	var pm dao.ProjectMember
	if err := db.Where("member_id = ? AND project_id = ?", userID, issue.ProjectId).First(&pm).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, apierrors.ErrProjectForbidden.MCPError()
		}
		return nil, nil, logger.Error(err)
	}
	return issue, &pm, nil
}

func deleteIssue(ctx context.Context, db *gorm.DB, bl *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	issueIdOrSeq, ok := request.GetArguments()["issue_id"].(string)
	if !ok || issueIdOrSeq == "" {
		return apierrors.ErrIssueNotFound.MCPError(), nil
	}

	issue, pm, errRes := loadIssueAndMember(db, user.ID, issueIdOrSeq)
	if errRes != nil {
		return errRes, nil
	}

	var project dao.Project
	if err := db.Where("id = ?", issue.ProjectId).First(&project).Error; err != nil {
		return logger.Error(err), nil
	}

	isAdmin := pm.Role == types.AdminRole
	if !isAdmin && (issue.CreatedById != user.ID || !project.IssueDeletionAllowed) {
		return apierrors.ErrDeleteIssueForbidden.MCPError(), nil
	}

	issue.Project = &project
	oldSnapshot := tracker.IssueToSnapshot(*issue)
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := bl.GetSnapshotTracker().TrackChanges(types.LayerProject, oldSnapshot, nil, project, user); err != nil {
			return err
		}
		return tx.Delete(issue).Error
	}); err != nil {
		return logger.Error(err), nil
	}

	return mcp.NewToolResultText("задача удалена"), nil
}

func getAvailableStates(ctx context.Context, db *gorm.DB, bl *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	issueIdOrSeq, ok := request.GetArguments()["issue_id"].(string)
	if !ok || issueIdOrSeq == "" {
		return apierrors.ErrIssueNotFound.MCPError(), nil
	}

	issue, pm, errRes := loadIssueAndMember(db, user.ID, issueIdOrSeq)
	if errRes != nil {
		return errRes, nil
	}

	query := db.Where("project_id = ?", issue.ProjectId).Order("sequence")
	if pm.Role != types.AdminRole {
		query = query.Where(db.Where("array_length(from_states, 1) IS NULL").
			Or("? = any(from_states)", issue.StateId))
	}

	var states []dao.State
	if err := query.Find(&states).Error; err != nil {
		return logger.Error(err), nil
	}

	return mcp.NewToolResultJSON(utils.SliceToSlice(&states, func(v *dao.State) dto.StateLight { return *v.ToLightDTO() }))
}

func getSubIssues(ctx context.Context, db *gorm.DB, bl *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	issueIdOrSeq, ok := request.GetArguments()["issue_id"].(string)
	if !ok || issueIdOrSeq == "" {
		return apierrors.ErrIssueNotFound.MCPError(), nil
	}

	issue, _, errRes := loadIssueAndMember(db, user.ID, issueIdOrSeq)
	if errRes != nil {
		return errRes, nil
	}

	var subIssues []dao.Issue
	if err := db.
		Where(&dao.Issue{ParentId: uuid.NullUUID{UUID: issue.ID, Valid: true}, ProjectId: issue.ProjectId}).
		Joins("State").
		Joins("Project").
		Joins("Workspace").
		Joins("Author").
		Order(`"State".sequence, sequence_id`).
		Find(&subIssues).Error; err != nil {
		return logger.Error(err), nil
	}

	stateDistribution := make(map[string]int)
	for _, si := range subIssues {
		if si.State != nil {
			stateDistribution[si.State.Group]++
		}
	}

	return mcp.NewToolResultJSON(dto.ResponseSubIssueList{
		SubIssues:         utils.SliceToSlice(&subIssues, func(i *dao.Issue) dto.Issue { return *i.ToDTO() }),
		StateDistribution: stateDistribution,
	})
}

func addSubIssues(ctx context.Context, db *gorm.DB, bl *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	issueIdOrSeq, ok := args["issue_id"].(string)
	if !ok || issueIdOrSeq == "" {
		return apierrors.ErrIssueNotFound.MCPError(), nil
	}

	rawIDs, ok := args["sub_issue_ids"].([]interface{})
	if !ok || len(rawIDs) == 0 {
		return mcp.NewToolResultJSON([]dto.IssueLight{})
	}

	parentIssue, pm, errRes := loadIssueAndMember(db, user.ID, issueIdOrSeq)
	if errRes != nil {
		return errRes, nil
	}

	var candidateIDs []string
	for _, raw := range rawIDs {
		idStr, ok := raw.(string)
		if !ok || idStr == "" {
			continue
		}
		candidateUUID, err := uuid.FromString(idStr)
		if err != nil {
			continue
		}
		rootID, err := getRootAncestorIDMCP(db, candidateUUID)
		if err != nil {
			return logger.Error(err), nil
		}
		if rootID != parentIssue.ID.String() {
			candidateIDs = append(candidateIDs, idStr)
		}
	}
	if len(candidateIDs) == 0 {
		return mcp.NewToolResultJSON([]dto.IssueLight{})
	}

	query := db.
		Preload("Project").
		Where("project_id = ?", parentIssue.ProjectId).
		Where("parent_id is null").
		Where("id in ?", candidateIDs)
	if pm.Role < types.AdminRole {
		query = query.Where("created_by_id = ?", user.ID)
	}

	var subIssues []dao.Issue
	if err := query.Find(&subIssues).Error; err != nil {
		return logger.Error(err), nil
	}

	var maxSortOrder int
	if err := db.Select("coalesce(max(sort_order), 0)").
		Where("parent_id = ?", parentIssue.ID).
		Model(&dao.Issue{}).
		Find(&maxSortOrder).Error; err != nil {
		return logger.Error(err), nil
	}

	oldSubIssuesData := make([]tracker.IssueSnapshot, len(subIssues))

	for i, si := range subIssues {
		oldSubIssuesData[i] = tracker.IssueToSnapshot(si)
	}

	parentNullID := uuid.NullUUID{UUID: parentIssue.ID, Valid: true}
	userID := uuid.NullUUID{UUID: user.ID, Valid: true}
	for i := range subIssues {
		if pm.Role != types.AdminRole && subIssues[i].CreatedById != user.ID {
			return apierrors.ErrPermissionParentIssue.MCPError(), nil
		}
		subIssues[i].ParentId = parentNullID
		subIssues[i].UpdatedById = userID
		subIssues[i].SortOrder = i + maxSortOrder + 1
	}

	if err := db.Save(&subIssues).Error; err != nil {
		return logger.Error(err), nil
	}

	for i, subIssue := range subIssues {
		subIssue.Parent = parentIssue
		newSnapshot := tracker.IssueToSnapshot(subIssue)
		if err := bl.GetSnapshotTracker().TrackChanges(types.LayerIssue, oldSubIssuesData[i], newSnapshot, subIssues[i], user); err != nil {
			slog.Error("MCP addSubIssues: track changes failed", "error", err)
		}
	}

	return mcp.NewToolResultJSON(utils.SliceToSlice(&subIssues, func(i *dao.Issue) dto.IssueLight { return *i.ToLightDTO() }))
}

func getRootAncestorIDMCP(tx *gorm.DB, issueID uuid.UUID) (string, error) {
	var rootID string
	err := tx.Raw(`
		WITH RECURSIVE ancestor_chain AS (
			SELECT id, parent_id FROM issues WHERE id = ?
			UNION ALL
			SELECT i.id, i.parent_id FROM issues i
			INNER JOIN ancestor_chain ac ON i.id = ac.parent_id
			WHERE ac.parent_id IS NOT NULL
		)
		SELECT id FROM ancestor_chain WHERE parent_id IS NULL;
	`, issueID).Scan(&rootID).Error
	return rootID, err
}

func getLinkedIssues(ctx context.Context, db *gorm.DB, bl *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	issueIdOrSeq, ok := request.GetArguments()["issue_id"].(string)
	if !ok || issueIdOrSeq == "" {
		return apierrors.ErrIssueNotFound.MCPError(), nil
	}

	issue, _, errRes := loadIssueAndMember(db, user.ID, issueIdOrSeq)
	if errRes != nil {
		return errRes, nil
	}

	if err := issue.FetchLinkedIssues(db); err != nil {
		return logger.Error(err), nil
	}

	var issues []dao.Issue
	if err := db.Where("project_id = ?", issue.ProjectId).
		Preload(clause.Associations).
		Where("id in (?)", issue.LinkedIssuesIDs).
		Order("sequence_id").Find(&issues).Error; err != nil {
		return logger.Error(err), nil
	}

	return mcp.NewToolResultJSON(utils.SliceToSlice(&issues, func(il *dao.Issue) dto.Issue { return *il.ToDTO() }))
}

func setLinkedIssues(ctx context.Context, db *gorm.DB, bl *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	issueIdOrSeq, ok := args["issue_id"].(string)
	if !ok || issueIdOrSeq == "" {
		return apierrors.ErrIssueNotFound.MCPError(), nil
	}

	rawIDs, _ := args["issue_ids"].([]interface{})
	var newIDs []uuid.UUID
	for _, raw := range rawIDs {
		idStr, ok := raw.(string)
		if !ok || idStr == "" {
			continue
		}
		newID, err := uuid.FromString(idStr)
		if err != nil {
			continue
		}
		newIDs = append(newIDs, newID)
	}

	issue, _, errRes := loadIssueAndMember(db, user.ID, issueIdOrSeq)
	if errRes != nil {
		return errRes, nil
	}

	if err := issue.FetchLinkedIssues(db); err != nil {
		return logger.Error(err), nil
	}

	oldSnapshot := tracker.IssueToSnapshot(*issue)

	var issues []dao.Issue
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id1 = ? or id2 = ?", issue.ID, issue.ID).Delete(&dao.LinkedIssues{}).Error; err != nil {
			return err
		}
		for _, id := range newIDs {
			if err := issue.AddLinkedIssue(tx, id); err != nil {
				return err
			}
		}
		if err := issue.FetchLinkedIssues(tx); err != nil {
			return err
		}
		return tx.Where("id in (?)", issue.LinkedIssuesIDs).Find(&issues).Error
	}); err != nil {
		return logger.Error(err), nil
	}

	newSnapshot := tracker.IssueToSnapshot(*issue)
	if err := bl.GetSnapshotTracker().TrackChanges(types.LayerIssue, oldSnapshot, newSnapshot, issue, user); err != nil {
		slog.Error("MCP issue action: track changes failed", "error", err)
	}
	return mcp.NewToolResultJSON(utils.SliceToSlice(&issues, func(i *dao.Issue) dto.IssueLight { return *i.ToLightDTO() }))
}

func createIssueLink(ctx context.Context, db *gorm.DB, bl *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	issueIdOrSeq, ok := args["issue_id"].(string)
	if !ok || issueIdOrSeq == "" {
		return apierrors.ErrIssueNotFound.MCPError(), nil
	}

	url, _ := args["url"].(string)
	title, _ := args["title"].(string)
	if url == "" || title == "" {
		return apierrors.ErrURLAndTitleRequired.MCPError(), nil
	}

	issue, _, errRes := loadIssueAndMember(db, user.ID, issueIdOrSeq)
	if errRes != nil {
		return errRes, nil
	}
	oldSnapshot := tracker.IssueToSnapshot(*issue)

	userID := uuid.NullUUID{UUID: user.ID, Valid: true}
	link := dao.IssueLink{
		Id:          dao.GenUUID(),
		Title:       title,
		Url:         url,
		CreatedById: userID,
		UpdatedById: userID,
		IssueId:     issue.ID,
		ProjectId:   issue.ProjectId,
		WorkspaceId: issue.WorkspaceId,
	}

	if err := db.Create(&link).Error; err != nil {
		return logger.Error(err), nil
	}

	if err := db.Preload("Links").Where("id = ?", issue.ID).First(&issue).Error; err != nil {
		return logger.Error(err), nil
	}

	newSnapshot := tracker.IssueToSnapshot(*issue)

	if err := bl.GetSnapshotTracker().TrackChanges(types.LayerIssue, oldSnapshot, newSnapshot, issue, user); err != nil {
		slog.Error("MCP issue action: track changes failed", "error", err)
	}

	return mcp.NewToolResultJSON(link.ToLightDTO())
}

func updateIssueLink(ctx context.Context, db *gorm.DB, bl *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	linkIdStr, _ := args["link_id"].(string)
	linkID, err := uuid.FromString(linkIdStr)
	if err != nil {
		return mcp.NewToolResultError("некорректный link_id"), nil
	}

	newURL, _ := args["url"].(string)
	newTitle, _ := args["title"].(string)
	if newURL == "" || newTitle == "" {
		return apierrors.ErrURLAndTitleRequired.MCPError(), nil
	}

	var link dao.IssueLink
	if err := db.
		Where("issue_links.id = ?", linkID).
		Where("project_id in (?)", db.Select("project_id").Where("member_id = ?", user.ID).Model(dao.ProjectMember{})).
		First(&link).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return mcp.NewToolResultError("ссылка не найдена"), nil
		}
		return logger.Error(err), nil
	}

	oldSnapshot := tracker.LinkToSnapshot(&link)

	if newURL == link.Url && newTitle == link.Title {
		return mcp.NewToolResultJSON(link.ToLightDTO())
	}

	link.Title = newTitle
	link.Url = newURL
	link.UpdatedAt = time.Now()
	link.UpdatedById = uuid.NullUUID{UUID: user.ID, Valid: true}

	if err := db.Omit(clause.Associations).Save(&link).Error; err != nil {
		return logger.Error(err), nil
	}
	newSnapshot := tracker.LinkToSnapshot(&link)

	var issue dao.Issue
	if err := db.Preload("Links").Where("id = ?", link.IssueId).First(&issue).Error; err != nil {
		return logger.Error(err), nil
	}
	if err := bl.GetSnapshotTracker().TrackChanges(types.LayerIssue, oldSnapshot, newSnapshot, issue, user); err != nil {
		slog.Error("MCP issue action: track changes failed", "error", err)
	}

	return mcp.NewToolResultJSON(link.ToLightDTO())
}

func deleteIssueLink(ctx context.Context, db *gorm.DB, bl *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	linkIdStr, _ := request.GetArguments()["link_id"].(string)
	linkID, err := uuid.FromString(linkIdStr)
	if err != nil {
		return mcp.NewToolResultError("некорректный link_id"), nil
	}

	var link dao.IssueLink
	if err := db.
		Where("issue_links.id = ?", linkID).
		Where("project_id in (?)", db.Select("project_id").Where("member_id = ?", user.ID).Model(dao.ProjectMember{})).
		First(&link).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return mcp.NewToolResultError("ссылка не найдена"), nil
		}
		return logger.Error(err), nil
	}
	var issue dao.Issue
	if err := db.Preload("Links").Where("id = ?", link.IssueId).First(&issue).Error; err != nil {
		return logger.Error(err), nil
	}

	oldSnapshot := tracker.IssueToSnapshot(issue)

	if err := db.Delete(&link).Error; err != nil {
		return logger.Error(err), nil
	}

	if err := db.Preload("Links").Where("id = ?", link.IssueId).First(&issue).Error; err != nil {
		return logger.Error(err), nil
	}

	newSnapshot := tracker.IssueToSnapshot(issue)

	if err := bl.GetSnapshotTracker().TrackChanges(types.LayerIssue, oldSnapshot, newSnapshot, issue, user); err != nil {
		slog.Error("MCP issue action: track changes failed", "error", err)
	}

	return mcp.NewToolResultText("ссылка удалена"), nil
}

func loadCommentForUser(db *gorm.DB, userID uuid.UUID, commentID uuid.UUID) (*dao.IssueComment, *dao.ProjectMember, *mcp.CallToolResult) {
	var comment dao.IssueComment
	if err := db.Preload("Issue").
		Where("id = ?", commentID).
		First(&comment).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, apierrors.ErrIssueCommentNotFound.MCPError()
		}
		return nil, nil, logger.Error(err)
	}

	var pm dao.ProjectMember
	if err := db.Where("member_id = ? AND project_id = ?", userID, comment.ProjectId).First(&pm).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, apierrors.ErrProjectForbidden.MCPError()
		}
		return nil, nil, logger.Error(err)
	}
	return &comment, &pm, nil
}

func updateIssueComment(ctx context.Context, db *gorm.DB, bl *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	commentID, err := GetUUIDArg(args, "comment_id")
	if err != nil || commentID == uuid.Nil {
		return apierrors.ErrIssueCommentNotFound.MCPError(), nil
	}

	newHTML, _ := args["comment_html"].(string)

	comment, _, errRes := loadCommentForUser(db, user.ID, commentID)
	if errRes != nil {
		return errRes, nil
	}

	oldSnapshot := tracker.CommentToSnapshot(comment)

	if !comment.ActorId.Valid || comment.ActorId.UUID != user.ID {
		return apierrors.ErrCommentEditForbidden.MCPError(), nil
	}

	if comment.CommentHtml.Body == newHTML {
		return mcp.NewToolResultJSON(comment.ToDTO())
	}

	body := types.RedactorHTML{Body: newHTML}
	stripped := types.RemoveInvisibleChars(newHTML)
	if body.StripTags() == "" && stripped == "" {
		return apierrors.ErrIssueCommentEmpty.MCPError(), nil
	}

	comment.CommentHtml = body
	comment.CommentStripped = stripped
	comment.UpdatedById = uuid.NullUUID{UUID: user.ID, Valid: true}

	if err := db.Omit(clause.Associations).Save(comment).Error; err != nil {
		return logger.Error(err), nil
	}

	newSnapshot := tracker.CommentToSnapshot(comment)

	comment.Actor = user
	if err := bl.GetSnapshotTracker().TrackChanges(types.LayerIssue, oldSnapshot, newSnapshot, comment.Issue, user); err != nil {
		slog.Error("MCP comment action: track changes failed", "error", err)
	}

	return mcp.NewToolResultJSON(comment.ToDTO())
}

func deleteIssueComment(ctx context.Context, db *gorm.DB, bl *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	commentID, err := GetUUIDArg(request.GetArguments(), "comment_id")
	if err != nil || commentID == uuid.Nil {
		return apierrors.ErrIssueCommentNotFound.MCPError(), nil
	}

	comment, pm, errRes := loadCommentForUser(db, user.ID, commentID)
	if errRes != nil {
		return errRes, nil
	}
	oldSnapshot := tracker.CommentToSnapshot(comment)
	issue := comment.Issue

	if pm.Role != types.AdminRole && (!comment.ActorId.Valid || comment.ActorId.UUID != user.ID) {
		return apierrors.ErrCommentEditForbidden.MCPError(), nil
	}

	if err := db.Delete(comment).Error; err != nil {
		return logger.Error(err), nil
	}

	if err := bl.GetSnapshotTracker().TrackChanges(types.LayerIssue, oldSnapshot, nil, issue, user); err != nil {
		slog.Error("MCP issue delete: track changes failed", "error", err)
	}

	return mcp.NewToolResultText("комментарий удалён"), nil
}

func addCommentReaction(ctx context.Context, db *gorm.DB, bl *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	commentID, err := GetUUIDArg(args, "comment_id")
	if err != nil || commentID == uuid.Nil {
		return apierrors.ErrIssueCommentNotFound.MCPError(), nil
	}
	reaction, _ := args["reaction"].(string)
	if !validReactionsMCP[reaction] {
		return apierrors.ErrInvalidReaction.MCPError(), nil
	}

	if _, _, errRes := loadCommentForUser(db, user.ID, commentID); errRes != nil {
		return errRes, nil
	}

	var existing dao.CommentReaction
	err = db.Where("user_id = ? AND comment_id = ? AND reaction = ?", user.ID, commentID, reaction).First(&existing).Error
	if err == nil {
		return mcp.NewToolResultJSON(existing.ToDTO())
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return logger.Error(err), nil
	}

	created := dao.CommentReaction{
		Id:        dao.GenUUID(),
		CreatedAt: time.Now(),
		UserId:    user.ID,
		CommentId: commentID,
		Reaction:  reaction,
	}
	if err := db.Create(&created).Error; err != nil {
		return logger.Error(err), nil
	}

	return mcp.NewToolResultJSON(created.ToDTO())
}

func removeCommentReaction(ctx context.Context, db *gorm.DB, bl *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	commentID, err := GetUUIDArg(args, "comment_id")
	if err != nil || commentID == uuid.Nil {
		return apierrors.ErrIssueCommentNotFound.MCPError(), nil
	}
	reaction, _ := args["reaction"].(string)
	if reaction == "" {
		return mcp.NewToolResultError("reaction обязателен"), nil
	}

	if _, _, errRes := loadCommentForUser(db, user.ID, commentID); errRes != nil {
		return errRes, nil
	}

	res := db.Where("user_id = ? AND comment_id = ? AND reaction = ?", user.ID, commentID, reaction).
		Delete(&dao.CommentReaction{})
	if res.Error != nil {
		return logger.Error(res.Error), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("удалено реакций: %d", res.RowsAffected)), nil
}

func getIssueHistory(ctx context.Context, db *gorm.DB, bl *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	issueIdOrSeq, ok := request.GetArguments()["issue_id"].(string)
	if !ok || issueIdOrSeq == "" {
		return apierrors.ErrIssueNotFound.MCPError(), nil
	}

	issue, _, errRes := loadIssueAndMember(db, user.ID, issueIdOrSeq)
	if errRes != nil {
		return errRes, nil
	}

	var issueActivities []dao.ActivityEvent
	if err := db.Preload(clause.Associations).
		Where("issue_id = ?", issue.ID).
		Where("project_id = ?", issue.ProjectId).
		Where("field != ?", activities.Comment.Field.String()).
		Where("entity_type = ?", types.LayerIssue).
		Order("created_at DESC").
		Find(&issueActivities).Error; err != nil {
		return logger.Error(err), nil
	}

	var issueComments []dao.IssueComment
	if err := db.Where("issue_id = ?", issue.ID).
		Where("project_id = ?", issue.ProjectId).
		Order("created_at DESC").
		Preload(clause.Associations).
		Find(&issueComments).Error; err != nil {
		return logger.Error(err), nil
	}

	type historyEntry struct {
		Kind      string                 `json:"kind"`
		CreatedAt time.Time              `json:"created_at"`
		Activity  *dto.ActivityEventFull `json:"activity,omitempty"`
		Comment   *dto.IssueComment      `json:"comment,omitempty"`
	}

	entries := make([]historyEntry, 0, len(issueActivities)+len(issueComments))
	for i := range issueActivities {
		a := issueActivities[i].ToDTO()
		entries = append(entries, historyEntry{Kind: "activity", CreatedAt: a.CreatedAt, Activity: a})
	}
	for i := range issueComments {
		c := issueComments[i].ToDTO()
		entries = append(entries, historyEntry{Kind: "comment", CreatedAt: c.CreatedAt, Comment: c})
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].CreatedAt.After(entries[j].CreatedAt) })

	return mcp.NewToolResultJSON(map[string]interface{}{
		"count":   len(entries),
		"history": entries,
	})
}

func getCommentHistory(ctx context.Context, db *gorm.DB, bl *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	issueIdOrSeq, ok := args["issue_id"].(string)
	if !ok || issueIdOrSeq == "" {
		return apierrors.ErrIssueNotFound.MCPError(), nil
	}
	commentID, err := GetUUIDArg(args, "comment_id")
	if err != nil || commentID == uuid.Nil {
		return apierrors.ErrIssueCommentNotFound.MCPError(), nil
	}

	issue, _, errRes := loadIssueAndMember(db, user.ID, issueIdOrSeq)
	if errRes != nil {
		return errRes, nil
	}

	limit := 100
	offset := -1
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
		if limit > 200 {
			limit = 200
		}
	}
	if o, ok := args["offset"].(float64); ok {
		offset = int(o)
	}

	var activities []dao.ActivityEvent
	query := db.
		Joins("Actor").
		Where("activity_events.project_id = ?", issue.ProjectId).
		Where("activity_events.issue_id = ?", issue.ID).
		Where("activity_events.new_identifier = ?", commentID).
		Where("entity_type = ?", types.LayerIssue).
		Order("activity_events.created_at DESC")

	resp, err := dao.PaginationRequest(offset, limit, query, &activities)
	if err != nil {
		return logger.Error(err), nil
	}

	result := utils.SliceToSlice(resp.Result.(*[]dao.ActivityEvent),
		func(a *dao.ActivityEvent) dto.CommentHistory {
			body := types.RedactorHTML{Body: a.NewValue}
			var commentNullID uuid.NullUUID
			if a.NewIssueComment != nil {
				commentNullID = uuid.NullUUID{UUID: a.NewIssueComment.Id, Valid: true}
			}
			return dto.CommentHistory{
				CommentHtml:     body,
				CommentStripped: body.StripTags(),
				UpdatedById:     a.ActorID,
				ActorUpdate:     a.Actor.ToLightDTO(),
				CommentId:       commentNullID,
				CreatedAt:       a.CreatedAt,
			}
		})

	resp.Result = result
	return mcp.NewToolResultJSON(resp)
}

const (
	relationParent = iota
	relationSub
	relationBlocks
	relationBlockers
	relationLinked
)

func getAvailableIssuesForRelation(ctx context.Context, db *gorm.DB, bl *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	issueIdOrSeq, ok := args["issue_id"].(string)
	if !ok || issueIdOrSeq == "" {
		return apierrors.ErrIssueNotFound.MCPError(), nil
	}
	relationStr, _ := args["relation"].(string)
	relationMap := map[string]int{
		"parent":   relationParent,
		"sub":      relationSub,
		"blocks":   relationBlocks,
		"blockers": relationBlockers,
		"linked":   relationLinked,
	}
	relationType, ok := relationMap[relationStr]
	if !ok {
		return mcp.NewToolResultError("неизвестный тип отношения"), nil
	}

	currentIssue, pm, errRes := loadIssueAndMember(db, user.ID, issueIdOrSeq)
	if errRes != nil {
		return errRes, nil
	}

	if currentIssue.Author == nil {
		var author dao.User
		if err := db.Where("id = ?", currentIssue.CreatedById).First(&author).Error; err == nil {
			currentIssue.Author = &author
		}
	}

	offset := 0
	limit := 100
	orderBy := "name"
	desc := false
	searchQuery := ""

	if v, ok := args["offset"].(float64); ok {
		offset = int(v)
	}
	if v, ok := args["limit"].(float64); ok && v > 0 {
		limit = int(v)
	}
	if limit > 100 {
		limit = 100
	}
	if v, ok := args["order_by"].(string); ok && v != "" {
		orderBy = v
	}
	if v, ok := args["desc"].(bool); ok {
		desc = v
	}
	if v, ok := args["search_query"].(string); ok {
		searchQuery = v
	}

	query := db.
		Preload("Workspace").
		Preload("Watchers").
		Preload("Assignees").
		Preload("Author").
		Joins("join projects p on p.id = issues.project_id").
		Where("issues.id != ?", currentIssue.ID).
		Where("project_id = ?", currentIssue.ProjectId).
		Order(clause.OrderByColumn{
			Column: clause.Column{Name: orderBy},
			Desc:   desc,
		})

	if pm.Role < types.AdminRole && (relationType == relationParent || relationType == relationSub) {
		query = query.Where("issues.created_by_id = ?", user.ID)
	}

	switch relationType {
	case relationParent:
		familyIDs, err := getDescendantIssueIDsMCP(db, currentIssue.ID)
		if err != nil {
			return logger.Error(err), nil
		}
		if len(familyIDs) > 0 {
			query = query.Where("issues.id NOT IN (?)", familyIDs)
		}
		if currentIssue.ParentId.Valid {
			query = query.Where("issues.id != ?", currentIssue.ParentId)
		}
	case relationSub:
		query = query.Where("parent_id is null")
		if currentIssue.ParentId.Valid {
			rootID, err := getRootAncestorIDMCP(db, currentIssue.ID)
			if err != nil {
				return logger.Error(err), nil
			}
			query = query.Where("issues.id != ?", rootID)
		}
	case relationBlocks:
		blockedIDs, err := getBlockedIssueIDsMCP(db, currentIssue.ID)
		if err != nil {
			return logger.Error(err), nil
		}
		if len(blockedIDs) > 0 {
			query = query.Where("issues.id NOT IN (?)", blockedIDs)
		}
		query = query.Where("issues.id NOT IN (?)",
			db.Select("block_id").
				Where("blocked_by_id = ?", currentIssue.ID).
				Where("project_id = ?", currentIssue.ProjectId).
				Model(&dao.IssueBlocker{}),
		)
	case relationBlockers:
		blockingIDs, err := getBlockingIssueIDsMCP(db, currentIssue.ID)
		if err != nil {
			return logger.Error(err), nil
		}
		if len(blockingIDs) > 0 {
			query = query.Where("issues.id NOT IN (?)", blockingIDs)
		}
		query = query.Where("issues.id NOT IN (?)",
			db.Select("blocked_by_id").
				Where("block_id = ?", currentIssue.ID).
				Where("project_id = ?", currentIssue.ProjectId).
				Model(&dao.IssueBlocker{}),
		)
	case relationLinked:
		authorMatch := currentIssue.Author != nil && currentIssue.Author.ID == user.ID
		if pm.Role == types.GuestRole || (pm.Role == types.MemberRole && !authorMatch) {
			query = query.Where("1 = 0")
		}
	}

	if searchQuery != "" {
		query = query.Where(dao.Issue{}.FullTextSearch(db, searchQuery))
	}

	var issues []dao.Issue
	resp, err := dao.PaginationRequest(offset, limit, query, &issues)
	if err != nil {
		return logger.Error(err), nil
	}

	resp.Result = utils.SliceToSlice(resp.Result.(*[]dao.Issue), func(i *dao.Issue) dto.Issue { return *i.ToDTO() })
	return mcp.NewToolResultJSON(resp)
}

func getDescendantIssueIDsMCP(tx *gorm.DB, issueID uuid.UUID) ([]string, error) {
	var ids []string
	err := tx.Raw(`
		WITH RECURSIVE descendant_chain AS (
			SELECT id, parent_id FROM issues WHERE id = ? OR parent_id = ?
			UNION ALL
			SELECT i.id, i.parent_id FROM issues i
			INNER JOIN descendant_chain dc ON i.parent_id = dc.id
		)
		SELECT id FROM descendant_chain WHERE id != ?;
	`, issueID, issueID, issueID).Scan(&ids).Error
	return ids, err
}

func getBlockedIssueIDsMCP(tx *gorm.DB, issueID uuid.UUID) ([]string, error) {
	var ids []string
	err := tx.Raw(`
		WITH RECURSIVE blocked_chain AS (
			SELECT block_id FROM issue_blockers WHERE blocked_by_id = ?
			UNION ALL
			SELECT ib.block_id FROM issue_blockers ib
			INNER JOIN blocked_chain bc ON ib.blocked_by_id = bc.block_id
		)
		SELECT DISTINCT block_id FROM blocked_chain;
	`, issueID).Scan(&ids).Error
	return ids, err
}

func getBlockingIssueIDsMCP(tx *gorm.DB, issueID uuid.UUID) ([]string, error) {
	var ids []string
	err := tx.Raw(`
		WITH RECURSIVE blocking_chain AS (
			SELECT blocked_by_id FROM issue_blockers WHERE block_id = ?
			UNION ALL
			SELECT ib.blocked_by_id FROM issue_blockers ib
			INNER JOIN blocking_chain bc ON ib.block_id = bc.blocked_by_id
		)
		SELECT DISTINCT blocked_by_id FROM blocking_chain;
	`, issueID).Scan(&ids).Error
	return ids, err
}

func moveSubIssue(ctx context.Context, db *gorm.DB, bl *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	issueIdOrSeq, ok := args["issue_id"].(string)
	if !ok || issueIdOrSeq == "" {
		return apierrors.ErrIssueNotFound.MCPError(), nil
	}
	subIssueID, err := GetUUIDArg(args, "sub_issue_id")
	if err != nil || subIssueID == uuid.Nil {
		return apierrors.ErrIssueNotFound.MCPError(), nil
	}
	direction, _ := args["direction"].(string)
	if direction != "up" && direction != "down" {
		return mcp.NewToolResultError("direction должен быть 'up' или 'down'"), nil
	}

	parentIssue, _, errRes := loadIssueAndMember(db, user.ID, issueIdOrSeq)
	if errRes != nil {
		return errRes, nil
	}

	if err := db.Transaction(func(tx *gorm.DB) error {
		var subIssue dao.Issue
		baseUpd := tx.Model(&subIssue).
			Clauses(clause.Returning{Columns: []clause.Column{{Name: "sort_order"}}}).
			Where("id = ?", subIssueID).
			Where("workspace_id = ?", parentIssue.WorkspaceId).
			Where("project_id = ?", parentIssue.ProjectId).
			Where("parent_id = ?", parentIssue.ID)

		var updateErr error
		if direction == "up" {
			updateErr = baseUpd.Update("sort_order", gorm.Expr("GREATEST(sort_order - 1, 0)")).Error
		} else {
			updateErr = baseUpd.Update("sort_order",
				gorm.Expr("LEAST(sort_order + 1, (?))",
					tx.Select("max(sort_order) + 1").
						Where("workspace_id = ?", parentIssue.WorkspaceId).
						Where("project_id = ?", parentIssue.ProjectId).
						Where("parent_id = ?", parentIssue.ID).
						Where("id != ?", subIssueID).
						Model(&dao.Issue{}),
				)).Error
		}
		if updateErr != nil {
			return updateErr
		}

		neighborDelta := 1
		if direction == "down" {
			neighborDelta = -1
		}
		return tx.Model(&dao.Issue{}).
			Where("workspace_id = ?", parentIssue.WorkspaceId).
			Where("project_id = ?", parentIssue.ProjectId).
			Where("parent_id = ?", parentIssue.ID).
			Where("sort_order = ?", subIssue.SortOrder).
			Where("id != ?", subIssueID).
			Update("sort_order", subIssue.SortOrder+neighborDelta).Error
	}); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apierrors.ErrIssueNotFound.MCPError(), nil
		}
		return logger.Error(err), nil
	}

	return mcp.NewToolResultText("подзадача перемещена"), nil
}

func setIssuePinned(db *gorm.DB, user *dao.User, issueIdOrSeq string, pinned bool) *mcp.CallToolResult {
	issue, _, errRes := loadIssueAndMember(db, user.ID, issueIdOrSeq)
	if errRes != nil {
		return errRes
	}
	if err := db.Model(&dao.Issue{}).Where("id = ?", issue.ID).UpdateColumn("pinned", pinned).Error; err != nil {
		return logger.Error(err)
	}
	if pinned {
		return mcp.NewToolResultText("задача закреплена")
	}
	return mcp.NewToolResultText("задача откреплена")
}

func pinIssue(ctx context.Context, db *gorm.DB, bl *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	issueIdOrSeq, ok := request.GetArguments()["issue_id"].(string)
	if !ok || issueIdOrSeq == "" {
		return apierrors.ErrIssueNotFound.MCPError(), nil
	}
	return setIssuePinned(db, user, issueIdOrSeq, true), nil
}

func unpinIssue(ctx context.Context, db *gorm.DB, bl *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	issueIdOrSeq, ok := request.GetArguments()["issue_id"].(string)
	if !ok || issueIdOrSeq == "" {
		return apierrors.ErrIssueNotFound.MCPError(), nil
	}
	return setIssuePinned(db, user, issueIdOrSeq, false), nil
}

func getIssueProperties(ctx context.Context, db *gorm.DB, bl *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	issueIdOrSeq, ok := request.GetArguments()["issue_id"].(string)
	if !ok || issueIdOrSeq == "" {
		return apierrors.ErrIssueNotFound.MCPError(), nil
	}

	issue, pm, errRes := loadIssueAndMember(db, user.ID, issueIdOrSeq)
	if errRes != nil {
		return errRes, nil
	}

	var templates []dao.ProjectPropertyTemplate
	if err := db.Where("project_id = ?", issue.ProjectId).
		Where("only_admin = ? OR only_admin = ?", false, pm.Role == types.AdminRole).
		Order("sort_order, created_at").
		Find(&templates).Error; err != nil {
		return logger.Error(err), nil
	}

	var existingProps []dao.IssueProperty
	if err := db.Where("issue_id = ?", issue.ID).Find(&existingProps).Error; err != nil {
		return logger.Error(err), nil
	}

	propsMap := make(map[uuid.UUID]dao.IssueProperty, len(existingProps))
	for _, p := range existingProps {
		propsMap[p.TemplateId] = p
	}

	result := make([]dto.IssueProperty, 0, len(templates))
	for _, tmpl := range templates {
		if tmpl.OnlyAdmin && pm.Role < types.AdminRole {
			continue
		}
		prop := dto.IssueProperty{
			TemplateId:  tmpl.Id,
			IssueId:     issue.ID,
			ProjectId:   issue.ProjectId,
			WorkspaceId: issue.WorkspaceId,
			Name:        tmpl.Name,
			Type:        tmpl.Type,
			Value:       defaultPropertyValueMCP(tmpl.Type),
		}
		if tmpl.Type == "select" {
			prop.Options = tmpl.Options
		}
		if existing, ok := propsMap[tmpl.Id]; ok {
			prop.Id = existing.Id
			prop.Value = parsePropertyValueMCP(tmpl.Type, existing.Value)
		}
		result = append(result, prop)
	}

	return mcp.NewToolResultJSON(result)
}

func setIssueProperty(ctx context.Context, db *gorm.DB, bl *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	issueIdOrSeq, ok := args["issue_id"].(string)
	if !ok || issueIdOrSeq == "" {
		return apierrors.ErrIssueNotFound.MCPError(), nil
	}
	templateID, err := GetUUIDArg(args, "template_id")
	if err != nil || templateID == uuid.Nil {
		return apierrors.ErrPropertyTemplateNotFound.MCPError(), nil
	}
	value, hasValue := args["value"]
	if !hasValue {
		return mcp.NewToolResultError("value обязателен"), nil
	}

	issue, pm, errRes := loadIssueAndMember(db, user.ID, issueIdOrSeq)
	if errRes != nil {
		return errRes, nil
	}

	var template dao.ProjectPropertyTemplate
	if err := db.Where("id = ? AND project_id = ?", templateID, issue.ProjectId).First(&template).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apierrors.ErrPropertyTemplateNotFound.MCPError(), nil
		}
		return logger.Error(err), nil
	}

	if template.OnlyAdmin && pm.Role < types.AdminRole {
		return apierrors.ErrPropertyOnlyAdminCanSet.MCPError(), nil
	}

	if err := validatePropertyValueMCP(template, value); err != nil {
		return apierrors.ErrPropertyValueValidationFailed.MCPError(), nil
	}

	valueStr := serializePropertyValueMCP(value)
	userID := uuid.NullUUID{UUID: user.ID, Valid: true}

	var existing dao.IssueProperty
	err = db.Where("issue_id = ? AND template_id = ?", issue.ID, templateID).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		existing = dao.IssueProperty{
			Id:          dao.GenUUID(),
			IssueId:     issue.ID,
			TemplateId:  templateID,
			ProjectId:   issue.ProjectId,
			WorkspaceId: issue.WorkspaceId,
			Value:       valueStr,
			CreatedById: userID,
			UpdatedById: userID,
		}
		if err := db.Create(&existing).Error; err != nil {
			return logger.Error(err), nil
		}
	} else if err != nil {
		return logger.Error(err), nil
	} else {
		existing.Value = valueStr
		existing.UpdatedById = userID
		if err := db.Save(&existing).Error; err != nil {
			return logger.Error(err), nil
		}
	}

	existing.Template = &template
	return mcp.NewToolResultJSON(existing.ToDTO())
}

func defaultPropertyValueMCP(propType string) any {
	switch propType {
	case "string":
		return ""
	case "boolean":
		return false
	default:
		return nil
	}
}

func parsePropertyValueMCP(propType, value string) any {
	switch propType {
	case "boolean":
		return value == "true"
	case "select":
		if value == "" {
			return nil
		}
		return value
	case "link":
		if value == "" {
			return nil
		}
		var m json.RawMessage
		if err := json.Unmarshal([]byte(value), &m); err != nil {
			return value
		}
		return m
	default:
		return value
	}
}

func validatePropertyValueMCP(template dao.ProjectPropertyTemplate, value any) error {
	schema := types.GenValueSchema(template.Type, template.Options)
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("schema.json", schema); err != nil {
		return err
	}
	sch, err := compiler.Compile("schema.json")
	if err != nil {
		return err
	}
	return sch.Validate(value)
}

func serializePropertyValueMCP(value any) string {
	if value == nil {
		return ""
	}
	if m, ok := value.(map[string]any); ok {
		b, err := json.Marshal(m)
		if err != nil {
			return fmt.Sprint(value)
		}
		return string(b)
	}
	return fmt.Sprint(value)
}
