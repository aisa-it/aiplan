package server

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

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
		s.mappedRoutes = map[string]struct{}{}
	}
	s.mappedRoutes[r.Method+" "+r.Path] = struct{}{}
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
	var missing []string
	for _, r := range e.Routes() {
		if _, ok := httpMethods[r.Method]; !ok {
			continue
		}
		if !strings.Contains(r.Path, issueScopePrefix) {
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

// checkRouteActions проверяет полноту разметки issue-роутов и освобождает
// вспомогательный набор: дальше он не нужен.
//
// Роут без разметки не получит действия в контексте, то есть окажется без
// проверки прав. Такую ошибку нельзя откладывать до прода — сервер не
// должен стартовать.
func (s *Services) checkRouteActions(e *echo.Echo) error {
	missing := s.unmappedIssueRoutes(e)
	s.mappedRoutes = nil
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("routes without action (%d): %s", len(missing), strings.Join(missing, ", "))
}
