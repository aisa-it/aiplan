package server

import (
	"net/http/httptest"
	"testing"

	apicontext "github.com/aisa-it/aiplan/aiplan.go/pkg/api-context"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/dao"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/types"
	"github.com/gofrs/uuid"
	"github.com/labstack/echo/v4"
)

// Характеризационные тесты прав на проект, спринт и пространство.
// БД не нужна — сущности подставляются через apicontext.NewPrefilled.
// Права на задачу проверяются таблицей решений движка (authz_matrix_test.go).

var (
	permUserID   = uuid.Must(uuid.NewV4())
	permOtherID  = uuid.Must(uuid.NewV4())
	permIssueID  = uuid.Must(uuid.NewV4())
	permProjID   = uuid.Must(uuid.NewV4())
	permWsID     = uuid.Must(uuid.NewV4())
	permMemberID = uuid.Must(uuid.NewV4())
)

// hasProjectAdminPermissions: только админ проекта, плюс исключение для настроек уведомлений
func TestHasProjectAdminPermissions(t *testing.T) {
	type adminCase struct {
		name   string
		method string
		path   string
		prRole int
		want   bool
	}

	const projectBase = "/api/auth/workspaces/:workspaceSlug/projects/:projectId"

	cases := []adminCase{
		{name: "админ проекта/POST", method: "POST", path: projectBase + "/states/", prRole: types.AdminRole, want: true},
		{name: "админ проекта/GET", method: "GET", path: projectBase + "/states/", prRole: types.AdminRole, want: true},
		{name: "участник/POST", method: "POST", path: projectBase + "/states/", prRole: types.MemberRole, want: false},
		{name: "участник/GET", method: "GET", path: projectBase + "/states/", prRole: types.MemberRole, want: false},
		{name: "гость/GET", method: "GET", path: projectBase + "/states/", prRole: types.GuestRole, want: false},
		{name: "участник/POST настроек уведомлений", method: "POST", path: projectBase + "/members/me/notifications/", prRole: types.MemberRole, want: true},
		{name: "гость/POST настроек уведомлений", method: "POST", path: projectBase + "/members/me/notifications/", prRole: types.GuestRole, want: true},
		{name: "гость/GET настроек уведомлений", method: "GET", path: projectBase + "/members/me/notifications/", prRole: types.GuestRole, want: false},
	}

	svc := &Services{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "/", nil)
			c := echo.New().NewContext(req, httptest.NewRecorder())
			c.SetPath(tc.path)
			apicontext.SetPrefilledContext(c, apicontext.Prefilled{
				User:            &dao.User{ID: permUserID},
				Workspace:       &dao.Workspace{ID: permWsID},
				WorkspaceMember: &dao.WorkspaceMember{Role: types.MemberRole, MemberId: permUserID},
				Project:         &dao.Project{ID: permProjID},
				ProjectMember:   &dao.ProjectMember{Role: tc.prRole, MemberId: permUserID},
			})

			has, err := svc.hasProjectAdminPermissions(c)
			if err != nil {
				t.Fatalf("неожиданная ошибка: %v", err)
			}
			if has != tc.want {
				t.Errorf("%s %s (pr=%d): получено %v, ожидалось %v", tc.method, tc.path, tc.prRole, has, tc.want)
			}
		})
	}
}

// hasProjectPermissions: safe-методы всем, создание задач от участника,
// изменения — админу проекта, руководителю проекта или админу пространства
func TestHasProjectPermissions(t *testing.T) {
	type projectCase struct {
		name     string
		method   string
		path     string
		wsRole   int
		prRole   int
		projLead bool
		want     bool
	}

	const projectBase = "/api/auth/workspaces/:workspaceSlug/projects/:projectId"

	cases := []projectCase{
		{name: "гость/GET", method: "GET", path: projectBase + "/", wsRole: types.GuestRole, prRole: types.GuestRole, want: true},
		{name: "гость/HEAD", method: "HEAD", path: projectBase + "/", wsRole: types.GuestRole, prRole: types.GuestRole, want: true},
		{name: "гость/POST настроек уведомлений", method: "POST", path: projectBase + "/members/me/notifications/", wsRole: types.GuestRole, prRole: types.GuestRole, want: true},
		{name: "гость/POST поиска задач", method: "POST", path: projectBase + "/issues/search/", wsRole: types.GuestRole, prRole: types.GuestRole, want: true},
		{name: "гость/POST представления проекта", method: "POST", path: projectBase + "/project-views/", wsRole: types.GuestRole, prRole: types.GuestRole, want: true},
		{name: "гость/POST создания задачи", method: "POST", path: projectBase + "/issues/", wsRole: types.GuestRole, prRole: types.GuestRole, want: false},
		{name: "участник/POST создания задачи", method: "POST", path: projectBase + "/issues/", wsRole: types.MemberRole, prRole: types.MemberRole, want: true},
		{name: "админ проекта/POST создания задачи", method: "POST", path: projectBase + "/issues/", wsRole: types.MemberRole, prRole: types.AdminRole, want: true},
		{name: "wsAdmin+гость проекта/POST создания задачи", method: "POST", path: projectBase + "/issues/", wsRole: types.AdminRole, prRole: types.GuestRole, want: true},
		{name: "участник/PATCH проекта", method: "PATCH", path: projectBase + "/", wsRole: types.MemberRole, prRole: types.MemberRole, want: false},
		{name: "админ проекта/PATCH проекта", method: "PATCH", path: projectBase + "/", wsRole: types.MemberRole, prRole: types.AdminRole, want: true},
		{name: "руководитель проекта-участник/PATCH проекта", method: "PATCH", path: projectBase + "/", wsRole: types.MemberRole, prRole: types.MemberRole, projLead: true, want: true},
		{name: "wsAdmin+гость проекта/DELETE проекта", method: "DELETE", path: projectBase + "/", wsRole: types.AdminRole, prRole: types.GuestRole, want: true},
		{name: "гость/DELETE проекта", method: "DELETE", path: projectBase + "/", wsRole: types.GuestRole, prRole: types.GuestRole, want: false},
	}

	svc := &Services{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "/", nil)
			c := echo.New().NewContext(req, httptest.NewRecorder())
			c.SetPath(tc.path)

			project := &dao.Project{ID: permProjID, ProjectLeadId: permOtherID}
			if tc.projLead {
				project.ProjectLeadId = permUserID
			}

			apicontext.SetPrefilledContext(c, apicontext.Prefilled{
				User:            &dao.User{ID: permUserID},
				Workspace:       &dao.Workspace{ID: permWsID},
				WorkspaceMember: &dao.WorkspaceMember{Role: tc.wsRole, MemberId: permUserID},
				Project:         project,
				ProjectMember:   &dao.ProjectMember{Role: tc.prRole, MemberId: permUserID},
			})

			has, err := svc.hasProjectPermissions(c)
			if err != nil {
				t.Fatalf("неожиданная ошибка: %v", err)
			}
			if has != tc.want {
				t.Errorf("%s %s (ws=%d pr=%d lead=%v): получено %v, ожидалось %v",
					tc.method, tc.path, tc.wsRole, tc.prRole, tc.projLead, has, tc.want)
			}
		})
	}
}

// hasSprintPermissions: автор, админ пространства и суперпользователь — всё;
// участник — чтение и отдельные POST-роуты
//
//nolint:funlen // таблица кейсов
func TestHasSprintPermissions(t *testing.T) {
	type sprintCase struct {
		name       string
		method     string
		path       string
		wsRole     int
		author     bool
		superuser  bool
		want       bool
		wantSprint bool // ожидание для hasSprintAdminPermissions
	}

	const sprintBase = "/api/auth/workspaces/:workspaceSlug/sprints/:sprintId"

	cases := []sprintCase{
		{name: "автор-гость/PATCH", method: "PATCH", path: sprintBase + "/", wsRole: types.GuestRole, author: true, want: true, wantSprint: false},
		{name: "wsAdmin/DELETE", method: "DELETE", path: sprintBase + "/", wsRole: types.AdminRole, want: true, wantSprint: true},
		{name: "суперпользователь-гость/PATCH", method: "PATCH", path: sprintBase + "/", wsRole: types.GuestRole, superuser: true, want: true, wantSprint: false},
		{name: "участник/GET", method: "GET", path: sprintBase + "/", wsRole: types.MemberRole, want: true, wantSprint: false},
		{name: "гость/GET", method: "GET", path: sprintBase + "/", wsRole: types.GuestRole, want: false, wantSprint: false},
		{name: "участник/PATCH чужого спринта", method: "PATCH", path: sprintBase + "/", wsRole: types.MemberRole, want: false, wantSprint: false},
		{name: "участник/POST поиска задач", method: "POST", path: sprintBase + "/issues/search/", wsRole: types.MemberRole, want: true, wantSprint: false},
		{name: "гость/POST поиска задач", method: "POST", path: sprintBase + "/issues/search/", wsRole: types.GuestRole, want: false, wantSprint: false},
		{name: "участник/POST представления спринта", method: "POST", path: sprintBase + "/sprint-view/", wsRole: types.MemberRole, want: true, wantSprint: false},
		{name: "гость/POST представления спринта", method: "POST", path: sprintBase + "/sprint-view/", wsRole: types.GuestRole, want: false, wantSprint: false},
	}

	svc := &Services{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			newCtx := func() echo.Context {
				req := httptest.NewRequest(tc.method, "/", nil)
				c := echo.New().NewContext(req, httptest.NewRecorder())
				c.SetPath(tc.path)

				sprint := &dao.Sprint{Id: permIssueID, CreatedById: permOtherID}
				if tc.author {
					sprint.CreatedById = permUserID
				}

				apicontext.SetPrefilledContext(c, apicontext.Prefilled{
					User:            &dao.User{ID: permUserID, IsSuperuser: tc.superuser},
					Workspace:       &dao.Workspace{ID: permWsID},
					WorkspaceMember: &dao.WorkspaceMember{Role: tc.wsRole, MemberId: permUserID},
					Sprint:          sprint,
				})
				return c
			}

			has, err := svc.hasSprintPermissions(newCtx())
			if err != nil {
				t.Fatalf("неожиданная ошибка: %v", err)
			}
			if has != tc.want {
				t.Errorf("hasSprintPermissions %s %s (ws=%d author=%v su=%v): получено %v, ожидалось %v",
					tc.method, tc.path, tc.wsRole, tc.author, tc.superuser, has, tc.want)
			}

			hasAdmin, err := svc.hasSprintAdminPermissions(newCtx())
			if err != nil {
				t.Fatalf("неожиданная ошибка: %v", err)
			}
			if hasAdmin != tc.wantSprint {
				t.Errorf("hasSprintAdminPermissions %s %s (ws=%d): получено %v, ожидалось %v",
					tc.method, tc.path, tc.wsRole, hasAdmin, tc.wantSprint)
			}
		})
	}
}
