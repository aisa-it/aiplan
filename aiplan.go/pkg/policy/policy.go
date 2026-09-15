// Пакет policy применяет правила движка, не зная о транспорте.
// Одни и те же проверки обслуживают HTTP-обработчики и MCP-инструменты,
// поэтому вместо echo.Context здесь engine.Subject, а вместо HTTP-ответа —
// ошибка ядра, умеющая представить себя в любом из транспортов.
package policy

import (
	"context"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/apierrors"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/engine"
	"gorm.io/gorm"
)

// Enforcer применяет правила подключённого движка, подставляя поведение
// ядра там, где движок решения не принял.
type Enforcer struct {
	primary  engine.Authorizer
	fallback engine.Authorizer

	states         engine.StatePolicy
	statesFallback engine.StatePolicy
}

// New собирает применитель правил.
//
// fallback — движок ядра: к нему уходят действия, по которым основной
// движок вернул DecisionDefault, и все действия, если основной движок
// правами не управляет вовсе.
func New(primary, fallback engine.Authorizer) *Enforcer {
	e := &Enforcer{primary: primary, fallback: fallback}
	if sp, ok := primary.(engine.StatePolicy); ok {
		e.states = sp
	}
	if sp, ok := fallback.(engine.StatePolicy); ok {
		e.statesFallback = sp
	}
	return e
}

// Option уточняет запрос на проверку права.
type Option func(*engine.AuthzRequest)

// On передаёт движку объект действия — уже загруженную обработчиком
// сущность: комментарий, вложение, значение поля. Без него проверяется
// только то, что известно из маршрута.
func On(target any) Option {
	return func(r *engine.AuthzRequest) { r.Target = target }
}

// Authorize возвращает nil, если действие разрешено, иначе — ошибку отказа.
//
// Неизвестное действие и отсутствие решения у обоих движков трактуются
// как запрет: расширение прав не должно быть следствием недосмотра.
func (p *Enforcer) Authorize(
	ctx context.Context,
	action engine.Action,
	s engine.Subject,
	opts ...Option,
) error {
	req := engine.AuthzRequest{Action: action, Subject: s}
	for _, opt := range opts {
		opt(&req)
	}

	if p.primary != nil {
		v, err := p.primary.Authorize(ctx, req)
		if err != nil {
			return err
		}
		if decided, e := verdictResult(v); decided {
			return e
		}
	}

	if p.fallback != nil {
		v, err := p.fallback.Authorize(ctx, req)
		if err != nil {
			return err
		}
		if decided, e := verdictResult(v); decided {
			return e
		}
	}

	return apierrors.ErrIssueForbidden
}

// AuthorizeAll проверяет набор действий: отказ хотя бы по одному
// запрещает операцию целиком.
func (p *Enforcer) AuthorizeAll(ctx context.Context, actions []engine.Action, s engine.Subject) error {
	for _, a := range actions {
		if err := p.Authorize(ctx, a, s); err != nil {
			return err
		}
	}
	return nil
}

// verdictResult переводит вердикт в результат проверки.
// Первое значение false — движок решения не принял.
func verdictResult(v engine.Verdict) (bool, error) {
	switch v.Decision {
	case engine.DecisionAllow:
		return true, nil
	case engine.DecisionDeny:
		if v.Error != nil {
			return true, *v.Error
		}
		return true, apierrors.ErrIssueForbidden
	default:
		return false, nil
	}
}

// CheckTransition проверяет допустимость перевода задачи в новый статус.
// Возвращает nil, если переход разрешён.
func (p *Enforcer) CheckTransition(ctx context.Context, req engine.StateTransition) error {
	if p.states != nil {
		v, err := p.states.CanTransition(ctx, req)
		if err != nil {
			return err
		}
		if decided, e := verdictResult(v); decided {
			return e
		}
	}

	if p.statesFallback != nil {
		v, err := p.statesFallback.CanTransition(ctx, req)
		if err != nil {
			return err
		}
		if decided, e := verdictResult(v); decided {
			return e
		}
	}

	return apierrors.ErrForbiddenState
}

// ScopeStates сужает запрос по статусам проекта до доступных пользователю.
//
// Это та же проверка, что и CheckTransition, но выраженная условием запроса:
// список доступных статусов обязан совпадать с тем, что примет CheckTransition.
func (p *Enforcer) ScopeStates(ctx context.Context, req engine.StateScopeRequest, q *gorm.DB) *gorm.DB {
	if p.states != nil {
		return p.states.ScopeAvailableStates(ctx, req, q)
	}
	if p.statesFallback != nil {
		return p.statesFallback.ScopeAvailableStates(ctx, req, q)
	}
	// Политики нет — показывать нечего: пустая выдача безопаснее полной.
	return q.Where("1 = 0")
}
