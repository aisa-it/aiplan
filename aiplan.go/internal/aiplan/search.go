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

func FetchIssuesByGroups(
	groupSize map[string]int,
	db *gorm.DB, // Clean session cause gorm reset not working
	groupSelectQuery *gorm.DB,
	groupByParam string,
	filters types.IssuesListFilters,
) (int, map[string]IssuesGroupResponse, error) {
	groupMap := make(map[string]IssuesGroupResponse, len(groupSize))

	totalCount := 0

	for group, size := range groupSize {
		totalCount += size

		q := groupSelectQuery.Session(&gorm.Session{})

		var entity any
		switch groupByParam {
		case "priority":
			if len(filters.Priorities) > 0 && !slices.Contains(filters.Priorities, group) {
				continue
			}
			if group != "" {
				q = q.Where("issues.priority = ?", group)
			} else {
				q = q.Where("issues.priority is null")
			}
			entity = group
		case "author":
			if len(filters.AuthorIds) > 0 && !slices.Contains(filters.AuthorIds, group) {
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
			if len(filters.StateIds) > 0 && !slices.Contains(filters.StateIds, group) {
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
			if len(filters.Labels) > 0 && !slices.Contains(filters.Labels, group) {
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
			if len(filters.AssigneeIds) > 0 && !slices.Contains(filters.AssigneeIds, group) {
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
			if len(filters.WatcherIds) > 0 && !slices.Contains(filters.WatcherIds, group) {
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

		switch groupByParam {
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
