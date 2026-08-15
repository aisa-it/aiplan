// JQL-поиск задач: POST /api/auth/issues/jql
// Текстовый запрос компилируется в IssuesListFilters и исполняется через
// штатный поисковый движок. Негативные условия (NOT, !=, NOT IN) применяются
// пост-фильтрацией по уже загруженным задачам, OR-группы — несколькими поисками
// с последующим объединением результатов.
package aiplan

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gofrs/uuid"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"

	apicontext "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/api-context"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dto"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/jql"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/search"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
)

const jqlMaxGroups = 5
const jqlGroupLimit = 100

type jqlSearchRequest struct {
	Query  string `json:"query"`
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
}

// jqlPredicate — негативное условие для пост-фильтрации.
type jqlPredicate struct {
	Clause jql.Clause
	IDs    []uuid.UUID // разрешённые ID (users/states/labels/sprints/projects)
	Names  []string    // имена состояний/меток/спринтов
}

func clauseValues(cl jql.Clause) []string {
	if len(cl.Value.List) > 0 {
		return cl.Value.List
	}
	if cl.Value.Str != "" {
		return []string{cl.Value.Str}
	}
	return nil
}

func lowerAll(vals []string) []string {
	out := make([]string, 0, len(vals))
	for _, v := range vals {
		out = append(out, strings.ToLower(strings.TrimSpace(v)))
	}
	return out
}

func resolveStates(db *gorm.DB, vals []string, contains bool) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	q := db.Model(&dao.State{})
	if contains {
		likes := make([]string, 0, len(vals))
		for _, v := range vals {
			likes = append(likes, "%"+strings.ToLower(v)+"%")
		}
		ors := make([]string, 0, len(likes))
		args := make([]any, 0, len(likes))
		for _, l := range likes {
			ors = append(ors, "lower(name) LIKE ?")
			args = append(args, l)
		}
		q = q.Where(strings.Join(ors, " OR "), args...)
	} else {
		q = q.Where("lower(name) IN ?", lowerAll(vals))
	}
	if err := q.Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

func resolveProjects(db *gorm.DB, vals []string, contains bool) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	q := db.Model(&dao.Project{})
	if contains {
		likes := make([]string, 0, len(vals))
		for _, v := range vals {
			likes = append(likes, "%"+strings.ToLower(v)+"%")
		}
		ors := make([]string, 0, len(likes))
		args := make([]any, 0, len(likes)*2)
		for _, l := range likes {
			ors = append(ors, "(lower(identifier) LIKE ? OR lower(name) LIKE ?)")
			args = append(args, l, l)
		}
		q = q.Where(strings.Join(ors, " OR "), args...)
	} else {
		lv := lowerAll(vals)
		q = q.Where("lower(identifier) IN ? OR lower(name) IN ?", lv, lv)
	}
	if err := q.Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

func resolveUsers(db *gorm.DB, vals []string, contains bool) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	q := db.Model(&dao.User{})
	if contains {
		likes := make([]string, 0, len(vals))
		for _, v := range vals {
			likes = append(likes, "%"+strings.ToLower(v)+"%")
		}
		ors := make([]string, 0, len(likes))
		args := make([]any, 0, len(likes)*3)
		for _, l := range likes {
			ors = append(ors, "(lower(email) LIKE ? OR lower(coalesce(username,'')) LIKE ? OR lower(concat(first_name,' ',last_name)) LIKE ?)")
			args = append(args, l, l, l)
		}
		q = q.Where(strings.Join(ors, " OR "), args...)
	} else {
		lv := lowerAll(vals)
		q = q.Where("lower(email) IN ? OR lower(coalesce(username,'')) IN ? OR lower(concat(first_name,' ',last_name)) IN ?", lv, lv, lv)
	}
	if err := q.Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

func resolveLabels(db *gorm.DB, vals []string, contains bool) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	q := db.Model(&dao.Label{})
	if contains {
		likes := make([]string, 0, len(vals))
		for _, v := range vals {
			likes = append(likes, "%"+strings.ToLower(v)+"%")
		}
		ors := make([]string, 0, len(likes))
		args := make([]any, 0, len(likes))
		for _, l := range likes {
			ors = append(ors, "lower(name) LIKE ?")
			args = append(args, l)
		}
		q = q.Where(strings.Join(ors, " OR "), args...)
	} else {
		q = q.Where("lower(name) IN ?", lowerAll(vals))
	}
	if err := q.Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

func resolveSprints(db *gorm.DB, vals []string, contains bool) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	q := db.Model(&dao.Sprint{})
	if contains {
		likes := make([]string, 0, len(vals))
		for _, v := range vals {
			likes = append(likes, "%"+strings.ToLower(v)+"%")
		}
		ors := make([]string, 0, len(likes))
		args := make([]any, 0, len(likes))
		for _, l := range likes {
			ors = append(ors, "lower(name) LIKE ?")
			args = append(args, l)
		}
		q = q.Where(strings.Join(ors, " OR "), args...)
	} else {
		q = q.Where("lower(name) IN ?", lowerAll(vals))
	}
	if err := q.Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

func clauseDate(cl jql.Clause) (time.Time, bool) {
	if cl.Value.HasDate {
		return cl.Value.Date, true
	}
	if cl.Value.HasRel {
		return time.Now().AddDate(0, 0, cl.Value.RelDays), true
	}
	return time.Time{}, false
}

func dayStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func jsonTime(t time.Time) types.JSONTime {
	return types.JSONTime(t)
}

func applyDateClause(f *types.IssuesListFilters, cl jql.Clause) error {
	base, ok := clauseDate(cl)
	if !ok {
		return fmt.Errorf("поле %q требует дату", cl.Field)
	}
	day := dayStart(base)
	next := day.AddDate(0, 0, 1)
	var from, to types.JSONTime
	switch cl.Op {
	case jql.OpGt:
		from = jsonTime(next)
	case jql.OpGte:
		from = jsonTime(day)
	case jql.OpLt:
		to = jsonTime(day)
	case jql.OpLte:
		to = jsonTime(next)
	case jql.OpEq:
		from = jsonTime(day)
		to = jsonTime(next)
	default:
		return fmt.Errorf("оператор %s не поддерживается для поля %q", cl.Op, cl.Field)
	}
	switch cl.Field {
	case "created_at":
		f.CreatedAtFrom, f.CreatedAtTo = from, to
	case "updated_at":
		f.UpdatedAtFrom, f.UpdatedAtTo = from, to
	case "start_date":
		f.StartDateFrom, f.StartDateTo = from, to
	case "target_date":
		f.TargetDateFrom, f.TargetDateTo = from, to
	}
	return nil
}

// resolveClause разрешает значения условия в ID (для id-полей).
func (s *Services) resolveClause(db *gorm.DB, user *dao.User, cl jql.Clause) ([]uuid.UUID, []string, error) {
	vals := clauseValues(cl)
	contains := cl.Op == jql.OpContains

	hasCurrentUser := false
	for _, v := range vals {
		if strings.EqualFold(v, "currentUser()") {
			hasCurrentUser = true
		}
	}
	filtered := make([]string, 0, len(vals))
	for _, v := range vals {
		if !strings.EqualFold(v, "currentUser()") {
			filtered = append(filtered, v)
		}
	}

	var ids []uuid.UUID
	var err error
	switch cl.Field {
	case "project":
		ids, err = resolveProjects(db, filtered, contains)
	case "state":
		ids, err = resolveStates(db, filtered, contains)
	case "assignee", "author", "watcher":
		ids, err = resolveUsers(db, filtered, contains)
	case "label":
		ids, err = resolveLabels(db, filtered, contains)
	case "sprint":
		ids, err = resolveSprints(db, filtered, contains)
	default:
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	if hasCurrentUser {
		ids = append(ids, user.ID)
	}
	return ids, nil, nil
}

// compileJQLGroup собирает фильтры и негативные предикаты для одной OR-группы.
func (s *Services) compileJQLGroup(db *gorm.DB, user *dao.User, group jql.Group) (types.IssuesListFilters, []jqlPredicate, bool, error) {
	var f types.IssuesListFilters
	var predicates []jqlPredicate

	for _, cl := range group.Clauses {
		// IS EMPTY (позитивный): assignee/watcher — IncludeEmpty,
		// остальные поля — пост-фильтрация
		if cl.Op == jql.OpIsEmpty && !cl.Negated {
			switch cl.Field {
			case "assignee":
				f.AssigneeIds = types.FilterUUIDs{IncludeEmpty: true}
			case "watcher":
				f.WatcherIds = types.FilterUUIDs{IncludeEmpty: true}
			default:
				predicates = append(predicates, jqlPredicate{Clause: cl})
			}
			continue
		}

		// Негативные условия — пост-фильтрация
		if cl.Negated || cl.Op == jql.OpNeq || cl.Op == jql.OpNotIn || cl.Op == jql.OpIsNotEmpty {
			ids, names, err := s.resolveClause(db, user, cl)
			if err != nil {
				return f, nil, false, err
			}
			predicates = append(predicates, jqlPredicate{Clause: cl, IDs: ids, Names: names})
			continue
		}

		switch cl.Field {
		case "text":
			for _, v := range clauseValues(cl) {
				f.SearchQuery = strings.TrimSpace(f.SearchQuery + " " + v)
			}
		case "priority":
			f.Priorities = append(f.Priorities, clauseValues(cl)...)
		case "project":
			ids, _, err := s.resolveClause(db, user, cl)
			if err != nil {
				return f, nil, false, err
			}
			if len(clauseValues(cl)) > 0 && len(ids) == 0 {
				return f, nil, true, nil
			}
			for _, id := range ids {
				f.ProjectIds = append(f.ProjectIds, id.String())
			}
		case "state":
			ids, _, err := s.resolveClause(db, user, cl)
			if err != nil {
				return f, nil, false, err
			}
			if len(clauseValues(cl)) > 0 && len(ids) == 0 {
				return f, nil, true, nil
			}
			f.StateIds = append(f.StateIds, ids...)
		case "assignee":
			ids, _, err := s.resolveClause(db, user, cl)
			if err != nil {
				return f, nil, false, err
			}
			if len(clauseValues(cl)) > 0 && len(ids) == 0 {
				return f, nil, true, nil
			}
			f.AssigneeIds.Array = append(f.AssigneeIds.Array, ids...)
		case "author":
			ids, _, err := s.resolveClause(db, user, cl)
			if err != nil {
				return f, nil, false, err
			}
			if len(clauseValues(cl)) > 0 && len(ids) == 0 {
				return f, nil, true, nil
			}
			f.AuthorIds = append(f.AuthorIds, toStrings(ids)...)
		case "watcher":
			ids, _, err := s.resolveClause(db, user, cl)
			if err != nil {
				return f, nil, false, err
			}
			if len(clauseValues(cl)) > 0 && len(ids) == 0 {
				return f, nil, true, nil
			}
			f.WatcherIds.Array = append(f.WatcherIds.Array, ids...)
		case "label":
			ids, _, err := s.resolveClause(db, user, cl)
			if err != nil {
				return f, nil, false, err
			}
			if len(clauseValues(cl)) > 0 && len(ids) == 0 {
				return f, nil, true, nil
			}
			f.Labels.Array = append(f.Labels.Array, ids...)
		case "sprint":
			ids, _, err := s.resolveClause(db, user, cl)
			if err != nil {
				return f, nil, false, err
			}
			if len(clauseValues(cl)) > 0 && len(ids) == 0 {
				return f, nil, true, nil
			}
			for _, id := range ids {
				f.SprintIds = append(f.SprintIds, id.String())
			}
		case "created_at", "updated_at", "start_date", "target_date":
			if err := applyDateClause(&f, cl); err != nil {
				return f, nil, false, err
			}
		}
	}
	return f, predicates, false, nil
}

func toStrings(ids []uuid.UUID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, id.String())
	}
	return out
}

func issueDate(issue *dto.IssueWithCount, field string) (time.Time, bool) {
	switch field {
	case "created_at":
		return issue.CreatedAt, true
	case "updated_at":
		return issue.UpdatedAt, true
	case "start_date":
		if issue.StartDate == nil {
			return time.Time{}, false
		}
		return issue.StartDate.Time, true
	case "target_date":
		if issue.TargetDate == nil {
			return time.Time{}, false
		}
		return issue.TargetDate.Time, true
	}
	return time.Time{}, false
}

// predicateMatches проверяет, удовлетворяет ли задача негативному условию
// (если да — задача исключается из результатов).
func predicateMatches(p jqlPredicate, issue *dto.IssueWithCount, userID uuid.UUID) bool {
	cl := p.Clause
	inner := cl
	inner.Negated = false

	var matches bool
	switch inner.Field {
	case "state":
		matches = issue.State != nil && containsFold(p.Names, issue.State.Name)
	case "priority":
		vals := clauseValues(inner)
		matches = issue.Priority != nil && containsFold(vals, *issue.Priority)
	case "assignee":
		matches = anyIDIn(p.IDs, issue.Assignees)
	case "author":
		matches = issue.Author != nil && idIn(p.IDs, issue.Author.ID)
	case "watcher":
		matches = anyIDIn(p.IDs, issue.Watchers)
	case "label":
		matches = anyLabelIDIn(p.IDs, issue.Labels)
	case "sprint":
		matches = anySprintIDIn(p.IDs, issue.Sprints)
	case "project":
		matches = issue.Project != nil && idIn(p.IDs, issue.Project.ID)
	case "text":
		hay := strings.ToLower(issue.Name)
		if issue.DescriptionStripped != nil {
			hay += " " + strings.ToLower(*issue.DescriptionStripped)
		}
		for _, v := range clauseValues(inner) {
			if strings.Contains(hay, strings.ToLower(v)) {
				matches = true
				break
			}
		}
	case "created_at", "updated_at", "start_date", "target_date":
		base, ok := clauseDate(inner)
		it, ok2 := issueDate(issue, inner.Field)
		if ok && ok2 {
			day := dayStart(base)
			switch inner.Op {
			case jql.OpGt:
				matches = it.After(day.AddDate(0, 0, 1).Add(-time.Nanosecond))
			case jql.OpGte:
				matches = !it.Before(day)
			case jql.OpLt:
				matches = it.Before(day)
			case jql.OpLte:
				matches = it.Before(day.AddDate(0, 0, 1))
			case jql.OpEq:
				matches = !it.Before(day) && it.Before(day.AddDate(0, 0, 1))
			case jql.OpNeq:
				matches = it.Before(day) || !it.Before(day.AddDate(0, 0, 1))
			}
		}
	}

	if cl.Negated {
		return matches
	}
	if cl.Op == jql.OpNeq || cl.Op == jql.OpNotIn {
		return matches
	}
	// IS EMPTY / IS NOT EMPTY: нарушение условия = исключение из результата
	if cl.Op == jql.OpIsEmpty {
		switch cl.Field {
		case "assignee":
			return len(issue.Assignees) > 0
		case "watcher":
			return len(issue.Watchers) > 0
		case "label":
			return len(issue.Labels) > 0
		case "sprint":
			return len(issue.Sprints) > 0
		case "priority":
			return issue.Priority != nil
		case "created_at", "updated_at", "start_date", "target_date":
			_, ok := issueDate(issue, cl.Field)
			return ok
		}
	}
	if cl.Op == jql.OpIsNotEmpty {
		switch cl.Field {
		case "assignee":
			return len(issue.Assignees) == 0
		case "watcher":
			return len(issue.Watchers) == 0
		case "label":
			return len(issue.Labels) == 0
		case "sprint":
			return len(issue.Sprints) == 0
		case "priority":
			return issue.Priority == nil
		case "created_at", "updated_at", "start_date", "target_date":
			_, ok := issueDate(issue, cl.Field)
			return !ok
		}
	}
	return matches
}

func containsFold(list []string, v string) bool {
	for _, item := range list {
		if strings.EqualFold(strings.TrimSpace(item), strings.TrimSpace(v)) {
			return true
		}
	}
	return false
}

func idIn(ids []uuid.UUID, id uuid.UUID) bool {
	for _, i := range ids {
		if i == id {
			return true
		}
	}
	return false
}

func anyIDIn(ids []uuid.UUID, users []dto.UserLight) bool {
	for _, u := range users {
		if idIn(ids, u.ID) {
			return true
		}
	}
	return false
}

func anyLabelIDIn(ids []uuid.UUID, labels []dto.LabelLight) bool {
	for _, l := range labels {
		if idIn(ids, l.ID) {
			return true
		}
	}
	return false
}

func anySprintIDIn(ids []uuid.UUID, sprints []dto.SprintLight) bool {
	for _, sp := range sprints {
		if idIn(ids, sp.Id) {
			return true
		}
	}
	return false
}

// sortJQLIssues сортирует объединённый список задач по полю из ORDER BY.
func sortJQLIssues(issues []dto.IssueWithCount, orderBy string, desc bool) {
	if orderBy == "" {
		return
	}
	sort.SliceStable(issues, func(a, b int) bool {
		less := false
		ia, ib := &issues[a], &issues[b]
		switch orderBy {
		case "sequence_id":
			less = ia.SequenceId < ib.SequenceId
		case "created_at":
			less = ia.CreatedAt.Before(ib.CreatedAt)
		case "updated_at":
			less = ia.UpdatedAt.Before(ib.UpdatedAt)
		case "name":
			less = strings.ToLower(ia.Name) < strings.ToLower(ib.Name)
		case "priority":
			pa, pb := ia.Priority, ib.Priority
			if pa == nil {
				less = pb != nil
			} else if pb == nil {
				less = false
			} else {
				less = *pa < *pb
			}
		case "target_date":
			da, db := ia.TargetDate, ib.TargetDate
			if da == nil {
				less = db != nil
			} else if db == nil {
				less = false
			} else {
				less = da.Time.Before(db.Time)
			}
		case "state":
			sa, sb := ia.State, ib.State
			na, nb := "", ""
			if sa != nil {
				na = sa.Name
			}
			if sb != nil {
				nb = sb.Name
			}
			less = strings.ToLower(na) < strings.ToLower(nb)
		default:
			less = ia.SequenceId < ib.SequenceId
		}
		if desc {
			return !less
		}
		return less
	})
}

// searchIssuesJQL godoc
// @id searchIssuesJQL
// @Summary Задачи: поиск задач по JQL-запросу
// @Description Выполняет поиск задач по текстовому запросу в стиле JQL. Пример: project = "PRJ" AND assignee = currentUser() AND updated > -7d ORDER BY priority DESC
// @Tags Issues
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param request body jqlSearchRequest true "JQL-запрос"
// @Success 200 {object} dto.IssuesSearchResponse "Результаты поиска"
// @Failure 400 {object} apierrors.DefinedError "Некорректный JQL-запрос"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Router /api/auth/issues/jql [post]
func (s *Services) searchIssuesJQL(c echo.Context) error {
	apiContext := apicontext.GetContext(c)
	user := apiContext.GetUser()

	var req jqlSearchRequest
	if err := c.Bind(&req); err != nil {
		return EErrorMsg(c, fmt.Errorf("JQL: некорректное тело запроса: %w", err))
	}
	if strings.TrimSpace(req.Query) == "" {
		return EErrorMsg(c, fmt.Errorf("JQL: запрос не может быть пустым"))
	}

	q, err := jql.Parse(req.Query)
	if err != nil {
		return EErrorMsg(c, fmt.Errorf("JQL: %w", err))
	}

	limit := req.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	db := s.DB(c)
	merged := make([]dto.IssueWithCount, 0, jqlGroupLimit)
	seen := map[uuid.UUID]bool{}

	orderBy := q.OrderBy
	if orderBy == "" {
		orderBy = "sequence_id"
	}

	for gi, group := range q.Groups {
		if gi >= jqlMaxGroups {
			break
		}
		filters, predicates, noMatch, err := s.compileJQLGroup(db, user, group)
		if err != nil {
			return EErrorMsg(c, err)
		}
		if noMatch {
			continue
		}

		sp := &types.SearchParams{
			Filters:      filters,
			Limit:        jqlGroupLimit,
			Offset:       0,
			OrderByParam: orderBy,
			Desc:         q.Desc,
		}
		res, err := search.GetIssueListData(db, *user, dao.ProjectMember{}, nil, true, sp, nil)
		if err != nil {
			return EError(c, err)
		}
		resp, ok := res.(dto.IssuesSearchResponse)
		if !ok {
			return EError(c, fmt.Errorf("JQL: неожиданный формат ответа поиска"))
		}

		for _, issue := range resp.Issues {
			if seen[issue.Id] {
				continue
			}
			excluded := false
			for _, p := range predicates {
				if predicateMatches(p, &issue, user.ID) {
					excluded = true
					break
				}
			}
			if excluded {
				continue
			}
			seen[issue.Id] = true
			merged = append(merged, issue)
		}
	}

	sortJQLIssues(merged, orderBy, q.Desc)

	total := len(merged)
	start := offset
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}

	return c.JSON(http.StatusOK, dto.IssuesSearchResponse{
		PaginationMeta: dto.PaginationMeta{
			Count:  total,
			Offset: offset,
			Limit:  limit,
		},
		Issues: merged[start:end],
	})
}
