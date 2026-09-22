package defaultengine

import (
	"context"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	apicontext "github.com/aisa-it/aiplan/aiplan.go/pkg/api-context"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/dao"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/engine"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/types"
	"github.com/gofrs/uuid"
	"github.com/labstack/echo/v4"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var (
	curState   = uuid.Must(uuid.NewV4())
	otherState = uuid.Must(uuid.NewV4())
)

func stateSubject(role int) engine.Subject {
	c := echo.New().NewContext(httptest.NewRequest("PATCH", "/", nil), httptest.NewRecorder())
	return apicontext.SetPrefilledContext(c, apicontext.Prefilled{
		User:          &dao.User{ID: meID},
		ProjectMember: &dao.ProjectMember{Role: role, MemberId: meID},
	})
}

func state(from ...uuid.UUID) dao.State {
	return dao.State{ID: otherState, FromStates: types.UUIDArray{Array: from}}
}

// Таблица решений перехода, снятая с поведения трекера до выноса правила
// в движок (см. pkg/states-flow/transition_test.go).
func TestCanTransitionMatrix(t *testing.T) {
	cases := []struct {
		name     string
		role     int
		from     []uuid.UUID
		isCreate bool
		want     bool
	}{
		{"участник / список пуст / смена", types.MemberRole, nil, false, true},
		{"участник / список пуст / создание", types.MemberRole, nil, true, true},
		{"участник / текущий статус разрешён / смена", types.MemberRole, []uuid.UUID{curState}, false, true},
		{"участник / текущий статус разрешён / создание", types.MemberRole, []uuid.UUID{curState}, true, false},
		{"участник / разрешён другой статус / смена", types.MemberRole, []uuid.UUID{otherState}, false, false},
		{"участник / стартовый / смена", types.MemberRole, []uuid.UUID{uuid.Nil}, false, false},
		{"участник / стартовый / создание", types.MemberRole, []uuid.UUID{uuid.Nil}, true, true},
		{"участник / стартовый и другой / создание", types.MemberRole, []uuid.UUID{uuid.Nil, otherState}, true, true},

		// Гость по этому правилу неотличим от участника: смотрится только
		// роль администратора. Гостя отсекают проверки прав.
		{"гость / список пуст / смена", types.GuestRole, nil, false, true},
		{"гость / разрешён другой статус / смена", types.GuestRole, []uuid.UUID{otherState}, false, false},

		{"админ проекта / разрешён другой статус / смена", types.AdminRole, []uuid.UUID{otherState}, false, true},
		{"админ проекта / стартовый / создание", types.AdminRole, []uuid.UUID{curState}, true, true},
	}

	e := New()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := engine.StateTransition{
				Subject: stateSubject(c.role),
				To:      state(c.from...),
			}
			if !c.isCreate {
				req.Issue = &dao.Issue{StateId: curState}
			}

			v, err := e.CanTransition(context.Background(), req)
			if err != nil {
				t.Fatalf("ошибка: %v", err)
			}
			got := v.Decision == engine.DecisionAllow
			if got != c.want {
				t.Errorf("переход %v, ожидался %v", got, c.want)
			}
		})
	}
}

// Отказ обязан нести ошибку о нарушении бизнес-процесса, а не общий отказ
// в доступе: клиент различает эти случаи.
func TestCanTransitionDenyError(t *testing.T) {
	e := New()
	v, err := e.CanTransition(context.Background(), engine.StateTransition{
		Subject: stateSubject(types.MemberRole),
		Issue:   &dao.Issue{StateId: curState},
		To:      state(otherState),
	})
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if v.Decision != engine.DecisionDeny {
		t.Fatalf("решение %d, ожидался отказ", v.Decision)
	}
	if v.Error == nil || v.Error.Code != engine.ErrForbiddenState.Code {
		t.Errorf("ошибка вердикта %v, ожидалась ErrForbiddenState", v.Error)
	}
}

// TestStateRulesAgree — инвариант политики статусов: состояние проходит
// ScopeAvailableStates тогда и только тогда, когда CanTransition его
// не отклоняет.
//
// Рассогласование даёт худший вид ошибки: интерфейс показывает статус,
// а сохранение его отклоняет (или наоборот — доступный статус скрыт).
// SQL здесь не выполняется: сравнивается предикат, который построил
// движок, с его же императивным решением.
//
//nolint:cyclop,funlen // перебор матрицы ролей, наборов статусов и режимов
func TestStateRulesAgree(t *testing.T) {
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "", DriverName: "", WithoutQuotingCheck: true, PreferSimpleProtocol: true,
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("тестовое соединение: %v", err)
	}

	roles := []int{types.GuestRole, types.MemberRole, types.AdminRole}
	fromSets := [][]uuid.UUID{
		nil,
		{curState},
		{otherState},
		{uuid.Nil},
		{uuid.Nil, otherState},
	}

	e := New()
	ctx := context.Background()

	for _, role := range roles {
		for _, isCreate := range []bool{false, true} {
			// Условия запроса строятся один раз на скоуп — как в обработчике.
			scope := engine.StateScopeRequest{Subject: stateSubject(role)}
			if !isCreate {
				scope.Issue = &dao.Issue{StateId: curState}
			}

			q := e.ScopeAvailableStates(ctx, scope, db.Session(&gorm.Session{}).Model(&dao.State{}))
			var states []dao.State
			q.Find(&states)
			sql := q.Statement.SQL.String()
			adminBypass := role == types.AdminRole

			for _, from := range fromSets {
				req := engine.StateTransition{Subject: stateSubject(role), To: state(from...)}
				if !isCreate {
					req.Issue = &dao.Issue{StateId: curState}
				}
				v, err := e.CanTransition(ctx, req)
				if err != nil {
					t.Fatalf("ошибка: %v", err)
				}
				imperative := v.Decision == engine.DecisionAllow

				// Что дал бы SQL-фильтр для этого набора from_states.
				var bySQL bool
				switch {
				case adminBypass:
					bySQL = true // условия не навешиваются
				case len(from) == 0:
					bySQL = true // array_length(...) IS NULL
				default:
					probe := curState
					if isCreate {
						probe = uuid.Nil
					}
					bySQL = slices.Contains(from, probe)
				}

				if imperative != bySQL {
					t.Errorf("роль=%d создание=%v from=%v: императивно=%v, по условию запроса=%v\n  SQL: %s",
						role, isCreate, from, imperative, bySQL, sql)
				}
			}

			// Проверяется фактический текст условия, а не его пересказ:
			// иначе тест подтверждал бы сам себя.
			const (
				anyPrevious = "array_length(from_states, 1) IS NULL"
				matchesFrom = "= any(from_states)"
			)

			if adminBypass {
				if strings.Contains(sql, "from_states") {
					t.Errorf("роль=%d: администратору навешан фильтр статусов\n  SQL: %s", role, sql)
				}
				continue
			}

			// Обе половины правила обязаны присутствовать: без первой
			// пропадут статусы, разрешённые из любого предыдущего;
			// без второй — разрешённые из текущего.
			if !strings.Contains(sql, anyPrevious) {
				t.Errorf("роль=%d создание=%v: в условии нет %q — статусы без ограничений "+
					"перестанут показываться\n  SQL: %s", role, isCreate, anyPrevious, sql)
			}
			if !strings.Contains(sql, matchesFrom) {
				t.Errorf("роль=%d создание=%v: в условии нет %q\n  SQL: %s",
					role, isCreate, matchesFrom, sql)
			}

			// Значение, которое ищется в списке разрешённых предыдущих.
			wantProbe := curState
			if isCreate {
				wantProbe = uuid.Nil
			}
			if len(q.Statement.Vars) == 0 || q.Statement.Vars[0] != wantProbe {
				t.Errorf("роль=%d создание=%v: в условии ищется %v, ожидалось %v",
					role, isCreate, q.Statement.Vars, wantProbe)
			}
		}
	}
}
