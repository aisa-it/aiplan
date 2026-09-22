package server

import (
	"context"
	"flag"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	apicontext "github.com/aisa-it/aiplan/aiplan.go/pkg/api-context"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/config"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/dao"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/engine"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/engine/defaultengine"
	filestorage "github.com/aisa-it/aiplan/aiplan.go/pkg/file-storage"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/types"
	"github.com/gofrs/uuid"
	"github.com/labstack/echo/v4"
)

// Тест эквивалентности прав на задачу: прежняя проверка по методу и пути
// (hasIssuePermissions) и решение движка по действию (Authorize) обязаны
// совпадать на всех реальных роутах задачи и всех значимых комбинациях
// входных данных. БД не нужна — сущности подставляются в apicontext.

// authzRoute — реальный роут задачи с его действием.
// updateGolden перезаписывает эталонную таблицу решений.
var updateGolden = flag.Bool("update", false, "обновить эталон решений")

type authzRoute struct {
	method string
	path   string
	action engine.Action
}

// issueRoutesWithActions поднимает настоящую регистрацию issue-роутов и
// возвращает соответствие «метод + путь → действие» так, как его задаёт код.
func issueRoutesWithActions(t *testing.T) []authzRoute {
	t.Helper()

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
	s.AddIssueServices(e.Group("/api/auth/"))

	if missing := s.unmappedIssueRoutes(e); len(missing) > 0 {
		t.Fatalf("роуты задачи без действия (%d): %v", len(missing), missing)
	}

	routes := make([]authzRoute, 0, len(s.mappedRoutes))
	for key, action := range s.mappedRoutes {
		method, path, ok := strings.Cut(key, " ")
		if !ok {
			t.Fatalf("некорректный ключ роута: %q", key)
		}
		// Проверка прав задачи навешивается только на роуты её скоупа:
		// поиск и экспорт списка живут на группе проекта.
		if !strings.Contains(path, issueScopePrefix) {
			continue
		}
		routes = append(routes, authzRoute{method: method, path: path, action: action})
	}
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].path == routes[j].path {
			return routes[i].method < routes[j].method
		}
		return routes[i].path < routes[j].path
	})
	return routes
}

// authzCase — комбинация входных данных проверки.
type authzCase struct {
	wsRole   int
	prRole   int
	author   bool
	assignee bool
	attach   bool // project.MemberAttachmentsAllowed
	props    bool // project.MemberPropertiesAllowed
}

func (tc authzCase) String() string {
	return fmt.Sprintf("ws=%d pr=%d author=%v assignee=%v attach=%v props=%v",
		tc.wsRole, tc.prRole, tc.author, tc.assignee, tc.attach, tc.props)
}

// authzCases — все значимые комбинации ролей, авторства и настроек проекта.
func authzCases() []authzCase {
	roles := []int{types.GuestRole, types.MemberRole, types.AdminRole}
	flags := []bool{false, true}

	cases := make([]authzCase, 0, len(roles)*len(roles)*len(flags)*len(flags)*len(flags)*len(flags))
	for _, ws := range roles {
		for _, pr := range roles {
			for _, author := range flags {
				for _, assignee := range flags {
					for _, attach := range flags {
						for _, props := range flags {
							cases = append(cases, authzCase{
								wsRole: ws, prRole: pr,
								author: author, assignee: assignee,
								attach: attach, props: props,
							})
						}
					}
				}
			}
		}
	}
	return cases
}

// newAuthzSubject собирает Subject с подставленными сущностями по описанию
// случая: ролями, авторством, назначением и настройками проекта.
func newAuthzSubject(r authzRoute, tc authzCase) *apicontext.APIContext {
	req := httptest.NewRequest(r.method, "/", nil)
	c := echo.New().NewContext(req, httptest.NewRecorder())
	c.SetPath(r.path)

	issue := &dao.Issue{ID: permIssueID, CreatedById: permOtherID}
	if tc.author {
		issue.CreatedById = permUserID
	}
	assignees := make([]dao.User, 0, 1)
	if tc.assignee {
		assignees = append(assignees, dao.User{ID: permUserID})
		issue.AssigneeIDs = []uuid.UUID{permUserID}
	}
	issue.Assignees = &assignees

	apiCtx := apicontext.SetPrefilledContext(c, apicontext.Prefilled{
		User:            &dao.User{ID: permUserID},
		Workspace:       &dao.Workspace{ID: permWsID},
		WorkspaceMember: &dao.WorkspaceMember{ID: permMemberID, Role: tc.wsRole, MemberId: permUserID},
		Project: &dao.Project{
			ID:                       permProjID,
			MemberAttachmentsAllowed: tc.attach,
			MemberPropertiesAllowed:  tc.props,
		},
		ProjectMember: &dao.ProjectMember{ID: permMemberID, Role: tc.prRole, MemberId: permUserID},
		Issue:         issue,
	})
	return apiCtx
}

// TestAuthzEquivalence сравнивает вердикты старой и новой реализаций
// на всех роутах задачи и всех комбинациях входных данных.
// authzGoldenPath — эталонная таблица решений движка ядра.
const authzGoldenPath = "testdata/authz_matrix.golden"

// TestAuthzMatrix сверяет решения движка ядра с эталонной таблицей.
//
// Таблица снята с поведения трекера до выноса прав в движок и служит
// защитой от незаметного изменения прав: любой сдвиг обязан быть
// осознанным и попасть в diff эталона.
//
// Обновление эталона: go test ./pkg/server/ -run TestAuthzMatrix -update
func TestAuthzMatrix(t *testing.T) {
	routes := issueRoutesWithActions(t)
	if len(routes) < 40 {
		t.Fatalf("найдено всего %d роутов задачи — похоже на потерю разметки", len(routes))
	}

	eng := defaultengine.New()
	ctx := context.Background()
	cases := authzCases()

	var got strings.Builder
	for _, r := range routes {
		for _, tc := range cases {
			subject := newAuthzSubject(r, tc)
			verdict, err := eng.Authorize(ctx, engine.AuthzRequest{Action: r.action, Subject: subject})
			if err != nil {
				t.Fatalf("%s %s (%s): движок вернул ошибку: %v", r.method, r.path, tc, err)
			}
			if verdict.Decision == engine.DecisionDefault {
				t.Errorf("%s %s [%s] (%s): движок ядра не принял решения",
					r.method, r.path, r.action, tc)
				continue
			}

			fmt.Fprintf(&got, "%s %s [%s] (%s) = %v\n",
				r.method, r.path, r.action, tc, verdict.Decision == engine.DecisionAllow)
		}
	}

	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(authzGoldenPath), 0o755); err != nil {
			t.Fatalf("создание каталога эталонов: %v", err)
		}
		if err := os.WriteFile(authzGoldenPath, []byte(got.String()), 0o600); err != nil {
			t.Fatalf("запись эталона: %v", err)
		}
		t.Logf("эталон обновлён: %s", authzGoldenPath)
		return
	}

	want, err := os.ReadFile(authzGoldenPath)
	if err != nil {
		t.Fatalf("чтение эталона (создать: -update): %v", err)
	}

	gotLines := strings.Split(strings.TrimSpace(got.String()), "\n")
	wantLines := strings.Split(strings.TrimSpace(string(want)), "\n")
	if len(gotLines) != len(wantLines) {
		t.Fatalf("решений %d, в эталоне %d — изменился состав роутов или комбинаций",
			len(gotLines), len(wantLines))
	}

	var diffs int
	for i := range gotLines {
		if gotLines[i] != wantLines[i] {
			diffs++
			if diffs <= 10 {
				t.Errorf("права изменились:\n  было: %s\n  стало: %s", wantLines[i], gotLines[i])
			}
		}
	}
	if diffs > 10 {
		t.Errorf("...и ещё %d расхождений", diffs-10)
	}

	t.Logf("роутов: %d, комбинаций: %d, решений: %d", len(routes), len(cases), len(gotLines))
}

// TestAuthzViewActionsCoverGetRoutes: прежняя проверка разрешала GET
// безусловно, движок — по списку просмотровых действий. Если действие
// GET-роута в этот список не входит, права на чтение сузились.
func TestAuthzViewActionsCoverGetRoutes(t *testing.T) {
	eng := defaultengine.New()
	ctx := context.Background()

	// Заведомо бесправный subject: гость везде, не автор и не исполнитель.
	guest := authzCase{wsRole: types.GuestRole, prRole: types.GuestRole}

	for _, r := range issueRoutesWithActions(t) {
		if r.method != "GET" {
			continue
		}
		subject := newAuthzSubject(r, guest)
		verdict, err := eng.Authorize(ctx, engine.AuthzRequest{Action: r.action, Subject: subject})
		if err != nil {
			t.Fatalf("%s %s: движок вернул ошибку: %v", r.method, r.path, err)
		}
		if verdict.Decision != engine.DecisionAllow {
			t.Errorf("GET %s [%s]: движок не считает действие просмотровым (вердикт %v), раньше GET разрешался безусловно",
				r.path, r.action, verdict.Decision)
		}
	}
}
