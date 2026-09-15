package defaultengine

import (
	"context"
	"slices"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/engine"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

// Переходы статусов по настройке проекта (states_flow).
//
// Правило хранится в `states.from_states` — списке статусов, из которых
// разрешён переход в данный. Пустой список означает переход из любого
// статуса, а uuid.Nil в списке — что статус можно выбрать при создании
// задачи. Администратор проекта не ограничен.
//
// CanTransition и ScopeAvailableStates — две стороны одного правила и
// обязаны оставаться согласованными, поэтому лежат рядом. Расхождение
// даёт худший вид ошибки: интерфейс показывает статус, а сохранение его
// отклоняет. Согласованность проверяется тестом.

// CanTransition решает, разрешён ли перевод задачи в новый статус.
func (e *Engine) CanTransition(_ context.Context, req engine.StateTransition) (engine.Verdict, error) {
	if isProjectAdmin(req.Subject) {
		return engine.Allow, nil
	}

	from := req.To.FromStates.Array
	if len(from) == 0 {
		return engine.Allow, nil
	}

	// При создании задачи проверяется не предыдущий статус, а признак
	// стартового.
	if req.IsCreate() {
		return verdictState(slices.Contains(from, uuid.Nil)), nil
	}
	if req.Issue == nil {
		return engine.Deny, nil
	}
	return verdictState(slices.Contains(from, req.Issue.StateId)), nil
}

// ScopeAvailableStates сужает запрос по статусам проекта до доступных
// пользователю. Администратору проекта условия не навешиваются.
func (e *Engine) ScopeAvailableStates(
	_ context.Context,
	req engine.StateScopeRequest,
	q *gorm.DB,
) *gorm.DB {
	if isProjectAdmin(req.Subject) {
		return q
	}

	// Статус, которому не заданы предыдущие, доступен всегда.
	probe := stateProbe(req)
	return q.Where(
		q.Session(&gorm.Session{NewDB: true}).
			Where("array_length(from_states, 1) IS NULL").
			Or("? = any(from_states)", probe),
	)
}

// stateProbe — значение, которое ищется в списке разрешённых предыдущих
// статусов: текущий статус задачи либо признак стартового при создании.
func stateProbe(req engine.StateScopeRequest) uuid.UUID {
	if req.IsCreate() {
		return uuid.Nil
	}
	return req.Issue.StateId
}

// verdictState переводит булево решение в вердикт с ошибкой о нарушении
// бизнес-процесса.
func verdictState(allowed bool) engine.Verdict {
	if allowed {
		return engine.Allow
	}
	forbidden := engine.ErrForbiddenState
	return engine.Verdict{Decision: engine.DecisionDeny, Error: &forbidden}
}

var _ engine.StatePolicy = (*Engine)(nil)
