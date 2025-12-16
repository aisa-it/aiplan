package types

import (
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/apierrors"
	"github.com/gofrs/uuid"
	"github.com/labstack/echo/v4"
)

// IssueGroupFields - допустимые поля для группировки задач
var IssueGroupFields = []string{"priority", "author", "state", "labels", "assignees", "watchers", "project"}

// SearchGroupSize - размер группы при группированном поиске
type SearchGroupSize struct {
	Count int
	Key   string
}

// IssuesGroupedResponse - ответ с группированными задачами
type IssuesGroupedResponse struct {
	Count   int                   `json:"count"`
	Offset  int                   `json:"offset"`
	Limit   int                   `json:"limit"`
	GroupBy string                `json:"group_by"`
	Issues  []IssuesGroupResponse `json:"issues"`
}

// IssuesGroupResponse - одна группа в группированном ответе
type IssuesGroupResponse struct {
	Entity any   `json:"entity"`
	Count  int   `json:"count"`
	Issues []any `json:"issues"`
}

// StreamCallback - callback для streaming группированных результатов
// Если nil - результаты собираются в массив и возвращаются целиком
type StreamCallback func(group IssuesGroupResponse) error

type SearchParams struct {
	HideSubIssues bool
	Draft         bool
	OrderByParam  string
	GroupByParam  string
	OnlyCount     bool
	Offset        int
	Limit         int
	Desc          bool
	LightSearch   bool
	OnlyActive    bool
	OnlyPinned    bool
	Stream        bool

	Filters IssuesListFilters
}

var validSortFields = map[string]struct{}{
	"id":                  {},
	"created_at":          {},
	"updated_at":          {},
	"name":                {},
	"priority":            {},
	"target_date":         {},
	"sequence_id":         {},
	"state":               {},
	"labels":              {},
	"sub_issues_count":    {},
	"link_count":          {},
	"attachment_count":    {},
	"linked_issues_count": {},
	"assignees":           {},
	"watchers":            {},
	"author":              {},
	"search_rank":         {},
}

func ParseSearchParams(c echo.Context) (*SearchParams, error) {
	sp := &SearchParams{}
	if err := echo.QueryParamsBinder(c).
		Bool("hide_sub_issues", &sp.HideSubIssues).
		Bool("draft", &sp.Draft).
		String("order_by", &sp.OrderByParam).
		String("group_by", &sp.GroupByParam).
		Int("offset", &sp.Offset).
		Int("limit", &sp.Limit).
		Bool("desc", &sp.Desc).
		Bool("only_count", &sp.OnlyCount).
		Bool("light", &sp.LightSearch).
		Bool("only_active", &sp.OnlyActive).
		Bool("only_pinned", &sp.OnlyPinned).
		Bool("stream", &sp.Stream).
		BindError(); err != nil {
		return nil, err
	}

	if sp.OrderByParam == "" {
		if sp.Filters.SearchQuery == "" {
			sp.OrderByParam = "sequence_id"
		} else {
			sp.OrderByParam = "search_rank"
		}
	}

	if _, ok := validSortFields[sp.OrderByParam]; !ok {
		return nil, apierrors.ErrUnsupportedSortParam.WithFormattedMessage(sp.OrderByParam)
	}

	if sp.Limit == 0 {
		sp.Limit = 10
	}

	if err := c.Bind(&sp.Filters); err != nil {
		return nil, err
	}
	return sp, nil
}

// ParseSearchParamsMCP парсит параметры поиска из map[string]any (для MCP tools)
func ParseSearchParamsMCP(args map[string]any) (*SearchParams, error) {
	sp := &SearchParams{
		LightSearch: true, // Для MCP используем легкий поиск
		Limit:       10,
		Desc:        true,
	}

	// Поисковый запрос
	if v, ok := args["search_query"].(string); ok && v != "" {
		sp.Filters.SearchQuery = v
		sp.OrderByParam = "search_rank"
	} else {
		sp.OrderByParam = "sequence_id"
	}

	// Фильтры по пространствам и проектам
	if v, ok := args["workspace_slugs"].([]interface{}); ok {
		for _, s := range v {
			if str, ok := s.(string); ok {
				sp.Filters.WorkspaceSlugs = append(sp.Filters.WorkspaceSlugs, str)
			}
		}
	}

	if v, ok := args["project_ids"].([]interface{}); ok {
		for _, s := range v {
			if str, ok := s.(string); ok {
				sp.Filters.ProjectIds = append(sp.Filters.ProjectIds, str)
			}
		}
	}

	// Фильтры по атрибутам задач
	if v, ok := args["priorities"].([]interface{}); ok {
		for _, s := range v {
			if str, ok := s.(string); ok {
				sp.Filters.Priorities = append(sp.Filters.Priorities, str)
			}
		}
	}

	if v, ok := args["state_ids"].([]interface{}); ok {
		for _, s := range v {
			if str, ok := s.(string); ok {
				if id, err := uuid.FromString(str); err == nil {
					sp.Filters.StateIds = append(sp.Filters.StateIds, id)
				}
			}
		}
	}

	if v, ok := args["author_ids"].([]interface{}); ok {
		for _, s := range v {
			if str, ok := s.(string); ok {
				sp.Filters.AuthorIds = append(sp.Filters.AuthorIds, str)
			}
		}
	}

	if v, ok := args["assignee_ids"].([]interface{}); ok {
		for _, s := range v {
			if str, ok := s.(string); ok {
				sp.Filters.AssigneeIds = append(sp.Filters.AssigneeIds, str)
			}
		}
	}

	if v, ok := args["watcher_ids"].([]interface{}); ok {
		for _, s := range v {
			if str, ok := s.(string); ok {
				sp.Filters.WatcherIds = append(sp.Filters.WatcherIds, str)
			}
		}
	}

	if v, ok := args["labels"].([]interface{}); ok {
		for _, s := range v {
			if str, ok := s.(string); ok {
				sp.Filters.Labels = append(sp.Filters.Labels, str)
			}
		}
	}

	if v, ok := args["sprint_ids"].([]interface{}); ok {
		for _, s := range v {
			if str, ok := s.(string); ok {
				sp.Filters.SprintIds = append(sp.Filters.SprintIds, str)
			}
		}
	}

	// Булевые фильтры
	if v, ok := args["assigned_to_me"].(bool); ok {
		sp.Filters.AssignedToMe = v
	}

	if v, ok := args["authored_by_me"].(bool); ok {
		sp.Filters.AuthoredByMe = v
	}

	if v, ok := args["watched_by_me"].(bool); ok {
		sp.Filters.WatchedByMe = v
	}

	if v, ok := args["only_active"].(bool); ok {
		sp.OnlyActive = v
	}

	// Сортировка
	if v, ok := args["order_by"].(string); ok && v != "" {
		sp.OrderByParam = v
	}

	if _, ok := validSortFields[sp.OrderByParam]; !ok {
		return nil, apierrors.ErrUnsupportedSortParam.WithFormattedMessage(sp.OrderByParam)
	}

	if v, ok := args["desc"].(bool); ok {
		sp.Desc = v
	}

	// Пагинация
	if v, ok := args["limit"].(float64); ok && v > 0 {
		sp.Limit = int(v)
		if sp.Limit > 100 {
			sp.Limit = 100
		}
	}

	if v, ok := args["offset"].(float64); ok && v >= 0 {
		sp.Offset = int(v)
	}

	return sp, nil
}
