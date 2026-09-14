package engine

import (
	"context"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/dao"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

// StateTransition — переход задачи в новый статус.
// При создании задачи Issue и From равны nil.
type StateTransition struct {
	Subject Subject
	Issue   *dao.Issue
	From    *dao.State
	To      dao.State
}

// IsCreate сообщает, что это выбор стартового статуса при создании задачи.
func (t StateTransition) IsCreate() bool { return t.Issue == nil }

// StateScopeRequest — запрос списка статусов, доступных пользователю.
// Issue == nil — нужны стартовые статусы для создания задачи.
type StateScopeRequest struct {
	Subject   Subject
	ProjectID uuid.UUID
	Issue     *dao.Issue
}

// IsCreate сообщает, что запрошены стартовые статусы.
func (r StateScopeRequest) IsCreate() bool { return r.Issue == nil }

// StatePolicy — правила переходов между статусами.
//
// Два метода описывают одно правило с разных сторон: CanTransition отвечает
// на вопрос об одном переходе, ScopeAvailableStates сужает выборку статусов
// для списка. Реализации ОБЯЗАНЫ держать их согласованными:
//
//	статус проходит ScopeAvailableStates  <=>  CanTransition(..., To: статус) != Deny
//
// Рассогласование даёт худший вид ошибки: интерфейс показывает статус,
// а сохранение его отклоняет. Ядро проверяет этот инвариант тестом.
type StatePolicy interface {
	CanTransition(ctx context.Context, req StateTransition) (Verdict, error)
	// ScopeAvailableStates навешивает условия на запрос по dao.State,
	// в который уже добавлен фильтр по проекту.
	ScopeAvailableStates(ctx context.Context, req StateScopeRequest, q *gorm.DB) *gorm.DB
}
