package tools

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/search"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/gofrs/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"gorm.io/gorm"
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
}

func GetIssuesTools(db *gorm.DB) []server.ServerTool {
	var resources []server.ServerTool
	for _, t := range issuesTools {
		resources = append(resources, server.ServerTool{
			Tool:    t.Tool,
			Handler: WrapTool(db, t.Handler),
		})
	}
	return resources
}

func getIssue(db *gorm.DB, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	issueIdOrSeq := request.GetArguments()["id"].(string)
	fmt.Println(issueIdOrSeq)

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
	if _, err := uuid.FromString(issueIdOrSeq); err == nil {
		// uuid id of issue
		query = query.Where("issues.id = ?", issueIdOrSeq)
	} else {
		var params []string
		if u, err := url.Parse(issueIdOrSeq); err == nil && u.Scheme != "" && u.Host != "" {
			params = filepath.SplitList(u.Path)
		} else {
			params = strings.Split(issueIdOrSeq, "-")
		}
		fmt.Println(params)
		if len(params) != 3 {
			return mcp.NewToolResultError("номер задачи не соответствует формату"), nil
		}

		// sequence id of issue
		query = query.Where(`"Workspace".slug = ? and "Project".identifier = ? and issues.sequence_id = ?`, params[0], params[1], params[2])
	}

	if err := query.
		First(&issue).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return mcp.NewToolResultError("задача не найдена"), nil
		}
		return mcp.NewToolResultError("internal error"), nil
	}

	// Fetch Author details
	if err := issue.Author.AfterFind(db); err != nil {
		return mcp.NewToolResultError("internal error"), nil
	}

	return mcp.NewToolResultJSON(issue.ToDTO())
}

func searchIssues(db *gorm.DB, user *dao.User, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	searchParams, err := types.ParseSearchParamsMCP(request.GetArguments())
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("ошибка парсинга параметров: %v", err)), nil
	}

	// Выполняем поиск (глобальный поиск, пустой ProjectMember)
	result, err := search.GetIssueListData(
		db,
		*user,
		dao.ProjectMember{}, // пустой - глобальный поиск
		nil,                 // без спринта
		true,                // globalSearch = true
		searchParams,
		nil, // без streaming
	)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("ошибка поиска: %v", err)), nil
	}

	return mcp.NewToolResultJSON(result)
}
