package aql

import (
	"github.com/aisa-it/aiplan/aiplan.go/pkg/dao"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type emptyFunc func(v *Visitor, negate bool) *gorm.DB

// Ключи в нижнем регистре: поиск идёт по canonicalField.
var emptyFieldsFunc = map[string]emptyFunc{
	//"workspace":     nil,
	//"project":       nil,
	//"issue":         nil,
	//"title":         nil,
	"description": descriptionEmpty,
	//"status":        nil,
	//"statusgroup":   nil,
	"priority": columnEmpty("issues.priority"),
	"labels":   relationEmpty(&dao.IssueLabel{}),
	"assignee": relationEmpty(&dao.IssueAssignee{}),
	//"author":        nil,
	"watcher": relationEmpty(&dao.IssueWatcher{}),
	"parent":  parentEmpty,
	"sprint":  relationEmpty(&dao.SprintIssue{}),
	//"created":       nil,
	//"updated":       nil,
	"completed":     columnEmpty("issues.completed_at"),
	"startdate":     columnEmpty("issues.start_date"),
	"duedate":       columnEmpty("issues.target_date"),
	"estimate":      nil,
	"spent":         nil,
	"remaining":     nil,
	"worklogauthor": nil,
	"worklogdate":   nil,
	"worklogtime":   nil,
	"article":       nil,
	"links":         linksEmpty,
	"blocks":        blockersEmpty("blocked_by_id", "block_id"),
	"blockedby":     blockersEmpty("block_id", "blocked_by_id"),
	"attachments":   relationEmpty(&dao.IssueAttachment{}),
	//"comment":       nil,
}

// В description_html пустое описание хранится как <p></p>, поэтому смотрим текст.
func descriptionEmpty(v *Visitor, negate bool) *gorm.DB {
	if negate {
		return v.newQuery().Where("COALESCE(TRIM(issues.description_stripped), '') <> ''")
	}
	return v.newQuery().Where("COALESCE(TRIM(issues.description_stripped), '') = ''")
}

func columnEmpty(column string) emptyFunc {
	return func(v *Visitor, negate bool) *gorm.DB {
		if negate {
			return v.newQuery().Clauses(clause.Neq{Column: column, Value: nil})
		}
		return v.newQuery().Clauses(clause.Eq{Column: column, Value: nil})
	}
}

func relationEmpty(model any) emptyFunc {
	return func(v *Visitor, negate bool) *gorm.DB {
		rows := v.newQuery().Model(model).Select("1").Where("issue_id = issues.id")
		return exists(v, rows, negate)
	}
}

// Связи со скрытыми задачами не считаются.

func parentEmpty(v *Visitor, negate bool) *gorm.DB {
	if negate {
		return v.visible("issues.parent_id")
	}
	return v.newQuery().Not(v.visible("issues.parent_id"))
}

func blockersEmpty(column, other string) emptyFunc {
	return func(v *Visitor, negate bool) *gorm.DB {
		rows := v.newQuery().Model(&dao.IssueBlocker{}).Select("1").
			Where(column + " = issues.id").
			Where(v.visible(other))
		return exists(v, rows, negate)
	}
}

func linksEmpty(v *Visitor, negate bool) *gorm.DB {
	rows := v.newQuery().Model(&dao.LinkedIssues{}).Select("1").
		Where(v.newQuery().Where("id1 = issues.id").Where(v.visible("id2"))).
		Or(v.newQuery().Where("id2 = issues.id").Where(v.visible("id1")))
	return exists(v, rows, negate)
}

func exists(v *Visitor, rows *gorm.DB, negate bool) *gorm.DB {
	if negate {
		return v.newQuery().Where("EXISTS (?)", rows)
	}
	return v.newQuery().Where("NOT EXISTS (?)", rows)
}
