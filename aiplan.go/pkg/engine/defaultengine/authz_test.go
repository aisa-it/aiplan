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
// TestAuthorizeSkipsForeignActions: по действиям, которых движок ядра не
// ведёт, он обязан отказаться от решения до обращения к сущностям запроса.
// Права на проект, пространство, спринт и документ движок решает
// (эталон — pkg/server/testdata/authz_scopes.golden), здесь их нет.
func TestAuthorizeSkipsForeignActions(t *testing.T) {
	foreign := []engine.Action{
		engine.ActionIssueMigrate,
		engine.ActionWorkspaceCreate,
		engine.ActionFormAnswer,
		engine.ActionFormAttachmentAdd,
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
				t.Errorf("решение %d, ожидалось DecisionDefault", v.Decision)
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
