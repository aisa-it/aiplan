package aql

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/dao"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/types"
	"gorm.io/gorm"
)

func ParseAQLQuery(db *gorm.DB, scope ScopeFunc, templateScope dao.PropertyTemplateScope, currentUser *dao.User, query string) (q *gorm.DB, err error) {
	if currentUser == nil {
		return nil, ErrNoUser
	}

	if len(query) > maxQuerySize {
		e := ErrQueryOverflow
		return nil, &e
	}

	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(Error); ok {
				err = &e
				return
			}
			panic(r)
		}
	}()

	tree, err := parseQuery(query)
	if err != nil {
		return nil, err
	}

	v := NewVisitor(db, currentUser, scope, templateScope)
	q = tree.Accept(v).(*gorm.DB)

	if !v.showDrafts {
		q = q.Where("issues.draft = false or issues.draft is null")
	}
	return
}

// CheckSyntax проверяет запрос по грамматике без визитора: к базе не обращается.
// Имена полей, функции и значения остаются непроверенными - их разбирает ParseAQLQuery.
func CheckSyntax(query string) error {
	if len(query) > maxQuerySize {
		e := ErrQueryOverflow
		return &e
	}

	_, err := parseQuery(query)
	return err
}

// ParseOldFilters переводит фильтры штатного поиска в запрос AQL.
// loc — часовой пояс пользователя: в нём AQL читает даты из запроса.
func ParseOldFilters(f types.IssuesListFilters, loc *time.Location) string {
	var conds []string
	add := func(ok bool, format string, args ...any) {
		if ok {
			conds = append(conds, fmt.Sprintf(format, args...))
		}
	}
	date := func(t types.JSONTime) string { return t.Time().In(loc).Format("2006-01-02 15:04") }

	add(len(f.AuthorIds) > 0, "author IN (%s)", list(f.AuthorIds, false))
	add(!f.AssigneeIds.IsEmpty(), "assignee IN (%s)", list(f.AssigneeIds.Array, f.AssigneeIds.IncludeEmpty))
	add(!f.WatcherIds.IsEmpty(), "watcher IN (%s)", list(f.WatcherIds.Array, f.WatcherIds.IncludeEmpty))
	add(len(f.StateIds) > 0, "status IN (%s)", list(f.StateIds, false))
	add(!f.IssueTypes.IsEmpty(), "type IN (%s)", list(f.IssueTypes.Array, f.IssueTypes.IncludeEmpty))
	add(len(f.Priorities) > 0, "priority IN (%s)", list(f.Priorities, false))
	add(!f.Labels.IsEmpty(), "labels IN (%s)", list(f.Labels.Array, f.Labels.IncludeEmpty))
	add(len(f.WorkspaceIds) > 0, "workspace IN (%s)", list(f.WorkspaceIds, false))
	add(len(f.WorkspaceSlugs) > 0, "workspace IN (%s)", list(f.WorkspaceSlugs, false))
	add(len(f.ProjectIds) > 0, "project IN (%s)", list(f.ProjectIds, false))
	add(len(f.SprintIds) > 0, "sprint IN (%s)", list(f.SprintIds, false))

	add(f.OnlyActive, "statusGroup NOT IN (completed, cancelled)")
	add(f.AssignedToMe, "assignee = currentUser()")
	add(f.WatchedByMe, "watcher = currentUser()")
	add(f.AuthoredByMe, "author = currentUser()")

	add(!f.CreatedAtFrom.IsNil(), "created >= %q", date(f.CreatedAtFrom))
	add(!f.CreatedAtTo.IsNil(), "created <= %q", date(f.CreatedAtTo))
	add(!f.UpdatedAtFrom.IsNil(), "updated >= %q", date(f.UpdatedAtFrom))
	add(!f.UpdatedAtTo.IsNil(), "updated <= %q", date(f.UpdatedAtTo))
	add(!f.StartDateFrom.IsNil(), "startDate >= %q", date(f.StartDateFrom))
	add(!f.StartDateTo.IsNil(), "startDate <= %q", date(f.StartDateTo))
	add(!f.TargetDateFrom.IsNil(), "dueDate >= %q", date(f.TargetDateFrom))
	add(!f.TargetDateTo.IsNil(), "dueDate <= %q", date(f.TargetDateTo))
	add(!f.CompletedAtFrom.IsNil(), "completed >= %q", date(f.CompletedAtFrom))
	add(!f.CompletedAtTo.IsNil(), "completed <= %q", date(f.CompletedAtTo))

	add(f.SearchQuery != "", "text ~ %q", f.SearchQuery)

	return strings.Join(conds, " AND ")
}

// list — значения в кавычках через запятую; пустое значение превращается в EMPTY.
func list[T any](values []T, withEmpty bool) string {
	items := make([]string, 0, len(values)+1)
	for _, v := range values {
		if s := fmt.Sprint(v); s != "" {
			items = append(items, strconv.Quote(s))
		} else {
			withEmpty = true
		}
	}
	if withEmpty {
		items = append(items, "EMPTY")
	}
	return strings.Join(items, ", ")
}
