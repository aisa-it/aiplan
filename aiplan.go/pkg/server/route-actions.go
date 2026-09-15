package server

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	apicontext "github.com/aisa-it/aiplan/aiplan.go/pkg/api-context"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/apierrors"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/engine"
	"github.com/labstack/echo/v4"
)

// actionContextKey — ключ, под которым действие роута лежит в контексте запроса.
const actionContextKey = "engine.action"

// route регистрирует роут вместе с предметным действием, которое он выполняет.
//
// Действие замыкается в middleware самого роута, а не ищется по таблице:
// echo не даёт доступа к метаданным роута из обработчика (c.Route() в v4
// нет), а хранить собственную копию маршрутов ради этого незачем.
//
// На роут вешается только то, что относится к нему одному. Проверки,
// общие для всей группы, остаются групповыми — дублировать их на каждом
// роуте незачем. Учесть при переносе прав в движок: роутовые middleware
// выполняются после групповых, поэтому групповая проверка действия
// из контекста не увидит.
//
//nolint:unparam // mw пока не используется размеченными роутами
func (s *Services) route(
	g *echo.Group,
	method, path string,
	action engine.Action,
	h echo.HandlerFunc,
	mw ...echo.MiddlewareFunc,
) {
	all := append([]echo.MiddlewareFunc{withAction(action)}, mw...)
	r := g.Add(method, path, h, all...)

	// Путь запоминается только чтобы проверить полноту разметки при старте;
	// после проверки набор очищается и в памяти не остаётся.
	if s.mappedRoutes == nil {
		s.mappedRoutes = map[string]engine.Action{}
	}
	s.mappedRoutes[r.Method+" "+r.Path] = action
}

// issueRoute регистрирует роут задачи: действие плюс проверка прав.
//
// Проверка навешивается на роут, а не на группу, потому что она читает
// действие из контекста запроса, а групповые middleware выполняются
// раньше роутовых. Групповые guard'ы (пространство, проект, поиск задачи)
// при этом по-прежнему отрабатывают до неё.
//
//nolint:unparam // mw пока не используется размеченными роутами
func (s *Services) issueRoute(
	g *echo.Group,
	method, path string,
	action engine.Action,
	h echo.HandlerFunc,
	mw ...echo.MiddlewareFunc,
) {
	all := append([]echo.MiddlewareFunc{s.IssuePermissionMiddleware}, mw...)
	s.route(g, method, path, action, h, all...)
}

// scopedRoute регистрирует роут области: действие плюс проверка прав.
// forbidden — ошибка отказа этой области (коды ошибок у областей свои).
func (s *Services) scopedRoute(
	g *echo.Group,
	method, path string,
	action engine.Action,
	forbidden apierrors.DefinedError,
	h echo.HandlerFunc,
	mw ...echo.MiddlewareFunc,
) {
	all := append([]echo.MiddlewareFunc{s.permissionMiddleware(forbidden)}, mw...)
	s.route(g, method, path, action, h, all...)
}

func (s *Services) workspaceRoute(g *echo.Group, method, path string, action engine.Action, h echo.HandlerFunc, mw ...echo.MiddlewareFunc) {
	s.scopedRoute(g, method, path, action, apierrors.ErrWorkspaceForbidden, h, mw...)
}

func (s *Services) projectRoute(g *echo.Group, method, path string, action engine.Action, h echo.HandlerFunc) {
	s.scopedRoute(g, method, path, action, apierrors.ErrProjectForbidden, h)
}

func (s *Services) sprintRoute(g *echo.Group, method, path string, action engine.Action, h echo.HandlerFunc) {
	s.scopedRoute(g, method, path, action, apierrors.ErrSprintForbidden, h)
}

func (s *Services) docRoute(g *echo.Group, method, path string, action engine.Action, h echo.HandlerFunc) {
	s.scopedRoute(g, method, path, action, apierrors.ErrDocForbidden, h)
}

func (s *Services) formRoute(g *echo.Group, method, path string, action engine.Action, h echo.HandlerFunc) {
	s.scopedRoute(g, method, path, action, apierrors.ErrFormForbidden, h)
}

// permissionMiddleware проверяет право на действие роута через движок.
//
// Действие берётся из контекста, куда его кладёт регистрация роута,
// поэтому middleware обязан быть роутовым: групповые выполняются раньше
// и действия не увидят. Неразмеченный роут отклоняется — молча пропустить
// запрос без проверки нельзя. Общий отказ движка переводится в ошибку
// области; ошибку, назначенную движком, клиент получает как есть.
func (s *Services) permissionMiddleware(forbidden apierrors.DefinedError) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			action, ok := ActionOf(c)
			if !ok {
				return EErrorDefined(c, forbidden)
			}

			apiContext := apicontext.GetContext(c)
			if apiContext == nil {
				return EError(c, errors.New("wrong context"))
			}

			if err := s.policy.Authorize(c.Request().Context(), action, apiContext); err != nil {
				if errors.Is(err, apierrors.ErrIssueForbidden) {
					return EErrorDefined(c, forbidden)
				}
				return EError(c, err)
			}
			return next(c)
		}
	}
}

// withAction кладёт действие роута в контекст запроса.
func withAction(action engine.Action) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set(actionContextKey, action)
			return next(c)
		}
	}
}

// ActionOf возвращает действие текущего роута.
// Второе значение false — роут не размечен: вызывающий обязан отказать
// в доступе, а не пропускать запрос.
func ActionOf(c echo.Context) (engine.Action, bool) {
	a, ok := c.Get(actionContextKey).(engine.Action)
	return a, ok
}

// issueScopePrefix — префикс роутов, работающих с конкретной задачей.
const issueScopePrefix = "/issues/:issueIdOrSeq"

// workspaceScopePrefix — префикс всех роутов, живущих внутри пространства.
// Каждый из них обязан быть размечен действием.
const workspaceScopePrefix = "workspaces/:workspaceSlug"

// httpMethods — методы, которые обслуживают обработчики.
//
// echo добавляет группам служебные записи с псевдометодом
// (echo_route_not_found — обработчик 404 группы). Они не являются
// обработчиками и действий не имеют.
var httpMethods = map[string]struct{}{
	http.MethodGet:    {},
	http.MethodPost:   {},
	http.MethodPut:    {},
	http.MethodPatch:  {},
	http.MethodDelete: {},
}

// unmappedIssueRoutes возвращает issue-роуты, зарегистрированные в обход route.
func (s *Services) unmappedIssueRoutes(e *echo.Echo) []string {
	return s.unmappedRoutes(e, issueScopePrefix)
}

// unmappedRoutes возвращает роуты с данным фрагментом пути, зарегистрированные
// в обход route.
func (s *Services) unmappedRoutes(e *echo.Echo, pathPart string) []string {
	var missing []string
	for _, r := range e.Routes() {
		if _, ok := httpMethods[r.Method]; !ok {
			continue
		}
		if !strings.Contains(r.Path, pathPart) {
			continue
		}
		key := r.Method + " " + r.Path
		if _, ok := s.mappedRoutes[key]; !ok {
			missing = append(missing, key)
		}
	}
	sort.Strings(missing)
	return missing
}

// checkRouteActions проверяет полноту разметки роутов пространства (задачи,
// проекты, спринты, документы, формы, бэкапы) и освобождает вспомогательный
// набор: дальше он не нужен.
//
// Роут без разметки не получит действия в контексте, то есть окажется без
// проверки прав. Такую ошибку нельзя откладывать до прода — сервер не
// должен стартовать.
func (s *Services) checkRouteActions(e *echo.Echo) error {
	missing := s.unmappedRoutes(e, workspaceScopePrefix)
	s.mappedRoutes = nil
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("routes without action (%d): %s", len(missing), strings.Join(missing, ", "))
}
