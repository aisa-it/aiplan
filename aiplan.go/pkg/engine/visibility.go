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
