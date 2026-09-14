package defaultengine

import (
	"github.com/aisa-it/aiplan/aiplan.go/pkg/dao"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/engine"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/types"
)

// authorizeTarget — правила, которые нельзя проверить до загрузки объекта
// действия: они смотрят на саму сущность, а не только на роль и задачу.
//
// Второе значение false — для этого действия правила по объекту нет,
// решение принимается обычным порядком.
func (e *Engine) authorizeTarget(req engine.AuthzRequest) (engine.Verdict, bool) {
	if req.Target == nil {
		return engine.Default, false
	}

	switch req.Action {
	case engine.ActionIssueCommentUpdate:
		comment, ok := engine.TargetAs[*dao.IssueComment](req)
		if !ok {
			return engine.Default, false
		}
		// Править комментарий может только его автор — роль не помогает.
		return verdict(isCommentAuthor(comment, req.Subject)), true

	case engine.ActionIssueCommentDelete:
		comment, ok := engine.TargetAs[*dao.IssueComment](req)
		if !ok {
			return engine.Default, false
		}
		// Удалить — автор либо администратор проекта.
		return verdict(isCommentAuthor(comment, req.Subject) || isProjectAdmin(req.Subject)), true

	case engine.ActionIssueDelete:
		issue, ok := engine.TargetAs[*dao.Issue](req)
		if !ok {
			return engine.Default, false
		}
		return e.authorizeIssueDelete(issue, req.Subject), true
	}

	return engine.Default, false
}

// authorizeIssueDelete — удалять задачи может администратор проекта;
// автор — только свои и только если это разрешено настройками проекта.
func (e *Engine) authorizeIssueDelete(issue *dao.Issue, s engine.Subject) engine.Verdict {
	if isProjectAdmin(s) {
		return engine.Allow
	}

	user := s.User()
	if user == nil || issue.CreatedById != user.ID {
		return engine.Deny
	}

	project := s.Project()
	if project == nil {
		return engine.Deny
	}
	return verdict(project.IssueDeletionAllowed)
}

// isCommentAuthor сообщает, оставил ли комментарий текущий пользователь.
func isCommentAuthor(comment *dao.IssueComment, s engine.Subject) bool {
	user := s.User()
	if user == nil || comment == nil {
		return false
	}
	return comment.ActorId.Valid && comment.ActorId.UUID == user.ID
}

// isProjectAdmin сообщает, администрирует ли пользователь проект.
func isProjectAdmin(s engine.Subject) bool {
	pm := s.ProjectMember()
	return pm != nil && pm.Role == types.AdminRole
}

// verdict переводит булево решение в вердикт.
func verdict(allowed bool) engine.Verdict {
	if allowed {
		return engine.Allow
	}
	return engine.Deny
}
