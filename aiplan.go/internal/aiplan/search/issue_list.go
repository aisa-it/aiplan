package search

import (
	"fmt"
	"slices"
	"strings"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/apierrors"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dto"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// BuildIssueListQuery - функция для построения запроса списка задач
// Возвращает сырые данные из БД без преобразования в DTO
// Не поддерживает группировку - для группировки используйте GetIssueListData
// Используется в MCP handlers для прямого доступа к DAO объектам
func BuildIssueListQuery(
	db *gorm.DB,
	user dao.User,
	projectMember dao.ProjectMember,
	sprint *dao.Sprint,
	globalSearch bool,
	searchParams *types.SearchParams,
) ([]dao.IssueWithCount, int, error) {
	// Валидация
	if searchParams.Limit > 100 {
		return nil, 0, apierrors.ErrLimitTooHigh
	}

	if searchParams.GroupByParam != "" {
		return nil, 0, apierrors.ErrUnsupportedGroup
	}

	searchParams.OrderByParam = strings.TrimPrefix(searchParams.OrderByParam, "-")

	var query *gorm.DB
	if searchParams.LightSearch {
		query = db.Preload("Author").Preload("State").Preload("Project").Preload("Workspace").Preload("Assignees").Preload("Watchers").Preload("Labels")
	} else {
		query = db.Preload(clause.Associations)
	}

	// Add membership info to project details on global search
	if globalSearch && !searchParams.LightSearch {
		query = query.Set("userId", user.ID)
	}

	// Fill filters
	if !globalSearch && projectMember.ProjectId != uuid.Nil {
		query = query.
			Where("issues.workspace_id = ?", projectMember.WorkspaceId).
			Where("issues.project_id = ?", projectMember.ProjectId)
	} else /* if !user.IsSuperuser */ {
		query = query.
			Where("issues.project_id in (?)", db.
				Select("project_id").
				Where("member_id = ?", user.ID).
				Model(&dao.ProjectMember{}),
			)
	}

	// Filter only sprint issues
	if sprint != nil {
		issuesID := make([]uuid.UUID, len(sprint.Issues))
		for i, issue := range sprint.Issues {
			issuesID[i] = issue.ID
		}
		query = query.Where("issues.id in (?)", issuesID)
	}

	// Filters
	{
		if len(searchParams.Filters.AuthorIds) > 0 {
			query = query.Where("issues.created_by_id in (?)", searchParams.Filters.AuthorIds)
		}

		if len(searchParams.Filters.AssigneeIds) > 0 {
			q := db.Where("issues.id in (?)",
				db.Select("issue_id").
					Where("assignee_id in (?)", searchParams.Filters.AssigneeIds).
					Model(&dao.IssueAssignee{}))
			if slices.Contains(searchParams.Filters.AssigneeIds, "") {
				q = q.Or("issues.id not in (?)", db.
					Select("issue_id").
					Model(&dao.IssueAssignee{}))
			}
			query = query.Where(q)
		}

		if len(searchParams.Filters.WatcherIds) > 0 {
			q := db.Where("issues.id in (?)",
				db.Select("issue_id").
					Where("watcher_id in (?)", searchParams.Filters.WatcherIds).
					Model(&dao.IssueWatcher{}))
			if slices.Contains(searchParams.Filters.WatcherIds, "") {
				q = q.Or("issues.id not in (?)", db.
					Select("issue_id").
					Model(&dao.IssueWatcher{}))
			}
			query = query.Where(q)
		}

		if len(searchParams.Filters.Priorities) > 0 {
			hasNull := false
			var arr []any
			for _, p := range searchParams.Filters.Priorities {
				if p != "" {
					arr = append(arr, p)
				} else {
					hasNull = true
				}
			}
			if hasNull {
				query = query.Where("issues.priority in (?) or issues.priority is null", arr)
			} else {
				query = query.Where("issues.priority in (?)", arr)
			}
		}

		if len(searchParams.Filters.Labels) > 0 {
			q := db.Where("issues.id in (?)", db.
				Model(&dao.IssueLabel{}).
				Select("issue_id").
				Where("label_id in (?)", searchParams.Filters.Labels))
			if slices.Contains(searchParams.Filters.Labels, "") {
				q = q.Or("issues.id not in (?)", db.
					Select("issue_id").
					Model(&dao.IssueLabel{}))
			}
			query = query.Where(q)
		}

		if len(searchParams.Filters.SprintIds) > 0 {
			q := db.Where("issues.id in (?)", db.
				Model(&dao.SprintIssue{}).
				Select("issue_id").
				Where("sprint_id in (?)", searchParams.Filters.SprintIds))
			if slices.Contains(searchParams.Filters.SprintIds, "") {
				q = q.Or("issues.id not in (?)", db.
					Select("issue_id").
					Model(&dao.SprintIssue{}))
			}
			query = query.Where(q)
		}

		if len(searchParams.Filters.WorkspaceIds) > 0 {
			query = query.Where("issues.workspace_id in (?)",
				db.Select("workspace_id").
					Model(&dao.WorkspaceMember{}).
					Where("member_id = ?", user.ID).
					Where("workspace_id in (?)", searchParams.Filters.WorkspaceIds))
		}

		if len(searchParams.Filters.WorkspaceSlugs) > 0 {
			query = query.Where("issues.workspace_id in (?)",
				db.Model(&dao.WorkspaceMember{}).
					Select("workspace_id").
					Where("member_id = ?", user.ID).
					Where("workspace_id in (?)", db.Model(&dao.Workspace{}).
						Select("id").
						Where("slug in (?)", searchParams.Filters.WorkspaceSlugs)))
		}

		if len(searchParams.Filters.ProjectIds) > 0 {
			query = query.Where("issues.project_id in (?)",
				db.Select("project_id").
					Model(&dao.WorkspaceMember{}).
					Where("member_id = ?", user.ID).
					Where("project_id in (?)", searchParams.Filters.ProjectIds))
		}

		// If workspace not specified, use all user workspaces
		if len(searchParams.Filters.WorkspaceIds) == 0 && len(searchParams.Filters.WorkspaceSlugs) == 0 && globalSearch && !user.IsSuperuser {
			query = query.Where("issues.workspace_id in (?)",
				db.Select("workspace_id").
					Model(&dao.WorkspaceMember{}).
					Where("member_id = ?", user.ID))
		}

		if searchParams.Filters.AssignedToMe {
			query = query.Where("issues.id in (?)", db.Select("issue_id").Model(&dao.IssueAssignee{}).Where("assignee_id = ?", user.ID))
		}

		if searchParams.Filters.WatchedByMe {
			query = query.Where("issues.id in (?)", db.Select("issue_id").Model(&dao.IssueWatcher{}).Where("watcher_id = ?", user.ID))
		}

		if searchParams.Filters.AuthoredByMe {
			query = query.Where("issues.created_by_id = ?", user.ID)
		}

		if searchParams.OnlyActive || len(searchParams.Filters.StateIds) > 0 {
			subQuery := db.Model(&dao.State{}).
				Select("id")

			if searchParams.OnlyActive {
				subQuery = subQuery.
					Where("\"group\" <> ?", "cancelled").
					Where("\"group\" <> ?", "completed")
			}

			if len(searchParams.Filters.StateIds) > 0 {
				subQuery = subQuery.
					Where("issues.state_id in (?)", searchParams.Filters.StateIds)
			}

			query = query.Where("issues.state_id in (?)", subQuery)
		}

		if searchParams.OnlyPinned {
			query = query.Where("issues.pinned = true")
		}

		if searchParams.Filters.SearchQuery != "" {
			query = query.Joins("join projects p on p.id = issues.project_id").
				Where("p.deleted_at IS NULL").
				Where(dao.Issue{}.FullTextSearch(db, searchParams.Filters.SearchQuery))
		}
	}

	// Ignore slave issues
	if searchParams.HideSubIssues {
		query = query.Where("issues.parent_id is null")
	}

	// Ignore draft issues
	if !searchParams.Draft {
		query = query.Where("issues.draft = false or issues.draft is null")
	}

	if searchParams.OnlyCount {
		var count int64
		if err := query.Model(&dao.Issue{}).Count(&count).Error; err != nil {
			return nil, 0, err
		}

		// Для OnlyCount возвращаем пустой слайс, но с правильным count
		return []dao.IssueWithCount{}, int(count), nil
	}

	var selectExprs []string
	var selectInterface []any

	// Fetch counters fo full search
	if !searchParams.LightSearch {
		selectExprs = []string{
			"issues.*",
			"count(*) over() as all_count",
			"(?) as sub_issues_count",
			"(?) as link_count",
			"(?) as attachment_count",
			"(?) as linked_issues_count",
			"(?) as comments_count",
		}
		selectInterface = []interface{}{
			db.Table("issues as \"child\"").Select("count(*)").Where("\"child\".parent_id = issues.id"),
			db.Select("count(*)").Where("issue_id = issues.id").Model(&dao.IssueLink{}),
			db.Select("count(*)").Where("issue_id = issues.id::uuid").Model(&dao.IssueAttachment{}),
			db.Select("count(*)").Where("id1 = issues.id OR id2 = issues.id").Model(&dao.LinkedIssues{}),
			db.Select("count(*)").Where("issue_id = issues.id").Model(&dao.IssueComment{}),
		}
	} else {
		selectExprs = []string{
			"issues.*",
			"count(*) over() as all_count",
		}
	}

	// Rank count
	if searchParams.Filters.SearchQuery != "" {
		searchSelects := []string{
			"ts_headline('russian', issues.name, plainto_tsquery('russian', ?)) as name_highlighted",
			"ts_headline('russian', issues.description_stripped, plainto_tsquery('russian', ?), 'MaxFragments=10, MaxWords=8, MinWords=3') as desc_highlighted",
			"calc_rank(tokens, p.identifier, issues.sequence_id, ?) as ts_rank",
		}
		searchInterface := []interface{}{
			searchParams.Filters.SearchQuery,
			searchParams.Filters.SearchQuery,
			searchParams.Filters.SearchQuery,
		}

		selectExprs = append(selectExprs, searchSelects...)
		selectInterface = append(selectInterface, searchInterface...)
	}

	order := &clause.OrderByColumn{Desc: searchParams.Desc}
	switch searchParams.OrderByParam {
	case "priority":
		order = nil
		sql := "case when priority='urgent' then 5 when priority='high' then 4 when priority='medium' then 3 when priority='low' then 2 when priority is null then 1 end"
		if searchParams.Desc {
			sql += " DESC"
		}
		query = query.Order(sql)
	case "author":
		selectExprs = append(selectExprs, "(?) as author_sort")
		selectInterface = append(selectInterface, db.Select("COALESCE(NULLIF(last_name,''), email)").Where("id = issues.created_by_id").Model(&dao.User{}))
		order.Column = clause.Column{Name: "author_sort"}
	case "state":
		selectExprs = append(selectExprs, "(?) as state_sort")
		selectInterface = append(selectInterface, db.Select(`concat(case "group" when 'backlog' then 1 when 'unstarted' then 2 when 'started' then 3 when 'completed' then 4 when 'cancelled' then 5 end, name, color)`).Where("id = issues.state_id").Model(&dao.State{}))
		order.Column = clause.Column{Name: "state_sort"}
	case "labels":
		selectExprs = append(selectExprs, "array(?) as labels_sort")
		selectInterface = append(selectInterface, db.Select("name").Where("id in (?)", db.Select("label_id").Where("issue_id = issues.id").Model(&dao.IssueLabel{})).Model(&dao.Label{}))
		order.Column = clause.Column{Name: "labels_sort"}
	case "sub_issues_count":
		fallthrough
	case "link_count":
		fallthrough
	case "linked_issues_count":
		fallthrough
	case "attachment_count":
		order.Column = clause.Column{Name: searchParams.OrderByParam}
	case "assignees":
		selectExprs = append(selectExprs, "array(?) as assignees_sort")
		selectInterface = append(selectInterface, db.Select("COALESCE(NULLIF(last_name,''), email)").Where("users.id in (?)", db.Select("assignee_id").Where("issue_id = issues.id").Model(&dao.IssueAssignee{})).Model(&dao.User{}))
		order.Column = clause.Column{Name: searchParams.OrderByParam + "_sort"}
	case "watchers":
		selectExprs = append(selectExprs, "array(?) as watchers_sort")
		selectInterface = append(selectInterface, db.Select("COALESCE(NULLIF(last_name,''), email)").Where("users.id in (?)", db.Select("watcher_id").Where("issue_id = issues.id").Model(&dao.IssueWatcher{})).Model(&dao.User{}))
		order.Column = clause.Column{Name: searchParams.OrderByParam + "_sort"}
	case "search_rank":
		order = nil
		query = query.Order("ts_rank desc")
	default:
		order.Column = clause.Column{Table: "issues", Name: searchParams.OrderByParam}
	}

	if order != nil {
		query = query.Order(*order)
	}
	query = query.Select(strings.Join(selectExprs, ", "), selectInterface...).Limit(searchParams.Limit).Offset(searchParams.Offset)

	var issues []dao.IssueWithCount
	if err := query.Find(&issues).Error; err != nil {
		return nil, 0, err
	}

	count := 0
	if len(issues) > 0 {
		count = issues[0].AllCount
	}

	if !searchParams.LightSearch {
		if err := FetchParentsDetails(db, issues); err != nil {
			return nil, 0, err
		}
	}

	return issues, count, nil
}

// GetIssueListData возвращает данные списка задач без привязки к HTTP контексту
// Используется для переиспользования логики в MCP tools и других местах
// Поддерживает группировку и streaming через callback
func GetIssueListData(
	db *gorm.DB,
	user dao.User,
	projectMember dao.ProjectMember,
	sprint *dao.Sprint,
	globalSearch bool,
	searchParams *types.SearchParams,
	streamCallback StreamCallback,
) (any, error) {
	// Валидация
	if searchParams.Limit > 100 {
		return nil, apierrors.ErrLimitTooHigh
	}

	if searchParams.GroupByParam != "" && !slices.Contains(types.IssueGroupFields, searchParams.GroupByParam) {
		return nil, apierrors.ErrUnsupportedGroup
	}

	// OnlyCount - особый случай, используем BuildIssueListQuery
	if searchParams.OnlyCount {
		issues, count, err := BuildIssueListQuery(db, user, projectMember, sprint, globalSearch, searchParams)
		if err != nil {
			return nil, err
		}
		// buildIssueListQuery уже вернула пустой слайс для OnlyCount
		_ = issues
		return map[string]any{
			"count": count,
		}, nil
	}

	// Группировка требует построения query вручную (не через buildIssueListQuery)
	if searchParams.GroupByParam != "" {
		searchParams.OrderByParam = strings.TrimPrefix(searchParams.OrderByParam, "-")

		var query *gorm.DB
		if searchParams.LightSearch {
			query = db.Preload("Author").Preload("State").Preload("Project").Preload("Workspace").Preload("Assignees").Preload("Watchers").Preload("Labels")
		} else {
			query = db.Preload(clause.Associations)
		}

		// Add membership info to project details on global search
		if globalSearch && !searchParams.LightSearch {
			query = query.Set("userId", user.ID)
		}

		// Fill filters
		if !globalSearch && projectMember.ProjectId != uuid.Nil {
			query = query.
				Where("issues.workspace_id = ?", projectMember.WorkspaceId).
				Where("issues.project_id = ?", projectMember.ProjectId)
		} else /* if !user.IsSuperuser */ {
			query = query.
				Where("issues.project_id in (?)", db.
					Select("project_id").
					Where("member_id = ?", user.ID).
					Model(&dao.ProjectMember{}),
				)
		}

		// Filter only sprint issues
		if sprint != nil {
			issuesID := make([]uuid.UUID, len(sprint.Issues))
			for i, issue := range sprint.Issues {
				issuesID[i] = issue.ID
			}
			query = query.Where("issues.id in (?)", issuesID)
		}

		// Filters
		{
			if len(searchParams.Filters.AuthorIds) > 0 {
				query = query.Where("issues.created_by_id in (?)", searchParams.Filters.AuthorIds)
			}

			if len(searchParams.Filters.AssigneeIds) > 0 {
				q := db.Where("issues.id in (?)",
					db.Select("issue_id").
						Where("assignee_id in (?)", searchParams.Filters.AssigneeIds).
						Model(&dao.IssueAssignee{}))
				if slices.Contains(searchParams.Filters.AssigneeIds, "") {
					q = q.Or("issues.id not in (?)", db.
						Select("issue_id").
						Model(&dao.IssueAssignee{}))
				}
				query = query.Where(q)
			}

			if len(searchParams.Filters.WatcherIds) > 0 {
				q := db.Where("issues.id in (?)",
					db.Select("issue_id").
						Where("watcher_id in (?)", searchParams.Filters.WatcherIds).
						Model(&dao.IssueWatcher{}))
				if slices.Contains(searchParams.Filters.WatcherIds, "") {
					q = q.Or("issues.id not in (?)", db.
						Select("issue_id").
						Model(&dao.IssueWatcher{}))
				}
				query = query.Where(q)
			}

			if len(searchParams.Filters.Priorities) > 0 {
				hasNull := false
				var arr []any
				for _, p := range searchParams.Filters.Priorities {
					if p != "" {
						arr = append(arr, p)
					} else {
						hasNull = true
					}
				}
				if hasNull {
					query = query.Where("issues.priority in (?) or issues.priority is null", arr)
				} else {
					query = query.Where("issues.priority in (?)", arr)
				}
			}

			if len(searchParams.Filters.Labels) > 0 {
				q := db.Where("issues.id in (?)", db.
					Model(&dao.IssueLabel{}).
					Select("issue_id").
					Where("label_id in (?)", searchParams.Filters.Labels))
				if slices.Contains(searchParams.Filters.Labels, "") {
					q = q.Or("issues.id not in (?)", db.
						Select("issue_id").
						Model(&dao.IssueLabel{}))
				}
				query = query.Where(q)
			}

			if len(searchParams.Filters.SprintIds) > 0 {
				q := db.Where("issues.id in (?)", db.
					Model(&dao.SprintIssue{}).
					Select("issue_id").
					Where("sprint_id in (?)", searchParams.Filters.SprintIds))
				if slices.Contains(searchParams.Filters.SprintIds, "") {
					q = q.Or("issues.id not in (?)", db.
						Select("issue_id").
						Model(&dao.SprintIssue{}))
				}
				query = query.Where(q)
			}

			if len(searchParams.Filters.WorkspaceIds) > 0 {
				query = query.Where("issues.workspace_id in (?)",
					db.Select("workspace_id").
						Model(&dao.WorkspaceMember{}).
						Where("member_id = ?", user.ID).
						Where("workspace_id in (?)", searchParams.Filters.WorkspaceIds))
			}

			if len(searchParams.Filters.WorkspaceSlugs) > 0 {
				query = query.Where("issues.workspace_id in (?)",
					db.Model(&dao.WorkspaceMember{}).
						Select("workspace_id").
						Where("member_id = ?", user.ID).
						Where("workspace_id in (?)", db.Model(&dao.Workspace{}).
							Select("id").
							Where("slug in (?)", searchParams.Filters.WorkspaceSlugs)))
			}

			if len(searchParams.Filters.ProjectIds) > 0 {
				query = query.Where("issues.project_id in (?)",
					db.Select("project_id").
						Model(&dao.WorkspaceMember{}).
						Where("member_id = ?", user.ID).
						Where("project_id in (?)", searchParams.Filters.ProjectIds))
			}

			// If workspace not specified, use all user workspaces
			if len(searchParams.Filters.WorkspaceIds) == 0 && len(searchParams.Filters.WorkspaceSlugs) == 0 && globalSearch && !user.IsSuperuser {
				query = query.Where("issues.workspace_id in (?)",
					db.Select("workspace_id").
						Model(&dao.WorkspaceMember{}).
						Where("member_id = ?", user.ID))
			}

			if searchParams.Filters.AssignedToMe {
				query = query.Where("issues.id in (?)", db.Select("issue_id").Model(&dao.IssueAssignee{}).Where("assignee_id = ?", user.ID))
			}

			if searchParams.Filters.WatchedByMe {
				query = query.Where("issues.id in (?)", db.Select("issue_id").Model(&dao.IssueWatcher{}).Where("watcher_id = ?", user.ID))
			}

			if searchParams.Filters.AuthoredByMe {
				query = query.Where("issues.created_by_id = ?", user.ID)
			}

			if searchParams.OnlyActive || len(searchParams.Filters.StateIds) > 0 {
				subQuery := db.Model(&dao.State{}).
					Select("id")

				if searchParams.OnlyActive {
					subQuery = subQuery.
						Where("\"group\" <> ?", "cancelled").
						Where("\"group\" <> ?", "completed")
				}

				if len(searchParams.Filters.StateIds) > 0 {
					subQuery = subQuery.
						Where("issues.state_id in (?)", searchParams.Filters.StateIds)
				}

				query = query.Where("issues.state_id in (?)", subQuery)
			}

			if searchParams.OnlyPinned {
				query = query.Where("issues.pinned = true")
			}

			if searchParams.Filters.SearchQuery != "" {
				query = query.Joins("join projects p on p.id = issues.project_id").
					Where("p.deleted_at IS NULL").
					Where(dao.Issue{}.FullTextSearch(db, searchParams.Filters.SearchQuery))
			}
		}

		// Ignore slave issues
		if searchParams.HideSubIssues {
			query = query.Where("issues.parent_id is null")
		}

		// Ignore draft issues
		if !searchParams.Draft {
			query = query.Where("issues.draft = false or issues.draft is null")
		}

		var selectExprs []string
		var selectInterface []any

		// Fetch counters fo full search
		if !searchParams.LightSearch {
			selectExprs = []string{
				"issues.*",
				"count(*) over() as all_count",
				"(?) as sub_issues_count",
				"(?) as link_count",
				"(?) as attachment_count",
				"(?) as linked_issues_count",
				"(?) as comments_count",
			}
			selectInterface = []interface{}{
				db.Table("issues as \"child\"").Select("count(*)").Where("\"child\".parent_id = issues.id"),
				db.Select("count(*)").Where("issue_id = issues.id").Model(&dao.IssueLink{}),
				db.Select("count(*)").Where("issue_id = issues.id::uuid").Model(&dao.IssueAttachment{}),
				db.Select("count(*)").Where("id1 = issues.id OR id2 = issues.id").Model(&dao.LinkedIssues{}),
				db.Select("count(*)").Where("issue_id = issues.id").Model(&dao.IssueComment{}),
			}
		} else {
			selectExprs = []string{
				"issues.*",
				"count(*) over() as all_count",
			}
		}

		// Rank count
		if searchParams.Filters.SearchQuery != "" {
			searchSelects := []string{
				"ts_headline('russian', issues.name, plainto_tsquery('russian', ?)) as name_highlighted",
				"ts_headline('russian', issues.description_stripped, plainto_tsquery('russian', ?), 'MaxFragments=10, MaxWords=8, MinWords=3') as desc_highlighted",
				"calc_rank(tokens, p.identifier, issues.sequence_id, ?) as ts_rank",
			}
			searchInterface := []interface{}{
				searchParams.Filters.SearchQuery,
				searchParams.Filters.SearchQuery,
				searchParams.Filters.SearchQuery,
			}

			selectExprs = append(selectExprs, searchSelects...)
			selectInterface = append(selectInterface, searchInterface...)
		}

		order := &clause.OrderByColumn{Desc: searchParams.Desc}
		switch searchParams.OrderByParam {
		case "priority":
			order = nil
			sql := "case when priority='urgent' then 5 when priority='high' then 4 when priority='medium' then 3 when priority='low' then 2 when priority is null then 1 end"
			if searchParams.Desc {
				sql += " DESC"
			}
			query = query.Order(sql)
		case "author":
			selectExprs = append(selectExprs, "(?) as author_sort")
			selectInterface = append(selectInterface, db.Select("COALESCE(NULLIF(last_name,''), email)").Where("id = issues.created_by_id").Model(&dao.User{}))
			order.Column = clause.Column{Name: "author_sort"}
		case "state":
			selectExprs = append(selectExprs, "(?) as state_sort")
			selectInterface = append(selectInterface, db.Select(`concat(case "group" when 'backlog' then 1 when 'unstarted' then 2 when 'started' then 3 when 'completed' then 4 when 'cancelled' then 5 end, name, color)`).Where("id = issues.state_id").Model(&dao.State{}))
			order.Column = clause.Column{Name: "state_sort"}
		case "labels":
			selectExprs = append(selectExprs, "array(?) as labels_sort")
			selectInterface = append(selectInterface, db.Select("name").Where("id in (?)", db.Select("label_id").Where("issue_id = issues.id").Model(&dao.IssueLabel{})).Model(&dao.Label{}))
			order.Column = clause.Column{Name: "labels_sort"}
		case "sub_issues_count":
			fallthrough
		case "link_count":
			fallthrough
		case "linked_issues_count":
			fallthrough
		case "attachment_count":
			order.Column = clause.Column{Name: searchParams.OrderByParam}
		case "assignees":
			selectExprs = append(selectExprs, "array(?) as assignees_sort")
			selectInterface = append(selectInterface, db.Select("COALESCE(NULLIF(last_name,''), email)").Where("users.id in (?)", db.Select("assignee_id").Where("issue_id = issues.id").Model(&dao.IssueAssignee{})).Model(&dao.User{}))
			order.Column = clause.Column{Name: searchParams.OrderByParam + "_sort"}
		case "watchers":
			selectExprs = append(selectExprs, "array(?) as watchers_sort")
			selectInterface = append(selectInterface, db.Select("COALESCE(NULLIF(last_name,''), email)").Where("users.id in (?)", db.Select("watcher_id").Where("issue_id = issues.id").Model(&dao.IssueWatcher{})).Model(&dao.User{}))
			order.Column = clause.Column{Name: searchParams.OrderByParam + "_sort"}
		case "search_rank":
			order = nil
			query = query.Order("ts_rank desc")
		default:
			order.Column = clause.Column{Table: "issues", Name: searchParams.OrderByParam}
		}

		if order != nil {
			query = query.Order(*order)
		}
		query = query.Select(strings.Join(selectExprs, ", "), selectInterface...).Limit(searchParams.Limit).Offset(searchParams.Offset)

		// Выполнение группировки
		groupSize, err := GetIssuesGroups(db, &user, projectMember.ProjectId.String(), sprint, searchParams)
		if err != nil {
			return nil, err
		}

		var groupMap []dto.IssuesGroupResponse

		totalCount, err := FetchIssuesByGroups(db, groupSize, query.Session(&gorm.Session{}), searchParams, func(group dto.IssuesGroupResponse) error {
			if streamCallback != nil {
				return streamCallback(group)
			}
			groupMap = append(groupMap, group)
			return nil
		})
		if err != nil {
			return nil, err
		}

		// Для streaming возвращаем nil - данные уже отправлены через callback
		if streamCallback != nil {
			return nil, nil
		}

		return dto.IssuesGroupedResponse{
			PaginationMeta: dto.PaginationMeta{
				Count:  totalCount,
				Offset: searchParams.Offset,
				Limit:  searchParams.Limit,
			},
			GroupBy: searchParams.GroupByParam,
			Issues:  groupMap,
		}, nil
	}

	// Обычный случай (без группировки) - используем BuildIssueListQuery
	issues, count, err := BuildIssueListQuery(db, user, projectMember, sprint, globalSearch, searchParams)
	if err != nil {
		return nil, err
	}

	paginationMeta := dto.PaginationMeta{
		Count:  count,
		Offset: searchParams.Offset,
		Limit:  searchParams.Limit,
	}

	// Преобразование в map с DTO
	if searchParams.LightSearch {
		return dto.IssuesLightSearchResponse{
			PaginationMeta: paginationMeta,
			Issues:         utils.SliceToSlice(&issues, func(iwc *dao.IssueWithCount) dto.SearchLightweightIssue { return iwc.ToSearchLightDTO() }),
		}, nil
	}

	return dto.IssuesSearchResponse{
		PaginationMeta: paginationMeta,
		Issues:         utils.SliceToSlice(&issues, func(iwc *dao.IssueWithCount) dto.IssueWithCount { return *iwc.ToDTO() }),
	}, nil
}

// FormatIssuesToMarkdownTable форматирует список задач в расширенную Markdown таблицу
// Используется для экономии токенов при передаче данных LLM через MCP протокол
func FormatIssuesToMarkdownTable(issues []dao.IssueWithCount, count, offset, limit int) string {
	var sb strings.Builder

	// Заголовок
	sb.WriteString(fmt.Sprintf("### Задачи (найдено: %d, показано: %d, offset: %d)\n\n", count, limit, offset))

	// Если пусто
	if len(issues) == 0 {
		sb.WriteString("Задачи не найдены.\n")
		return sb.String()
	}

	// Заголовок таблицы (расширенный)
	sb.WriteString("| ID | Название | Статус | Приоритет | Исполнители | Наблюдатели | Автор | Создана | Срок | Начало | Завершение | Родитель | Блокирует | Заблокирована | Подзадач | Ссылок | Вложений | Связей | Комментариев |\n")
	sb.WriteString("|----|----------|--------|-----------|-------------|-------------|-------|---------|------|--------|------------|----------|-----------|---------------|----------|--------|----------|--------|-------------|\n")

	// Строки
	for _, issue := range issues {
		sb.WriteString(issue.ToMCP())
		sb.WriteString("\n")
	}

	return sb.String()
}
