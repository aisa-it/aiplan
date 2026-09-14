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

// Характеризационные тесты прав доступа к задачам: фиксируют текущее поведение
// hasIssuePermissions до выноса логики в интерфейс движка. БД не нужна —
// сущности подставляются через apicontext.NewPrefilled.

const issueBase = "/api/auth/workspaces/:workspaceSlug/projects/:projectId/issues/:issueIdOrSeq"

const (
	pathIssue         = issueBase + "/"
	pathComments      = issueBase + "/comments/"
	pathComment       = issueBase + "/comments/:commentId/"
	pathReactions     = issueBase + "/comments/:commentId/reactions/"
	pathReaction      = issueBase + "/comments/:commentId/reactions/:reaction"
	pathAttachments   = issueBase + "/issue-attachments/"
	pathAttachment    = issueBase + "/issue-attachments/:attachmentId/"
	pathProperties    = issueBase + "/properties/:templateId/"
	pathSubIssues     = issueBase + "/sub-issues/"
	pathIssueLinks    = issueBase + "/issue-links/"
	pathLinkedIssues  = issueBase + "/linked-issues/"
	pathPin           = issueBase + "/pin/"
	pathDescLock      = issueBase + "/description-lock/"
	pathIssueLabels   = issueBase + "/issue-labels/"
	pathIssueHistory  = issueBase + "/history/"
	pathAvailStates   = issueBase + "/available-states/"
	pathPropsTemplate = issueBase + "/properties/"
)

type issuePermCase struct {
	name     string
	method   string
	path     string
	wsRole   int
	prRole   int
	author   bool
	assignee bool
	attach   bool // project.MemberAttachmentsAllowed
	props    bool // project.MemberPropertiesAllowed
	want     bool
}

var (
	permUserID   = uuid.Must(uuid.NewV4())
	permOtherID  = uuid.Must(uuid.NewV4())
	permIssueID  = uuid.Must(uuid.NewV4())
	permProjID   = uuid.Must(uuid.NewV4())
	permWsID     = uuid.Must(uuid.NewV4())
	permMemberID = uuid.Must(uuid.NewV4())
)

// newIssuePermContext собирает echo-контекст с prefilled-сущностями по описанию кейса
func newIssuePermContext(tc issuePermCase) echo.Context {
	req := httptest.NewRequest(tc.method, "/", nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetPath(tc.path)

	user := &dao.User{ID: permUserID}

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

	project := &dao.Project{
		ID:                       permProjID,
		MemberAttachmentsAllowed: tc.attach,
		MemberPropertiesAllowed:  tc.props,
	}

	apicontext.SetPrefilledContext(c, apicontext.Prefilled{
		User:            user,
		Workspace:       &dao.Workspace{ID: permWsID},
		WorkspaceMember: &dao.WorkspaceMember{ID: permMemberID, Role: tc.wsRole, MemberId: permUserID},
		Project:         project,
		ProjectMember:   &dao.ProjectMember{ID: permMemberID, Role: tc.prRole, MemberId: permUserID},
		Issue:           issue,
	})
	return c
}

//nolint:funlen // таблица кейсов
func TestHasIssuePermissions(t *testing.T) {
	const (
		guest  = types.GuestRole
		member = types.MemberRole
		admin  = types.AdminRole
	)

	cases := []issuePermCase{
		// --- safe-методы: разрешены всем до загрузки сущностей ---
		{name: "GET/гость везде/чужая задача", method: "GET", path: pathIssue, wsRole: guest, prRole: guest, want: true},
		{name: "GET/гость/комментарии", method: "GET", path: pathComments, wsRole: guest, prRole: guest, want: true},
		{name: "GET/гость/вложения", method: "GET", path: pathAttachments, wsRole: guest, prRole: guest, want: true},
		{name: "GET/гость/история", method: "GET", path: pathIssueHistory, wsRole: guest, prRole: guest, want: true},
		{name: "GET/гость/доступные статусы", method: "GET", path: pathAvailStates, wsRole: guest, prRole: guest, want: true},
		{name: "GET/гость/список параметров", method: "GET", path: pathPropsTemplate, wsRole: guest, prRole: guest, want: true},
		{name: "HEAD/гость", method: "HEAD", path: pathIssue, wsRole: guest, prRole: guest, want: true},
		{name: "OPTIONS/гость", method: "OPTIONS", path: pathIssue, wsRole: guest, prRole: guest, want: true},

		// --- автор задачи: разрешено всё ---
		{name: "автор-гость/PATCH задачи", method: "PATCH", path: pathIssue, wsRole: guest, prRole: guest, author: true, want: true},
		{name: "автор-гость/DELETE задачи", method: "DELETE", path: pathIssue, wsRole: guest, prRole: guest, author: true, want: true},
		{name: "автор-гость/POST вложения без разрешения", method: "POST", path: pathAttachments, wsRole: guest, prRole: guest, author: true, want: true},
		{name: "автор-гость/POST параметра без разрешения", method: "POST", path: pathProperties, wsRole: guest, prRole: guest, author: true, want: true},
		{name: "автор-гость/POST меток", method: "POST", path: pathIssueLabels, wsRole: guest, prRole: guest, author: true, want: true},
		{name: "автор-гость/POST подзадач", method: "POST", path: pathSubIssues, wsRole: guest, prRole: guest, author: true, want: true},

		// --- админ пространства: разрешено всё, роль в проекте не важна ---
		{name: "wsAdmin+гость проекта/PATCH задачи", method: "PATCH", path: pathIssue, wsRole: admin, prRole: guest, want: true},
		{name: "wsAdmin+гость проекта/DELETE задачи", method: "DELETE", path: pathIssue, wsRole: admin, prRole: guest, want: true},
		{name: "wsAdmin+гость проекта/POST вложения без разрешения", method: "POST", path: pathAttachments, wsRole: admin, prRole: guest, want: true},
		{name: "wsAdmin+гость проекта/POST подзадач", method: "POST", path: pathSubIssues, wsRole: admin, prRole: guest, want: true},
		{name: "wsAdmin+гость проекта/POST меток не исполнителю", method: "POST", path: pathIssueLabels, wsRole: admin, prRole: guest, want: true},

		// --- ветка issue-labels: только автор или исполнитель, раньше проверки роли в проекте ---
		{name: "метки/POST/исполнитель-гость", method: "POST", path: pathIssueLabels, wsRole: guest, prRole: guest, assignee: true, want: true},
		{name: "метки/POST/участник не автор и не исполнитель", method: "POST", path: pathIssueLabels, wsRole: member, prRole: member, want: false},
		{name: "метки/POST/админ проекта не автор и не исполнитель", method: "POST", path: pathIssueLabels, wsRole: member, prRole: admin, want: false},
		{name: "метки/PATCH участнику запрещён как обычный путь", method: "PATCH", path: pathIssueLabels, wsRole: member, prRole: member, want: false},
		{name: "метки/POST/админ проекта исполнитель", method: "POST", path: pathIssueLabels, wsRole: member, prRole: admin, assignee: true, want: true},

		// --- админ проекта: разрешено всё, кроме ветки issue-labels выше ---
		{name: "админ проекта/PATCH задачи", method: "PATCH", path: pathIssue, wsRole: member, prRole: admin, want: true},
		{name: "админ проекта/DELETE задачи", method: "DELETE", path: pathIssue, wsRole: member, prRole: admin, want: true},
		{name: "админ проекта/POST вложения без разрешения", method: "POST", path: pathAttachments, wsRole: member, prRole: admin, want: true},
		{name: "админ проекта/DELETE вложения", method: "DELETE", path: pathAttachment, wsRole: member, prRole: admin, want: true},
		{name: "админ проекта/POST параметра без разрешения", method: "POST", path: pathProperties, wsRole: member, prRole: admin, want: true},
		{name: "админ проекта/POST подзадач", method: "POST", path: pathSubIssues, wsRole: member, prRole: admin, want: true},
		{name: "админ проекта/POST закрепления", method: "POST", path: pathPin, wsRole: member, prRole: admin, want: true},
		{name: "админ проекта+гость пространства/PATCH задачи", method: "PATCH", path: pathIssue, wsRole: guest, prRole: admin, want: true},

		// --- участник проекта, не автор и не исполнитель ---
		{name: "участник/PATCH чужой задачи", method: "PATCH", path: pathIssue, wsRole: member, prRole: member, want: true},
		{name: "участник/DELETE чужой задачи", method: "DELETE", path: pathIssue, wsRole: member, prRole: member, want: true},
		{name: "участник/PUT чужой задачи", method: "PUT", path: pathIssue, wsRole: member, prRole: member, want: true},
		{name: "участник/POST комментария", method: "POST", path: pathComments, wsRole: member, prRole: member, want: true},
		{name: "участник/PATCH чужого комментария", method: "PATCH", path: pathComment, wsRole: member, prRole: member, want: true},
		{name: "участник/DELETE чужого комментария", method: "DELETE", path: pathComment, wsRole: member, prRole: member, want: true},
		{name: "участник/POST реакции", method: "POST", path: pathReactions, wsRole: member, prRole: member, want: true},
		{name: "участник/DELETE реакции", method: "DELETE", path: pathReaction, wsRole: member, prRole: member, want: true},
		{name: "участник/POST подзадач в чужую задачу", method: "POST", path: pathSubIssues, wsRole: member, prRole: member, want: false},
		{name: "участник/POST ссылки в чужую задачу", method: "POST", path: pathIssueLinks, wsRole: member, prRole: member, want: false},
		{name: "участник/POST связанных задач", method: "POST", path: pathLinkedIssues, wsRole: member, prRole: member, want: false},
		{name: "участник/POST закрепления чужой задачи", method: "POST", path: pathPin, wsRole: member, prRole: member, want: false},
		{name: "участник/POST блокировки описания", method: "POST", path: pathDescLock, wsRole: member, prRole: member, want: false},

		// --- настройки проекта, расширяющие права участника ---
		{name: "участник/POST вложения/MemberAttachmentsAllowed=false", method: "POST", path: pathAttachments, wsRole: member, prRole: member, attach: false, want: false},
		{name: "участник/POST вложения/MemberAttachmentsAllowed=true", method: "POST", path: pathAttachments, wsRole: member, prRole: member, attach: true, want: true},
		{name: "участник/DELETE вложения/MemberAttachmentsAllowed=true", method: "DELETE", path: pathAttachment, wsRole: member, prRole: member, attach: true, want: false},
		{name: "участник/POST вложения по пути с id/MemberAttachmentsAllowed=true", method: "POST", path: pathAttachment, wsRole: member, prRole: member, attach: true, want: false},
		{name: "участник/POST параметра/MemberPropertiesAllowed=false", method: "POST", path: pathProperties, wsRole: member, prRole: member, props: false, want: false},
		{name: "участник/POST параметра/MemberPropertiesAllowed=true", method: "POST", path: pathProperties, wsRole: member, prRole: member, props: true, want: true},
		{name: "участник/POST параметра/только вложения разрешены", method: "POST", path: pathProperties, wsRole: member, prRole: member, attach: true, props: false, want: false},
		{name: "участник/POST вложения/только параметры разрешены", method: "POST", path: pathAttachments, wsRole: member, prRole: member, attach: false, props: true, want: false},

		// --- участник-исполнитель: разрешено всё в задаче ---
		{name: "участник-исполнитель/POST подзадач", method: "POST", path: pathSubIssues, wsRole: member, prRole: member, assignee: true, want: true},
		{name: "участник-исполнитель/POST вложения без разрешения", method: "POST", path: pathAttachments, wsRole: member, prRole: member, assignee: true, want: true},
		{name: "участник-исполнитель/POST параметра без разрешения", method: "POST", path: pathProperties, wsRole: member, prRole: member, assignee: true, want: true},
		{name: "участник-исполнитель/POST закрепления", method: "POST", path: pathPin, wsRole: member, prRole: member, assignee: true, want: true},

		// --- гость проекта: всё небезопасное запрещено ---
		{name: "гость проекта/PATCH задачи", method: "PATCH", path: pathIssue, wsRole: member, prRole: guest, want: false},
		{name: "гость проекта/DELETE задачи", method: "DELETE", path: pathIssue, wsRole: member, prRole: guest, want: false},
		{name: "гость проекта/POST комментария", method: "POST", path: pathComments, wsRole: member, prRole: guest, want: false},
		{name: "гость проекта/POST вложения при MemberAttachmentsAllowed=true", method: "POST", path: pathAttachments, wsRole: member, prRole: guest, attach: true, want: false},
		{name: "гость проекта/POST параметра при MemberPropertiesAllowed=true", method: "POST", path: pathProperties, wsRole: member, prRole: guest, props: true, want: false},
		{name: "гость проекта-исполнитель/PATCH задачи", method: "PATCH", path: pathIssue, wsRole: member, prRole: guest, assignee: true, want: false},
		{name: "гость проекта-исполнитель/POST комментария", method: "POST", path: pathComments, wsRole: member, prRole: guest, assignee: true, want: false},
		{name: "гость проекта-автор/PATCH задачи", method: "PATCH", path: pathIssue, wsRole: member, prRole: guest, author: true, want: true},
	}

	svc := &Services{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			has, err := svc.hasIssuePermissions(newIssuePermContext(tc))
			if err != nil {
				t.Fatalf("неожиданная ошибка: %v", err)
			}
			if has != tc.want {
				t.Errorf("%s %s (ws=%d pr=%d author=%v assignee=%v attach=%v props=%v): получено %v, ожидалось %v",
					tc.method, tc.path, tc.wsRole, tc.prRole, tc.author, tc.assignee, tc.attach, tc.props, has, tc.want)
			}
		})
	}
}

// Ветка issue-labels смотрит на issue.Assignees (подгружаемый список),
// а ветка участника — на issue.AssigneeIDs. При расхождении данных права разные.
func TestHasIssuePermissionsAssigneeSourceMismatch(t *testing.T) {
	newCtx := func(method, path string) echo.Context {
		req := httptest.NewRequest(method, "/", nil)
		c := echo.New().NewContext(req, httptest.NewRecorder())
		c.SetPath(path)
		empty := make([]dao.User, 0)
		apicontext.SetPrefilledContext(c, apicontext.Prefilled{
			User:            &dao.User{ID: permUserID},
			Workspace:       &dao.Workspace{ID: permWsID},
			WorkspaceMember: &dao.WorkspaceMember{Role: types.MemberRole, MemberId: permUserID},
			Project:         &dao.Project{ID: permProjID},
			ProjectMember:   &dao.ProjectMember{Role: types.MemberRole, MemberId: permUserID},
			// исполнитель есть в AssigneeIDs, но список Assignees пуст
			Issue: &dao.Issue{ID: permIssueID, CreatedById: permOtherID, AssigneeIDs: []uuid.UUID{permUserID}, Assignees: &empty},
		})
		return c
	}

	svc := &Services{}

	has, err := svc.hasIssuePermissions(newCtx("POST", pathIssueLabels))
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if has {
		t.Errorf("метки: ожидался отказ при пустом Assignees, получено %v", has)
	}

	has, err = svc.hasIssuePermissions(newCtx("POST", pathSubIssues))
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if !has {
		t.Errorf("подзадачи: ожидалось разрешение по AssigneeIDs, получено %v", has)
	}
}

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
