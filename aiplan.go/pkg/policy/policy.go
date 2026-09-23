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

	comments         engine.CommentHooks
	commentsFallback engine.CommentHooks

	properties         engine.PropertyPolicy
	propertiesFallback engine.PropertyPolicy
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
	if h, ok := primary.(engine.CommentHooks); ok {
		e.comments = h
	}
	if h, ok := fallback.(engine.CommentHooks); ok {
		e.commentsFallback = h
	}
	if pp, ok := primary.(engine.PropertyPolicy); ok {
		e.properties = pp
	}
	if pp, ok := fallback.(engine.PropertyPolicy); ok {
		e.propertiesFallback = pp
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

// ScopePropertyTemplates ограничивает выборку шаблонов кастомных полей
// видимыми субъекту: подключённый движок, иначе движок ядра — как у
// ScopeStates, частичного переопределения нет. Без политики — пустая
// выборка: пустая выдача безопаснее полной.
func (p *Enforcer) ScopePropertyTemplates(ctx context.Context, s engine.Subject, q *gorm.DB) *gorm.DB {
	if p.properties != nil {
		return p.properties.ScopePropertyTemplates(ctx, s, q)
	}
	if p.propertiesFallback != nil {
		return p.propertiesFallback.ScopePropertyTemplates(ctx, s, q)
	}
	return q.Where("1 = 0")
}

// PropertyTemplateScope — то же ограничение в виде gorm-scope для dao:
// обработчик строит его один раз и отдаёт в выборки шаблонов.
func (p *Enforcer) PropertyTemplateScope(ctx context.Context, s engine.Subject) dao.PropertyTemplateScope {
	return func(q *gorm.DB) *gorm.DB { return p.ScopePropertyTemplates(ctx, s, q) }
}

// Хуки задачи и комментария.
//
// Before-хуки — реакции, а не права: подключённый движок решает первым,
// DecisionDefault отдаёт слово движку ядра (Lua-скриптам проекта), а
// отсутствие решения у обоих ничего не запрещает. Ошибка отказа — та,
// что вернул движок.
//
// After-хуки уведомляют оба движка: изменение уже сохранено, и отменить
// его хук не может. Ошибки собираются и возвращаются вызывающему для лога.

func (p *Enforcer) BeforeIssueCreate(ctx context.Context, s engine.Subject, issue dao.Issue) error {
	return firstVerdict(p.issueHooks(), func(h engine.IssueHooks) (engine.Verdict, error) {
		return h.BeforeIssueCreate(ctx, s, issue)
	})
}

func (p *Enforcer) AfterIssueCreate(ctx context.Context, s engine.Subject, issue dao.Issue) error {
	return notifyAll(p.issueHooks(), func(h engine.IssueHooks) error {
		return h.AfterIssueCreate(ctx, s, issue)
	})
}

func (p *Enforcer) BeforeStateChange(ctx context.Context, ev engine.StateTransition) error {
	return firstVerdict(p.issueHooks(), func(h engine.IssueHooks) (engine.Verdict, error) {
		return h.BeforeStateChange(ctx, ev)
	})
}

func (p *Enforcer) AfterStateChange(ctx context.Context, ev engine.StateTransition) error {
	return notifyAll(p.issueHooks(), func(h engine.IssueHooks) error {
		return h.AfterStateChange(ctx, ev)
	})
}

func (p *Enforcer) BeforeAssigneesChange(ctx context.Context, s engine.Subject, issue dao.Issue, users []dao.User) error {
	return firstVerdict(p.issueHooks(), func(h engine.IssueHooks) (engine.Verdict, error) {
		return h.BeforeAssigneesChange(ctx, s, issue, users)
	})
}

func (p *Enforcer) BeforeWatchersChange(ctx context.Context, s engine.Subject, issue dao.Issue, users []dao.User) error {
	return firstVerdict(p.issueHooks(), func(h engine.IssueHooks) (engine.Verdict, error) {
		return h.BeforeWatchersChange(ctx, s, issue, users)
	})
}

func (p *Enforcer) BeforeLabelsChange(ctx context.Context, s engine.Subject, issue dao.Issue, labels []dao.Label) error {
	return firstVerdict(p.issueHooks(), func(h engine.IssueHooks) (engine.Verdict, error) {
		return h.BeforeLabelsChange(ctx, s, issue, labels)
	})
}

func (p *Enforcer) BeforePropertyChange(ctx context.Context, s engine.Subject, issue dao.Issue, tpl dao.ProjectPropertyTemplate, newValue string) error {
	return firstVerdict(p.issueHooks(), func(h engine.IssueHooks) (engine.Verdict, error) {
		return h.BeforePropertyChange(ctx, s, issue, tpl, newValue)
	})
}

func (p *Enforcer) BeforeCommentCreate(ctx context.Context, s engine.Subject, issue dao.Issue, comment dao.IssueComment) error {
	return firstVerdict(p.commentHooks(), func(h engine.CommentHooks) (engine.Verdict, error) {
		return h.BeforeCommentCreate(ctx, s, issue, comment)
	})
}

func (p *Enforcer) AfterCommentCreate(ctx context.Context, s engine.Subject, issue dao.Issue, comment dao.IssueComment) error {
	return notifyAll(p.commentHooks(), func(h engine.CommentHooks) error {
		return h.AfterCommentCreate(ctx, s, issue, comment)
	})
}

// issueHooks и commentHooks — движки в порядке опроса: подключённый, затем ядро.
func (p *Enforcer) issueHooks() []engine.IssueHooks {
	return []engine.IssueHooks{p.hooks, p.hooksFallback}
}

func (p *Enforcer) commentHooks() []engine.CommentHooks {
	return []engine.CommentHooks{p.comments, p.commentsFallback}
}

// firstVerdict возвращает решение первого движка, который его принял.
// Нереализованный движок (nil) пропускается.
func firstVerdict[H any](hooks []H, call func(H) (engine.Verdict, error)) error {
	for _, h := range hooks {
		if any(h) == nil {
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

// notifyAll вызывает after-хук у всех движков и собирает их ошибки.
func notifyAll[H any](hooks []H, call func(H) error) error {
	var errs []error
	for _, h := range hooks {
		if any(h) == nil {
			continue
		}
		if err := call(h); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Permissions считает решения по набору действий для одного субъекта.
//
// Порядок тот же, что у Authorize: подключённый движок, затем движок ядра;
// действие, по которому никто не решил, — запрет. Движок с BulkAuthorizer
// отвечает одним вызовом, остальные опрашиваются по действию — Subject
// кеширует загруженное, поэтому в БД это один поход за ролями.
func (p *Enforcer) Permissions(
	ctx context.Context,
	s engine.Subject,
	actions []engine.Action,
	opts ...Option,
) (engine.PermissionSet, error) {
	base := engine.AuthzRequest{Subject: s}
	for _, opt := range opts {
		opt(&base)
	}

	result := make(engine.PermissionSet, len(actions))
	remaining := actions
	for _, a := range []engine.Authorizer{p.primary, p.fallback} {
		if a == nil || len(remaining) == 0 {
			continue
		}
		decided, err := decidePermissions(ctx, a, base, remaining)
		if err != nil {
			return nil, err
		}
		undecided := remaining[:0:0]
		for _, action := range remaining {
			v, ok := decided[action]
			if !ok {
				undecided = append(undecided, action)
				continue
			}
			result[action] = v
		}
		remaining = undecided
	}
	for _, action := range remaining {
		result[action] = false
	}
	return result, nil
}

// decidePermissions — решения одного движка; недосказанные действия в наборе отсутствуют.
func decidePermissions(
	ctx context.Context,
	a engine.Authorizer,
	base engine.AuthzRequest,
	actions []engine.Action,
) (engine.PermissionSet, error) {
	if bulk, ok := a.(engine.BulkAuthorizer); ok {
		return bulk.Permissions(ctx, engine.PermissionsRequest{
			Subject: base.Subject, Actions: actions, Target: base.Target,
		})
	}

	set := make(engine.PermissionSet, len(actions))
	for _, action := range actions {
		req := base
		req.Action = action
		v, err := a.Authorize(ctx, req)
		if err != nil {
			return nil, err
		}
		switch v.Decision {
		case engine.DecisionAllow:
			set[action] = true
		case engine.DecisionDeny:
			set[action] = false
		}
	}
	return set, nil
}

// Наборы прав для выдачи наружу: все действия области (engine.PermissionActions),
// сама сущность — в роли объекта. HTTP и MCP обязаны отдавать права этими
// методами: иначе «показано» и «разрешено» разойдутся.

// IssuePermissions — набор прав субъекта на задачу.
func (p *Enforcer) IssuePermissions(ctx context.Context, s engine.Subject, issue *dao.Issue) (engine.PermissionSet, error) {
	return p.Permissions(ctx, s, engine.PermissionActions(engine.AreaIssue), On(issue))
}

// ProjectPermissions — набор прав субъекта на проект, включая создание,
// поиск и массовые операции с задачами в нём.
func (p *Enforcer) ProjectPermissions(ctx context.Context, s engine.Subject, project *dao.Project) (engine.PermissionSet, error) {
	return p.Permissions(ctx, s, engine.PermissionActions(engine.AreaProject), On(project))
}

// WorkspacePermissions — набор прав субъекта на пространство, включая
// создание проектов, спринтов, документов и форм в нём.
func (p *Enforcer) WorkspacePermissions(ctx context.Context, s engine.Subject, workspace *dao.Workspace) (engine.PermissionSet, error) {
	return p.Permissions(ctx, s, engine.PermissionActions(engine.AreaWorkspace), On(workspace))
}
