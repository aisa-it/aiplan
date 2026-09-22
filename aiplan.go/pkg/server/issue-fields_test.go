package server

import (
	"os"
	"regexp"
	"testing"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/engine"
	actField "github.com/aisa-it/aiplan/aiplan.go/pkg/types/activities"
)

//nolint:funlen // табличный перечень случаев
func TestActionsForIssueFields(t *testing.T) {
	cases := []struct {
		name string
		data map[string]any
		want []engine.Action
	}{
		{
			name: "только общие поля — отдельных прав не требуют",
			data: map[string]any{"name": "x", "description_html": "y", "target_date": "z"},
			want: nil,
		},
		{
			name: "смена статуса",
			data: map[string]any{"state_id": "s1"},
			want: []engine.Action{engine.ActionIssueSetState},
		},
		{
			name: "два написания статуса дают одно право",
			data: map[string]any{"state": "s1", "state_id": "s1"},
			want: []engine.Action{engine.ActionIssueSetState},
		},
		{
			name: "блокировки в обе стороны — одно право",
			data: map[string]any{"blockers_list": nil, "blocks_list": nil},
			want: []engine.Action{engine.ActionIssueSetBlockers},
		},
		{
			name: "смешанный запрос: каждое поле приносит своё право",
			data: map[string]any{
				"name":           "x",
				"state_id":       "s1",
				"assignees_list": nil,
				"labels_list":    nil,
			},
			want: []engine.Action{
				engine.ActionIssueSetAssignees,
				engine.ActionIssueSetLabels,
				engine.ActionIssueSetState,
			},
		},
		{
			name: "служебные поля сервера прав не требуют",
			data: map[string]any{"updated_at": nil, "updated_by_id": nil, "completed_at": nil},
			want: nil,
		},
		{
			name: "пустое тело",
			data: map[string]any{},
			want: nil,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := actionsForIssueFields(c.data)
			if len(got) != len(c.want) {
				t.Fatalf("права %v, ожидались %v", got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("права %v, ожидались %v", got, c.want)
					break
				}
			}
		})
	}
}

// TestIssueFieldsCoverage: поля, которые обработчик реально применяет,
// должны быть либо сопоставлены праву, либо осознанно отнесены к общему
// редактированию. Тест ловит появление нового поля, для которого забыли
// принять это решение.
func TestIssueFieldsCoverage(t *testing.T) {
	// Поля, покрываемые действием роута (issue.update) — обычное
	// редактирование, и служебные, проставляемые сервером.
	generic := map[string]struct{}{
		"name": {}, "description_html": {}, "target_date": {}, "start_date": {},
		"sort_order": {}, "completed_at": {}, "updated_at": {}, "updated_by_id": {},
	}

	src, err := os.ReadFile("http-issue.go")
	if err != nil {
		t.Fatalf("чтение обработчика: %v", err)
	}

	found := map[string]struct{}{}
	for _, m := range regexp.MustCompile(`data\["([a-z_]+)"\]`).FindAllStringSubmatch(string(src), -1) {
		found[m[1]] = struct{}{}
	}
	if len(found) == 0 {
		t.Fatal("в обработчике не найдено ни одного поля — проверка сломана")
	}

	for field := range found {
		if _, ok := issueFieldActions[field]; ok {
			continue
		}
		if _, ok := generic[field]; ok {
			continue
		}
		t.Errorf("поле %q меняется обработчиком, но не отнесено ни к праву, "+
			"ни к общему редактированию: добавьте его в issueFieldActions "+
			"или в generic этого теста", field)
	}
}

// TestIssueFieldNamesMatchTracker: имена полей в таблице прав совпадают
// с описаниями трекера активностей. Если поле переименуют в трекере,
// таблица прав не должна остаться со старым именем.
func TestIssueFieldNamesMatchTracker(t *testing.T) {
	fromTracker := []struct {
		name string
		req  string
	}{
		{"Status", actField.Status.Req},
		{"Assignees", actField.Assignees.Req},
		{"Watchers", actField.Watchers.Req},
		{"Label", actField.Label.Req},
		{"Parent", actField.Parent.Req},
		{"Sprint", actField.Sprint.Req},
		{"Blocks", actField.Blocks.Req},
		{"Blocking", actField.Blocking.Req},
		{"Linked", actField.Linked.Req},
		{"Issues", actField.Issues.Req},
	}

	for _, f := range fromTracker {
		if f.req == "" {
			t.Errorf("поле %s трекера потеряло имя в запросе", f.name)
			continue
		}
		if _, ok := issueFieldActions[f.req]; !ok {
			t.Errorf("поле %s (%q) есть в трекере, но не сопоставлено праву", f.name, f.req)
		}
	}
}
