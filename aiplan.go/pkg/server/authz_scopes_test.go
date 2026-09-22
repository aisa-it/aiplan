package server

import (
	"context"
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
	"github.com/aisa-it/aiplan/aiplan.go/pkg/types"
	"github.com/gofrs/uuid"
	"github.com/labstack/echo/v4"
)

// Таблица решений движка ядра по правам на проект, пространство, спринт,
// документ и форму. Снята с прежних проверок по методу и пути роута
// (has*Permissions) перед их удалением; любой сдвиг обязан быть осознанным
// и попасть в diff эталона.
//
// Обновление эталона: go test ./pkg/server/ -run TestAuthzScopesMatrix -update

const authzScopesGoldenPath = "testdata/authz_scopes.golden"

var (
	scopeDocID  = uuid.Must(uuid.NewV4())
	scopeFormID = uuid.Must(uuid.NewV4())
)

// scopedRoutesWithActions поднимает регистрацию роутов пространства, проекта,
// спринта, документов, форм и бэкапов и возвращает «метод + путь → действие».
func scopedRoutesWithActions(t *testing.T) []authzRoute {
	t.Helper()

	webURL, err := url.Parse("http://localhost:8080")
	if err != nil {
		t.Fatalf("разбор URL: %v", err)
	}
	prev := cfg
	cfg = &config.Config{WebURL: types.JsonURL{URL: webURL}}
	defer func() { cfg = prev }()

	e := echo.New()
	s := &Services{cfg: cfg}
	g := e.Group("/api/auth/")
	s.AddWorkspaceServices(g)
	s.AddProjectServices(g)
	s.AddSprintServices(g)
	s.AddDocServices(g)
	s.AddFormServices(g)
	s.AddBackupServices(g)

	if missing := s.unmappedRoutes(e, workspaceScopePrefix); len(missing) > 0 {
		t.Fatalf("роуты пространства без действия (%d): %v", len(missing), missing)
	}

	routes := make([]authzRoute, 0, len(s.mappedRoutes))
	for key, action := range s.mappedRoutes {
		method, path, ok := strings.Cut(key, " ")
		if !ok {
			t.Fatalf("некорректный ключ роута: %q", key)
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

// routeArea — область роута по его пути.
func routeArea(path string) string {
	switch {
	case strings.Contains(path, "/projects/:projectId"):
		return "project"
	case strings.Contains(path, "/sprints/:sprintId"):
		return "sprint"
	case strings.Contains(path, "/doc/:docId"):
		return "doc"
	case strings.Contains(path, "/forms/:formSlug"):
		return "form"
	default:
		return "workspace"
	}
}

// scopeCase — комбинация входных данных; для каждой области перебираются
// только значимые для неё измерения.
type scopeCase struct {
	wsRole, prRole                   int
	owner, lead, superuser, author   bool
	readerRole, editorRole           int
	inReaders, inEditors, inWatchers bool
}

func (c scopeCase) String() string {
	return fmt.Sprintf("ws=%d pr=%d owner=%v lead=%v su=%v author=%v reader=%d editor=%d inR=%v inE=%v inW=%v",
		c.wsRole, c.prRole, c.owner, c.lead, c.superuser, c.author,
		c.readerRole, c.editorRole, c.inReaders, c.inEditors, c.inWatchers)
}

var (
	scopeRoles = []int{types.GuestRole, types.MemberRole, types.AdminRole}
	scopeFlags = []bool{false, true}
)

func scopeCases(area string) []scopeCase {
	var cases []scopeCase
	base := scopeCase{prRole: types.MemberRole, readerRole: types.GuestRole, editorRole: types.MemberRole}
	for _, ws := range scopeRoles {
		c := base
		c.wsRole = ws
		switch area {
		case "workspace":
			for _, owner := range scopeFlags {
				for _, su := range scopeFlags {
					c.owner, c.superuser = owner, su
					cases = append(cases, c)
				}
			}
		case "project":
			for _, pr := range scopeRoles {
				for _, lead := range scopeFlags {
					c.prRole, c.lead = pr, lead
					cases = append(cases, c)
				}
			}
		case "sprint":
			for _, owner := range scopeFlags {
				for _, su := range scopeFlags {
					for _, author := range scopeFlags {
						c.owner, c.superuser, c.author = owner, su, author
						cases = append(cases, c)
					}
				}
			}
		case "doc":
			for _, su := range scopeFlags {
				for _, author := range scopeFlags {
					for _, reader := range []int{types.GuestRole, types.AdminRole} {
						for _, editor := range []int{types.MemberRole, types.AdminRole} {
							for _, inR := range scopeFlags {
								for _, inE := range scopeFlags {
									for _, inW := range scopeFlags {
										c.superuser, c.author = su, author
										c.readerRole, c.editorRole = reader, editor
										c.inReaders, c.inEditors, c.inWatchers = inR, inE, inW
										cases = append(cases, c)
									}
								}
							}
						}
					}
				}
			}
		case "form":
			for _, owner := range scopeFlags {
				c.owner = owner
				cases = append(cases, c)
			}
		}
	}
	return cases
}

// newScopeSubject собирает Subject по описанию случая.
func newScopeSubject(r authzRoute, tc scopeCase) *apicontext.APIContext {
	req := httptest.NewRequest(r.method, "/", nil)
	c := echo.New().NewContext(req, httptest.NewRecorder())
	c.SetPath(r.path)

	ownerID, leadID, authorID := permOtherID, permOtherID, permOtherID
	if tc.owner {
		ownerID = permUserID
	}
	if tc.lead {
		leadID = permUserID
	}
	if tc.author {
		authorID = permUserID
	}
	listWith := func(in bool) []uuid.UUID {
		if in {
			return []uuid.UUID{permUserID}
		}
		return nil
	}

	return apicontext.SetPrefilledContext(c, apicontext.Prefilled{
		User:            &dao.User{ID: permUserID, IsSuperuser: tc.superuser},
		Workspace:       &dao.Workspace{ID: permWsID, OwnerId: ownerID},
		WorkspaceMember: &dao.WorkspaceMember{ID: permMemberID, Role: tc.wsRole, MemberId: permUserID},
		Project:         &dao.Project{ID: permProjID, ProjectLeadId: leadID},
		ProjectMember:   &dao.ProjectMember{ID: permMemberID, Role: tc.prRole, MemberId: permUserID},
		Sprint:          &dao.Sprint{Id: permIssueID, CreatedById: authorID},
		Doc: &dao.Doc{
			ID: scopeDocID, CreatedById: authorID,
			ReaderRole: tc.readerRole, EditorRole: tc.editorRole,
			ReaderIDs: listWith(tc.inReaders), EditorsIDs: listWith(tc.inEditors), WatcherIDs: listWith(tc.inWatchers),
		},
		Form: &dao.Form{ID: scopeFormID, CreatedById: authorID},
	})
}

func TestAuthzScopesMatrix(t *testing.T) {
	routes := scopedRoutesWithActions(t)
	if len(routes) < 140 {
		t.Fatalf("найдено всего %d роутов пространства — похоже на потерю разметки", len(routes))
	}

	eng := defaultengine.New()
	ctx := context.Background()

	var got strings.Builder
	for _, r := range routes {
		for _, tc := range scopeCases(routeArea(r.path)) {
			subject := newScopeSubject(r, tc)
			verdict, err := eng.Authorize(ctx, engine.AuthzRequest{Action: r.action, Subject: subject})
			if err != nil {
				t.Fatalf("%s %s (%s): движок вернул ошибку: %v", r.method, r.path, tc, err)
			}
			if verdict.Decision == engine.DecisionDefault {
				t.Errorf("%s %s [%s] (%s): движок ядра не принял решения", r.method, r.path, r.action, tc)
				continue
			}
			fmt.Fprintf(&got, "%s %s [%s] (%s) = %v\n",
				r.method, r.path, r.action, tc, verdict.Decision == engine.DecisionAllow)
		}
	}

	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(authzScopesGoldenPath), 0o755); err != nil {
			t.Fatalf("создание каталога эталонов: %v", err)
		}
		if err := os.WriteFile(authzScopesGoldenPath, []byte(got.String()), 0o600); err != nil {
			t.Fatalf("запись эталона: %v", err)
		}
		t.Logf("эталон обновлён: %s", authzScopesGoldenPath)
		return
	}

	want, err := os.ReadFile(authzScopesGoldenPath)
	if err != nil {
		t.Fatalf("чтение эталона (создать: -update): %v", err)
	}
	gotLines := strings.Split(strings.TrimSpace(got.String()), "\n")
	wantLines := strings.Split(strings.TrimSpace(string(want)), "\n")
	if len(gotLines) != len(wantLines) {
		t.Fatalf("решений %d, в эталоне %d — изменился состав роутов или комбинаций", len(gotLines), len(wantLines))
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
	t.Logf("роутов: %d, решений: %d", len(routes), len(gotLines))
}
