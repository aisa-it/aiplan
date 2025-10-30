package aiplan

import (
	"maps"
	"slices"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dto"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"gorm.io/gorm"
)

func GetIssuesGroupsSize(db *gorm.DB, user *dao.User, projectId string, searchParams *types.SearchParams) (map[string]int, error) {
	query := db.Session(&gorm.Session{})
	switch searchParams.GroupByParam {
	case "priority":
		query = query.Where("project_id = ?", projectId).Table("issues i").Select("count(*) as Count, coalesce(priority, '') as \"Key\"")
	case "author":
		query = db.
			Model(&dao.ProjectMember{}).
			Joins("LEFT JOIN issues i on project_members.member_id = i.created_by_id and i.project_id = ?", projectId).
			Where("project_members.project_id = ?", projectId).
			Select("count(i.created_by_id) as Count, project_members.member_id as \"Key\"")
	case "state":
		query = db.
			Model(&dao.State{}).
			Joins("LEFT JOIN issues i on states.id = i.state_id and i.project_id = ?", projectId).
			Where("states.project_id = ?", projectId).
			Select("count(i.state_id) as Count, states.id as \"Key\"")
		if searchParams.OnlyActive {
			query = query.Where("states.\"group\" <> ?", "cancelled").Where("states.\"group\" <> ?", "completed")
		}
		if len(searchParams.Filters.StateIds) > 0 {
			query = query.Where("states.id in (?)", searchParams.Filters.StateIds)
		}
	case "labels":
		query = query.Select("count(i.id) as Count, l.id as \"Key\"").
			Table("labels l").
			Joins("left join issue_labels il on il.label_id = l.id").
			Joins("right join issues i on il.issue_id = i.id").
			Where("(il.project_id = ? or il.project_id is null)", projectId).
			Where("(i.project_id = ? or i.project_id is null)", projectId).
			Where("(l.project_id = ? or l.project_id is null)", projectId)
	case "assignees":
		query = query.Select("count(i.id) as Count, u.id as \"Key\"").
			Table("users u").
			Joins("left join issue_assignees ia on ia.assignee_id = u.id").
			Joins("right join issues i on ia.issue_id = i.id").
			Where("(ia.project_id = ? or ia.project_id is null)", projectId).
			Where("(i.project_id = ? or i.project_id is null)", projectId)
	case "watchers":
		query = query.Select("count(i.id) as Count, u.id as \"Key\"").
			Table("users u").
			Joins("left join issue_watchers iw on iw.watcher_id = u.id").
			Joins("right join issues i on iw.issue_id = i.id").
			Where("(iw.project_id = ? or iw.project_id is null)", projectId).
			Where("(i.project_id = ? or i.project_id is null)", projectId)
	}

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

	if !searchParams.ShowSubIssues {
		query = query.Where("i.parent_id is null")
	}

	query = query.Offset(-1).Limit(-1).Group("Key")

	var count []struct {
		Count int
		Key   string
	}
	if err := query.Scan(&count).Error; err != nil {
		return nil, err
	}

	res := make(map[string]int, len(count))
	for _, c := range count {
		res[c.Key] = c.Count
	}
	return res, nil
}

func FetchIssuesByGroups(
	groupSize map[string]int,
	db *gorm.DB, // Clean session cause gorm reset not working
	groupSelectQuery *gorm.DB,
	searchParams *types.SearchParams,
) (int, map[string]IssuesGroupResponse, error) {
	groupMap := make(map[string]IssuesGroupResponse, len(groupSize))

	totalCount := 0

	for group, size := range groupSize {
		totalCount += size

		q := groupSelectQuery.Session(&gorm.Session{})

		var entity any
		switch searchParams.GroupByParam {
		case "priority":
			if len(searchParams.Filters.Priorities) > 0 && !slices.Contains(searchParams.Filters.Priorities, group) {
				continue
			}
			if group != "" {
				q = q.Where("issues.priority = ?", group)
			} else {
				q = q.Where("issues.priority is null")
			}
			entity = group
		case "author":
			if len(searchParams.Filters.AuthorIds) > 0 && !slices.Contains(searchParams.Filters.AuthorIds, group) {
				continue
			}
			q = q.Where("created_by_id = ?", group)
			if size == 0 {
				var user dao.User
				if err := db.Where("id = ?", group).First(&user).Error; err != nil {
					return 0, nil, err
				}
				entity = user.ToLightDTO()
			}
		case "state":
			if len(searchParams.Filters.StateIds) > 0 && !slices.Contains(searchParams.Filters.StateIds, group) {
				continue
			}
			q = q.Where("state_id = ?", group)
			if size == 0 {
				var state dao.State
				if err := db.Where("id = ?", group).First(&state).Error; err != nil {
					return 0, nil, err
				}
				entity = state.ToLightDTO()
			}
		case "labels":
			if len(searchParams.Filters.Labels) > 0 && !slices.Contains(searchParams.Filters.Labels, group) {
				continue
			}
			if group == "" {
				q = q.Where("not exists (select 1 from issue_labels where issue_id = issues.id)")
			} else {
				q = q.Where("exists (select 1 from issue_labels where label_id = ? and issue_id = issues.id)", group)
			}
			if group != "" {
				var label dao.Label
				if err := db.Where("id = ?", group).First(&label).Error; err != nil {
					return 0, nil, err
				}
				entity = label.ToLightDTO()
			}
		case "assignees":
			if len(searchParams.Filters.AssigneeIds) > 0 && !slices.Contains(searchParams.Filters.AssigneeIds, group) {
				continue
			}
			if group == "" {
				q = q.Where("not exists (select 1 from issue_assignees where issue_id = issues.id)")
			} else {
				q = q.Where("exists (select 1 from issue_assignees where assignee_id = ? and issue_id = issues.id)", group)
			}
			if group != "" {
				var u dao.User
				if err := db.Where("id = ?", group).First(&u).Error; err != nil {
					return 0, nil, err
				}
				entity = u.ToLightDTO()
			}
		case "watchers":
			if len(searchParams.Filters.WatcherIds) > 0 && !slices.Contains(searchParams.Filters.WatcherIds, group) {
				continue
			}
			if group == "" {
				q = q.Where("not exists (select 1 from issue_watchers where issue_id = issues.id)")
			} else {
				q = q.Where("exists (select 1 from issue_watchers where watcher_id = ? and issue_id = issues.id)", group)
			}
			if group != "" {
				var u dao.User
				if err := db.Where("id = ?", group).First(&u).Error; err != nil {
					return 0, nil, err
				}
				entity = u.ToLightDTO()
			}
		}

		if size == 0 {
			groupMap[group] = IssuesGroupResponse{
				Entity: entity,
				Count:  size,
			}
			continue
		}

		var issues []dao.IssueWithCount
		if err := q.Find(&issues).Error; err != nil {
			return 0, nil, err
		}

		switch searchParams.GroupByParam {
		case "author":
			entity = issues[0].Author.ToLightDTO()
		case "state":
			entity = issues[0].State.ToLightDTO()
		}

		groupMap[group] = IssuesGroupResponse{
			Entity: entity,
			Count:  size,
			Issues: utils.SliceToSlice(&issues, func(i *dao.IssueWithCount) *dto.IssueWithCount { return i.ToDTO() }),
		}
	}
	return totalCount, groupMap, nil
}

func SortIssuesGroups(groupByParam string, issuesGroups map[string]IssuesGroupResponse) []IssuesGroupResponse {
	return slices.SortedFunc(maps.Values(issuesGroups), func(e1, e2 IssuesGroupResponse) int {
		switch groupByParam {
		case "priority":
			entity1, _ := e1.Entity.(string) // use _ for nil transform into empty string
			entity2, _ := e2.Entity.(string)
			return utils.PrioritiesSortValues[entity1] - utils.PrioritiesSortValues[entity2]
		case "author":
			entity1 := e1.Entity.(*dto.UserLight)
			entity2 := e2.Entity.(*dto.UserLight)
			return utils.CompareUsers(entity1, entity2)
		case "state":
			entity1, _ := e1.Entity.(*dto.StateLight)
			entity2, _ := e2.Entity.(*dto.StateLight)

			groupOrder1 := getStateGroupOrder(entity1)
			groupOrder2 := getStateGroupOrder(entity2)

			if groupOrder1 == groupOrder2 {
				// Compare state names
				if entity1 == nil || (entity2 != nil && entity1.Name > entity2.Name) {
					return 1
				}
				return -1
			}
			if groupOrder1 > groupOrder2 {
				return 1
			}
			return -1
		case "labels":
			entity1, _ := e1.Entity.(*dto.LabelLight)
			entity2, _ := e2.Entity.(*dto.LabelLight)
			if entity1 == entity2 {
				return 0
			}
			if entity1 == nil || (entity2 != nil && entity1.Name > entity2.Name) {
				return 1
			} else {
				return -1
			}
		case "assignees":
			entity1, _ := e1.Entity.(*dto.UserLight)
			entity2, _ := e2.Entity.(*dto.UserLight)
			return utils.CompareUsers(entity1, entity2)
		case "watchers":
			entity1, _ := e1.Entity.(*dto.UserLight)
			entity2, _ := e2.Entity.(*dto.UserLight)
			return utils.CompareUsers(entity1, entity2)
		}
		return 0
	})
}

func getStateGroupOrder(state *dto.StateLight) int {
	if state == nil {
		return 0
	}
	switch state.Group {
	case "backlog":
		return 1
	case "unstarted":
		return 2
	case "started":
		return 3
	case "completed":
		return 4
	case "cancelled":
		return 5
	}
	return 0
}
