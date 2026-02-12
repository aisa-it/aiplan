// Package search содержит логику поиска задач.
// Позволяет использовать поиск из разных мест приложения (HTTP handlers, MCP tools и др.)
package search

import (
	"log/slog"
	"slices"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dto"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

// GetIssuesGroups возвращает группы задач с количеством в каждой группе
func GetIssuesGroups(db *gorm.DB, user *dao.User, projectId string, sprint *dao.Sprint, searchParams *types.SearchParams) ([]types.SearchGroupSize, error) {
	query := db.Session(&gorm.Session{})

	// Определение запроса для фильтрации по проектам
	// Если указан спринт, выбираем project_id из таблицы SprintIssue для данного спринта
	// Если указан конкретный projectId, используем его напрямую
	// В противном случае используем список ProjectIds из параметров поиска
	var projectQuery any
	if sprint != nil {
		projectQuery = db.Select("project_id").Where("sprint_id = ?", sprint.Id).Model(&dao.SprintIssue{})
	} else if projectId != "" {
		projectQuery = projectId
	} else {
		projectQuery = searchParams.Filters.ProjectIds
	}

	switch searchParams.GroupByParam {
	case "priority":
		query = query.
			Table("issues i").
			Select("count(*) as Count, coalesce(priority, '') as \"Key\"").
			Where("i.deleted_at is null").
			Where("project_id in (?)", projectQuery).
			Group("priority").
			Order("case when priority='urgent' then 5 when priority='high' then 4 when priority='medium' then 3 when priority='low' then 2 when priority is null then 1 end")
	case "author":
		query = db.
			Model(&dao.ProjectMember{}).
			Joins("LEFT JOIN issues i on project_members.member_id = i.created_by_id and i.project_id in (?) and i.deleted_at is null", projectQuery).
			Joins("LEFT JOIN users u on u.id = i.created_by_id").
			Where("project_members.project_id in (?)", projectQuery).
			Select("count(i.created_by_id) as Count, project_members.member_id as \"Key\", coalesce((u.first_name || ' ' || u.last_name), u.email) as sub").
			Group(`"Key", sub`).
			Order("sub")
	case "state":
		query = db.
			Model(&dao.State{}).
			Joins("LEFT JOIN issues i on states.id = i.state_id and i.project_id in (?) and i.deleted_at is null", projectQuery).
			Where("states.project_id in (?)", projectQuery).
			Select("count(i.state_id) as Count, states.name || states.color || states.group as \"Key\", max(states.name) as state_name").
			Group(`"Key", states.group`).
			Order("case when states.group='cancelled' then 5 when states.group='completed' then 4 when states.group='started' then 3 when states.group='unstarted' then 2 when states.group='backlog' then 1 end, state_name")

		if searchParams.OnlyActive {
			query = query.Where("states.\"group\" <> ?", "cancelled").Where("states.\"group\" <> ?", "completed")
		}
		if len(searchParams.Filters.StateIds) > 0 {
			query = query.Where("states.id in (?)", searchParams.Filters.StateIds)
		}
	case "labels":
		query = query.Select("count(i.id) as Count, l.id as \"Key\", max(l.name) as label_name").
			Table("labels l").
			Joins("left join issue_labels il on il.label_id = l.id").
			Joins("right join issues i on il.issue_id = i.id and i.deleted_at is null").
			Where("il.project_id in (?) or il.project_id is null", projectQuery).
			Where("i.project_id in (?) or i.project_id is null", projectQuery).
			Where("l.project_id in (?) or l.project_id is null", projectQuery).
			Group(`"Key"`).
			Order("label_name")
	case "assignees":
		query = query.Select("count(i.id) as Count, u.id as \"Key\", coalesce((u.first_name || ' ' || u.last_name), u.email) as sub").
			Table("users u").
			Joins("left join issue_assignees ia on ia.assignee_id = u.id").
			Joins("right join issues i on ia.issue_id = i.id and i.deleted_at is null").
			Where("ia.project_id in (?) or ia.project_id is null", projectQuery).
			Where("i.project_id in (?) or i.project_id is null", projectQuery).
			Group(`"Key"`).
			Order("sub")
	case "watchers":
		query = query.Select("count(i.id) as Count, u.id as \"Key\", coalesce((u.first_name || ' ' || u.last_name), u.email) as sub").
			Table("users u").
			Joins("left join issue_watchers iw on iw.watcher_id = u.id").
			Joins("right join issues i on iw.issue_id = i.id and i.deleted_at is null").
			Where("iw.project_id in (?) or iw.project_id is null", projectQuery).
			Where("i.project_id in (?) or i.project_id is null", projectQuery).
			Group(`"Key"`).
			Order("sub")
	case "project":
		query = db.
			Table("projects p").
			Joins("LEFT JOIN issues i on p.id = i.project_id and i.deleted_at is null").
			Select("count(i.project_id) as Count, p.id as \"Key\", p.name as sub").
			Where("p.id in (?)", projectQuery).
			Group(`"Key", sub`).
			Order("sub")
		if len(searchParams.Filters.WorkspaceIds) > 0 {
			query = query.Where("p.workspace_id in (?)", searchParams.Filters.WorkspaceIds)
		}

		if !user.IsSuperuser {
			query = query.
				Where("p.id in (?)", db.
					Select("project_id").
					Where("member_id = ?", user.ID).
					Model(&dao.ProjectMember{}),
				)
		}
	}

	if sprint != nil {
		query = query.Where("i.id in (?)", sprint.GetIssuesIDs())
	}

	{
		if searchParams.Filters.AssignedToMe {
			query = query.Where("i.id in (?)", db.Select("issue_id").Model(&dao.IssueAssignee{}).Where("assignee_id = ?", user.ID))
		}

		if searchParams.Filters.WatchedByMe {
			query = query.Where("i.id in (?)", db.Select("issue_id").Model(&dao.IssueWatcher{}).Where("watcher_id = ?", user.ID))
		}

		if searchParams.Filters.AuthoredByMe {
			query = query.Where("i.created_by_id = ?", user.ID)
		}

		if !searchParams.Draft {
			query = query.Where("i.draft = false or i.draft is null")
		}

		if searchParams.HideSubIssues {
			query = query.Where("i.parent_id is null")
		}

		if (searchParams.OnlyActive || len(searchParams.Filters.StateIds) > 0) && searchParams.GroupByParam != "state" {
			query = query.Joins("left join states on i.state_id = states.id")
			if searchParams.OnlyActive {
				query = query.
					Where("states.\"group\" <> ?", "cancelled").
					Where("states.\"group\" <> ?", "completed")
			}

			if len(searchParams.Filters.StateIds) > 0 {
				query = query.Where("states.id in (?)", searchParams.Filters.StateIds)
			}
		}
	}

	var count []types.SearchGroupSize
	if err := query.Scan(&count).Error; err != nil {
		return nil, err
	}
	return count, nil
}

// StreamCallback - callback для streaming группированных результатов
// Если nil - результаты собираются в массив и возвращаются целиком
type StreamCallback func(group dto.IssuesGroupResponse) error

// FetchIssuesByGroups выполняет поиск задач по группам и вызывает callback для каждой группы
func FetchIssuesByGroups(
	db *gorm.DB,
	groupSize []types.SearchGroupSize,
	groupSelectQuery *gorm.DB,
	searchParams *types.SearchParams,
	iterFunc StreamCallback,
) (int, error) {
	totalCount := 0

	for _, group := range groupSize {
		totalCount += group.Count

		q := groupSelectQuery.Session(&gorm.Session{})

		var entity any
		switch searchParams.GroupByParam {
		case "priority":
			if len(searchParams.Filters.Priorities) > 0 && !slices.Contains(searchParams.Filters.Priorities, group.Key) {
				continue
			}
			if group.Key != "" {
				q = q.Where("issues.priority = ?", group.Key)
			} else {
				q = q.Where("issues.priority is null")
			}
			entity = group.Key
		case "author":
			if len(searchParams.Filters.AuthorIds) > 0 && !slices.Contains(searchParams.Filters.AuthorIds, group.Key) {
				continue
			}
			q = q.Where("created_by_id = ?", group.Key)
			if group.Count == 0 {
				var user dao.User
				if err := db.Where("id = ?", group.Key).First(&user).Error; err != nil {
					return 0, err
				}
				entity = user.ToLightDTO()
			}
		case "state":
			q = q.Where("state_id in (select id from states where concat(name, color, \"group\") = ?)", group.Key)
			if group.Count == 0 {
				var state dao.State
				if err := db.Where("states.name || states.color || states.group = ?", group.Key).First(&state).Error; err != nil {
					return 0, err
				}
				entity = state.ToLightDTO()
			}
		case "labels":
			if len(searchParams.Filters.Labels) > 0 && !slices.Contains(searchParams.Filters.Labels, group.Key) {
				continue
			}
			if group.Key == "" {
				q = q.Where("not exists (select 1 from issue_labels where issue_id = issues.id)")
			} else {
				q = q.Where("exists (select 1 from issue_labels where label_id = ? and issue_id = issues.id)", group.Key)
			}
			if group.Key != "" {
				var label dao.Label
				if err := db.Where("id = ?", group.Key).First(&label).Error; err != nil {
					return 0, err
				}
				entity = label.ToLightDTO()
			}
		case "assignees":
			if !searchParams.Filters.AssigneeIds.IsEmpty() && !searchParams.Filters.AssigneeIds.Contains(group.Key) {
				//fmt.Println(searchParams.Filters.AssigneeIds.Array, group.Key)
				continue
			}
			if group.Key == "" {
				q = q.Where("not exists (select 1 from issue_assignees where issue_id = issues.id)")
			} else {
				q = q.Where("exists (select 1 from issue_assignees where assignee_id = ? and issue_id = issues.id)", group.Key)
			}
			if group.Key != "" {
				var u dao.User
				if err := db.Where("id = ?", group.Key).First(&u).Error; err != nil {
					return 0, err
				}
				entity = u.ToLightDTO()
			}
		case "watchers":
			if !searchParams.Filters.WatcherIds.IsEmpty() && !searchParams.Filters.WatcherIds.Contains(group.Key) {
				continue
			}
			if group.Key == "" {
				q = q.Where("not exists (select 1 from issue_watchers where issue_id = issues.id)")
			} else {
				q = q.Where("exists (select 1 from issue_watchers where watcher_id = ? and issue_id = issues.id)", group.Key)
			}
			if group.Key != "" {
				var u dao.User
				if err := db.Where("id = ?", group.Key).First(&u).Error; err != nil {
					return 0, err
				}
				entity = u.ToLightDTO()
			}
		case "project":
			if len(searchParams.Filters.ProjectIds) > 0 && !slices.Contains(searchParams.Filters.ProjectIds, group.Key) {
				continue
			}
			q = q.Where("issues.project_id = ?", group.Key)
			if group.Count == 0 {
				var project dao.Project
				if err := db.Where("id = ?", group.Key).First(&project).Error; err != nil {
					return 0, err
				}
				entity = project.ToLightDTO()
			}
		}

		if group.Count == 0 {
			if err := iterFunc(dto.IssuesGroupResponse{
				Entity: entity,
				Count:  group.Count,
			}); err != nil {
				return 0, err
			}
			continue
		}

		var issues []dao.IssueWithCount
		if err := q.Find(&issues).Error; err != nil {
			return 0, err
		}

		if err := FetchParentsDetails(db, issues); err != nil {
			return 0, err
		}

		if len(issues) == 0 {
			slog.Error("Empty search result for not empty group", "groupBy", searchParams.GroupByParam, "groupKey", group.Key, "groupCount", group.Count)
			continue
		}

		switch searchParams.GroupByParam {
		case "author":
			entity = issues[0].Author.ToLightDTO()
		case "state":
			entity = issues[0].State.ToLightDTO()
		case "project":
			entity = issues[0].Project.ToLightDTO()
		}

		if err := iterFunc(dto.IssuesGroupResponse{
			Entity: entity,
			Count:  group.Count,
			Issues: utils.SliceToSlice(&issues, func(i *dao.IssueWithCount) any { return i.ToDTO() }),
		}); err != nil {
			return 0, err
		}
	}
	return totalCount, nil
}

// FetchParentsDetails загружает детали родительских задач
func FetchParentsDetails(db *gorm.DB, issues []dao.IssueWithCount) error {
	var parentIds []uuid.NullUUID
	for _, issue := range issues {
		if issue.ParentId.Valid {
			parentIds = append(parentIds, issue.ParentId)
		}
	}
	var parents []dao.Issue
	if err := db.Where("id in (?)", parentIds).Find(&parents).Error; err != nil {
		return err
	}
	parentsMap := make(map[string]*dao.Issue)
	for i := range parents {
		parentsMap[parents[i].ID.String()] = &parents[i]
	}
	for i := range issues {
		if issues[i].ParentId.Valid {
			issues[i].Parent = parentsMap[issues[i].ParentId.UUID.String()]
		}
	}
	return nil
}
