package tools

import (
	"context"
	"errors"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	tracker "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/activity-tracker"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/apierrors"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/business"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/mcp/logger"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/search"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/gofrs/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var issuesTools = []Tool{
	{
		mcp.NewTool(
			"get_issue",
			mcp.WithDescription("Получение инфо о задаче по ее id или индетификатору"),
			mcp.WithIdempotentHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("id",
				mcp.Required(),
				mcp.Description("Индетификатор задачи. Индетификатор должен быть вида UUID, {workspace.slug}-{project.identifier}-{issue.sequence} или короткой ссылки https://{host}/i/{workspace.slug}/{project.identifier}/{issue.sequence}"),
			),
		),
		getIssue,
	},
	{
		mcp.NewTool(
			"search_issues",
			mcp.WithDescription("Поиск задач с фильтрацией и сортировкой"),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("search_query",
				mcp.Description("Поисковый запрос (полнотекстовый поиск по названию и описанию)"),
			),
			mcp.WithArray("workspace_slugs",
				mcp.Description("Фильтр по slug'ам пространств"),
				mcp.Items(map[string]interface{}{"type": "string"}),
			),
			mcp.WithArray("project_ids",
				mcp.Description("Фильтр по ID проектов"),
				mcp.Items(map[string]interface{}{"type": "string"}),
			),
			mcp.WithArray("priorities",
				mcp.Description("Фильтр по приоритетам (urgent, high, medium, low)"),
				mcp.Items(map[string]interface{}{"type": "string"}),
				mcp.WithStringEnumItems([]string{"urgent", "high", "medium", "low"}),
			),
			mcp.WithArray("state_ids",
				mcp.Description("Фильтр по ID статусов (UUID)"),
				mcp.Items(map[string]interface{}{"type": "string"}),
			),
			mcp.WithArray("author_ids",
				mcp.Description("Фильтр по ID авторов (UUID)"),
				mcp.Items(map[string]interface{}{"type": "string"}),
			),
			mcp.WithArray("assignee_ids",
				mcp.Description("Фильтр по ID исполнителей (UUID)"),
				mcp.Items(map[string]interface{}{"type": "string"}),
			),
			mcp.WithArray("watcher_ids",
				mcp.Description("Фильтр по ID наблюдателей (UUID)"),
				mcp.Items(map[string]interface{}{"type": "string"}),
			),
			mcp.WithArray("labels",
				mcp.Description("Фильтр по ID меток (UUID)"),
				mcp.Items(map[string]interface{}{"type": "string"}),
			),
			mcp.WithArray("sprint_ids",
				mcp.Description("Фильтр по ID спринтов (UUID)"),
				mcp.Items(map[string]interface{}{"type": "string"}),
			),
			mcp.WithBoolean("assigned_to_me",
				mcp.Description("Только задачи, назначенные на меня"),
			),
			mcp.WithBoolean("authored_by_me",
				mcp.Description("Только задачи, созданные мной"),
			),
			mcp.WithBoolean("watched_by_me",
				mcp.Description("Только задачи, где я наблюдатель"),
			),
			mcp.WithBoolean("only_active",
				mcp.Description("Только активные задачи (не завершенные и не отмененные)"),
			),
			mcp.WithString("order_by",
				mcp.Description("Поле для сортировки (sequence_id, created_at, updated_at, name, priority, target_date, search_rank)"),
				mcp.Enum("sequence_id", "created_at", "updated_at", "name", "priority", "target_date", "search_rank"),
			),
			mcp.WithBoolean("desc",
				mcp.Description("Сортировка по убыванию"),
			),
			mcp.WithNumber("limit",
				mcp.Description("Лимит записей (по умолчанию 10, максимум 100)"),
			),
			mcp.WithNumber("offset",
				mcp.Description("Смещение для пагинации"),
			),
		),
		searchIssues,
	},
	{
		mcp.NewTool(
			"create_issue",
			mcp.WithDescription("Создание новой задачи в проекте"),
			mcp.WithIdempotentHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(true),
			mcp.WithString("project_id",
				mcp.Required(),
				mcp.Description("ID проекта (UUID)"),
			),
			mcp.WithString("name",
				mcp.Required(),
				mcp.Description("Название задачи"),
			),
			mcp.WithString("description_html",
				mcp.Description("Описание задачи в HTML формате"),
			),
			mcp.WithString("priority",
				mcp.Description("Приоритет задачи"),
				mcp.Enum("urgent", "high", "medium", "low"),
			),
			mcp.WithString("state_id",
				mcp.Description("ID статуса задачи (UUID). Если не указан, используется статус по умолчанию"),
			),
			mcp.WithString("parent_id",
				mcp.Description("ID родительской задачи (UUID) для создания подзадачи"),
			),
			mcp.WithString("target_date",
				mcp.Description("Целевая дата завершения (формат: 2024-12-31)"),
			),
			mcp.WithArray("assignee_ids",
				mcp.Description("Список ID исполнителей (UUID)"),
				mcp.Items(map[string]interface{}{"type": "string"}),
			),
			mcp.WithArray("label_ids",
				mcp.Description("Список ID меток (UUID)"),
				mcp.Items(map[string]interface{}{"type": "string"}),
			),
			mcp.WithBoolean("draft",
				mcp.Description("Создать как черновик"),
			),
		),
		createIssue,
	},
	{
		mcp.NewTool(
			"update_issue",
			mcp.WithDescription("Обновление задачи. Администратор/автор могут менять все поля, остальные участники - только статус"),
			mcp.WithIdempotentHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(true),
			mcp.WithString("issue_id",
				mcp.Required(),
				mcp.Description("ID задачи (UUID или workspace-PROJECT-123)"),
			),
			mcp.WithString("name",
				mcp.Description("Новое название задачи"),
			),
			mcp.WithString("description_html",
				mcp.Description("Описание задачи в HTML формате"),
			),
			mcp.WithString("priority",
				mcp.Description("Приоритет задачи (пустая строка для сброса)"),
				mcp.Enum("urgent", "high", "medium", "low", ""),
			),
			mcp.WithString("state_id",
				mcp.Description("ID нового статуса задачи (UUID)"),
			),
			mcp.WithString("parent_id",
				mcp.Description("ID родительской задачи (UUID). Пустая строка для удаления родителя"),
			),
			mcp.WithString("target_date",
				mcp.Description("Целевая дата завершения (формат: 2024-12-31). Пустая строка для сброса"),
			),
			mcp.WithNumber("estimate_point",
				mcp.Description("Оценка в story points"),
			),
			mcp.WithArray("assignee_ids",
				mcp.Description("Список ID исполнителей (UUID). Заменяет существующих"),
				mcp.Items(map[string]interface{}{"type": "string"}),
			),
			mcp.WithArray("label_ids",
				mcp.Description("Список ID меток (UUID). Заменяет существующие"),
				mcp.Items(map[string]interface{}{"type": "string"}),
			),
			mcp.WithBoolean("draft",
				mcp.Description("Статус черновика"),
			),
		),
		updateIssue,
	},
	// ========== READ-ONLY ИНСТРУМЕНТЫ ДЛЯ АНАЛИЗА ==========
	{
		mcp.NewTool(
			"get_sprints",
			mcp.WithDescription("Получение списка спринтов рабочего пространства с их статистикой"),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("workspace_slug",
				mcp.Required(),
				mcp.Description("Slug рабочего пространства"),
			),
			mcp.WithNumber("limit",
				mcp.Description("Лимит записей (по умолчанию 50)"),
			),
			mcp.WithNumber("offset",
				mcp.Description("Смещение для пагинации"),
			),
		),
		getSprints,
	},
	{
		mcp.NewTool(
			"get_issue_comments",
			mcp.WithDescription("Получение комментариев к задаче с пагинацией"),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("issue_id",
				mcp.Required(),
				mcp.Description("ID задачи (UUID или workspace-PROJECT-123)"),
			),
			mcp.WithNumber("limit",
				mcp.Description("Лимит записей (по умолчанию 50)"),
			),
			mcp.WithNumber("offset",
				mcp.Description("Смещение для пагинации"),
			),
		),
		getIssueComments,
	},
	{
		mcp.NewTool(
			"get_issue_activity",
			mcp.WithDescription("Получение истории изменений задачи"),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("issue_id",
				mcp.Required(),
				mcp.Description("ID задачи (UUID или workspace-PROJECT-123)"),
			),
			mcp.WithString("field",
				mcp.Description("Фильтр по полю (state, assignee, label, priority и т.д.)"),
			),
			mcp.WithNumber("limit",
				mcp.Description("Лимит записей (по умолчанию 100)"),
			),
			mcp.WithNumber("offset",
				mcp.Description("Смещение для пагинации"),
			),
		),
		getIssueActivity,
	},
	{
		mcp.NewTool(
			"get_project_labels",
			mcp.WithDescription("Получение списка меток (тегов) проекта"),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("project_id",
				mcp.Required(),
				mcp.Description("ID проекта (UUID)"),
			),
			mcp.WithString("search_query",
				mcp.Description("Поисковый запрос по названию метки"),
			),
		),
		getProjectLabels,
	},
	{
		mcp.NewTool(
			"get_issue_links",
			mcp.WithDescription("Получение внешних ссылок задачи"),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("issue_id",
				mcp.Required(),
				mcp.Description("ID задачи (UUID или workspace-PROJECT-123)"),
			),
		),
		getIssueLinks,
	},
}

func GetIssuesTools(db *gorm.DB, bl *business.Business) []server.ServerTool {
	var resources []server.ServerTool
	for _, t := range issuesTools {
		resources = append(resources, server.ServerTool{
			Tool:    t.Tool,
			Handler: WrapTool(db, bl, t.Handler),
		})
	}
	return resources
}

func getIssue(ctx context.Context, db *gorm.DB, bl *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	issueIdOrSeq := request.GetArguments()["id"].(string)

	query := db.
		Joins("Parent").
		Joins("Workspace").
		Joins("State").
		Joins("Project").
		Preload("Sprints").
		Preload("Assignees").
		Preload("Watchers").
		Preload("Labels").
		Preload("Links").
		Joins("Author").
		Preload("Links.CreatedBy").
		Preload("Labels.Workspace").
		Preload("Labels.Project")

	var issue dao.Issue
	issue.FullLoad = true
	if id, err := uuid.FromString(issueIdOrSeq); err == nil {
		// uuid id of issue
		query = query.Where("issues.id = ?", id)
	} else {
		var params []string
		if u, err := url.Parse(issueIdOrSeq); err == nil && u.Scheme != "" && u.Host != "" {
			params = filepath.SplitList(u.Path)
		} else {
			params = strings.Split(issueIdOrSeq, "-")
		}

		if len(params) != 3 {
			return logger.Error(apierrors.ErrIssueNotFound, "некорректный формат задачи"), nil
		}

		// sequence id of issue
		query = query.Where(`"Workspace".slug = ? and "Project".identifier = ? and issues.sequence_id = ?`, params[0], params[1], params[2])
	}

	if err := query.
		First(&issue).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return logger.Error(apierrors.ErrIssueNotFound), nil
		}
		return logger.Error(err), nil
	}

	// Fetch Author details
	if err := issue.Author.AfterFind(db); err != nil {
		return logger.Error(err), nil
	}

	return mcp.NewToolResultJSON(issue.ToDTO())
}

func searchIssues(ctx context.Context, db *gorm.DB, bl *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	searchParams, err := types.ParseSearchParamsMCP(request.GetArguments())
	if err != nil {
		return logger.Error(err), nil
	}

	// Проверка на группировку - не поддерживается для Markdown
	if searchParams.GroupByParam != "" {
		return mcp.NewToolResultError("группировка не поддерживается в MCP search_issues"), nil
	}

	// Получаем сырые данные из БД напрямую через BuildIssueListQuery
	issues, count, err := search.BuildIssueListQuery(
		db,
		*user,
		dao.ProjectMember{}, // пустой - глобальный поиск
		nil,                 // без спринта
		true,                // globalSearch = true
		searchParams,
	)
	if err != nil {
		return logger.Error(err), nil
	}

	// Форматируем в Markdown таблицу для экономии токенов
	markdown := search.FormatIssuesToMarkdownTable(issues, count, searchParams.Offset, searchParams.Limit)

	return mcp.NewToolResultText(markdown), nil
}

// createIssue создаёт новую задачу в проекте
func createIssue(ctx context.Context, db *gorm.DB, bl *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	// Получаем обязательные параметры
	projectIdStr, ok := args["project_id"].(string)
	if !ok || projectIdStr == "" {
		return apierrors.ErrProjectIdentifierRequired.MCPError(), nil
	}

	projectId, err := uuid.FromString(projectIdStr)
	if err != nil {
		return apierrors.ErrProjectIdentifierRequired.MCPError(), nil
	}

	name, ok := args["name"].(string)
	if !ok || len(strings.TrimSpace(name)) == 0 {
		return apierrors.ErrIssueNameEmpty.MCPError(), nil
	}

	// Получаем проект и проверяем членство
	var project dao.Project
	if err := db.Preload("Workspace").Where("id = ?", projectId).First(&project).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apierrors.ErrProjectNotFound.MCPError(), nil
		}
		return logger.Error(err), nil
	}

	var projectMember dao.ProjectMember
	if err := db.Where("member_id = ? AND project_id = ?", user.ID, projectId).First(&projectMember).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apierrors.ErrProjectForbidden.MCPError(), nil
		}
		return logger.Error(err), nil
	}

	// Получаем опциональные параметры
	var descriptionHtml string
	if d, ok := args["description_html"].(string); ok {
		descriptionHtml = d
	}

	var priority *string
	if p, ok := args["priority"].(string); ok && p != "" {
		priority = &p
	}

	var stateId uuid.UUID
	if s, ok := args["state_id"].(string); ok && s != "" {
		stateId, _ = uuid.FromString(s)
	}

	var parentId uuid.NullUUID
	if p, ok := args["parent_id"].(string); ok && p != "" {
		if pid, err := uuid.FromString(p); err == nil {
			parentId = uuid.NullUUID{UUID: pid, Valid: true}
		}
	}

	var targetDate *types.TargetDateTimeZ
	if t, ok := args["target_date"].(string); ok && t != "" {
		if parsed, err := time.Parse("2006-01-02", t); err == nil {
			targetDate = &types.TargetDateTimeZ{Time: parsed}
		}
	}

	var draft bool
	if d, ok := args["draft"].(bool); ok {
		draft = d
	}

	userID := uuid.NullUUID{UUID: user.ID, Valid: true}
	issueNew := dao.Issue{
		ID:              dao.GenUUID(),
		Name:            name,
		Priority:        priority,
		TargetDate:      targetDate,
		CreatedById:     user.ID,
		ParentId:        parentId,
		ProjectId:       projectId,
		StateId:         stateId,
		UpdatedById:     userID,
		WorkspaceId:     project.WorkspaceId,
		DescriptionHtml: descriptionHtml,
		Draft:           draft,
		LLMContent:      true,
	}

	// Транзакция: создание задачи и связей
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := dao.CreateIssue(tx, &issueNew); err != nil {
			return err
		}

		// Добавление assignees
		if assigneeIds, ok := args["assignee_ids"].([]interface{}); ok && len(assigneeIds) > 0 {
			var newAssignees []dao.IssueAssignee
			for _, a := range assigneeIds {
				if assigneeStr, ok := a.(string); ok {
					assigneeUUID := uuid.FromStringOrNil(assigneeStr)
					if assigneeUUID != uuid.Nil {
						newAssignees = append(newAssignees, dao.IssueAssignee{
							Id:          dao.GenUUID(),
							AssigneeId:  assigneeUUID,
							IssueId:     issueNew.ID,
							ProjectId:   projectId,
							WorkspaceId: issueNew.WorkspaceId,
							CreatedById: userID,
							UpdatedById: userID,
						})
					}
				}
			}
			if len(newAssignees) > 0 {
				if err := tx.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(&newAssignees, 10).Error; err != nil {
					return err
				}
			}
		}

		// Добавление labels
		if labelIds, ok := args["label_ids"].([]interface{}); ok && len(labelIds) > 0 {
			var newLabels []dao.IssueLabel
			for _, l := range labelIds {
				if labelStr, ok := l.(string); ok {
					labelUUID, err := uuid.FromString(labelStr)
					if err == nil {
						newLabels = append(newLabels, dao.IssueLabel{
							Id:          dao.GenUUID(),
							LabelId:     labelUUID,
							IssueId:     issueNew.ID,
							ProjectId:   projectId,
							WorkspaceId: issueNew.WorkspaceId,
							CreatedById: userID,
							UpdatedById: userID,
						})
					}
				}
			}
			if len(newLabels) > 0 {
				if err := tx.CreateInBatches(&newLabels, 10).Error; err != nil {
					return err
				}
			}
		}

		return nil
	}); err != nil {
		return logger.Error(err), nil
	}

	// Activity tracking
	issueNew.Project = &project
	issueNew.Workspace = project.Workspace
	if err := tracker.TrackActivity[dao.Issue, dao.ProjectActivity](bl.GetTracker(), activities.EntityCreateActivity, nil, nil, issueNew, user); err != nil {
		return logger.Error(err), nil
	}

	// Загружаем созданную задачу с связями для ответа
	var createdIssue dao.Issue
	if err := db.
		Joins("Parent").
		Joins("Workspace").
		Joins("State").
		Joins("Project").
		Preload("Assignees").
		Preload("Labels").
		Joins("Author").
		Where("issues.id = ?", issueNew.ID).
		First(&createdIssue).Error; err != nil {
		return logger.Error(err), nil
	}

	return mcp.NewToolResultJSON(createdIssue.ToDTO())
}

// updateIssue обновляет существующую задачу
func updateIssue(ctx context.Context, db *gorm.DB, bl *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	issueIdOrSeq, ok := args["issue_id"].(string)
	if !ok || issueIdOrSeq == "" {
		return apierrors.ErrIssueNotFound.MCPError(), nil
	}

	// Поиск задачи (аналогично getIssue)
	query := db.
		Joins("Parent").
		Joins("Workspace").
		Joins("State").
		Joins("Project").
		Preload("Assignees").
		Preload("Labels").
		Joins("Author")

	var issue dao.Issue
	if id, err := uuid.FromString(issueIdOrSeq); err == nil {
		query = query.Where("issues.id = ?", id)
	} else {
		var params []string
		if u, err := url.Parse(issueIdOrSeq); err == nil && u.Scheme != "" && u.Host != "" {
			params = filepath.SplitList(u.Path)
		} else {
			params = strings.Split(issueIdOrSeq, "-")
		}

		if len(params) != 3 {
			return logger.Error(apierrors.ErrIssueNotFound, "некорректный формат задачи"), nil
		}

		query = query.Where(`"Workspace".slug = ? and "Project".identifier = ? and issues.sequence_id = ?`, params[0], params[1], params[2])
	}

	if err := query.First(&issue).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apierrors.ErrIssueNotFound.MCPError(), nil
		}
		return logger.Error(err), nil
	}

	// Проверка членства в проекте
	var projectMember dao.ProjectMember
	if err := db.Where("member_id = ? AND project_id = ?", user.ID, issue.ProjectId).First(&projectMember).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apierrors.ErrProjectForbidden.MCPError(), nil
		}
		return logger.Error(err), nil
	}

	// Определение прав: admin или автор могут менять все поля
	updateAll := projectMember.Role == types.AdminRole || issue.CreatedById == user.ID

	// Сохраняем снимок старых данных для activity tracking
	oldIssue := issue
	data := make(map[string]interface{})

	userID := uuid.NullUUID{UUID: user.ID, Valid: true}

	// Обновляем поля
	if name, ok := args["name"].(string); ok {
		if !updateAll {
			return apierrors.ErrIssueForbidden.MCPError(), nil
		}
		if len(strings.TrimSpace(name)) == 0 {
			return apierrors.ErrIssueNameEmpty.MCPError(), nil
		}
		issue.Name = name
		data["name"] = name
	}

	if desc, ok := args["description_html"].(string); ok {
		if !updateAll {
			return apierrors.ErrIssueForbidden.MCPError(), nil
		}
		issue.DescriptionHtml = desc
		data["description_html"] = desc
	}

	if priority, ok := args["priority"].(string); ok {
		if !updateAll {
			return apierrors.ErrIssueForbidden.MCPError(), nil
		}
		if priority == "" {
			issue.Priority = nil
		} else {
			issue.Priority = &priority
		}
		data["priority"] = priority
	}

	if stateIdStr, ok := args["state_id"].(string); ok && stateIdStr != "" {
		stateId, err := uuid.FromString(stateIdStr)
		if err == nil {
			issue.StateId = stateId
			data["state_id"] = stateId
		}
	}

	if parentIdStr, ok := args["parent_id"].(string); ok {
		if !updateAll {
			return apierrors.ErrIssueForbidden.MCPError(), nil
		}
		if parentIdStr == "" {
			issue.ParentId = uuid.NullUUID{Valid: false}
		} else {
			if pid, err := uuid.FromString(parentIdStr); err == nil {
				// Проверка циклических зависимостей
				var ancestorIDs []string
				if err := db.Raw(`
					WITH RECURSIVE ancestor_chain AS (
						SELECT id, parent_id FROM issues WHERE id = ?
						UNION ALL
						SELECT i.id, i.parent_id FROM issues i
						JOIN ancestor_chain ac ON i.id = ac.parent_id
					)
					SELECT id FROM ancestor_chain WHERE id != ?
				`, pid, pid).Scan(&ancestorIDs).Error; err != nil {
					return logger.Error(err), nil
				}

				for _, aid := range ancestorIDs {
					if aid == issue.ID.String() {
						return apierrors.ErrChildDependency.MCPError(), nil
					}
				}

				issue.ParentId = uuid.NullUUID{UUID: pid, Valid: true}
			}
		}
		data["parent_id"] = parentIdStr
	}

	if targetDateStr, ok := args["target_date"].(string); ok {
		if !updateAll {
			return apierrors.ErrIssueForbidden.MCPError(), nil
		}
		if targetDateStr == "" {
			issue.TargetDate = nil
		} else {
			if parsed, err := time.Parse("2006-01-02", targetDateStr); err == nil {
				if time.Now().After(parsed) {
					return apierrors.ErrIssueTargetDateExp.MCPError(), nil
				}
				issue.TargetDate = &types.TargetDateTimeZ{Time: parsed}
			}
		}
		data["target_date"] = targetDateStr
	}

	if estimatePoint, ok := args["estimate_point"].(float64); ok {
		if !updateAll {
			return apierrors.ErrIssueForbidden.MCPError(), nil
		}
		issue.EstimatePoint = int(estimatePoint)
		data["estimate_point"] = int(estimatePoint)
	}

	if draft, ok := args["draft"].(bool); ok {
		if !updateAll {
			return apierrors.ErrIssueForbidden.MCPError(), nil
		}
		issue.Draft = draft
		data["draft"] = draft
	}

	issue.UpdatedById = userID
	issue.LLMContent = true

	// Транзакция: обновление задачи и связей
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Omit(clause.Associations).Save(&issue).Error; err != nil {
			return err
		}

		// Обновление assignees (полная замена)
		if assigneeIds, ok := args["assignee_ids"].([]interface{}); ok {
			if !updateAll {
				return apierrors.ErrIssueForbidden
			}
			// Удаляем существующих
			if err := tx.Where("issue_id = ?", issue.ID).Unscoped().Delete(&dao.IssueAssignee{}).Error; err != nil {
				return err
			}
			// Создаём новых
			var newAssignees []dao.IssueAssignee
			for _, a := range assigneeIds {
				if assigneeStr, ok := a.(string); ok {
					assigneeUUID := uuid.FromStringOrNil(assigneeStr)
					if assigneeUUID != uuid.Nil {
						newAssignees = append(newAssignees, dao.IssueAssignee{
							Id:          dao.GenUUID(),
							AssigneeId:  assigneeUUID,
							IssueId:     issue.ID,
							ProjectId:   issue.ProjectId,
							WorkspaceId: issue.WorkspaceId,
							CreatedById: userID,
							UpdatedById: userID,
						})
					}
				}
			}
			if len(newAssignees) > 0 {
				if err := tx.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(&newAssignees, 10).Error; err != nil {
					return err
				}
			}
			data["assignees_list"] = assigneeIds
		}

		// Обновление labels (полная замена)
		if labelIds, ok := args["label_ids"].([]interface{}); ok {
			if !updateAll {
				return apierrors.ErrIssueForbidden
			}
			// Удаляем существующие
			if err := tx.Where("issue_id = ?", issue.ID).Unscoped().Delete(&dao.IssueLabel{}).Error; err != nil {
				return err
			}
			// Создаём новые
			var newLabels []dao.IssueLabel
			for _, l := range labelIds {
				if labelStr, ok := l.(string); ok {
					labelUUID, err := uuid.FromString(labelStr)
					if err == nil {
						newLabels = append(newLabels, dao.IssueLabel{
							Id:          dao.GenUUID(),
							LabelId:     labelUUID,
							IssueId:     issue.ID,
							ProjectId:   issue.ProjectId,
							WorkspaceId: issue.WorkspaceId,
							CreatedById: userID,
							UpdatedById: userID,
						})
					}
				}
			}
			if len(newLabels) > 0 {
				if err := tx.CreateInBatches(&newLabels, 10).Error; err != nil {
					return err
				}
			}
			data["labels_list"] = labelIds
		}

		return nil
	}); err != nil {
		if errors.Is(err, apierrors.ErrIssueForbidden) {
			return apierrors.ErrIssueForbidden.MCPError(), nil
		}
		return logger.Error(err), nil
	}

	// Activity tracking
	if len(data) > 0 {
		oldData := structToMap(oldIssue)
		if err := tracker.TrackActivity[dao.Issue, dao.IssueActivity](bl.GetTracker(), activities.EntityUpdatedActivity, data, oldData, issue, user); err != nil {
			return logger.Error(err), nil
		}
	}

	// Загружаем обновлённую задачу с связями для ответа
	var updatedIssue dao.Issue
	if err := db.
		Joins("Parent").
		Joins("Workspace").
		Joins("State").
		Joins("Project").
		Preload("Assignees").
		Preload("Labels").
		Joins("Author").
		Where("issues.id = ?", issue.ID).
		First(&updatedIssue).Error; err != nil {
		return logger.Error(err), nil
	}

	return mcp.NewToolResultJSON(updatedIssue.ToDTO())
}

// structToMap конвертирует структуру в map для activity tracking
// Все значения должны быть примитивными типами (string, int, bool) или nil
func structToMap(issue dao.Issue) map[string]interface{} {
	result := make(map[string]interface{})
	result["name"] = issue.Name
	result["description_html"] = issue.DescriptionHtml
	// priority - *string, нужно разыменовать
	if issue.Priority != nil {
		result["priority"] = *issue.Priority
	} else {
		result["priority"] = nil
	}
	// state_id - uuid, конвертируем в строку
	result["state_id"] = issue.StateId.String()
	// parent_id - uuid.NullUUID
	if issue.ParentId.Valid {
		result["parent_id"] = issue.ParentId.UUID.String()
	} else {
		result["parent_id"] = nil
	}
	// target_date - *types.TargetDateTimeZ, конвертируем в строку
	if issue.TargetDate != nil {
		result["target_date"] = issue.TargetDate.Time.Format("2006-01-02")
	} else {
		result["target_date"] = nil
	}
	result["estimate_point"] = issue.EstimatePoint
	result["draft"] = issue.Draft

	var assigneeIds []string
	if issue.Assignees != nil {
		for _, a := range *issue.Assignees {
			assigneeIds = append(assigneeIds, a.ID.String())
		}
	}
	result["assignees_list"] = assigneeIds

	var labelIds []string
	if issue.Labels != nil {
		for _, l := range *issue.Labels {
			labelIds = append(labelIds, l.ID.String())
		}
	}
	result["labels_list"] = labelIds

	return result
}

// ========== READ-ONLY HANDLERS ДЛЯ АНАЛИЗА ПРОЕКТОВ ==========

// getSprints возвращает список спринтов рабочего пространства с их статистикой
func getSprints(ctx context.Context, db *gorm.DB, bl *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	workspaceSlug, ok := args["workspace_slug"].(string)
	if !ok || workspaceSlug == "" {
		return mcp.NewToolResultError("workspace_slug обязателен"), nil
	}

	// Получаем workspace и проверяем членство
	var workspace dao.Workspace
	if err := db.Where("slug = ?", workspaceSlug).First(&workspace).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return mcp.NewToolResultError("Workspace не найден"), nil
		}
		return logger.Error(err), nil
	}

	var workspaceMember dao.WorkspaceMember
	if err := db.Where("member_id = ? AND workspace_id = ?", user.ID, workspace.ID).First(&workspaceMember).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return mcp.NewToolResultError("Нет доступа к workspace"), nil
		}
		return logger.Error(err), nil
	}

	// Параметры пагинации
	limit := 50
	offset := 0
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
		if limit > 100 {
			limit = 100
		}
	}
	if o, ok := args["offset"].(float64); ok && o >= 0 {
		offset = int(o)
	}

	// Получаем спринты с задачами для статистики
	var sprints []dao.Sprint
	if err := db.
		Set("issueProgress", true).
		Preload("Issues.State").
		Where("workspace_id = ?", workspace.ID).
		Order("sequence_id DESC").
		Limit(limit).
		Offset(offset).
		Find(&sprints).Error; err != nil {
		return logger.Error(err), nil
	}

	// Подсчитываем статистику
	for i := range sprints {
		sprints[i].Stats.AllIssues = len(sprints[i].Issues)
		for _, issue := range sprints[i].Issues {
			switch issue.IssueProgress.Status {
			case types.InProgress:
				sprints[i].Stats.InProgress++
			case types.Pending:
				sprints[i].Stats.Pending++
			case types.Cancelled:
				sprints[i].Stats.Cancelled++
			case types.Completed:
				sprints[i].Stats.Completed++
			}
		}
	}

	// Преобразуем в DTO
	type sprintResponse struct {
		ID          string            `json:"id"`
		SequenceID  int               `json:"sequence_id"`
		Name        string            `json:"name"`
		Description string            `json:"description"`
		StartDate   *time.Time        `json:"start_date"`
		EndDate     *time.Time        `json:"end_date"`
		Stats       types.SprintStats `json:"stats"`
		CreatedAt   time.Time         `json:"created_at"`
	}

	result := make([]sprintResponse, len(sprints))
	for i, s := range sprints {
		var startDate, endDate *time.Time
		if s.StartDate.Valid {
			startDate = &s.StartDate.Time
		}
		if s.EndDate.Valid {
			endDate = &s.EndDate.Time
		}
		result[i] = sprintResponse{
			ID:          s.Id.String(),
			SequenceID:  s.SequenceId,
			Name:        s.Name,
			Description: s.Description.Body,
			StartDate:   startDate,
			EndDate:     endDate,
			Stats:       s.Stats,
			CreatedAt:   s.CreatedAt,
		}
	}

	return mcp.NewToolResultJSON(map[string]interface{}{
		"count":   len(result),
		"offset":  offset,
		"limit":   limit,
		"sprints": result,
	})
}

// getIssueComments возвращает комментарии к задаче с пагинацией
func getIssueComments(ctx context.Context, db *gorm.DB, bl *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	issueIdOrSeq, ok := args["issue_id"].(string)
	if !ok || issueIdOrSeq == "" {
		return apierrors.ErrIssueNotFound.MCPError(), nil
	}

	// Находим задачу
	issue, err := findIssueByIdOrSeq(db, issueIdOrSeq)
	if err != nil {
		return logger.Error(err), nil
	}
	if issue == nil {
		return apierrors.ErrIssueNotFound.MCPError(), nil
	}

	// Проверяем членство в проекте
	var projectMember dao.ProjectMember
	if err := db.Where("member_id = ? AND project_id = ?", user.ID, issue.ProjectId).First(&projectMember).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apierrors.ErrProjectForbidden.MCPError(), nil
		}
		return logger.Error(err), nil
	}

	// Параметры пагинации
	limit := 50
	offset := 0
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
		if limit > 100 {
			limit = 100
		}
	}
	if o, ok := args["offset"].(float64); ok && o >= 0 {
		offset = int(o)
	}

	// Получаем комментарии
	var comments []dao.IssueComment
	query := db.
		Joins("Actor").
		Joins("OriginalComment").
		Preload("Reactions").
		Where("issue_comments.issue_id = ?", issue.ID).
		Order("issue_comments.created_at DESC").
		Limit(limit).
		Offset(offset)

	if err := query.Find(&comments).Error; err != nil {
		return logger.Error(err), nil
	}

	// Подсчёт общего количества
	var total int64
	db.Model(&dao.IssueComment{}).Where("issue_id = ?", issue.ID).Count(&total)

	// Формируем ответ
	type commentResponse struct {
		ID               string         `json:"id"`
		CreatedAt        time.Time      `json:"created_at"`
		UpdatedAt        time.Time      `json:"updated_at"`
		CommentHTML      string         `json:"comment_html"`
		ActorID          string         `json:"actor_id,omitempty"`
		ActorName        string         `json:"actor_name,omitempty"`
		ActorEmail       string         `json:"actor_email,omitempty"`
		ReplyToCommentID string         `json:"reply_to_comment_id,omitempty"`
		Reactions        map[string]int `json:"reactions"`
	}

	result := make([]commentResponse, len(comments))
	for i, c := range comments {
		// Подсчёт реакций
		reactionCounts := make(map[string]int)
		for _, r := range c.Reactions {
			reactionCounts[r.Reaction]++
		}

		resp := commentResponse{
			ID:          c.Id.String(),
			CreatedAt:   c.CreatedAt,
			UpdatedAt:   c.UpdatedAt,
			CommentHTML: c.CommentHtml.Body,
			Reactions:   reactionCounts,
		}

		if c.Actor != nil {
			resp.ActorID = c.Actor.ID.String()
			resp.ActorName = c.Actor.GetName()
			resp.ActorEmail = c.Actor.Email
		}

		if c.ReplyToCommentId.Valid {
			resp.ReplyToCommentID = c.ReplyToCommentId.UUID.String()
		}

		result[i] = resp
	}

	return mcp.NewToolResultJSON(map[string]interface{}{
		"count":    total,
		"offset":   offset,
		"limit":    limit,
		"comments": result,
	})
}

// getIssueActivity возвращает историю изменений задачи
func getIssueActivity(ctx context.Context, db *gorm.DB, bl *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	issueIdOrSeq, ok := args["issue_id"].(string)
	if !ok || issueIdOrSeq == "" {
		return apierrors.ErrIssueNotFound.MCPError(), nil
	}

	// Находим задачу
	issue, err := findIssueByIdOrSeq(db, issueIdOrSeq)
	if err != nil {
		return logger.Error(err), nil
	}
	if issue == nil {
		return apierrors.ErrIssueNotFound.MCPError(), nil
	}

	// Проверяем членство в проекте
	var projectMember dao.ProjectMember
	if err := db.Where("member_id = ? AND project_id = ?", user.ID, issue.ProjectId).First(&projectMember).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apierrors.ErrProjectForbidden.MCPError(), nil
		}
		return logger.Error(err), nil
	}

	// Параметры
	limit := 100
	offset := 0
	field := ""
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
		if limit > 200 {
			limit = 200
		}
	}
	if o, ok := args["offset"].(float64); ok && o >= 0 {
		offset = int(o)
	}
	if f, ok := args["field"].(string); ok {
		field = f
	}

	// Получаем историю
	query := db.
		Joins("Actor").
		Where("issue_activities.issue_id = ?", issue.ID).
		Order("issue_activities.created_at DESC").
		Limit(limit).
		Offset(offset)

	if field != "" {
		query = query.Where("issue_activities.field = ?", field)
	}

	var activities []dao.IssueActivity
	if err := query.Find(&activities).Error; err != nil {
		return logger.Error(err), nil
	}

	// Подсчёт общего количества
	var total int64
	countQuery := db.Model(&dao.IssueActivity{}).Where("issue_id = ?", issue.ID)
	if field != "" {
		countQuery = countQuery.Where("field = ?", field)
	}
	countQuery.Count(&total)

	// Формируем ответ
	type activityResponse struct {
		ID            string        `json:"id"`
		CreatedAt     time.Time     `json:"created_at"`
		Verb          string        `json:"verb"`
		Field         string        `json:"field,omitempty"`
		OldValue      string        `json:"old_value,omitempty"`
		NewValue      string        `json:"new_value,omitempty"`
		Comment       string        `json:"comment,omitempty"`
		ActorID       string        `json:"actor_id,omitempty"`
		ActorName     string        `json:"actor_name,omitempty"`
		NewIdentifier uuid.NullUUID `json:"new_identifier"`
		OldIdentifier uuid.NullUUID `json:"old_identifier"`
	}

	result := make([]activityResponse, len(activities))
	for i, a := range activities {
		resp := activityResponse{
			ID:            a.Id.String(),
			CreatedAt:     a.CreatedAt,
			Verb:          a.Verb,
			NewValue:      a.NewValue,
			Comment:       a.Comment,
			NewIdentifier: a.NewIdentifier,
			OldIdentifier: a.OldIdentifier,
		}

		if a.Field != nil {
			resp.Field = *a.Field
		}
		if a.OldValue != nil {
			resp.OldValue = *a.OldValue
		}
		if a.Actor != nil {
			resp.ActorID = a.Actor.ID.String()
			resp.ActorName = a.Actor.GetName()
		}

		result[i] = resp
	}

	return mcp.NewToolResultJSON(map[string]interface{}{
		"count":      total,
		"offset":     offset,
		"limit":      limit,
		"activities": result,
	})
}

// getProjectLabels возвращает список меток проекта
func getProjectLabels(ctx context.Context, db *gorm.DB, bl *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	projectIdStr, ok := args["project_id"].(string)
	if !ok || projectIdStr == "" {
		return apierrors.ErrProjectIdentifierRequired.MCPError(), nil
	}

	projectId, err := uuid.FromString(projectIdStr)
	if err != nil {
		return apierrors.ErrProjectIdentifierRequired.MCPError(), nil
	}

	// Проверяем проект и членство
	var project dao.Project
	if err := db.Where("id = ?", projectId).First(&project).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apierrors.ErrProjectNotFound.MCPError(), nil
		}
		return logger.Error(err), nil
	}

	var projectMember dao.ProjectMember
	if err := db.Where("member_id = ? AND project_id = ?", user.ID, projectId).First(&projectMember).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apierrors.ErrProjectForbidden.MCPError(), nil
		}
		return logger.Error(err), nil
	}

	// Получаем метки
	query := db.
		Where("project_id = ?", projectId).
		Preload("Parent").
		Order("name")

	// Поиск по названию
	if searchQuery, ok := args["search_query"].(string); ok && searchQuery != "" {
		escapedQuery := "%" + strings.ToLower(searchQuery) + "%"
		query = query.Where("lower(name) LIKE ?", escapedQuery)
	}

	var labels []dao.Label
	if err := query.Find(&labels).Error; err != nil {
		return logger.Error(err), nil
	}

	// Формируем ответ
	type labelResponse struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Color       string `json:"color"`
		ParentID    string `json:"parent_id,omitempty"`
		ParentName  string `json:"parent_name,omitempty"`
	}

	result := make([]labelResponse, len(labels))
	for i, l := range labels {
		resp := labelResponse{
			ID:          l.ID.String(),
			Name:        l.Name,
			Description: l.Description,
			Color:       l.Color,
		}
		if l.Parent != nil {
			resp.ParentID = l.Parent.ID.String()
			resp.ParentName = l.Parent.Name
		}
		result[i] = resp
	}

	return mcp.NewToolResultJSON(map[string]interface{}{
		"count":  len(result),
		"labels": result,
	})
}

// getIssueLinks возвращает внешние ссылки задачи
func getIssueLinks(ctx context.Context, db *gorm.DB, bl *business.Business, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	issueIdOrSeq, ok := args["issue_id"].(string)
	if !ok || issueIdOrSeq == "" {
		return apierrors.ErrIssueNotFound.MCPError(), nil
	}

	// Находим задачу
	issue, err := findIssueByIdOrSeq(db, issueIdOrSeq)
	if err != nil {
		return logger.Error(err), nil
	}
	if issue == nil {
		return apierrors.ErrIssueNotFound.MCPError(), nil
	}

	// Проверяем членство в проекте
	var projectMember dao.ProjectMember
	if err := db.Where("member_id = ? AND project_id = ?", user.ID, issue.ProjectId).First(&projectMember).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apierrors.ErrProjectForbidden.MCPError(), nil
		}
		return logger.Error(err), nil
	}

	// Получаем ссылки
	var links []dao.IssueLink
	if err := db.
		Joins("CreatedBy").
		Where("issue_id = ?", issue.ID).
		Order("created_at DESC").
		Find(&links).Error; err != nil {
		return logger.Error(err), nil
	}

	// Формируем ответ
	type linkResponse struct {
		ID          string                 `json:"id"`
		Title       string                 `json:"title"`
		URL         string                 `json:"url"`
		Metadata    map[string]interface{} `json:"metadata,omitempty"`
		CreatedAt   time.Time              `json:"created_at"`
		CreatedByID string                 `json:"created_by_id,omitempty"`
		CreatedBy   string                 `json:"created_by,omitempty"`
	}

	result := make([]linkResponse, len(links))
	for i, l := range links {
		resp := linkResponse{
			ID:        l.Id.String(),
			Title:     l.Title,
			URL:       l.Url,
			Metadata:  l.Metadata,
			CreatedAt: l.CreatedAt,
		}
		if l.CreatedBy != nil {
			resp.CreatedByID = l.CreatedBy.ID.String()
			resp.CreatedBy = l.CreatedBy.GetName()
		}
		result[i] = resp
	}

	return mcp.NewToolResultJSON(map[string]interface{}{
		"count": len(result),
		"links": result,
	})
}

// findIssueByIdOrSeq — вспомогательная функция для поиска задачи по ID или sequence
func findIssueByIdOrSeq(db *gorm.DB, issueIdOrSeq string) (*dao.Issue, error) {
	query := db.Joins("Project").Joins("Workspace")

	var issue dao.Issue
	if id, err := uuid.FromString(issueIdOrSeq); err == nil {
		query = query.Where("issues.id = ?", id)
	} else {
		var params []string
		if u, err := url.Parse(issueIdOrSeq); err == nil && u.Scheme != "" && u.Host != "" {
			params = filepath.SplitList(u.Path)
		} else {
			params = strings.Split(issueIdOrSeq, "-")
		}

		if len(params) != 3 {
			return nil, nil
		}

		query = query.Where(`"Workspace".slug = ? AND "Project".identifier = ? AND issues.sequence_id = ?`, params[0], params[1], params[2])
	}

	if err := query.First(&issue).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	return &issue, nil
}
