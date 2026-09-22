package engine

import (
	"context"
	"slices"
	"sync"
)

// PermissionSet — решения по набору действий. Отсутствие действия в наборе
// означает, что движок решения не принял (аналог DecisionDefault).
type PermissionSet map[Action]bool

// Strings — набор в виде для API: ключи-строки, без типов движка.
func (p PermissionSet) Strings() map[string]bool {
	out := make(map[string]bool, len(p))
	for action, allowed := range p {
		out[string(action)] = allowed
	}
	return out
}

// PermissionsRequest — запрос набора прав субъекта.
type PermissionsRequest struct {
	Subject Subject
	Actions []Action
	// Target — объект действия, если известен (та же роль, что в AuthzRequest).
	Target any
}

// BulkAuthorizer — движок, умеющий посчитать набор действий разом.
// Опциональный: без него ядро опрашивает Authorize по каждому действию.
type BulkAuthorizer interface {
	Permissions(ctx context.Context, req PermissionsRequest) (PermissionSet, error)
}

// Area — область действий: какие действия имеют смысл для сущности.
type Area string

const (
	AreaIssue     Area = "issue"
	AreaProject   Area = "project"
	AreaWorkspace Area = "workspace"
	AreaSprint    Area = "sprint"
	AreaDoc       Area = "doc"
	AreaForm      Area = "form"
)

// ObjectActions — действия, право на которые зависит от конкретного объекта
// (комментария), а не от задачи. В набор прав задачи они не входят и
// решаются при обращении: иначе интерфейс спрятал бы доступные операции.
var ObjectActions = map[Action]struct{}{
	ActionIssueCommentUpdate: {},
	ActionIssueCommentDelete: {},
}

// UncheckedActions — действия, которые ядро не проверяет: роут без проверки
// прав либо действие вне сущности (создать пространство — не «в пространстве»).
// В наборы прав не входят: false там означал бы запрет, которого нет.
var UncheckedActions = map[Action]struct{}{
	ActionIssueMigrate:      {},
	ActionWorkspaceCreate:   {},
	ActionFormAnswer:        {},
	ActionFormAttachmentAdd: {},
}

// PermissionActions — действия области, которые имеет смысл отдавать наружу
// набором: без зависящих от объекта и без непроверяемых.
func PermissionActions(area Area) []Action {
	return slices.DeleteFunc(ActionsFor(area), func(a Action) bool {
		_, byObject := ObjectActions[a]
		_, unchecked := UncheckedActions[a]
		return byObject || unchecked
	})
}

var (
	actionsMu sync.RWMutex
	actions   = map[Area][]Action{
		AreaIssue: {
			ActionIssueView, ActionIssueUpdate, ActionIssueDelete,
			ActionIssueViewActivity, ActionIssueExport,
			ActionIssueSetState, ActionIssueSetAssignees, ActionIssueSetWatchers,
			ActionIssueSetLabels, ActionIssueSetProperty, ActionIssueSetParent,
			ActionIssueSetSprint, ActionIssueSetBlockers, ActionIssueSetLinked,
			ActionIssueSetSubIssues,
			ActionIssueCommentView, ActionIssueCommentCreate, ActionIssueCommentUpdate,
			ActionIssueCommentDelete, ActionIssueCommentReact,
			ActionIssueAttachmentView, ActionIssueAttachmentAdd, ActionIssueAttachmentDelete,
			ActionIssueLinkManage, ActionIssueRelationManage,
			ActionIssuePin, ActionIssueDescriptionLock,
		},
		AreaProject: {
			ActionProjectView, ActionProjectUpdate, ActionProjectDelete, ActionProjectAdmin,
			ActionProjectArchive, ActionProjectStats, ActionProjectActivity,
			ActionProjectMemberView, ActionProjectMemberManage, ActionProjectStateManage,
			ActionProjectRulesManage, ActionProjectLabelManage, ActionProjectEstimateManage,
			ActionProjectTemplateManage, ActionProjectPropertyManage,
			ActionProjectDictionaryView, ActionProjectDictionaryManage, ActionProjectViewManage,
			ActionProjectJoin, ActionProjectSelfSettings, ActionProjectRulesLogView,
			ActionIssueCreate, ActionIssueSearch, ActionIssueBulkEdit, ActionIssueMigrate,
		},
		AreaWorkspace: {
			ActionWorkspaceView, ActionWorkspaceUpdate, ActionWorkspaceDelete, ActionWorkspaceAdmin,
			ActionWorkspaceActivity, ActionWorkspaceMemberView, ActionWorkspaceMemberManage,
			ActionWorkspaceInvite, ActionWorkspaceIntegrationManage, ActionWorkspaceTokenManage,
			ActionWorkspaceTokenView, ActionWorkspaceBackup, ActionWorkspaceBackupView,
			ActionWorkspaceImport, ActionWorkspaceSelfSettings, ActionProjectCreate,
		},
		AreaSprint: {
			ActionSprintList, ActionSprintView, ActionSprintCreate, ActionSprintUpdate, ActionSprintDelete,
			ActionSprintAdmin, ActionSprintActivity, ActionSprintIssueManage,
			ActionSprintWatchManage, ActionSprintSearchIssues, ActionSprintViewManage,
			ActionSprintFolderManage,
		},
		AreaDoc: {
			ActionDocList, ActionDocCreateRoot,
			ActionDocView, ActionDocCreate, ActionDocAccessManage, ActionDocUpdate, ActionDocDelete, ActionDocMove,
			ActionDocActivity, ActionDocHistoryView,
			ActionDocCommentView, ActionDocCommentCreate, ActionDocCommentUpdate,
			ActionDocCommentDelete, ActionDocCommentReact,
			ActionDocAttachmentView, ActionDocAttachmentAdd, ActionDocAttachmentDel,
		},
		AreaForm: {
			ActionFormView, ActionFormCreate, ActionFormUpdate, ActionFormDelete,
			ActionFormAnswer, ActionFormAnswerView, ActionFormAttachmentAdd,
		},
	}
)

// ActionsFor возвращает действия области — копию, в порядке регистрации.
func ActionsFor(area Area) []Action {
	actionsMu.RLock()
	defer actionsMu.RUnlock()
	return slices.Clone(actions[area])
}

// RegisterActions добавляет действия движка в область. Повторы отбрасываются.
// Вызывается из Engine.Init через Core, до старта сервера.
func RegisterActions(area Area, add ...Action) {
	actionsMu.Lock()
	defer actionsMu.Unlock()
	for _, a := range add {
		if !slices.Contains(actions[area], a) {
			actions[area] = append(actions[area], a)
		}
	}
}
