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
	"golang.org/x/sync/errgroup"
	"gorm.io/gorm"
)

// getIssuesGroups возвращает группы задач с количеством в каждой группе
func getIssuesGroups(db *gorm.DB, user *dao.User, projectId uuid.UUID, sprint *dao.Sprint, searchParams *types.SearchParams) ([]types.SearchGroupSize, error) {
	query := db.Session(&gorm.Session{})

	// Определение запроса для фильтрации по проектам
	// Если указан спринт, выбираем project_id из таблицы SprintIssue для данного спринта
	// Если указан конкретный projectId, используем его напрямую
	// Если указан список ProjectIds в параметрах поиска, используем его напрямую
	// В противном случае используем список проектов в которых состоит пользователь
	var projectQuery any
	if sprint != nil {
		projectQuery = db.Select("project_id").Where("sprint_id = ?", sprint.Id).Model(&dao.SprintIssue{})
	} else if !projectId.IsNil() {
		projectQuery = projectId
	} else if len(searchParams.Filters.ProjectIds) > 0 {
		projectQuery = searchParams.Filters.ProjectIds
	} else {
		projectQuery = db.Select("project_id").Where("member_id = ?", user.ID).Model(&dao.ProjectMember{})
	}

	switch searchParams.GroupByParam {
	case "priority":
		query = query.
			Table("issues i").
			Select("count(*) as Count, coalesce(priority, '') as \"Key\"").
			Where("i.deleted_at is null").
			Where("i.project_id in (?)", projectQuery).
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
			Select("count(i.state_id) as Count, states.name || states.color || states.group as \"Key\", max(states.name) as state_name, max(states.sequence) as state_seq").
			Group(`"Key", states.group`).
			Order("case when states.group='cancelled' then 5 when states.group='completed' then 4 when states.group='started' then 3 when states.group='unstarted' then 2 when states.group='backlog' then 1 end, state_seq, state_name")

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

// fetchIssuesByGroups выполняет поиск задач по группам и вызывает callback для каждой группы
func fetchIssuesByGroups(
	db *gorm.DB,
	groupSize []types.SearchGroupSize,
	groupSelectQuery *gorm.DB,
	searchParams *types.SearchParams,
	iterFunc StreamCallback,
) (int, error) {
	totalCount := 0

	groupsEntity, err := fetchGroupsEntity(db, searchParams.GroupByParam, groupSize)
	if err != nil {
		return 0, err
	}

	g := errgroup.Group{}
	g.SetLimit(10)

	for i, group := range groupSize {
		totalCount += group.Count
		g.Go(func() error {
			q := groupSelectQuery.Session(&gorm.Session{})

			if group.Count == 0 {
				return iterFunc(dto.IssuesGroupResponse{
					SortId: i,
					Entity: groupsEntity[group.Key],
					Count:  group.Count,
				})
			}

			switch searchParams.GroupByParam {
			case "priority":
				if len(searchParams.Filters.Priorities) > 0 && !slices.Contains(searchParams.Filters.Priorities, group.Key) {
					return nil
				}
				if group.Key != "" {
					q = q.Where("issues.priority = ?", group.Key)
				} else {
					q = q.Where("issues.priority is null")
				}
			case "author":
				if len(searchParams.Filters.AuthorIds) > 0 && !slices.Contains(searchParams.Filters.AuthorIds, group.Key) {
					return nil
				}
				q = q.Where("created_by_id = ?", group.Key)
			case "state":
				q = q.Where("state_id in (select id from states where concat(name, color, \"group\") = ?)", group.Key)
			case "labels":
				if !searchParams.Filters.Labels.IsEmpty() && !searchParams.Filters.Labels.Contains(group.Key) {
					return nil
				}
				if group.Key == "" {
					q = q.Where("not exists (select 1 from issue_labels where issue_id = issues.id)")
				} else {
					q = q.Where("exists (select 1 from issue_labels where label_id = ? and issue_id = issues.id)", group.Key)
				}
			case "assignees":
				if !searchParams.Filters.AssigneeIds.IsEmpty() && !searchParams.Filters.AssigneeIds.Contains(group.Key) {
					//fmt.Println(searchParams.Filters.AssigneeIds.Array, group.Key)
					return nil
				}
				if group.Key == "" {
					q = q.Where("not exists (select 1 from issue_assignees where issue_id = issues.id)")
				} else {
					q = q.Where("exists (select 1 from issue_assignees where assignee_id = ? and issue_id = issues.id)", group.Key)
				}
			case "watchers":
				if !searchParams.Filters.WatcherIds.IsEmpty() && !searchParams.Filters.WatcherIds.Contains(group.Key) {
					return nil
				}
				if group.Key == "" {
					q = q.Where("not exists (select 1 from issue_watchers where issue_id = issues.id)")
				} else {
					q = q.Where("exists (select 1 from issue_watchers where watcher_id = ? and issue_id = issues.id)", group.Key)
				}
			case "project":
				if len(searchParams.Filters.ProjectIds) > 0 && !slices.Contains(searchParams.Filters.ProjectIds, group.Key) {
					return nil
				}
				q = q.Where("issues.project_id = ?", group.Key)
			}

			var issues []dao.IssueWithCount
			if err := q.Find(&issues).Error; err != nil {
				return err
			}

			if len(issues) == 0 {
				slog.Error("Empty search result for not empty group", "groupBy", searchParams.GroupByParam, "groupKey", group.Key, "groupCount", group.Count)
				return nil
			}

			populateAuthors(issues)

			return iterFunc(dto.IssuesGroupResponse{
				SortId: i,
				Entity: groupsEntity[group.Key],
				Count:  group.Count,
				Issues: utils.SliceToSlice(&issues, func(i *dao.IssueWithCount) *dto.IssueWithCount { return i.ToDTO() }),
			})
		})
	}

	return totalCount, g.Wait()
}

func fetchGroupsEntity(db *gorm.DB, groupBy string, groups []types.SearchGroupSize) (map[string]any, error) {
	ids := make([]string, 0, len(groups))

	for _, group := range groups {
		if group.Key == "" {
			continue
		}
		ids = append(ids, group.Key)
	}

	entityMap := make(map[string]any, len(ids))

	switch groupBy {
	case "priority":
		for _, priority := range ids {
			entityMap[priority] = priority
		}
	case "author":
		var users []dao.User
		if err := db.Where("id in (?)", ids).Find(&users).Error; err != nil {
			return nil, err
		}
		for _, user := range users {
			entityMap[user.ID.String()] = user.ToLightDTO()
		}
	case "state":
		var states []dao.State
		if err := db.Where("states.name || states.color || states.group in (?)", ids).Find(&states).Error; err != nil {
			return nil, err
		}
		for _, state := range states {
			entityMap[state.Name+state.Color+state.Group] = state.ToLightDTO()
		}
	case "labels":
		var labels []dao.Label
		if err := db.Where("id in (?)", ids).Find(&labels).Error; err != nil {
			return nil, err
		}
		for _, label := range labels {
			entityMap[label.ID.String()] = label.ToLightDTO()
		}
	case "assignees", "watchers":
		var users []dao.User
		if err := db.Where("id in (?)", ids).Find(&users).Error; err != nil {
			return nil, err
		}
		for _, user := range users {
			entityMap[user.ID.String()] = user.ToLightDTO()
		}
	case "project":
		var projects []dao.Project
		if err := db.Where("id in (?)", ids).Find(&projects).Error; err != nil {
			return nil, err
		}
		for _, project := range projects {
			entityMap[project.ID.String()] = project.ToLightDTO()
		}
	}

	return entityMap, nil
}
