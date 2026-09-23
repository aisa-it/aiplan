package defaultengine

import (
	"context"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/dao"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/engine"
	"gorm.io/gorm"
)

// Видимость шаблонов кастомных полей по роли в проекте.
//
// Шаблон виден, если роль пользователя в проекте шаблона не ниже reader_role.
// Роль берётся коррелированным подзапросом по членству, поэтому одно условие
// обслуживает и запрос по одному проекту, и списки задач из многих. Не
// участник проекта (роль 0) не видит ничего. Право менять значение — та же
// шкала, editor_role, — проверяется правилом по объекту в authorizeTarget.

// ScopePropertyTemplates ограничивает выборку шаблонов видимыми субъекту.
func (e *Engine) ScopePropertyTemplates(_ context.Context, s engine.Subject, q *gorm.DB) *gorm.DB {
	user := s.User()
	if user == nil {
		return q.Where("1 = 0")
	}
	role := q.Session(&gorm.Session{NewDB: true}).
		Model(&dao.ProjectMember{}).
		Select("role").
		Where("project_members.project_id = project_property_templates.project_id").
		Where("project_members.member_id = ?", user.ID)
	return q.Where("project_property_templates.reader_role <= COALESCE((?), 0)", role)
}

var _ engine.PropertyPolicy = (*Engine)(nil)
