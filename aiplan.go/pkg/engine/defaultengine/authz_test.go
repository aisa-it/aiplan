package defaultengine

import (
	"context"
	"testing"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/engine"
)

// TestAuthorizeSkipsForeignActions: движок ведёт только права на задачу.
// По остальным действиям он обязан вернуть DecisionDefault — отказ здесь
// выглядел бы как запрет, хотя правила просто нет, и ядро не смогло бы
// применить своё поведение.
func TestAuthorizeSkipsForeignActions(t *testing.T) {
	foreign := []engine.Action{
		engine.ActionProjectView, engine.ActionProjectUpdate, engine.ActionProjectAdmin,
		engine.ActionProjectStateManage, engine.ActionProjectRulesManage,
		engine.ActionWorkspaceView, engine.ActionWorkspaceAdmin, engine.ActionWorkspaceInvite,
		engine.ActionSprintView, engine.ActionSprintUpdate,
		engine.ActionDocView, engine.ActionDocUpdate,
		engine.ActionFormView, engine.ActionFormAnswer,

		// Выполняются до того, как задача известна: ограничиваются
		// правилами видимости, а не правами на задачу.
		engine.ActionIssueSearch,
		engine.ActionIssueCreate,
	}

	e := New()
	for _, a := range foreign {
		t.Run(string(a), func(t *testing.T) {
			// Subject намеренно nil: движок обязан отказаться от решения
			// до обращения к сущностям запроса.
			v, err := e.Authorize(context.Background(), engine.AuthzRequest{Action: a})
			if err != nil {
				t.Fatalf("ошибка вместо отказа от решения: %v", err)
			}
			if v.Decision != engine.DecisionDefault {
				t.Errorf("решение %d, ожидалось DecisionDefault: движок взялся "+
					"за действие вне своей области", v.Decision)
			}
		})
	}
}

// TestAuthorizeHandlesIssueActions: действия над задачей движок ведёт сам
// и решение по ним принимает всегда.
func TestAuthorizeHandlesIssueActions(t *testing.T) {
	own := []engine.Action{
		engine.ActionIssueView, engine.ActionIssueUpdate, engine.ActionIssueDelete,
		engine.ActionIssueSetState, engine.ActionIssueCommentCreate,
		engine.ActionIssueAttachmentAdd, engine.ActionIssueSetProperty,
	}

	e := New()
	for _, a := range own {
		if !isIssueScopeAction(a) {
			t.Errorf("действие %q над задачей не отнесено к области движка", a)
		}
	}
	_ = e
}
