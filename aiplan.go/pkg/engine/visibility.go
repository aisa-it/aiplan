package engine

import (
	"context"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/dao"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/types"
	"gorm.io/gorm"
)

// ScopeKind — режим выборки задач.
//
// Режимы не равноправны: глобальный поиск идёт по всем проектам
// пользователя и игнорирует выбранный проект.
type ScopeKind uint8

const (
	// ScopeGlobal — поиск по всем проектам, где пользователь состоит.
	ScopeGlobal ScopeKind = iota
	// ScopeProject — задачи одного проекта.
	ScopeProject
	// ScopeSprint — задачи спринта.
	ScopeSprint
)

// IssueScope — запрос на выборку задач.
//
// Геттеры Subject ленивые, поэтому реализация обращается только к тому,
// что есть в режиме: ProjectMember() — в ScopeProject, Sprint() — в
// ScopeSprint. В ScopeSprint ядро гарантирует, что спринт загружен вместе
// с задачами.
type IssueScope struct {
	Subject Subject
	Kind    ScopeKind
	Params  *types.SearchParams
}

// VisibilityPolicy — правила видимости задач.
//
// Это единственное, что ограничивает выдачу поиска: ручка поиска доступна
// любому участнику, и разграничение держится на условиях запроса. Поэтому
// ошибка здесь не приводит к отказу в доступе — она приводит к выдаче чужих
// задач. Ядро при ошибке политики обязано сузить выборку до пустой, а не
// выполнять запрос без ограничений.
type VisibilityPolicy interface {
	// VisibleProjects — подзапрос, возвращающий колонку project_id.
	// Единственный источник правды о видимости: и выборка задач, и подсчёт
	// групп опираются на него, поэтому они не могут разойтись.
	VisibleProjects(ctx context.Context, req IssueScope, db *gorm.DB) *gorm.DB
	// ScopeIssues навешивает ограничения на запрос по issues.
	ScopeIssues(ctx context.Context, req IssueScope, q *gorm.DB) *gorm.DB
	// CanViewIssue — построчная проверка для получения одной задачи.
	CanViewIssue(ctx context.Context, s Subject, issue *dao.Issue) (Verdict, error)
}

// RowVisibilityPolicy — необязательное дополнение VisibilityPolicy для
// движков, которые скрывают отдельные задачи внутри видимого проекта.
//
// VisibleProjects отвечает только за набор проектов, поэтому подсчёты по
// проекту (группы поиска) без этого дополнения считают и скрытые задачи.
// Движок, сужающий выдачу построчно, сообщает об этом — и ядро применяет
// ScopeIssues к подсчётам. Движку без построчных правил реализовывать
// интерфейс не нужно: лишнее условие в подсчётах стоит запроса.
type RowVisibilityPolicy interface {
	// RestrictsRows — сужает ли ScopeIssues выдачу этого запроса сверх
	// набора проектов.
	RestrictsRows(ctx context.Context, req IssueScope) bool
}

// CommentPolicy — видимость комментариев задачи. Необязательный интерфейс:
// без него комментарии видит каждый, кому разрешено issue.comment.view.
//
// Нужен движкам, у которых комментарии одной задачи видны разным
// пользователям по-разному. Ядро применяет условие к списку комментариев,
// одному комментарию, его истории и к событиям активности о комментариях.
type CommentPolicy interface {
	// ScopeComments навешивает ограничения на запрос по issue_comments.
	// Задача субъекта известна: Subject.Issue().
	ScopeComments(ctx context.Context, s Subject, q *gorm.DB) *gorm.DB
}
