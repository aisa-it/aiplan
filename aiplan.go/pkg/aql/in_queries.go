package aql

import (
	"database/sql"
	"strconv"
	"strings"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/dao"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/utils"
	"github.com/antlr4-go/antlr/v4"
	"github.com/gofrs/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

// inFunc — условие «поле IN (values)». Через него же работают = и !=.
type inFunc func(v *Visitor, values []any) (*gorm.DB, error)

var inFieldsFunc = map[string]inFunc{
	"project":     refIn("issues.project_id", &dao.Project{}, "identifier"),
	"workspace":   refIn("issues.workspace_id", &dao.Workspace{}, "slug"),
	"issue":       issueIn("issues.id IN (@ids)"),
	"parent":      issueIn("issues.parent_id IN (@ids)"),
	"links":       issueIn("issues.id IN (SELECT id2 FROM linked_issues WHERE id1 IN (@ids) UNION ALL SELECT id1 FROM linked_issues WHERE id2 IN (@ids))"),
	"blocks":      issueIn("issues.id IN (SELECT blocked_by_id FROM issue_blockers WHERE deleted_at IS NULL AND block_id IN (@ids))"),
	"blockedby":   issueIn("issues.id IN (SELECT block_id FROM issue_blockers WHERE deleted_at IS NULL AND blocked_by_id IN (@ids))"),
	"status":      refIn("issues.state_id", &dao.State{}, "name"),
	"statusgroup": refIn("issues.state_id", &dao.State{}, `"group"`),
	"type":        refIn("issues.issue_type_id", &dao.IssueType{}, "name"),
	"priority":    valueIn("issues.priority"),
	"labels":      relationIn(&dao.IssueLabel{}, "label_id", &dao.Label{}, "name"),
	"sprint":      relationIn(&dao.SprintIssue{}, "sprint_id", &dao.Sprint{}, "name"),

	"author":   usersRefIn("issues.created_by_id"),
	"assignee": usersRelationIn(&dao.IssueAssignee{}, "assignee_id"),
	"watcher":  usersRelationIn(&dao.IssueWatcher{}, "watcher_id"),
}

// inQuery — EMPTY в значениях (nil) добавляет через OR условие «поле пустое».
func (v *Visitor) inQuery(f fieldFuncs, values []any, negate bool, valuesCtx antlr.ParserRuleContext) *gorm.DB {
	var filled []any
	withEmpty := false
	for _, value := range values {
		if value == nil {
			withEmpty = true
		} else {
			filled = append(filled, value)
		}
	}

	cond := v.newQuery()
	if len(filled) > 0 {
		in, err := f.in(v, filled)
		if err != nil {
			panic(at(err, valuesCtx))
		}
		cond = cond.Or(in)
	}
	if withEmpty {
		if f.empty == nil {
			panic(at(ErrEmptyUnsupported, valuesCtx))
		}
		cond = cond.Or(f.empty(v, false))
	}
	if negate {
		return v.newQuery().Not(cond)
	}
	return cond
}

// valueIn — значения хранятся в самой колонке задачи.
func valueIn(column string) inFunc {
	return func(v *Visitor, values []any) (*gorm.DB, error) {
		names, err := lowerStrings(values)
		if err != nil {
			return nil, err
		}
		return v.newQuery().Where(column+" IN (?)", names), nil
	}
}

// refIn — колонка задачи ссылается на запись model, которую ищем по id или имени.
func refIn(column string, model any, nameColumn string) inFunc {
	return func(v *Visitor, values []any) (*gorm.DB, error) {
		ids, err := refIds(v, values, model, nameColumn)
		if err != nil {
			return nil, err
		}
		return v.newQuery().Where(column+" IN (?)", ids), nil
	}
}

func usersRefIn(column string) inFunc {
	return func(v *Visitor, values []any) (*gorm.DB, error) {
		ids, err := refUsers(v, values)
		if err != nil {
			return nil, err
		}
		return v.newQuery().Where(column+" IN (?)", ids), nil
	}
}

// relationIn — задача связана с записями model через таблицу relation.
// EXISTS, а не issues.id IN (…): под NOT и OR он остаётся поиском по индексу issue_id.
func relationIn(relation any, relationColumn string, model any, nameColumn string) inFunc {
	return func(v *Visitor, values []any) (*gorm.DB, error) {
		ids, err := refIds(v, values, model, nameColumn)
		if err != nil {
			return nil, err
		}
		rows := v.newQuery().Model(relation).Select("1").Where("issue_id = issues.id").Where(relationColumn+" IN (?)", ids)
		return v.newQuery().Where("EXISTS (?)", rows), nil
	}
}

func usersRelationIn(relation any, relationColumn string) inFunc {
	return func(v *Visitor, values []any) (*gorm.DB, error) {
		ids, err := refUsers(v, values)
		if err != nil {
			return nil, err
		}
		rows := v.newQuery().Model(relation).Select("1").Where("issue_id = issues.id").Where(relationColumn+" IN (?)", ids)
		return v.newQuery().Where("EXISTS (?)", rows), nil
	}
}

// issueIn — значения: ключи задач или подзапросы списочных функций; @ids в where — подзапрос их id.
func issueIn(where string) inFunc {
	return func(v *Visitor, values []any) (*gorm.DB, error) {
		cond := v.newQuery()
		for _, value := range values {
			ids, ok := value.(*gorm.DB)
			if !ok {
				var err error
				if ids, err = v.issueRef(value); err != nil {
					return nil, err
				}
			}
			cond = cond.Or(where, sql.Named("ids", ids))
		}
		return cond, nil
	}
}

func refIds(v *Visitor, values []any, model any, nameColumn string) (*gorm.DB, error) {
	names, err := lowerStrings(values)
	if err != nil {
		return nil, err
	}
	var ids []uuid.UUID
	for _, name := range names {
		if id, err := uuid.FromString(name); err == nil {
			ids = append(ids, id)
		}
	}
	return v.newQuery().Model(model).Select("id").Where("id IN (?) OR LOWER("+nameColumn+") IN (?)", ids, names), nil
}

func refUsers(v *Visitor, values []any) (*gorm.DB, error) {
	names, err := lowerStrings(values)
	if err != nil {
		return nil, err
	}
	var ids []uuid.UUID
	for _, name := range names {
		if id, err := uuid.FromString(name); err == nil {
			ids = append(ids, id)
		}
	}

	likeNames := make([]string, 0, len(names))
	for _, name := range names {
		likeNames = append(likeNames, utils.PrepareSearchRequest(name))
	}
	return v.newQuery().Model(&dao.User{}).Select("id").Where("id IN (?) OR LOWER(username) IN (?) OR LOWER(email) in (?) OR LOWER(last_name) LIKE ANY(?) OR LOWER(first_name) LIKE ANY(?) OR LOWER(first_name || ' ' || last_name) LIKE ANY(?) OR LOWER(last_name || ' ' || first_name) LIKE ANY(?)",
		ids,
		names,
		names,
		pq.Array(likeNames),
		pq.Array(likeNames),
		pq.Array(likeNames),
		pq.Array(likeNames)), nil
}

func lowerStrings(values []any) ([]string, error) {
	res := make([]string, 0, len(values))
	for _, value := range values {
		switch val := value.(type) {
		case string:
			res = append(res, strings.ToLower(val))
		case int:
			res = append(res, strconv.Itoa(val))
		default:
			return nil, ErrValue
		}
	}
	return res, nil
}
