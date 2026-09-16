package defaultengine

import (
	"context"
	"strings"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/dao"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/engine"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/types"
)

// isIssueScopeAction сообщает, относится ли действие к конкретной задаче.
//
// Поиск, создание, массовое удаление и миграция задач сюда не входят: они
// выполняются до того, как задача известна, и решаются правилами проекта.
func isIssueScopeAction(a engine.Action) bool {
	switch a {
	case engine.ActionIssueSearch, engine.ActionIssueCreate, engine.ActionIssueBulkEdit, engine.ActionIssueMigrate:
		return false
	}
	return strings.HasPrefix(string(a), issueActionPrefix)
}

// issueActionPrefix — общее начало действий над задачей.
const issueActionPrefix = "issue."

// viewActions — действия, не изменяющие задачу.
//
// Раньше они определялись HTTP-методом (GET/HEAD/OPTIONS разрешались
// безусловно до загрузки сущностей). В терминах действий это набор
// «просмотровых»: читать задачу может любой участник проекта, роль и
// авторство не смотрятся.
var viewActions = map[engine.Action]struct{}{
	engine.ActionIssueView:           {},
	engine.ActionIssueViewActivity:   {},
	engine.ActionIssueCommentView:    {},
	engine.ActionIssueAttachmentView: {},
	engine.ActionIssueExport:         {},
}

// memberFreeActions — что участник проекта может делать с любой задачей,
// включая чужую: редактировать саму задачу и работать с комментариями.
var memberFreeActions = map[engine.Action]struct{}{
	engine.ActionIssueUpdate:        {},
	engine.ActionIssueDelete:        {},
	engine.ActionIssueCommentCreate: {},
	engine.ActionIssueCommentUpdate: {},
	engine.ActionIssueCommentDelete: {},
	engine.ActionIssueCommentReact:  {},

	// Поля, изменяемые вместе с задачей: раз участнику разрешено менять
	// саму задачу, ему разрешены и её поля. Движок заказчика может сузить
	// это по отдельным полям — ради этого они и разделены.
	engine.ActionIssueSetState:     {},
	engine.ActionIssueSetAssignees: {},
	engine.ActionIssueSetWatchers:  {},
	engine.ActionIssueSetLabels:    {},
	engine.ActionIssueSetParent:    {},
	engine.ActionIssueSetSprint:    {},
	engine.ActionIssueSetBlockers:  {},
	engine.ActionIssueSetLinked:    {},
	engine.ActionIssueSetSubIssues: {},
}

// Authorize решает, разрешено ли действие над задачей.
//
// Порядок проверок повторяет прежнее поведение трекера:
// просмотр открыт всем участникам проекта, автор и администратор
// пространства могут всё, администратор проекта — тоже, участнику
// доступны правка задачи и комментарии, к чужим задачам — только
// то, что разрешено настройками проекта.
func (e *Engine) Authorize(_ context.Context, req engine.AuthzRequest) (engine.Verdict, error) {
	// Правила по конкретной сущности точнее общих: если объект действия
	// известен, решение принимается по нему — в любой области.
	if v, ok := e.authorizeTarget(req); ok {
		return v, nil
	}

	if !isIssueScopeAction(req.Action) {
		return e.authorizeArea(req)
	}

	if _, ok := viewActions[req.Action]; ok {
		return engine.Allow, nil
	}

	s := req.Subject
	user, issue, err := issueSubject(s)
	if err != nil {
		return engine.Default, err
	}
	if user == nil || issue == nil {
		return engine.Deny, nil
	}

	// Автор своей задачи и администратор пространства не ограничены.
	if user.ID == issue.CreatedById || isWorkspaceAdmin(s) {
		return engine.Allow, nil
	}

	return e.authorizeByProjectRole(req.Action, s, issue.IsAssignee(user.ID))
}

// authorizeByProjectRole — решение по роли пользователя в проекте.
func (e *Engine) authorizeByProjectRole(
	action engine.Action,
	s engine.Subject,
	isAssignee bool,
) (engine.Verdict, error) {
	projectMember := s.ProjectMember()
	if projectMember == nil {
		return engine.Deny, nil
	}

	switch projectMember.Role {
	case types.AdminRole:
		return engine.Allow, nil
	case types.MemberRole:
		return e.authorizeMember(action, s, isAssignee)
	}

	// Гость: всё, кроме просмотра и собственных задач, запрещено.
	return engine.Deny, nil
}

// isWorkspaceAdmin сообщает, администрирует ли пользователь пространство.
func isWorkspaceAdmin(s engine.Subject) bool {
	wm := s.WorkspaceMember()
	return wm != nil && wm.Role == types.AdminRole
}

// issueSubject подгружает участников и задачу запроса.
// Порядок обращений повторяет прежний, чтобы ошибки загрузки
// возникали в тех же случаях, что и раньше.
func issueSubject(s engine.Subject) (*dao.User, *dao.Issue, error) {
	s.WorkspaceMember()
	s.ProjectMember()
	issue := s.Issue()
	if err := s.Err(); err != nil {
		return nil, nil, err
	}
	return s.User(), issue, nil
}

// authorizeMember — права участника проекта на чужую задачу.
func (e *Engine) authorizeMember(
	action engine.Action,
	s engine.Subject,
	isAssignee bool,
) (engine.Verdict, error) {
	if _, ok := memberFreeActions[action]; ok {
		return engine.Allow, nil
	}
	if isAssignee {
		return engine.Allow, nil
	}

	return memberProjectSettingAllows(action, s)
}

// memberProjectSettingAllows — настройки проекта, расширяющие права
// участника на чужие задачи. Проект грузится лениво: остальным веткам
// он не нужен.
func memberProjectSettingAllows(action engine.Action, s engine.Subject) (engine.Verdict, error) {
	var allowed bool

	switch action {
	case engine.ActionIssueAttachmentAdd:
		project := s.Project()
		if err := s.Err(); err != nil {
			return engine.Default, err
		}
		allowed = project != nil && project.MemberAttachmentsAllowed
	case engine.ActionIssueSetProperty:
		project := s.Project()
		if err := s.Err(); err != nil {
			return engine.Default, err
		}
		allowed = project != nil && project.MemberPropertiesAllowed
	}

	if allowed {
		return engine.Allow, nil
	}
	return engine.Deny, nil
}

var _ engine.Authorizer = (*Engine)(nil)
