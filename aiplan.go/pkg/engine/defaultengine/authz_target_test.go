package defaultengine

import (
	"context"
	"net/http/httptest"
	"testing"

	apicontext "github.com/aisa-it/aiplan/aiplan.go/pkg/api-context"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/dao"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/engine"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/types"
	"github.com/gofrs/uuid"
	"github.com/labstack/echo/v4"
)

var (
	meID    = uuid.Must(uuid.NewV4())
	otherID = uuid.Must(uuid.NewV4())
)

// subjectFor собирает Subject с подставленными сущностями.
func subjectFor(t *testing.T, projectRole int, deletionAllowed bool, issueAuthor uuid.UUID) engine.Subject {
	t.Helper()
	c := echo.New().NewContext(httptest.NewRequest("DELETE", "/", nil), httptest.NewRecorder())
	return apicontext.SetPrefilledContext(c, apicontext.Prefilled{
		User:            &dao.User{ID: meID},
		Workspace:       &dao.Workspace{},
		WorkspaceMember: &dao.WorkspaceMember{Role: types.MemberRole, MemberId: meID},
		Project:         &dao.Project{IssueDeletionAllowed: deletionAllowed},
		ProjectMember:   &dao.ProjectMember{Role: projectRole, MemberId: meID},
		Issue:           &dao.Issue{CreatedById: issueAuthor},
	})
}

func comment(author uuid.UUID) *dao.IssueComment {
	return &dao.IssueComment{ActorId: uuid.NullUUID{UUID: author, Valid: true}}
}

// Правка комментария: только автор, роль не помогает.
func TestAuthorizeCommentUpdate(t *testing.T) {
	cases := []struct {
		name   string
		role   int
		author uuid.UUID
		want   engine.Decision
	}{
		{"автор-участник", types.MemberRole, meID, engine.DecisionAllow},
		{"автор-гость", types.GuestRole, meID, engine.DecisionAllow},
		{"не автор, участник", types.MemberRole, otherID, engine.DecisionDeny},
		{"не автор, админ проекта", types.AdminRole, otherID, engine.DecisionDeny},
	}

	e := New()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v, err := e.Authorize(context.Background(), engine.AuthzRequest{
				Action:  engine.ActionIssueCommentUpdate,
				Subject: subjectFor(t, c.role, false, otherID),
				Target:  comment(c.author),
			})
			if err != nil {
				t.Fatalf("ошибка: %v", err)
			}
			if v.Decision != c.want {
				t.Errorf("решение %d, ожидалось %d", v.Decision, c.want)
			}
		})
	}
}

// Удаление комментария: автор или администратор проекта.
func TestAuthorizeCommentDelete(t *testing.T) {
	cases := []struct {
		name   string
		role   int
		author uuid.UUID
		want   engine.Decision
	}{
		{"автор-участник", types.MemberRole, meID, engine.DecisionAllow},
		{"не автор, админ проекта", types.AdminRole, otherID, engine.DecisionAllow},
		{"не автор, участник", types.MemberRole, otherID, engine.DecisionDeny},
		{"не автор, гость", types.GuestRole, otherID, engine.DecisionDeny},
	}

	e := New()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v, err := e.Authorize(context.Background(), engine.AuthzRequest{
				Action:  engine.ActionIssueCommentDelete,
				Subject: subjectFor(t, c.role, false, otherID),
				Target:  comment(c.author),
			})
			if err != nil {
				t.Fatalf("ошибка: %v", err)
			}
			if v.Decision != c.want {
				t.Errorf("решение %d, ожидалось %d", v.Decision, c.want)
			}
		})
	}
}

// Удаление задачи: админ всегда, автор — только при разрешении в проекте.
func TestAuthorizeIssueDelete(t *testing.T) {
	cases := []struct {
		name      string
		role      int
		author    uuid.UUID
		deletable bool
		want      engine.Decision
	}{
		{"админ проекта, удаление запрещено настройкой", types.AdminRole, otherID, false, engine.DecisionAllow},
		{"автор, удаление разрешено", types.MemberRole, meID, true, engine.DecisionAllow},
		{"автор, удаление запрещено", types.MemberRole, meID, false, engine.DecisionDeny},
		{"не автор, участник", types.MemberRole, otherID, true, engine.DecisionDeny},
	}

	e := New()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			issue := &dao.Issue{CreatedById: c.author}
			v, err := e.Authorize(context.Background(), engine.AuthzRequest{
				Action:  engine.ActionIssueDelete,
				Subject: subjectFor(t, c.role, c.deletable, c.author),
				Target:  issue,
			})
			if err != nil {
				t.Fatalf("ошибка: %v", err)
			}
			if v.Decision != c.want {
				t.Errorf("решение %d, ожидалось %d", v.Decision, c.want)
			}
		})
	}
}

// Без объекта действия правила по объекту не применяются:
// проверка «до объекта» должна проходить обычным порядком.
func TestAuthorizeWithoutTarget(t *testing.T) {
	e := New()
	v, err := e.Authorize(context.Background(), engine.AuthzRequest{
		Action:  engine.ActionIssueCommentUpdate,
		Subject: subjectFor(t, types.MemberRole, false, otherID),
	})
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if v.Decision != engine.DecisionAllow {
		t.Errorf("решение %d: проверка без объекта должна пропускать участника "+
			"до загрузки комментария", v.Decision)
	}
}
