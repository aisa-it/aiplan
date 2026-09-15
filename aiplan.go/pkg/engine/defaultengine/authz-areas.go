package defaultengine

import (
	"slices"
	"strings"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/dao"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/engine"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/types"
	"github.com/gofrs/uuid"
)

// Права на проект, пространство, спринт, документ и форму.
//
// Правила воспроизводят прежние проверки по методу и пути роута, поэтому
// выражены ярусами: «любой участник» (прежние safe-методы и личные настройки),
// «участник», «управление» и «администратор». Просмотровые действия
// разрешаются без загрузки членства — так работали и прежние проверки.

type actionSet map[engine.Action]struct{}

func (s actionSet) has(a engine.Action) bool { _, ok := s[a]; return ok }

func newActionSet(actions ...engine.Action) actionSet {
	s := make(actionSet, len(actions))
	for _, a := range actions {
		s[a] = struct{}{}
	}
	return s
}

// Пространство.
var (
	workspaceAnyMember = newActionSet(
		engine.ActionWorkspaceView, engine.ActionWorkspaceActivity, engine.ActionWorkspaceMemberView,
		engine.ActionWorkspaceSelfSettings, engine.ActionWorkspaceBackupView,
		engine.ActionSprintList, engine.ActionDocList, engine.ActionDocCreateRoot, engine.ActionFormView,
	)
	// Токен интеграции читает администратор, владелец или суперпользователь.
	workspaceTokenView    = newActionSet(engine.ActionWorkspaceTokenView)
	workspaceAdminOrOwner = newActionSet(
		engine.ActionWorkspaceUpdate, engine.ActionWorkspaceDelete, engine.ActionWorkspaceInvite,
		engine.ActionWorkspaceMemberManage, engine.ActionWorkspaceTokenManage,
		engine.ActionWorkspaceIntegrationManage, engine.ActionWorkspaceBackup, engine.ActionWorkspaceImport,
		engine.ActionProjectCreate, engine.ActionProjectJoin,
		engine.ActionSprintCreate, engine.ActionSprintFolderManage,
	)
	// Формами управляет только администратор: владельцу пространства этого мало.
	workspaceAdminOnly = newActionSet(
		engine.ActionWorkspaceAdmin,
		engine.ActionFormCreate, engine.ActionFormUpdate, engine.ActionFormDelete, engine.ActionFormAnswerView,
	)
)

// Проект.
var (
	projectAnyMember = newActionSet(
		engine.ActionProjectView, engine.ActionProjectActivity, engine.ActionProjectMemberView,
		engine.ActionProjectStats, engine.ActionProjectDictionaryView, engine.ActionProjectSelfSettings,
		engine.ActionProjectViewManage, engine.ActionIssueSearch,
	)
	projectMember = newActionSet(engine.ActionIssueCreate)
	projectManage = newActionSet(
		engine.ActionProjectUpdate, engine.ActionProjectDelete, engine.ActionProjectMemberManage,
		engine.ActionProjectEstimateManage, engine.ActionProjectLabelManage, engine.ActionProjectStateManage,
		engine.ActionProjectRulesLogView, engine.ActionIssueBulkEdit,
	)
	projectAdminOnly = newActionSet(
		engine.ActionProjectAdmin, engine.ActionProjectArchive, engine.ActionProjectRulesManage,
		engine.ActionProjectTemplateManage, engine.ActionProjectPropertyManage,
		engine.ActionProjectDictionaryManage,
	)
)

// Спринт.
var (
	sprintRead   = newActionSet(engine.ActionSprintView, engine.ActionSprintActivity)
	sprintMember = newActionSet(engine.ActionSprintSearchIssues, engine.ActionSprintViewManage)
	sprintAdmin  = newActionSet(
		engine.ActionSprintAdmin, engine.ActionSprintUpdate, engine.ActionSprintDelete,
		engine.ActionSprintIssueManage, engine.ActionSprintWatchManage,
	)
)

// Документ. Комментировать может каждый, кто читает документ.
var (
	docRead = newActionSet(
		engine.ActionDocView, engine.ActionDocHistoryView, engine.ActionDocActivity,
		engine.ActionDocCommentView, engine.ActionDocCommentCreate, engine.ActionDocCommentUpdate,
		engine.ActionDocCommentDelete, engine.ActionDocCommentReact, engine.ActionDocAttachmentView,
	)
	docEdit = newActionSet(
		engine.ActionDocCreate, engine.ActionDocUpdate, engine.ActionDocDelete, engine.ActionDocMove,
		engine.ActionDocAttachmentAdd, engine.ActionDocAttachmentDel,
	)
)

// authorizeArea — решение по действию вне области задачи.
func (e *Engine) authorizeArea(req engine.AuthzRequest) (engine.Verdict, error) {
	a, s := req.Action, req.Subject
	switch {
	case workspaceAnyMember.has(a) || workspaceAdminOrOwner.has(a) || workspaceAdminOnly.has(a) || workspaceTokenView.has(a):
		return authorizeWorkspace(a, s)
	case strings.HasPrefix(string(a), "project.") || projectAnyMember.has(a) ||
		projectMember.has(a) || projectManage.has(a):
		return authorizeProject(a, s)
	case strings.HasPrefix(string(a), "sprint."):
		return authorizeSprint(a, s)
	case strings.HasPrefix(string(a), "doc."):
		return authorizeDoc(a, s)
	}
	return engine.Default, nil
}

func authorizeWorkspace(a engine.Action, s engine.Subject) (engine.Verdict, error) {
	if workspaceAnyMember.has(a) {
		return engine.Allow, nil
	}
	wm := s.WorkspaceMember()
	if workspaceAdminOnly.has(a) {
		if err := s.Err(); err != nil {
			return engine.Default, err
		}
		return verdict(wm != nil && wm.Role == types.AdminRole), nil
	}
	user, workspace := s.User(), s.Workspace()
	if err := s.Err(); err != nil {
		return engine.Default, err
	}
	if user == nil || workspace == nil {
		return engine.Deny, nil
	}
	adminOrOwner := (wm != nil && wm.Role == types.AdminRole) || workspace.OwnerId == user.ID
	if workspaceTokenView.has(a) {
		return verdict(adminOrOwner || user.IsSuperuser), nil
	}
	return verdict(adminOrOwner), nil
}

func authorizeProject(a engine.Action, s engine.Subject) (engine.Verdict, error) {
	if projectAnyMember.has(a) {
		return engine.Allow, nil
	}
	if !projectMember.has(a) && !projectManage.has(a) && !projectAdminOnly.has(a) {
		return engine.Default, nil
	}

	user, wm, project, pm := s.User(), s.WorkspaceMember(), s.Project(), s.ProjectMember()
	if err := s.Err(); err != nil {
		return engine.Default, err
	}
	if user == nil || wm == nil || project == nil || pm == nil {
		return engine.Deny, nil
	}

	switch {
	case projectAdminOnly.has(a):
		return verdict(pm.Role == types.AdminRole), nil
	case wm.Role == types.AdminRole:
		return engine.Allow, nil
	case projectMember.has(a):
		return verdict(pm.Role > types.GuestRole), nil
	default:
		return verdict(pm.Role == types.AdminRole || project.ProjectLeadId == user.ID), nil
	}
}

func authorizeSprint(a engine.Action, s engine.Subject) (engine.Verdict, error) {
	if !sprintRead.has(a) && !sprintMember.has(a) && !sprintAdmin.has(a) {
		return engine.Default, nil
	}
	wm, sprint, user := s.WorkspaceMember(), s.Sprint(), s.User()
	if err := s.Err(); err != nil {
		return engine.Default, err
	}
	if wm == nil || sprint == nil || user == nil {
		return engine.Deny, nil
	}

	switch {
	case sprintAdmin.has(a):
		return verdict(wm.Role == types.AdminRole), nil
	case sprintMember.has(a):
		return verdict(wm.Role > types.GuestRole), nil
	default:
		return verdict(user.ID == sprint.CreatedById || wm.Role == types.AdminRole ||
			user.IsSuperuser || wm.Role >= types.MemberRole), nil
	}
}

func authorizeDoc(a engine.Action, s engine.Subject) (engine.Verdict, error) {
	if !docRead.has(a) && !docEdit.has(a) && a != engine.ActionDocAccessManage {
		return engine.Default, nil
	}
	wm, doc, user := s.WorkspaceMember(), s.Doc(), s.User()
	if err := s.Err(); err != nil {
		return engine.Default, err
	}
	if wm == nil || doc == nil || user == nil {
		return engine.Deny, nil
	}

	// Доступом к документу распоряжаются автор и администратор пространства.
	if a == engine.ActionDocAccessManage {
		return verdict(user.ID == doc.CreatedById || wm.Role == types.AdminRole), nil
	}

	if user.ID == doc.CreatedById || user.IsSuperuser || wm.Role == types.AdminRole {
		return engine.Allow, nil
	}
	if docRead.has(a) {
		return verdict(docReadable(user.ID, wm.Role, doc)), nil
	}
	return verdict(docEditable(user.ID, wm.Role, doc)), nil
}

// docReadable — роль не ниже порога чтения или правки, либо персональный доступ.
func docReadable(userID uuid.UUID, role int, doc *dao.Doc) bool {
	if role >= doc.ReaderRole || role >= doc.EditorRole {
		return true
	}
	return slices.Contains(doc.ReaderIDs, userID) || slices.Contains(doc.EditorsIDs, userID) ||
		slices.Contains(doc.WatcherIDs, userID)
}

// docEditable — роль не ниже порога правки либо персональный доступ редактора.
func docEditable(userID uuid.UUID, role int, doc *dao.Doc) bool {
	return role >= doc.EditorRole || slices.Contains(doc.EditorsIDs, userID)
}
