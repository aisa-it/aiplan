package server

import (
	"slices"
	"strings"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/engine"
	actField "github.com/aisa-it/aiplan/aiplan.go/pkg/types/activities"
)

// issueFieldActions — какое право требуется для изменения поля задачи.
//
// Роут PATCH задачи один, а прав за ним несколько: сменить статус, назначить
// исполнителя и переименовать задачу — разные действия, и движок должен
// решать по каждому отдельно. Поэтому действие роута (issue.update) — только
// грубый фильтр на входе, а точная проверка идёт по составу тела запроса.
//
// Имена полей берутся из описаний трекера активностей: там они уже собраны
// и там же меняются, если поле переименуют. Литералы допустимы только для
// вариантов написания, которых трекер не знает.
//
// Поля, которых здесь нет, считаются обычным редактированием и покрываются
// действием роута. Служебные поля, проставляемые сервером (completed_at,
// updated_at, updated_by_id), в проверке не участвуют: от клиента они
// не приходят.
var issueFieldActions = map[string]engine.Action{
	actField.Status.Req:    engine.ActionIssueSetState,
	"state_id":             engine.ActionIssueSetState, // второе написание статуса
	actField.Assignees.Req: engine.ActionIssueSetAssignees,
	actField.Watchers.Req:  engine.ActionIssueSetWatchers,
	actField.Label.Req:     engine.ActionIssueSetLabels,
	actField.Parent.Req:    engine.ActionIssueSetParent,
	"parent_id":            engine.ActionIssueSetParent, // второе написание родителя
	actField.Sprint.Req:    engine.ActionIssueSetSprint,

	// Связи между задачами: блокировки в обе стороны, связанные и подзадачи.
	actField.Blocks.Req:   engine.ActionIssueRelationManage,
	actField.Blocking.Req: engine.ActionIssueRelationManage,
	actField.Linked.Req:   engine.ActionIssueRelationManage,
	actField.Issues.Req:   engine.ActionIssueRelationManage,
}

// actionsForIssueFields возвращает права, которых требует изменение
// переданных полей, без повторов и в стабильном порядке.
//
// Учитывается сам факт присутствия поля в запросе, а не изменение значения:
// так же работает и применение изменений в обработчике.
func actionsForIssueFields(data map[string]any) []engine.Action {
	seen := make(map[engine.Action]struct{}, len(data))
	for field := range data {
		if a, ok := issueFieldActions[field]; ok {
			seen[a] = struct{}{}
		}
	}

	actions := make([]engine.Action, 0, len(seen))
	for a := range seen {
		actions = append(actions, a)
	}
	slices.SortFunc(actions, func(i, j engine.Action) int { return strings.Compare(string(i), string(j)) })

	return actions
}
