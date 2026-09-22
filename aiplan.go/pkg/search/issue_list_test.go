package search

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	apicontext "github.com/aisa-it/aiplan/aiplan.go/pkg/api-context"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/dao"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/engine"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/engine/defaultengine"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/policy"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/types"
	"github.com/gofrs/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Характеризационные (golden) тесты запроса поиска задач.
//
// Назначение: зафиксировать текущий SQL до выноса правила видимости в интерфейс
// движка. Тест падает при ЛЮБОМ изменении генерируемого SQL — если изменение
// осознанное, эталон перегенерируется флагом -update.
//
// Главное, что охраняется: ручка поиска доступна любому участнику, и
// разграничение доступа к чужим задачам держится исключительно на WHERE
// внутри buildSearchQuery. Потеря этого условия = тихая утечка чужих задач,
// без ошибки в логах. Поэтому поверх сравнения полного SQL есть отдельная
// проверка наличия ограничения видимости.
//
// БД не нужна: gorm работает в DryRun, соединение не открывается.

var updateGolden = flag.Bool("update", false, "перегенерировать эталонный SQL")

const goldenPath = "testdata/issue_list_query.golden"

// Фиксированные идентификаторы: SQL и список аргументов должны быть детерминированы.
var (
	testUserID      = uuid.FromStringOrNil("11111111-1111-1111-1111-111111111111")
	testWorkspaceID = uuid.FromStringOrNil("22222222-2222-2222-2222-222222222222")
	testProjectID   = uuid.FromStringOrNil("33333333-3333-3333-3333-333333333333")
	testSprintID    = uuid.FromStringOrNil("44444444-4444-4444-4444-444444444444")
	testIssueID     = uuid.FromStringOrNil("55555555-5555-5555-5555-555555555555")
	testStateID     = uuid.FromStringOrNil("66666666-6666-6666-6666-666666666666")
	testAssigneeID  = uuid.FromStringOrNil("77777777-7777-7777-7777-777777777777")
	testLabelID     = uuid.FromStringOrNil("88888888-8888-8888-8888-888888888888")
)

// dryRunDB — gorm с postgres-диалектором без живого соединения.
func dryRunDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN:                  "",
		DriverName:           "",
		WithoutQuotingCheck:  true,
		PreferSimpleProtocol: true,
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("не удалось создать DryRun-соединение: %v", err)
	}
	return db
}

func testUser(superuser bool) dao.User {
	return dao.User{ID: testUserID, IsSuperuser: superuser}
}

func projectMemberIn() *dao.ProjectMember {
	return &dao.ProjectMember{ProjectId: testProjectID, WorkspaceId: testWorkspaceID, MemberId: testUserID}
}

// testSearcher — поиск с политикой видимости в том же виде, что и в сервере:
// применитель поверх движка ядра.
func testSearcher() *Searcher {
	return New(policy.New(nil, defaultengine.New()))
}

func (c searchCase) scope(params *types.SearchParams) engine.IssueScope {
	user := c.user
	return engine.IssueScope{
		Subject: apicontext.NewSubject(apicontext.Prefilled{
			User:          &user,
			ProjectMember: c.projectMember,
			Sprint:        c.sprint,
		}),
		Kind:   c.kind,
		Params: params,
	}
}

func testSprint() *dao.Sprint {
	return &dao.Sprint{Id: testSprintID, Issues: []dao.Issue{{ID: testIssueID}}}
}

// kind по умолчанию — ScopeGlobal; проектный и спринтовый режимы задаются явно.
type searchCase struct {
	name          string
	user          dao.User
	projectMember *dao.ProjectMember
	sprint        *dao.Sprint
	kind          engine.ScopeKind
	params        types.SearchParams
}

//nolint:funlen // табличный перечень кейсов
func searchCases() []searchCase {
	return []searchCase{
		{
			name:   "global/без_проекта/пусто",
			user:   testUser(false),
			params: types.SearchParams{},
		},
		{
			name:   "global/без_проекта/light",
			user:   testUser(false),
			params: types.SearchParams{LightSearch: true},
		},
		{
			name:   "global/без_проекта/only_count",
			user:   testUser(false),
			params: types.SearchParams{OnlyCount: true},
		},
		{
			name:   "global/без_проекта/суперпользователь",
			user:   testUser(true),
			params: types.SearchParams{},
		},
		{
			// Глобальный режим игнорирует членство в контексте:
			// видимость всё равно считается по project_members.
			name: "global/С_проектом/пусто",
			user: testUser(false), projectMember: projectMemberIn(),
			params: types.SearchParams{},
		},
		{
			name: "global/без_проекта/фильтр_авторы",
			user: testUser(false),
			params: types.SearchParams{Filters: types.IssuesListFilters{
				AuthorIds: []string{testUserID.String()},
			}},
		},
		{
			name: "global/без_проекта/фильтр_исполнители",
			user: testUser(false),
			params: types.SearchParams{Filters: types.IssuesListFilters{
				AssigneeIds: types.FilterUUIDs{Array: []uuid.UUID{testAssigneeID}},
			}},
		},
		{
			name: "global/без_проекта/фильтр_исполнители_с_пустыми",
			user: testUser(false),
			params: types.SearchParams{Filters: types.IssuesListFilters{
				AssigneeIds: types.FilterUUIDs{Array: []uuid.UUID{testAssigneeID}, IncludeEmpty: true},
			}},
		},
		{
			name: "global/без_проекта/фильтр_статусы+only_active",
			user: testUser(false),
			params: types.SearchParams{OnlyActive: true, Filters: types.IssuesListFilters{
				StateIds: []uuid.UUID{testStateID},
			}},
		},
		{
			name: "global/без_проекта/фильтр_метки",
			user: testUser(false),
			params: types.SearchParams{Filters: types.IssuesListFilters{
				Labels: types.FilterUUIDs{Array: []uuid.UUID{testLabelID}},
			}},
		},
		{
			name: "global/без_проекта/фильтр_пространства",
			user: testUser(false),
			params: types.SearchParams{Filters: types.IssuesListFilters{
				WorkspaceIds: []string{testWorkspaceID.String()},
			}},
		},
		{
			name: "global/без_проекта/фильтр_слаги_пространств",
			user: testUser(false),
			params: types.SearchParams{Filters: types.IssuesListFilters{
				WorkspaceSlugs: []string{"ws"},
			}},
		},
		{
			name: "global/без_проекта/фильтр_проекты",
			user: testUser(false),
			params: types.SearchParams{Filters: types.IssuesListFilters{
				ProjectIds: []string{testProjectID.String()},
			}},
		},
		{
			name: "global/без_проекта/мои_задачи",
			user: testUser(false),
			params: types.SearchParams{Filters: types.IssuesListFilters{
				AssignedToMe: true, WatchedByMe: true, AuthoredByMe: true,
			}},
		},
		{
			name: "global/без_проекта/спринт",
			user: testUser(false), sprint: testSprint(), kind: engine.ScopeSprint,
			params: types.SearchParams{},
		},
		{
			name: "global/без_проекта/полнотекстовый_поиск",
			user: testUser(false),
			params: types.SearchParams{OrderByParam: "search_rank", Filters: types.IssuesListFilters{
				SearchQuery: "тест",
			}},
		},
		{
			name: "проект/пусто",
			user: testUser(false), projectMember: projectMemberIn(), kind: engine.ScopeProject,
			params: types.SearchParams{},
		},
		{
			name: "проект/light",
			user: testUser(false), projectMember: projectMemberIn(), kind: engine.ScopeProject,
			params: types.SearchParams{LightSearch: true},
		},
		{
			name: "проект/only_count",
			user: testUser(false), projectMember: projectMemberIn(), kind: engine.ScopeProject,
			params: types.SearchParams{OnlyCount: true},
		},
		{
			name: "проект/фильтр_авторы+исполнители",
			user: testUser(false), projectMember: projectMemberIn(), kind: engine.ScopeProject,
			params: types.SearchParams{Filters: types.IssuesListFilters{
				AuthorIds:   []string{testUserID.String()},
				AssigneeIds: types.FilterUUIDs{Array: []uuid.UUID{testAssigneeID}},
			}},
		},
		{
			name: "проект/сортировка_по_статусу_desc",
			user: testUser(false), projectMember: projectMemberIn(), kind: engine.ScopeProject,
			params: types.SearchParams{OrderByParam: "-state", Desc: true},
		},
		{
			name: "проект/сортировка_по_исполнителям",
			user: testUser(false), projectMember: projectMemberIn(), kind: engine.ScopeProject,
			params: types.SearchParams{OrderByParam: "assignees"},
		},
		{
			name: "проект/скрыть_подзадачи+черновики+закреплённые",
			user: testUser(false), projectMember: projectMemberIn(), kind: engine.ScopeProject,
			params: types.SearchParams{HideSubIssues: true, Draft: true, OnlyPinned: true},
		},
		{
			// Проектный режим без участника в контексте. Движок обязан свалиться
			// в подзапрос по project_members, иначе видимость теряется.
			name:   "не_global/без_проекта/пусто",
			user:   testUser(false),
			kind:   engine.ScopeProject,
			params: types.SearchParams{},
		},
		{
			name: "не_global/без_проекта/фильтр_статусы",
			user: testUser(false),
			kind: engine.ScopeProject,
			params: types.SearchParams{Filters: types.IssuesListFilters{
				StateIds: []uuid.UUID{testStateID},
			}},
		},
	}
}

// Ограничение видимости: либо проектный режим (issues.project_id = $N),
// либо подзапрос по project_members текущего пользователя.
var (
	projectScopeRe = regexp.MustCompile(`issues\.project_id = \$\d+`)
	memberScopeRe  = regexp.MustCompile(`issues\.project_id in \(SELECT project_id FROM project_members WHERE member_id = \$\d+\)`)
)

func renderQuery(t *testing.T, c searchCase) (string, []any) {
	t.Helper()
	db := dryRunDB(t)
	params := c.params
	q := testSearcher().buildSearchQuery(context.Background(), db.Session(&gorm.Session{DryRun: true, NewDB: true}), c.scope(&params))
	if params.OnlyCount {
		var count int64
		q.Count(&count)
	} else {
		var issues []dao.Issue
		q.Find(&issues)
	}
	return q.Statement.SQL.String(), q.Statement.Vars
}

// TestBuildSearchQueryVisibility — ключевая проверка: ни один набор параметров
// не должен давать запрос без ограничения видимости задач.
func TestBuildSearchQueryVisibility(t *testing.T) {
	for _, c := range searchCases() {
		t.Run(c.name, func(t *testing.T) {
			sql, _ := renderQuery(t, c)
			byProject := projectScopeRe.MatchString(sql)
			byMember := memberScopeRe.MatchString(sql)
			if !byProject && !byMember {
				t.Fatalf("ПОТЕРЯНО ОГРАНИЧЕНИЕ ВИДИМОСТИ ЗАДАЧ: в SQL нет ни фильтра по проекту, "+
					"ни подзапроса по project_members — поиск вернёт чужие задачи.\nSQL: %s", sql)
			}
			// Проектный фильтр только в проектном режиме с участником в контексте.
			wantProject := c.kind == engine.ScopeProject && c.projectMember != nil
			if wantProject && !byProject {
				t.Fatalf("ожидался проектный фильтр видимости, получено: %s", sql)
			}
			if !wantProject && !byMember {
				t.Fatalf("ожидался подзапрос по project_members, получено: %s", sql)
			}
		})
	}
}

// TestBuildSearchQueryGolden — полное сравнение SQL и аргументов с эталоном.
func TestBuildSearchQueryGolden(t *testing.T) {
	var b strings.Builder
	for _, c := range searchCases() {
		sql, vars := renderQuery(t, c)
		fmt.Fprintf(&b, "=== %s\nSQL:  %s\nVARS: %s\n\n", c.name, sql, formatVars(vars))
	}
	got := b.String()

	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenPath, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Logf("эталон перезаписан: %s", goldenPath)
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("нет эталона %s (создать: go test ./pkg/search/ -run Golden -update): %v", goldenPath, err)
	}
	if got != string(want) {
		t.Errorf("SQL поиска изменился относительно эталона.\n"+
			"Если изменение осознанное — проверить, что ограничение видимости на месте, "+
			"и обновить: go test ./pkg/search/ -run Golden -update\n\n--- эталон ---\n%s\n--- получено ---\n%s",
			string(want), got)
	}
}

func formatVars(vars []any) string {
	parts := make([]string, 0, len(vars))
	for _, v := range vars {
		parts = append(parts, fmt.Sprintf("%v", v))
	}
	return "[" + strings.Join(parts, " ") + "]"
}

// --- memberProjectsQuery: сужение видимых проектов для подсчёта групп ---

func renderMemberProjects(t *testing.T, params types.SearchParams) (string, []any) {
	t.Helper()
	db := dryRunDB(t).Session(&gorm.Session{DryRun: true, NewDB: true})
	c := searchCase{user: testUser(false)}
	s := testSearcher()
	scope := c.scope(&params)
	q := memberProjectsQuery(s.visibility.VisibleProjects(context.Background(), scope, db), db, &params)
	var out []dao.ProjectMember
	q.Find(&out)
	return q.Statement.SQL.String(), q.Statement.Vars
}

func TestMemberProjectsQueryGolden(t *testing.T) {
	cases := []struct {
		name   string
		params types.SearchParams
		want   string
	}{
		{
			name:   "без_фильтров",
			params: types.SearchParams{},
			want:   "SELECT project_id FROM project_members WHERE member_id = $1",
		},
		{
			name: "фильтр_пространства",
			params: types.SearchParams{Filters: types.IssuesListFilters{
				WorkspaceIds: []string{testWorkspaceID.String()},
			}},
			want: "SELECT project_id FROM project_members WHERE member_id = $1 AND project_id in (SELECT id FROM projects WHERE workspace_id in ($2) AND projects.deleted_at IS NULL)",
		},
		{
			name: "фильтр_слаги",
			params: types.SearchParams{Filters: types.IssuesListFilters{
				WorkspaceSlugs: []string{"ws"},
			}},
			want: "SELECT project_id FROM project_members WHERE member_id = $1 AND project_id in (SELECT id FROM projects WHERE workspace_id in (SELECT id FROM workspaces WHERE slug in ($2) AND workspaces.deleted_at IS NULL) AND projects.deleted_at IS NULL)",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sql, vars := renderMemberProjects(t, c.params)
			if sql != c.want {
				t.Errorf("SQL memberProjectsQuery изменился.\nожидалось: %s\nполучено:  %s\nvars: %s",
					c.want, sql, formatVars(vars))
			}
			if !strings.Contains(sql, "member_id = $1") {
				t.Fatalf("ПОТЕРЯНО ОГРАНИЧЕНИЕ ВИДИМОСТИ: memberProjectsQuery не фильтрует по member_id.\nSQL: %s", sql)
			}
		})
	}
}

// TestVisibilityRulesEquivalent — buildSearchQuery и memberProjectsQuery опираются
// на один подзапрос VisibleProjects движка; тест сторожит, что это так и осталось.
func TestVisibilityRulesEquivalent(t *testing.T) {
	sqlSearch, varsSearch := renderQuery(t, searchCase{
		user: testUser(false), params: types.SearchParams{},
	})

	const scope = "SELECT project_id FROM project_members WHERE member_id = $1"
	if !strings.Contains(sqlSearch, "issues.project_id in ("+scope+")") {
		t.Fatalf("подзапрос видимости в buildSearchQuery разошёлся с ожидаемым %q.\nSQL: %s", scope, sqlSearch)
	}
	if len(varsSearch) == 0 || fmt.Sprintf("%v", varsSearch[0]) != testUserID.String() {
		t.Fatalf("первым аргументом подзапроса видимости должен быть id текущего пользователя, получено: %s",
			formatVars(varsSearch))
	}

	sqlMember, varsMember := renderMemberProjects(t, types.SearchParams{})
	if sqlMember != scope {
		t.Fatalf("memberProjectsQuery разошёлся с подзапросом buildSearchQuery.\n"+
			"buildSearchQuery: %s\nmemberProjectsQuery: %s", scope, sqlMember)
	}
	if fmt.Sprintf("%v", varsMember[0]) != testUserID.String() {
		t.Fatalf("memberProjectsQuery фильтрует не по текущему пользователю: %s", formatVars(varsMember))
	}
}
