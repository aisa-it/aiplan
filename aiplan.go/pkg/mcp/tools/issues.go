package tools

import (
	"context"
	"errors"
	"log/slog"
	"maps"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	tracker "github.com/aisa-it/aiplan/aiplan.go/pkg/activity-tracker"
	apicontext "github.com/aisa-it/aiplan/aiplan.go/pkg/api-context"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/apierrors"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/business"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/dao"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/dto"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/engine"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/mcp/logger"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/search"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/types"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/utils"
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
			mcp.WithString("issue_id",
				mcp.Required(),
				mcp.Description("Индетификатор задачи. Индетификатор должен быть вида UUID, {workspace.slug}-{project.identifier}-{issue.sequence}, короткой ссылки https://{host}/i/{workspace.slug}/{project.identifier}/{issue.sequence} или полной ссылки https://{host}/{workspace}/projects/{project.id}/issues/{issue.sequence}"),
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
			"get_issue_comment",
			mcp.WithDescription("Получение комментария к задаче"),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("comment_id",
				mcp.Required(),
				mcp.Description("ID комментария(UUID)"),
			),
		),
		getIssueComment,
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
	{
		mcp.NewTool(
			"get_issue_attachments",
			mcp.WithDescription("Получение вложений задачи"),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("issue_id",
				mcp.Required(),
				mcp.Description("ID задачи (UUID или workspace-PROJECT-123)"),
			),
		),
		getIssueAttachments,
	},
	{
		mcp.NewTool(
			"create_issue_comment",
			mcp.WithDescription("Создание комментария к задаче"),
			mcp.WithIdempotentHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(true),
			mcp.WithString("issue_id",
				mcp.Required(),
				mcp.Description("ID задачи (UUID или workspace-PROJECT-123)"),
			),
			mcp.WithString("comment_html",
				mcp.Required(),
				mcp.Description("Текст комментария в HTML формате"),
			),
		),
		createIssueComment,
	},
}

func GetIssuesTools(d Deps) []server.ServerTool {
	all := append([]Tool{}, issuesTools...)
	all = append(all, issuesActionsTools...)

	var resources []server.ServerTool
	for _, t := range all {
		resources = append(resources, server.ServerTool{
			Tool:    t.Tool,
			Handler: WrapTool(d, t.Handler),
		})
	}
	return resources
}

func getIssue(ctx context.Context, d Deps, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	issueIdOrSeq, ok := request.GetArguments()["issue_id"].(string)
	if !ok {
		return apierrors.ErrIssueIDsRequired.MCPError("не указан индетификатор задачи"), nil
	}

	query := d.DB.
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
		ref, ok := parseIssueRef(issueIdOrSeq)
		if !ok {
			return logger.Error(apierrors.ErrIssueNotFound, "некорректный формат задачи"), nil
		}

		// sequence id of issue
		query = ref.apply(query)
	}

	if err := query.
		First(&issue).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return logger.Error(apierrors.ErrIssueNotFound), nil
		}
		return logger.Error(err), nil
	}

	// Видимость задачи решает движок — тот же, что и в поиске.
	subject := apicontext.NewSubject(apicontext.Prefilled{DB: d.DB, User: user})
	if err := d.Policy.CanViewIssue(ctx, subject, &issue); err != nil {
		return mcpError(err), nil
	}

	// Fetch Author details
	if err := issue.Author.AfterFind(d.DB); err != nil {
		return logger.Error(err), nil
	}

	return mcp.NewToolResultJSON(issue.ToDTO())
}

func searchIssues(ctx context.Context, d Deps, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	searchParams, err := types.ParseSearchParamsMCP(request.GetArguments())
	if err != nil {
		return logger.Error(err), nil
	}

	// Проверка на группировку - не поддерживается для Markdown
	if searchParams.GroupByParam != "" {
		return mcp.NewToolResultError("группировка не поддерживается в MCP search_issues"), nil
	}

	// Видимость задач ограничивает тот же движок, что и в HTTP.
	issues, count, err := d.Search.SearchIssuesList(ctx, d.DB, engine.IssueScope{
		Subject: apicontext.NewSubject(apicontext.Prefilled{User: user}),
		Kind:    engine.ScopeGlobal,
		Params:  searchParams,
	})
	if err != nil {
		return logger.Error(err), nil
	}

	if len(issues) > 0 {
		ids := make([]uuid.UUID, len(issues))
		for i := range issues {
			ids[i] = issues[i].ID
		}
		var withWatchers []dao.Issue
		if err := d.DB.Select("issues.id").
			Where("id in (?)", ids).
			Preload("Watchers").
			Find(&withWatchers).Error; err != nil {
			return logger.Error(err), nil
		}
		watchersMap := make(map[uuid.UUID]*[]dao.User, len(withWatchers))
		for i := range withWatchers {
			watchersMap[withWatchers[i].ID] = withWatchers[i].Watchers
		}
		for i := range issues {
			issues[i].Watchers = watchersMap[issues[i].ID]
		}
	}

	// Форматируем в Markdown таблицу для экономии токенов
	markdown := search.FormatIssuesToMarkdownTable(issues, count, searchParams.Offset, searchParams.Limit)

	return mcp.NewToolResultText(markdown), nil
}

// createIssue создаёт новую задачу в проекте
func createIssue(ctx context.Context, d Deps, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
	if err := d.DB.Preload("Workspace").Where("id = ?", projectId).First(&project).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apierrors.ErrProjectNotFound.MCPError(), nil
		}
		return logger.Error(err), nil
	}

	var projectMember dao.ProjectMember
	if err := d.DB.Where("member_id = ? AND project_id = ?", user.ID, projectId).First(&projectMember).Error; err != nil {
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
	// Стартовый статус: задачи ещё нет, движку передаётся переход без исходного.
	if stateId != uuid.Nil {
		var state dao.State
		if err := d.DB.Select("from_states").Where("id = ? AND project_id = ?", stateId, projectId).First(&state).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apierrors.ErrProjectStateNotFound.MCPError(), nil
			}
			return logger.Error(err), nil
		}
		subject := apicontext.NewSubject(apicontext.Prefilled{
			User: user, Project: &project, ProjectMember: &projectMember,
		})
		if err := d.Policy.CheckTransition(ctx, engine.StateTransition{Subject: subject, To: state}); err != nil {
			return mcpError(err), nil
		}
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
	if err := d.DB.Transaction(func(tx *gorm.DB) error {
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

	// Загружаем созданную задачу с связями для ответа
	var createdIssue dao.Issue
	if err := d.DB.
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

	err = d.BL.GetSnapshotTracker().TrackChanges(types.LayerProject, nil, tracker.IssueToSnapshot(issueNew), &project, user)
	if err != nil {
		slog.Error("MCP createIssue: track changes failed", "error", err)
	}

	return mcp.NewToolResultJSON(createdIssue.ToDTO())
}

// updateIssue обновляет существующую задачу
// updateIssueArgActions — какое право требуется для аргумента update_issue.
// Имена аргументов свои, поэтому таблица отдельная от HTTP; набор действий тот же.
// Остальные аргументы покрываются действием issue.update.
var updateIssueArgActions = map[string]engine.Action{
	"state_id":     engine.ActionIssueSetState,
	"parent_id":    engine.ActionIssueSetParent,
	"assignee_ids": engine.ActionIssueSetAssignees,
	"label_ids":    engine.ActionIssueSetLabels,
}

// actionsForUpdateArgs возвращает права, которых требуют переданные аргументы.
func actionsForUpdateArgs(args map[string]any) []engine.Action {
	actions := make([]engine.Action, 0, len(updateIssueArgActions))
	for _, arg := range slices.Sorted(maps.Keys(updateIssueArgActions)) {
		if _, ok := args[arg]; ok {
			actions = append(actions, updateIssueArgActions[arg])
		}
	}
	return actions
}

func updateIssue(ctx context.Context, d Deps, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	issueIdOrSeq, ok := args["issue_id"].(string)
	if !ok || issueIdOrSeq == "" {
		return apierrors.ErrIssueNotFound.MCPError(), nil
	}

	// Поиск задачи (аналогично getIssue)
	query := d.DB.
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
		ref, ok := parseIssueRef(issueIdOrSeq)
		if !ok {
			return logger.Error(apierrors.ErrIssueNotFound, "некорректный формат задачи"), nil
		}

		query = ref.apply(query)
	}

	if err := query.First(&issue).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apierrors.ErrIssueNotFound.MCPError(), nil
		}
		return logger.Error(err), nil
	}

	// Сохраняем снимок старых данных для activity tracking
	oldIssue := issue

	subject, err := apicontext.LoadIssueSubject(d.DB, user, &oldIssue)
	if err != nil {
		return mcpError(err), nil
	}
	// Право на правку задачи вообще, затем права на отдельные поля по
	// составу аргументов — тот же порядок, что и в HTTP.
	if errRes := authorize(ctx, d, engine.ActionIssueUpdate, subject); errRes != nil {
		return errRes, nil
	}
	if err := d.Policy.AuthorizeAll(ctx, actionsForUpdateArgs(args), subject); err != nil {
		return mcpError(err), nil
	}
	data := make(map[string]interface{})

	userID := uuid.NullUUID{UUID: user.ID, Valid: true}

	// Обновляем поля
	if name, ok := args["name"].(string); ok {
		if len(strings.TrimSpace(name)) == 0 {
			return apierrors.ErrIssueNameEmpty.MCPError(), nil
		}
		issue.Name = name
		data["name"] = name
	}

	if desc, ok := args["description_html"].(string); ok {
		issue.DescriptionHtml = desc
		data["description_html"] = desc
	}

	if priority, ok := args["priority"].(string); ok {
		if priority == "" {
			issue.Priority = nil
		} else {
			issue.Priority = &priority
		}
		data["priority"] = priority
	}

	// Смена статуса: как в updateIssue (http-issue.go) — статус только из проекта задачи,
	// с прогоном Lua-правил проекта
	var statusChange bool
	var newState dao.State
	if stateIdStr, ok := args["state_id"].(string); ok && stateIdStr != "" {
		stateId, err := uuid.FromString(stateIdStr)
		if err != nil {
			return apierrors.ErrProjectStateNotFound.MCPError(), nil
		}
		if err := d.DB.Where("id = ?", stateId).
			Where("project_id = ?", issue.ProjectId).
			First(&newState).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apierrors.ErrProjectStateNotFound.MCPError(), nil
			}
			return logger.Error(err), nil
		}
		statusChange = true
		issue.StateId = stateId
		data["state_id"] = stateId
	}

	if parentIdStr, ok := args["parent_id"].(string); ok {
		if parentIdStr == "" {
			issue.ParentId = uuid.NullUUID{Valid: false}
		} else {
			if pid, err := uuid.FromString(parentIdStr); err == nil {
				// Проверка циклических зависимостей
				var ancestorIDs []string
				if err := d.DB.Raw(`
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
		issue.EstimatePoint = int(estimatePoint)
		data["estimate_point"] = int(estimatePoint)
	}

	if draft, ok := args["draft"].(bool); ok {
		issue.Draft = draft
		data["draft"] = draft
	}

	issue.UpdatedById = userID
	issue.LLMContent = true

	// Хуки движка и допустимость перехода — тот же порядок, что и в HTTP.
	if statusChange {
		transition := engine.StateTransition{Subject: subject, Issue: &oldIssue, To: newState}
		if err := d.Policy.BeforeStateChange(ctx, transition); err != nil {
			return mcpError(err), nil
		}
		if err := d.Policy.CheckTransition(ctx, transition); err != nil {
			return mcpError(err), nil
		}
	}

	// Транзакция: обновление задачи и связей
	if err := d.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Omit(clause.Associations).Save(&issue).Error; err != nil {
			return err
		}

		// Обновление assignees (полная замена)
		if assigneeIds, ok := args["assignee_ids"].([]interface{}); ok {
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

	// Загружаем обновлённую задачу с связями для ответа
	var updatedIssue dao.Issue
	if err := d.DB.
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

	err = d.BL.GetSnapshotTracker().TrackChanges(types.LayerIssue,
		tracker.IssueToSnapshot(oldIssue),
		tracker.IssueToSnapshot(updatedIssue), &updatedIssue, user)
	if err != nil {
		slog.Error("MCP updateIssue: track changes failed", "error", err)
	}

	if statusChange {
		// Изменение уже сохранено: after-хук его не отменяет, отказ — только в лог.
		if err := d.Policy.AfterStateChange(ctx, engine.StateTransition{
			Subject: subject, Issue: &oldIssue, To: newState,
		}); err != nil {
			slog.Error("MCP updateIssue: after state change hook", "error", err)
		}
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
func getSprints(ctx context.Context, d Deps, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	workspaceSlug, ok := args["workspace_slug"].(string)
	if !ok || workspaceSlug == "" {
		return mcp.NewToolResultError("workspace_slug обязателен"), nil
	}

	// Получаем workspace и проверяем членство
	var workspace dao.Workspace
	if err := d.DB.Where("slug = ?", workspaceSlug).First(&workspace).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return mcp.NewToolResultError("Workspace не найден"), nil
		}
		return logger.Error(err), nil
	}

	var workspaceMember dao.WorkspaceMember
	if err := d.DB.Where("member_id = ? AND workspace_id = ?", user.ID, workspace.ID).First(&workspaceMember).Error; err != nil {
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
	if err := d.DB.
		Where("workspace_id = ?", workspace.ID).
		Order("sequence_id DESC").
		Limit(limit).
		Offset(offset).
		Find(&sprints).Error; err != nil {
		return logger.Error(err), nil
	}

	statsBySprint, err := business.GetSprintStatsByWorkspace(d.DB.WithContext(ctx), workspace.ID)
	if err != nil {
		return logger.Error(err), nil
	}
	for i := range sprints {
		sprints[i].Stats = statsBySprint[sprints[i].Id]
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
func getIssueComments(ctx context.Context, d Deps, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	issueIdOrSeq, ok := args["issue_id"].(string)
	if !ok || issueIdOrSeq == "" {
		return apierrors.ErrIssueNotFound.MCPError(), nil
	}

	// Находим задачу
	issue, err := findIssueByIdOrSeq(d.DB, issueIdOrSeq)
	if err != nil {
		return logger.Error(err), nil
	}
	if issue == nil {
		return apierrors.ErrIssueNotFound.MCPError(), nil
	}

	// Проверяем членство в проекте
	var projectMember dao.ProjectMember
	if err := d.DB.Where("member_id = ? AND project_id = ?", user.ID, issue.ProjectId).First(&projectMember).Error; err != nil {
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
	query := d.DB.
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
	d.DB.Model(&dao.IssueComment{}).Where("issue_id = ?", issue.ID).Count(&total)

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

func getIssueComment(ctx context.Context, d Deps, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	commentId, err := GetUUIDArg(request.GetArguments(), "comment_id")
	if err != nil {
		return logger.Error(err), nil
	}

	query := d.DB.
		Joins("Actor").
		Joins("OriginalComment").
		Joins("OriginalComment.Actor").
		Preload("Reactions").
		Where("issue_comments.project_id in (?)", d.DB.Select("project_id").Where("member_id = ?", user.ID).Model(dao.ProjectMember{})).
		Where("issue_comments.id = ? or issue_comments.original_id = ?", commentId, commentId).
		Order("issue_comments.created_at DESC")

	var comment dao.IssueComment
	if err := query.First(&comment).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return apierrors.ErrIssueCommentNotFound.MCPError(), nil
		}
		return logger.Error(err), nil
	}

	return mcp.NewToolResultJSON(comment.ToDTO())
}

// getIssueActivity возвращает историю изменений задачи
func getIssueActivity(ctx context.Context, d Deps, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	issueIdOrSeq, ok := args["issue_id"].(string)
	if !ok || issueIdOrSeq == "" {
		return apierrors.ErrIssueNotFound.MCPError(), nil
	}

	// Находим задачу
	issue, err := findIssueByIdOrSeq(d.DB, issueIdOrSeq)
	if err != nil {
		return logger.Error(err), nil
	}
	if issue == nil {
		return apierrors.ErrIssueNotFound.MCPError(), nil
	}

	// Проверяем членство в проекте
	var projectMember dao.ProjectMember
	if err := d.DB.Where("member_id = ? AND project_id = ?", user.ID, issue.ProjectId).First(&projectMember).Error; err != nil {
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
	query := d.DB.
		Joins("Actor").
		Where("activity_events.entity_type = ?", types.LayerIssue).
		Where("activity_events.issue_id = ?", issue.ID).
		Order("activity_events.created_at DESC").
		Limit(limit).
		Offset(offset)

	if field != "" {
		query = query.Where("activity_events.field = ?", field)
	}

	var activities []dao.ActivityEvent
	if err := dao.LoadActivitiesBatched(query, &activities); err != nil {
		return logger.Error(err), nil
	}

	// Подсчёт общего количества
	var total int64
	countQuery := d.DB.Model(&dao.ActivityEvent{}).
		Where("activity_events.entity_type = ?", types.LayerIssue).
		Where("issue_id = ?", issue.ID)
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
			ID:            a.ID.String(),
			CreatedAt:     a.CreatedAt,
			Verb:          a.Verb,
			NewValue:      a.NewValue,
			OldValue:      a.OldValue,
			Comment:       a.Comment(),
			Field:         a.Field.String(),
			NewIdentifier: a.NewIdentifier,
			OldIdentifier: a.OldIdentifier,
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
func getProjectLabels(ctx context.Context, d Deps, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
	if err := d.DB.Where("id = ?", projectId).First(&project).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apierrors.ErrProjectNotFound.MCPError(), nil
		}
		return logger.Error(err), nil
	}

	var projectMember dao.ProjectMember
	if err := d.DB.Where("member_id = ? AND project_id = ?", user.ID, projectId).First(&projectMember).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apierrors.ErrProjectForbidden.MCPError(), nil
		}
		return logger.Error(err), nil
	}

	// Получаем метки
	query := d.DB.
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
func getIssueLinks(ctx context.Context, d Deps, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	issueIdOrSeq, ok := args["issue_id"].(string)
	if !ok || issueIdOrSeq == "" {
		return apierrors.ErrIssueNotFound.MCPError(), nil
	}

	// Находим задачу
	issue, err := findIssueByIdOrSeq(d.DB, issueIdOrSeq)
	if err != nil {
		return logger.Error(err), nil
	}
	if issue == nil {
		return apierrors.ErrIssueNotFound.MCPError(), nil
	}

	// Проверяем членство в проекте
	var projectMember dao.ProjectMember
	if err := d.DB.Where("member_id = ? AND project_id = ?", user.ID, issue.ProjectId).First(&projectMember).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apierrors.ErrProjectForbidden.MCPError(), nil
		}
		return logger.Error(err), nil
	}

	// Получаем ссылки
	var links []dao.IssueLink
	if err := d.DB.
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

// getIssueAttachments возвращает вложения задачи
func getIssueAttachments(ctx context.Context, d Deps, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	issueIdOrSeq, ok := args["issue_id"].(string)
	if !ok || issueIdOrSeq == "" {
		return apierrors.ErrIssueNotFound.MCPError(), nil
	}

	// Находим задачу
	issue, err := findIssueByIdOrSeq(d.DB, issueIdOrSeq)
	if err != nil {
		return logger.Error(err), nil
	}
	if issue == nil {
		return apierrors.ErrIssueNotFound.MCPError(), nil
	}

	// Проверяем членство в проекте
	var projectMember dao.ProjectMember
	if err := d.DB.Where("member_id = ? AND project_id = ?", user.ID, issue.ProjectId).First(&projectMember).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apierrors.ErrProjectForbidden.MCPError(), nil
		}
		return logger.Error(err), nil
	}

	var attachments []dao.IssueAttachment
	if err := d.DB.
		Joins("Asset").
		Where("issue_attachments.issue_id = ?", issue.ID).
		Order("issue_attachments.created_at").
		Find(&attachments).Error; err != nil {
		return logger.Error(err), nil
	}

	return listResult(utils.SliceToSlice(&attachments, func(ia *dao.IssueAttachment) dto.Attachment { return *ia.ToLightDTO() }))
}

// createIssueComment создаёт комментарий к задаче
func createIssueComment(ctx context.Context, d Deps, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	issueIdOrSeq, ok := args["issue_id"].(string)
	if !ok || issueIdOrSeq == "" {
		return apierrors.ErrIssueNotFound.MCPError(), nil
	}

	commentHtml, ok := args["comment_html"].(string)
	if !ok || strings.TrimSpace(commentHtml) == "" {
		return apierrors.ErrIssueCommentEmpty.MCPError(), nil
	}

	// Находим задачу
	issue, err := findIssueByIdOrSeq(d.DB, issueIdOrSeq)
	if err != nil {
		return logger.Error(err), nil
	}
	if issue == nil {
		return apierrors.ErrIssueNotFound.MCPError(), nil
	}

	subject, err := apicontext.LoadIssueSubject(d.DB, user, issue)
	if err != nil {
		return mcpError(err), nil
	}
	if errRes := authorize(ctx, d, engine.ActionIssueCommentCreate, subject); errRes != nil {
		return errRes, nil
	}

	// Проверка кулдауна
	var lastCommentTime time.Time
	if err := d.DB.Select("created_at").
		Where("workspace_id = ?", issue.WorkspaceId).
		Where("actor_id = ?", user.ID).
		Order("created_at desc").
		Model(&dao.IssueComment{}).
		First(&lastCommentTime).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return logger.Error(err), nil
	}
	if time.Since(lastCommentTime) <= types.CommentsCooldown {
		return apierrors.ErrTooManyComments.MCPError(), nil
	}

	userID := uuid.NullUUID{UUID: user.ID, Valid: true}
	comment := dao.IssueComment{
		Id:              dao.GenUUID(),
		ProjectId:       issue.ProjectId,
		Project:         issue.Project,
		IssueId:         issue.ID,
		WorkspaceId:     issue.WorkspaceId,
		Workspace:       issue.Workspace,
		ActorId:         userID,
		Issue:           issue,
		CommentHtml:     types.RedactorHTML{Body: commentHtml},
		CommentStripped: types.RemoveInvisibleChars(commentHtml),
	}

	if err := d.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Omit(clause.Associations).Create(&comment).Error; err != nil {
			return err
		}

		issue.UpdatedAt = time.Now()
		return tx.Select("updated_at").Updates(issue).Error
	}); err != nil {
		return logger.Error(err), nil
	}

	// Activity tracking
	comment.Actor = user

	newSnapshot := tracker.CommentToSnapshot(&comment)
	err = d.BL.GetSnapshotTracker().TrackChanges(types.LayerIssue, nil, newSnapshot, issue, user)
	if err != nil {
		slog.Error("MCP createIssueComment: track changes failed", "error", err)
	}

	return mcp.NewToolResultJSON(comment.ToDTO())
}

// issueRef — разобранная ссылка на задачу: пространство, проект и порядковый номер.
// Пространство и проект хранятся строками, потому что могут быть как slug/identifier,
// так и UUID (в полной ссылке на задачу) — что именно, решает apply.
type issueRef struct {
	Workspace  string
	Project    string
	SequenceId int
}

// apply добавляет к запросу условия поиска задачи по разобранной ссылке.
func (r issueRef) apply(query *gorm.DB) *gorm.DB {
	if id, err := uuid.FromString(r.Workspace); err == nil {
		query = query.Where(`"Workspace".id = ?`, id)
	} else {
		query = query.Where(`"Workspace".slug = ?`, r.Workspace)
	}

	if id, err := uuid.FromString(r.Project); err == nil {
		query = query.Where(`"Project".id = ?`, id)
	} else {
		query = query.Where(`"Project".identifier = ?`, r.Project)
	}

	return query.Where("issues.sequence_id = ?", r.SequenceId)
}

// parseIssueRef разбирает идентификатор задачи: строку {workspace.slug}-{project.identifier}-{issue.sequence}
// или ссылку на задачу (короткую и полную).
func parseIssueRef(issueIdOrSeq string) (issueRef, bool) {
	issueIdOrSeq = strings.TrimSpace(issueIdOrSeq)

	if u, err := url.Parse(issueIdOrSeq); err == nil && u.Scheme != "" && u.Host != "" {
		return parseIssueURL(u)
	}

	// Slug пространства сам может содержать дефисы, а identifier проекта и sequence — нет,
	// поэтому строка режется справа: последний сегмент — номер, предпоследний — проект.
	lastDash := strings.LastIndex(issueIdOrSeq, "-")
	if lastDash < 0 {
		return issueRef{}, false
	}
	prevDash := strings.LastIndex(issueIdOrSeq[:lastDash], "-")
	if prevDash < 0 {
		return issueRef{}, false
	}

	return newIssueRef(issueIdOrSeq[:prevDash], issueIdOrSeq[prevDash+1:lastDash], issueIdOrSeq[lastDash+1:])
}

// parseIssueURL разбирает ссылку на задачу: короткую /i/{workspace}/{project}/{sequence}
// и полную /{workspace}/projects/{projectId}/issues/{sequence}.
func parseIssueURL(u *url.URL) (issueRef, bool) {
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")

	if len(parts) == 4 && parts[0] == "i" {
		return newIssueRef(parts[1], parts[2], parts[3])
	}

	if len(parts) == 5 && parts[1] == "projects" && parts[3] == "issues" {
		return newIssueRef(parts[0], parts[2], parts[4])
	}

	return issueRef{}, false
}

// newIssueRef собирает issueRef, проверяя, что все сегменты заполнены, а номер задачи — положительное число.
func newIssueRef(workspace, project, sequence string) (issueRef, bool) {
	sequenceId, err := strconv.Atoi(sequence)
	if workspace == "" || project == "" || err != nil || sequenceId <= 0 {
		return issueRef{}, false
	}

	return issueRef{Workspace: workspace, Project: project, SequenceId: sequenceId}, true
}

// findIssueByIdOrSeq — вспомогательная функция для поиска задачи по ID или sequence
// findIssueByIdOrSeq грузит задачу с проектом, пространством и исполнителями —
// в таком виде её ждёт движок прав.
func findIssueByIdOrSeq(db *gorm.DB, issueIdOrSeq string) (*dao.Issue, error) {
	query := db.Joins("Project").Joins("Workspace").Preload("Assignees")

	var issue dao.Issue
	if id, err := uuid.FromString(issueIdOrSeq); err == nil {
		query = query.Where("issues.id = ?", id)
	} else {
		ref, ok := parseIssueRef(issueIdOrSeq)
		if !ok {
			return nil, nil
		}

		query = ref.apply(query)
	}

	if err := query.First(&issue).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	return &issue, nil
}
