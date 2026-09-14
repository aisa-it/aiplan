package statesflow

import (
	"slices"
	"testing"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/types"
	"github.com/gofrs/uuid"
)

// Характеризационный тест правила перехода статуса (states_flow).
//
// Само правило сейчас не функция, а inline-код внутри ручек, продублированный
// в пяти местах. Тест фиксирует таблицу решений как исполняемую спецификацию —
// будущий StatePolicy.CanTransition обязан ей соответствовать.
//
// Места, которые должны совпадать с этой таблицей:
//   - pkg/server/http-issue.go:797    updateIssue        — императивная проверка
//   - pkg/server/http-project.go:1835 createIssue        — стартовый статус (uuid.Nil в FromStates)
//   - pkg/mcp/tools/issues.go:854     updateIssue (MCP)  — копия правила из updateIssue
//   - pkg/server/http-issue.go:1271   getAvailableStates — SQL-вариант для смены статуса
//   - pkg/server/http-project.go:2753 getProjectStartStates — SQL-вариант для создания
//
// Семантика:
//   - пустой FromStates — переход разрешён из любого статуса;
//   - uuid.Nil в списке — статус разрешён как стартовый при создании задачи;
//   - администратор проекта правило обходит.
//
// БД не нужна.

// canTransition — эталон императивного правила (updateIssue / createIssue / MCP).
// isCreate=true — создание задачи: вместо текущего статуса проверяется uuid.Nil.
func canTransition(role int, fromStates []uuid.UUID, currentStateID uuid.UUID, isCreate bool) bool {
	if role == types.AdminRole {
		return true
	}
	if len(fromStates) == 0 {
		return true
	}
	if isCreate {
		return slices.Contains(fromStates, uuid.Nil)
	}
	return slices.Contains(fromStates, currentStateID)
}

// sqlAllows — модель SQL-варианта того же правила:
// array_length(from_states, 1) IS NULL OR ? = any(from_states),
// где ? — текущий статус (смена) либо uuid.Nil (создание).
func sqlAllows(role int, fromStates []uuid.UUID, probe uuid.UUID) bool {
	if role == types.AdminRole {
		return true // SQL-фильтр администратору вообще не навешивается
	}
	if len(fromStates) == 0 {
		return true // array_length(...) IS NULL
	}
	return slices.Contains(fromStates, probe) // ? = any(from_states)
}

var (
	stateCurrent = uuid.FromStringOrNil("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	stateOther   = uuid.FromStringOrNil("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")
)

type transitionCase struct {
	name       string
	role       int
	fromStates []uuid.UUID
	isCreate   bool
	want       bool
}

func transitionCases() []transitionCase {
	// Гость и участник по этому правилу неразличимы: проверяется только AdminRole.
	// Гостя от редактирования отсекают более ранние проверки прав, не states_flow.
	roles := []struct {
		name string
		role int
	}{
		{"гость", types.GuestRole},
		{"участник", types.MemberRole},
	}

	cases := make([]transitionCase, 0, 10*len(roles)+10)
	for _, r := range roles {
		cases = append(cases,
			// Смена статуса существующей задачи.
			transitionCase{r.name + "/from_states_пуст/смена", r.role, nil, false, true},
			transitionCase{r.name + "/from_states=[текущий]/смена", r.role, []uuid.UUID{stateCurrent}, false, true},
			transitionCase{r.name + "/from_states=[другой]/смена", r.role, []uuid.UUID{stateOther}, false, false},
			transitionCase{r.name + "/from_states=[nil]/смена", r.role, []uuid.UUID{uuid.Nil}, false, false},
			transitionCase{r.name + "/from_states=[nil,другой]/смена", r.role, []uuid.UUID{uuid.Nil, stateOther}, false, false},
			// Создание задачи: статус должен быть разрешён как стартовый.
			transitionCase{r.name + "/from_states_пуст/создание", r.role, nil, true, true},
			transitionCase{r.name + "/from_states=[текущий]/создание", r.role, []uuid.UUID{stateCurrent}, true, false},
			transitionCase{r.name + "/from_states=[другой]/создание", r.role, []uuid.UUID{stateOther}, true, false},
			transitionCase{r.name + "/from_states=[nil]/создание", r.role, []uuid.UUID{uuid.Nil}, true, true},
			transitionCase{r.name + "/from_states=[nil,другой]/создание", r.role, []uuid.UUID{uuid.Nil, stateOther}, true, true},
		)
	}

	// Администратор проекта обходит правило в любом сочетании.
	for _, fs := range [][]uuid.UUID{
		nil,
		{stateCurrent},
		{stateOther},
		{uuid.Nil},
		{uuid.Nil, stateOther},
	} {
		for _, isCreate := range []bool{false, true} {
			mode := "смена"
			if isCreate {
				mode = "создание"
			}
			cases = append(cases, transitionCase{
				name:       "админ/" + fromStatesLabel(fs) + "/" + mode,
				role:       types.AdminRole,
				fromStates: fs,
				isCreate:   isCreate,
				want:       true,
			})
		}
	}
	return cases
}

func fromStatesLabel(fs []uuid.UUID) string {
	switch {
	case len(fs) == 0:
		return "from_states_пуст"
	case len(fs) == 1 && fs[0] == stateCurrent:
		return "from_states=[текущий]"
	case len(fs) == 1 && fs[0] == stateOther:
		return "from_states=[другой]"
	case len(fs) == 1 && fs[0] == uuid.Nil:
		return "from_states=[nil]"
	default:
		return "from_states=[nil,другой]"
	}
}

// TestCanTransition — таблица решений правила перехода статуса.
func TestCanTransition(t *testing.T) {
	for _, c := range transitionCases() {
		t.Run(c.name, func(t *testing.T) {
			got := canTransition(c.role, c.fromStates, stateCurrent, c.isCreate)
			if got != c.want {
				t.Errorf("правило перехода изменилось: role=%d from_states=%v isCreate=%v -> %v, ожидалось %v",
					c.role, c.fromStates, c.isCreate, got, c.want)
			}
		})
	}
}

// TestImperativeAndSQLRulesAgree — инвариант: императивная проверка в ручках и
// SQL-фильтр списков доступных статусов обязаны давать одинаковый ответ.
// Расхождение = статус показан в списке, но запрещён на сохранении (или наоборот).
func TestImperativeAndSQLRulesAgree(t *testing.T) {
	for _, c := range transitionCases() {
		t.Run(c.name, func(t *testing.T) {
			probe := stateCurrent
			if c.isCreate {
				probe = uuid.Nil
			}
			imperative := canTransition(c.role, c.fromStates, stateCurrent, c.isCreate)
			sql := sqlAllows(c.role, c.fromStates, probe)
			if imperative != sql {
				t.Errorf("императивное правило и SQL-фильтр разошлись: role=%d from_states=%v isCreate=%v "+
					"императивно=%v, SQL=%v", c.role, c.fromStates, c.isCreate, imperative, sql)
			}
		})
	}
}
