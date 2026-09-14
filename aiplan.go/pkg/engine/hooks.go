package engine

import (
	"context"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/dao"
)

// IssueHooks — реакции движка на изменение задачи.
//
// Before-хуки могут запретить изменение, вернув Deny. After-хуки
// вызываются после успешного сохранения и запретить ничего не могут:
// их ошибка логируется, но операцию не отменяет.
//
// Дефолтный движок реализует хуки через пользовательские Lua-скрипты
// проекта (пакет rules).
type IssueHooks interface {
	BeforeStateChange(ctx context.Context, ev StateTransition) (Verdict, error)
	AfterStateChange(ctx context.Context, ev StateTransition) error

	BeforeAssigneesChange(ctx context.Context, s Subject, issue dao.Issue, newAssignees []dao.User) (Verdict, error)
	BeforeWatchersChange(ctx context.Context, s Subject, issue dao.Issue, newWatchers []dao.User) (Verdict, error)
	BeforeLabelsChange(ctx context.Context, s Subject, issue dao.Issue, newLabels []dao.Label) (Verdict, error)
	BeforePropertyChange(ctx context.Context, s Subject, issue dao.Issue, tpl dao.ProjectPropertyTemplate, newValue string) (Verdict, error)
}
