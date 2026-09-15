package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/config"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/engine"
	filestorage "github.com/aisa-it/aiplan/aiplan.go/pkg/file-storage"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/types"
	"github.com/labstack/echo/v4"
)

// noop — обработчик-заглушка для регистрации роутов в тестах.
func noop(c echo.Context) error { return nil }

// newTestServices — Services без зависимостей, для проверки разметки роутов.
func newTestServices() *Services { return &Services{} }

// TestRouteSetsActionInContext: обработчик получает действие своего роута
// из контекста запроса.
func TestRouteSetsActionInContext(t *testing.T) {
	e := echo.New()
	s := newTestServices()

	api := e.Group("/api/")
	auth := api.Group("auth/")
	issues := auth.Group("workspaces/:workspaceSlug/projects/:projectId/issues/:issueIdOrSeq")

	var gotAction engine.Action
	var gotOK bool
	capture := func(c echo.Context) error {
		gotAction, gotOK = ActionOf(c)
		return c.NoContent(http.StatusOK)
	}

	s.route(issues, http.MethodPatch, "/", engine.ActionIssueUpdate, capture)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch,
		"/api/auth/workspaces/ws/projects/p1/issues/42/", nil)
	e.ServeHTTP(rec, req)

	if !gotOK {
		t.Fatal("обработчик не получил действие из контекста")
	}
	if gotAction != engine.ActionIssueUpdate {
		t.Errorf("действие %q, ожидалось %q", gotAction, engine.ActionIssueUpdate)
	}
}

// TestActionOfUnmappedRoute: роут без разметки не выдаёт действие
// по умолчанию — вызывающий обязан отказать, а не пропустить запрос.
func TestActionOfUnmappedRoute(t *testing.T) {
	e := echo.New()

	var ok bool
	e.GET("/plain/", func(c echo.Context) error {
		_, ok = ActionOf(c)
		return c.NoContent(http.StatusOK)
	})

	e.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/plain/", nil))

	if ok {
		t.Fatal("для неразмеченного роута вернулось действие")
	}
}

// TestCheckRouteActionsFindsUnmapped: роут задачи, зарегистрированный в обход
// s.route, обязан быть найден проверкой и уронить старт сервера.
func TestCheckRouteActionsFindsUnmapped(t *testing.T) {
	e := echo.New()
	s := newTestServices()

	issues := e.Group("/api/auth/workspaces/:workspaceSlug/projects/:projectId/issues/:issueIdOrSeq")
	s.route(issues, http.MethodGet, "/", engine.ActionIssueView, noop)
	// Регистрация напрямую — забытая разметка.
	issues.POST("/danger/", noop)

	missing := s.unmappedIssueRoutes(e)
	if len(missing) != 1 {
		t.Fatalf("ожидался 1 неразмеченный роут, получено %d: %v", len(missing), missing)
	}
	if !strings.Contains(missing[0], "/danger/") {
		t.Errorf("найден не тот роут: %s", missing[0])
	}

	err := s.checkRouteActions(e)
	if err == nil {
		t.Fatal("checkRouteActions не вернула ошибку при неразмеченном роуте")
	}
	if !strings.Contains(err.Error(), "/danger/") {
		t.Errorf("ошибка не называет проблемный роут: %v", err)
	}
}

// TestCheckRouteActionsIgnoresNonIssue: роуты вне скоупа задачи проверку не
// затрагивают — разметка обязательна пока только для них.
// TestCheckRouteActionsCoversWorkspaceScope: проверка полноты разметки
// покрывает все роуты внутри пространства (проекты, спринты, документы,
// формы), а роуты вне него не трогает.
func TestCheckRouteActionsCoversWorkspaceScope(t *testing.T) {
	e := echo.New()
	s := newTestServices()
	e.POST("/api/auth/users/me/", noop)
	if err := s.checkRouteActions(e); err != nil {
		t.Fatalf("проверка сработала на роуте вне пространства: %v", err)
	}

	e = echo.New()
	s = newTestServices()
	e.GET("/api/auth/workspaces/:workspaceSlug/projects/:projectId/", noop)
	if err := s.checkRouteActions(e); err == nil {
		t.Fatal("неразмеченный роут проекта прошёл проверку")
	}
}

// TestAddIssueServicesMapsEveryRoute поднимает настоящую регистрацию
// issue-роутов и проверяет, что каждый размечен действием.
//
// Boot-check в New делает то же самое, но только при запуске с базой;
// этот тест ловит забытую разметку сразу, без окружения.
func TestAddIssueServicesMapsEveryRoute(t *testing.T) {
	storage, err := filestorage.NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatalf("локальное хранилище: %v", err)
	}

	webURL, err := url.Parse("http://localhost:8080")
	if err != nil {
		t.Fatalf("разбор URL: %v", err)
	}
	prev := cfg
	cfg = &config.Config{WebURL: types.JsonURL{URL: webURL}}
	defer func() { cfg = prev }()

	e := echo.New()
	s := &Services{storage: storage, cfg: cfg}

	auth := e.Group("/api/auth/")
	s.AddIssueServices(auth)

	if missing := s.unmappedIssueRoutes(e); len(missing) > 0 {
		t.Fatalf("роуты задачи без действия (%d): %v", len(missing), missing)
	}

	// Разметка должна покрывать все роуты группы, а не пару штук.
	if len(s.mappedRoutes) < 40 {
		t.Errorf("размечено всего %d роутов — похоже на потерю разметки", len(s.mappedRoutes))
	}
}
