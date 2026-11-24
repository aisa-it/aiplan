package aql

import (
	"strconv"
	"strings"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/aql/parser"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/dao"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

// ScopeFunc ограничивает запрос по issues видимыми пользователю задачами; правила задаёт вызывающий.
type ScopeFunc func(q *gorm.DB) *gorm.DB

// NewVisitor: условия AQL добавляются к query, контекст и транзакция query переходят во все подзапросы.
func NewVisitor(query *gorm.DB, user *dao.User, scope ScopeFunc, templateScope dao.PropertyTemplateScope) *Visitor {
	return &Visitor{
		BaseAQLVisitor: &parser.BaseAQLVisitor{},
		query:          query,
		currentUser:    user,
		scopeIssues:    scope,
		scopeTemplates: templateScope,
		templatesCache: make(map[string][]dao.ProjectPropertyTemplate),
	}
}

// newQuery — чистый запрос для подзапросов и групп условий: без условий query, но с его контекстом.
func (v *Visitor) newQuery() *gorm.DB {
	return v.query.Session(&gorm.Session{NewDB: true})
}

// visibleIssues — подзапрос id видимых задач. Задачи из аргументов функций ищутся только среди них:
// скрытая задача неотличима от несуществующей.
func (v *Visitor) visibleIssues() *gorm.DB {
	q := v.newQuery().Model(&dao.Issue{}).Select("issues.id")
	if v.scopeIssues == nil {
		return q.Where("1 = 0")
	}
	return v.scopeIssues(q)
}

// visible — задача из column видима: проверка одной задачи по ключу, а не IN по всем видимым задачам.
func (v *Visitor) visible(column string) *gorm.DB {
	return v.newQuery().Where("EXISTS (SELECT 1 FROM (?) AS vis WHERE vis.id = "+column+")", v.visibleIssues())
}

// visibleTemplates — шаблоны параметров, видимые пользователю: скрытый параметр неотличим от несуществующего.
func (v *Visitor) visibleTemplates(q *gorm.DB) *gorm.DB {
	if v.scopeTemplates == nil {
		return q.Where("1 = 0")
	}
	return v.scopeTemplates(q)
}

// issueRef — подзапрос id видимой задачи по ключу PRJ-123 или id.
func (v *Visitor) issueRef(arg any) (*gorm.DB, error) {
	ref, ok := arg.(string)
	if !ok {
		return nil, ErrIssueRef
	}
	if id, err := uuid.FromString(ref); err == nil {
		return v.visibleIssues().Where("issues.id = ?", id), nil
	}

	dash := strings.LastIndex(ref, "-")
	if dash <= 0 {
		return nil, ErrIssueRef
	}
	seq, err := strconv.Atoi(ref[dash+1:])
	if err != nil || seq <= 0 {
		return nil, ErrIssueRef
	}
	projects := v.newQuery().Model(&dao.Project{}).Select("id").
		Where("identifier = ?", strings.ToUpper(ref[:dash]))
	return v.visibleIssues().Where("issues.sequence_id = ?", seq).Where("issues.project_id IN (?)", projects), nil
}
