package jql

import "testing"

func mustParse(t *testing.T, q string) *Query {
	t.Helper()
	res, err := Parse(q)
	if err != nil {
		t.Fatalf("Parse(%q) error: %v", q, err)
	}
	return res
}

func mustFail(t *testing.T, q string) {
	t.Helper()
	if _, err := Parse(q); err == nil {
		t.Fatalf("Parse(%q) должен был вернуть ошибку", q)
	}
}

func TestParseSimple(t *testing.T) {
	q := mustParse(t, `project = "PRJ"`)
	if len(q.Groups) != 1 || len(q.Groups[0].Clauses) != 1 {
		t.Fatalf("ожидалась 1 группа с 1 условием, получено %+v", q)
	}
	c := q.Groups[0].Clauses[0]
	if c.Field != "project" || c.Op != OpEq || c.Value.Str != "PRJ" {
		t.Fatalf("неверное условие: %+v", c)
	}
}

func TestParseAndOr(t *testing.T) {
	q := mustParse(t, `status = "Новая" AND priority = high OR status = "Готово"`)
	if len(q.Groups) != 2 {
		t.Fatalf("ожидалось 2 OR-группы, получено %d", len(q.Groups))
	}
	if len(q.Groups[0].Clauses) != 2 {
		t.Fatalf("в первой группе ожидалось 2 условия, получено %d", len(q.Groups[0].Clauses))
	}
}

func TestParseNot(t *testing.T) {
	q := mustParse(t, `NOT priority = low AND status = "В работе"`)
	c := q.Groups[0].Clauses[0]
	if !c.Negated || c.Field != "priority" || c.Op != OpEq {
		t.Fatalf("неверное NOT-условие: %+v", c)
	}
}

func TestParseInAndNotIn(t *testing.T) {
	q := mustParse(t, `status IN ("Новая", "В работе") AND label NOT IN (bug, tech-debt)`)
	if q.Groups[0].Clauses[0].Op != OpIn || len(q.Groups[0].Clauses[0].Value.List) != 2 {
		t.Fatalf("неверное IN: %+v", q.Groups[0].Clauses[0])
	}
	notIn := q.Groups[0].Clauses[1]
	if notIn.Op != OpNotIn || len(notIn.Value.List) != 2 {
		t.Fatalf("неверное NOT IN: %+v", notIn)
	}
}

func TestParseDates(t *testing.T) {
	q := mustParse(t, `updated > -7d AND created >= 2026-01-01 AND target < -1w`)
	c1 := q.Groups[0].Clauses[0]
	if c1.Op != OpGt || !c1.Value.HasRel || c1.Value.RelDays != -7 {
		t.Fatalf("неверная относительная дата: %+v", c1)
	}
	c2 := q.Groups[0].Clauses[1]
	if c2.Op != OpGte || !c2.Value.HasDate {
		t.Fatalf("неверная абсолютная дата: %+v", c2)
	}
}

func TestParseIsEmpty(t *testing.T) {
	q := mustParse(t, `assignee IS EMPTY AND watcher IS NOT EMPTY`)
	if q.Groups[0].Clauses[0].Op != OpIsEmpty {
		t.Fatalf("ожидался IS EMPTY: %+v", q.Groups[0].Clauses[0])
	}
	if q.Groups[0].Clauses[1].Op != OpIsNotEmpty {
		t.Fatalf("ожидался IS NOT EMPTY: %+v", q.Groups[0].Clauses[1])
	}
}

func TestParseCurrentUser(t *testing.T) {
	q := mustParse(t, `assignee = currentUser() OR author = currentUser()`)
	if len(q.Groups) != 2 {
		t.Fatalf("ожидалось 2 группы")
	}
	c := q.Groups[0].Clauses[0]
	if !c.Value.IsFunc || c.Value.Str != "currentUser()" {
		t.Fatalf("неверный currentUser: %+v", c)
	}
}

func TestParseOrderBy(t *testing.T) {
	q := mustParse(t, `text ~ "ошибка" ORDER BY priority DESC`)
	if q.OrderBy != "priority" || !q.Desc {
		t.Fatalf("неверный ORDER BY: %+v", q)
	}
}

func TestParseOrderByAliases(t *testing.T) {
	cases := map[string]string{
		"name":     "name",
		"sequence": "sequence_id",
		"created":  "created_at",
		"target":   "target_date",
		"status":   "state",
		"priority": "priority",
	}
	for in, want := range cases {
		q := mustParse(t, "project = PRJ ORDER BY "+in)
		if q.OrderBy != want {
			t.Fatalf("ORDER BY %s: ожидалось %s, получено %s", in, want, q.OrderBy)
		}
	}
	mustFail(t, "project = PRJ ORDER BY labels")
	mustFail(t, "project = PRJ ORDER BY assignees")
}

func TestParseParensOr(t *testing.T) {
	q := mustParse(t, `project = ALF1 AND (status = "Готово" OR label = bug)`)
	if len(q.Groups) != 2 {
		t.Fatalf("ожидалось 2 группы после раскрытия скобок, получено %d", len(q.Groups))
	}
	if len(q.Groups[0].Clauses) != 2 || len(q.Groups[1].Clauses) != 2 {
		t.Fatalf("ожидалось по 2 условия в группах: %+v", q.Groups)
	}
}

func TestParseParensAndNot(t *testing.T) {
	q := mustParse(t, `project = ALF1 AND NOT (status = "Готово" AND label = bug)`)
	if len(q.Groups) != 2 {
		t.Fatalf("ожидалось 2 группы после De Morgan, получено %d", len(q.Groups))
	}
	for _, g := range q.Groups {
		if len(g.Clauses) != 2 {
			t.Fatalf("ожидалось по 2 условия: %+v", q.Groups)
		}
		if g.Clauses[0].Field != "project" || g.Clauses[0].Negated {
			t.Fatalf("первое условие должно быть project без отрицания: %+v", q.Groups)
		}
		if !g.Clauses[1].Negated {
			t.Fatalf("второе условие должно быть отрицательным (NOT внутри скобок): %+v", q.Groups)
		}
	}
}

func TestParseParensNested(t *testing.T) {
	q := mustParse(t, `(priority = high OR priority = urgent) AND (label = bug OR label = feature)`)
	if len(q.Groups) != 4 {
		t.Fatalf("ожидалось 4 группы (2x2), получено %d", len(q.Groups))
	}
}

func TestParseIsEmptyDateAndPriority(t *testing.T) {
	mustParse(t, `target IS EMPTY`)
	mustParse(t, `priority IS NOT EMPTY`)
	mustParse(t, `start IS NOT EMPTY`)
}

func TestParseErrors(t *testing.T) {
	mustFail(t, `bogusfield = 1`)
	mustFail(t, `status === "x"`)
	mustFail(t, `updated > "не дата"`)
	mustFail(t, `text IN (a, b)`)
	mustFail(t, `project = "x" garbage`)
	mustFail(t, "assignee IS EMPTY x")
	mustFail(t, "")
}
