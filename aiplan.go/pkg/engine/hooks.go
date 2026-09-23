package engine

import (
	"context"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/dao"
)

// IssueHooks — реакции движка на создание и изменение задачи.
//
// Before-хуки могут запретить изменение, вернув Deny. After-хуки
// вызываются после успешного сохранения и запретить ничего не могут:
// их ошибка логируется, но операцию не отменяет.
//
// Хуки — реакции, а не права: право на само действие (issue.create,
// issue.update, …) проверяет Authorizer до них.
//
// Дефолтный движок реализует хуки изменения через пользовательские
// Lua-скрипты проекта (пакет rules); на создание задачи ядро правил
// не накладывает.
type IssueHooks interface {
	// BeforeIssueCreate вызывается до сохранения задачи. issue уже собрана
	// (проект, автор, стартовый статус, тип, описание), но исполнители,
	// наблюдатели и метки ещё не привязаны. StateId может быть пустым —
	// тогда ядро подставит статус проекта по умолчанию при сохранении.
	// Subject — в скоупе проекта: Subject.Issue() == nil.
	BeforeIssueCreate(ctx context.Context, s Subject, issue dao.Issue) (Verdict, error)
	// AfterIssueCreate получает сохранённую задачу с подгруженными статусом,
	// типом, исполнителями и наблюдателями.
	AfterIssueCreate(ctx context.Context, s Subject, issue dao.Issue) error

	BeforeStateChange(ctx context.Context, ev StateTransition) (Verdict, error)
	AfterStateChange(ctx context.Context, ev StateTransition) error

	BeforeAssigneesChange(ctx context.Context, s Subject, issue dao.Issue, newAssignees []dao.User) (Verdict, error)
	BeforeWatchersChange(ctx context.Context, s Subject, issue dao.Issue, newWatchers []dao.User) (Verdict, error)
	BeforeLabelsChange(ctx context.Context, s Subject, issue dao.Issue, newLabels []dao.Label) (Verdict, error)
	BeforePropertyChange(ctx context.Context, s Subject, issue dao.Issue, tpl dao.ProjectPropertyTemplate, newValue string) (Verdict, error)
}

// CommentHooks — реакции движка на комментарий к задаче.
//
// Отдельный от IssueHooks интерфейс: движку, которому нужны только
// комментарии, не приходится реализовывать хуки задачи. Семантика та же:
// Before может запретить, After — только отреагировать.
type CommentHooks interface {
	// BeforeCommentCreate вызывается до сохранения комментария. comment уже
	// заполнен (задача, автор, текст, ответ-на), вложения ещё не загружены.
	BeforeCommentCreate(ctx context.Context, s Subject, issue dao.Issue, comment dao.IssueComment) (Verdict, error)
	// AfterCommentCreate получает сохранённый комментарий с вложениями.
	AfterCommentCreate(ctx context.Context, s Subject, issue dao.Issue, comment dao.IssueComment) error
}
