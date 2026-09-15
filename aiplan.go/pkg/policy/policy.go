// Пакет policy применяет правила движка, не зная о транспорте.
// Одни и те же проверки обслуживают HTTP-обработчики и MCP-инструменты,
// поэтому вместо echo.Context здесь engine.Subject, а вместо HTTP-ответа —
// ошибка ядра, умеющая представить себя в любом из транспортов.
package policy

import (
	"context"
	"errors"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/apierrors"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/dao"
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

	visibility         engine.VisibilityPolicy
	visibilityFallback engine.VisibilityPolicy

	hooks         engine.IssueHooks
	hooksFallback engine.IssueHooks
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
	if vp, ok := primary.(engine.VisibilityPolicy); ok {
		e.visibility = vp
	}
	if vp, ok := fallback.(engine.VisibilityPolicy); ok {
		e.visibilityFallback = vp
	}
	if h, ok := primary.(engine.IssueHooks); ok {
		e.hooks = h
	}
	if h, ok := fallback.(engine.IssueHooks); ok {
		e.hooksFallback = h
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

// visibilityPolicy — действующая политика видимости: подключённый движок
// целиком, иначе движок ядра. Частичного переопределения нет: VisibleProjects
// и ScopeIssues обязаны быть согласованы, а это возможно только внутри одной
// реализации.
func (p *Enforcer) visibilityPolicy() engine.VisibilityPolicy {
	if p.visibility != nil {
		return p.visibility
	}
	return p.visibilityFallback
}

// VisibleProjects — подзапрос project_id, видимых субъекту.
// Без политики — пустой: пустая выдача безопаснее полной.
func (p *Enforcer) VisibleProjects(ctx context.Context, req engine.IssueScope, db *gorm.DB) *gorm.DB {
	if vp := p.visibilityPolicy(); vp != nil {
		return vp.VisibleProjects(ctx, req, db)
	}
	return db.Select("project_id").Model(&dao.ProjectMember{}).Where("1 = 0")
}

// ScopeIssues ограничивает выборку задач видимыми субъекту.
func (p *Enforcer) ScopeIssues(ctx context.Context, req engine.IssueScope, q *gorm.DB) *gorm.DB {
	if vp := p.visibilityPolicy(); vp != nil {
		return vp.ScopeIssues(ctx, req, q)
	}
	return q.Where("1 = 0")
}

// CanViewIssue проверяет право просмотра одной задачи.
// Без политики и без решения — запрет.
func (p *Enforcer) CanViewIssue(ctx context.Context, s engine.Subject, issue *dao.Issue) error {
	for _, vp := range []engine.VisibilityPolicy{p.visibility, p.visibilityFallback} {
		if vp == nil {
			continue
		}
		v, err := vp.CanViewIssue(ctx, s, issue)
		if err != nil {
			return err
		}
		if decided, e := verdictResult(v); decided {
			return e
		}
	}
	return apierrors.ErrIssueForbidden
}

// Хуки изменения задачи.
//
// Before-хуки — реакции, а не права: подключённый движок решает первым,
// DecisionDefault отдаёт слово движку ядра (Lua-скриптам проекта), а
// отсутствие решения у обоих ничего не запрещает. Ошибка отказа — та,
// что вернул движок.

func (p *Enforcer) BeforeStateChange(ctx context.Context, ev engine.StateTransition) error {
	return p.hookVerdict(func(h engine.IssueHooks) (engine.Verdict, error) {
		return h.BeforeStateChange(ctx, ev)
	})
}

func (p *Enforcer) BeforeAssigneesChange(ctx context.Context, s engine.Subject, issue dao.Issue, users []dao.User) error {
	return p.hookVerdict(func(h engine.IssueHooks) (engine.Verdict, error) {
		return h.BeforeAssigneesChange(ctx, s, issue, users)
	})
}

func (p *Enforcer) BeforeWatchersChange(ctx context.Context, s engine.Subject, issue dao.Issue, users []dao.User) error {
	return p.hookVerdict(func(h engine.IssueHooks) (engine.Verdict, error) {
		return h.BeforeWatchersChange(ctx, s, issue, users)
	})
}

func (p *Enforcer) BeforeLabelsChange(ctx context.Context, s engine.Subject, issue dao.Issue, labels []dao.Label) error {
	return p.hookVerdict(func(h engine.IssueHooks) (engine.Verdict, error) {
		return h.BeforeLabelsChange(ctx, s, issue, labels)
	})
}

func (p *Enforcer) BeforePropertyChange(ctx context.Context, s engine.Subject, issue dao.Issue, tpl dao.ProjectPropertyTemplate, newValue string) error {
	return p.hookVerdict(func(h engine.IssueHooks) (engine.Verdict, error) {
		return h.BeforePropertyChange(ctx, s, issue, tpl, newValue)
	})
}

// AfterStateChange уведомляет оба движка: изменение уже сохранено, и отменить
// его хук не может. Ошибки собираются и возвращаются вызывающему для лога.
func (p *Enforcer) AfterStateChange(ctx context.Context, ev engine.StateTransition) error {
	var errs []error
	for _, h := range []engine.IssueHooks{p.hooks, p.hooksFallback} {
		if h == nil {
			continue
		}
		if err := h.AfterStateChange(ctx, ev); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (p *Enforcer) hookVerdict(call func(engine.IssueHooks) (engine.Verdict, error)) error {
	for _, h := range []engine.IssueHooks{p.hooks, p.hooksFallback} {
		if h == nil {
			continue
		}
		v, err := call(h)
		if err != nil {
			return err
		}
		if decided, e := verdictResult(v); decided {
			return e
		}
	}
	return nil
}
