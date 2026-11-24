package aql

import (
	"github.com/aisa-it/aiplan/aiplan.go/pkg/dao"
	"gorm.io/gorm/clause"
)

type sortFunc func(*Visitor) clause.OrderByColumn

var sortMap = map[string]sortFunc{
	"priority":            prioritySort,
	"author":              authorSort,
	"status":              stateSort,
	"labels":              labelsSort,
	"sub_issues_count":    columnRename("sub_issues_count"),
	"link_count":          columnRename("link_count"),
	"linked_issues_count": columnRename("linked_issues_count"),
	"attachment_count":    columnRename("attachment_count"),
	"assignee":            assigneesSort,
	"watcher":             watchersSort,

	"issue":     columnRename("sequence_id"),
	"title":     columnRename("name"),
	"created":   columnRename("created_at"),
	"updated":   columnRename("updated_at"),
	"completed": columnRename("completed_at"),
	"startdate": columnRename("start_date"),
	"duedate":   columnRename("target_date"),
}

func (v *Visitor) parseSortOrder(orderBy string) (clause.OrderByColumn, error) {
	fn, ok := sortMap[canonicalField(orderBy)]
	if !ok {
		return clause.OrderByColumn{}, ErrUnsupportedSort
	}
	return fn(v), nil
}

func prioritySort(v *Visitor) clause.OrderByColumn {
	sql := "case when priority='urgent' then 5 when priority='high' then 4 when priority='medium' then 3 when priority='low' then 2 when priority is null then 1 end"
	return clause.OrderByColumn{
		Column: clause.Column{Name: sql, Raw: true},
	}
}

func authorSort(v *Visitor) clause.OrderByColumn {
	v.selectExprs = append(v.selectExprs, "(?) as author_sort")
	v.selectInterface = append(v.selectInterface, v.newQuery().Select("COALESCE(NULLIF(last_name,''), email)").Where("id = issues.created_by_id").Model(&dao.User{}))
	return clause.OrderByColumn{
		Column: clause.Column{Name: "author_sort"},
	}
}

func stateSort(v *Visitor) clause.OrderByColumn {
	v.selectExprs = append(v.selectExprs, "(?) as state_sort")
	v.selectInterface = append(v.selectInterface, v.newQuery().Select(`concat(case "group" when 'backlog' then 1 when 'unstarted' then 2 when 'started' then 3 when 'completed' then 4 when 'cancelled' then 5 end, sequence, name, color)`).Where("id = issues.state_id").Model(&dao.State{}))
	return clause.OrderByColumn{
		Column: clause.Column{Name: "state_sort"},
	}
}

func labelsSort(v *Visitor) clause.OrderByColumn {
	v.selectExprs = append(v.selectExprs, "array(?) as labels_sort")
	v.selectInterface = append(v.selectInterface, v.newQuery().Select("name").Where("id in (?)", v.newQuery().Select("label_id").Where("issue_id = issues.id").Model(&dao.IssueLabel{})).Model(&dao.Label{}))
	return clause.OrderByColumn{
		Column: clause.Column{Name: "labels_sort"},
	}
}

func assigneesSort(v *Visitor) clause.OrderByColumn {
	v.selectExprs = append(v.selectExprs, "array(?) as assignees_sort")
	v.selectInterface = append(v.selectInterface, v.newQuery().Select("COALESCE(NULLIF(last_name,''), email)").Where("users.id in (?)", v.newQuery().Select("assignee_id").Where("issue_id = issues.id").Model(&dao.IssueAssignee{})).Model(&dao.User{}))
	return clause.OrderByColumn{
		Column: clause.Column{Name: "assignees_sort"},
	}
}

func watchersSort(v *Visitor) clause.OrderByColumn {
	v.selectExprs = append(v.selectExprs, "array(?) as watchers_sort")
	v.selectInterface = append(v.selectInterface, v.newQuery().Select("COALESCE(NULLIF(last_name,''), email)").Where("users.id in (?)", v.newQuery().Select("watcher_id").Where("issue_id = issues.id").Model(&dao.IssueWatcher{})).Model(&dao.User{}))
	return clause.OrderByColumn{
		Column: clause.Column{Name: "watchers_sort"},
	}
}

func columnRename(column string) sortFunc {
	return func(v *Visitor) clause.OrderByColumn {
		return clause.OrderByColumn{
			Column: clause.Column{Name: column},
		}
	}
}
