package defaultengine

import (
	"context"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/dao"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/engine"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

// Видимость задач по членству в проектах.
//
// Правило одно: пользователь видит задачи проектов, где он состоит
// участником. VisibleProjects — его единственное выражение в SQL; ScopeIssues
// и подсчёт групп в поиске опираются на него и разойтись не могут.

// VisibleProjects — подзапрос project_id по членству пользователя.
func (e *Engine) VisibleProjects(_ context.Context, req engine.IssueScope, db *gorm.DB) *gorm.DB {
	q := db.Select("project_id").Model(&dao.ProjectMember{})
	user := req.Subject.User()
	if user == nil {
		return q.Where("1 = 0")
	}
	return q.Where("member_id = ?", user.ID)
}

// ScopeIssues ограничивает выборку задач. В проектном режиме — задачами
// проекта, членство в котором уже проверено при загрузке участника; иначе —
// проектами по VisibleProjects.
func (e *Engine) ScopeIssues(ctx context.Context, req engine.IssueScope, q *gorm.DB) *gorm.DB {
	if req.Kind == engine.ScopeProject {
		if pm := req.Subject.ProjectMember(); pm != nil && pm.ProjectId != uuid.Nil {
			return q.
				Where("issues.workspace_id = ?", pm.WorkspaceId).
				Where("issues.project_id = ?", pm.ProjectId)
		}
	}
	return q.Where("issues.project_id in (?)",
		e.VisibleProjects(ctx, req, q.Session(&gorm.Session{NewDB: true})))
}

// CanViewIssue разрешает просмотр участнику проекта задачи. Членство
// проверяется запросом: субъект может быть собран вне проектного контекста.
func (e *Engine) CanViewIssue(_ context.Context, s engine.Subject, issue *dao.Issue) (engine.Verdict, error) {
	user := s.User()
	if user == nil || issue == nil {
		return engine.Deny, nil
	}
	var member bool
	if err := s.DB().Model(&dao.ProjectMember{}).
		Select("count(*) > 0").
		Where("member_id = ? AND project_id = ?", user.ID, issue.ProjectId).
		Find(&member).Error; err != nil {
		return engine.Default, err
	}
	return verdict(member), nil
}

var _ engine.VisibilityPolicy = (*Engine)(nil)
