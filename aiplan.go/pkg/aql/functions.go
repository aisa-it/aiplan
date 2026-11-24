package aql

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

type scalarFn func(v *Visitor, args ...any) (any, error)

// listFn отдаёт подзапрос: в IN (?) он встаёт так же, как список значений.
type listFn func(v *Visitor, args ...any) (*gorm.DB, error)

var scalarFuncs = map[string]scalarFn{
	"currentUser":  currentUserFn,
	"now":          nowFn,
	"startOfDay":   boundFn(dayPeriod, false),
	"endOfDay":     boundFn(dayPeriod, true),
	"startOfWeek":  boundFn(weekPeriod, false),
	"endOfWeek":    boundFn(weekPeriod, true),
	"startOfMonth": boundFn(monthPeriod, false),
	"endOfMonth":   boundFn(monthPeriod, true),
	"startOfYear":  boundFn(yearPeriod, false),
	"endOfYear":    boundFn(yearPeriod, true),
}

var listFuncs = map[string]listFn{
	"childrenOf":   childrenFn,
	"linkedTo":     linkedFn,
	"linkedIssues": linkedFn,
}

func currentUserFn(v *Visitor, args ...any) (any, error) {
	return v.currentUser.ID.String(), nil
}

func nowFn(v *Visitor, args ...any) (any, error) {
	return time.Now(), nil
}

// childrenFn — все подзадачи на любую глубину. Обход идёт только по видимым задачам,
// UNION вместо UNION ALL не даёт зациклиться.
func childrenFn(v *Visitor, args ...any) (*gorm.DB, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("childrenOf: %w", ErrFuncArgs)
	}
	parent, err := v.issueRef(args[0])
	if err != nil {
		return nil, fmt.Errorf("childrenOf: %w", err)
	}
	return v.newQuery().Raw(`WITH RECURSIVE visible AS (?), children AS (
		SELECT i.id FROM issues i WHERE i.parent_id IN (?) AND i.id IN (SELECT id FROM visible)
		UNION
		SELECT i.id FROM issues i JOIN children c ON i.parent_id = c.id WHERE i.id IN (SELECT id FROM visible)
	) SELECT id FROM children`, v.visibleIssues(), parent), nil
}

// linkedFn — задачи, связанные с указанной. Связь хранится одной парой id1 < id2, поэтому смотрим обе стороны;
// повторов между сторонами нет, дедупликация UNION не нужна.
func linkedFn(v *Visitor, args ...any) (*gorm.DB, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("linkedTo: %w", ErrFuncArgs)
	}
	issue, err := v.issueRef(args[0])
	if err != nil {
		return nil, fmt.Errorf("linkedTo: %w", err)
	}
	return v.newQuery().Raw(`WITH anchor AS (?)
		SELECT id2 FROM linked_issues WHERE id1 IN (SELECT id FROM anchor)
		UNION ALL
		SELECT id1 FROM linked_issues WHERE id2 IN (SELECT id FROM anchor)`, issue), nil
}

type period struct {
	start func(t time.Time) time.Time
	add   func(t time.Time, n int) time.Time
}

func truncDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

var (
	dayPeriod = period{
		start: truncDay,
		add:   func(t time.Time, n int) time.Time { return t.AddDate(0, 0, n) },
	}
	// Неделя с понедельника.
	weekPeriod = period{
		start: func(t time.Time) time.Time {
			d := truncDay(t)
			return d.AddDate(0, 0, -((int(d.Weekday()) + 6) % 7))
		},
		add: func(t time.Time, n int) time.Time { return t.AddDate(0, 0, 7*n) },
	}
	monthPeriod = period{
		start: func(t time.Time) time.Time { return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location()) },
		add:   func(t time.Time, n int) time.Time { return t.AddDate(0, n, 0) },
	}
	yearPeriod = period{
		start: func(t time.Time) time.Time { return time.Date(t.Year(), 1, 1, 0, 0, 0, 0, t.Location()) },
		add:   func(t time.Time, n int) time.Time { return t.AddDate(n, 0, 0) },
	}
)

// boundFn — startOf*/endOf* со сдвигом: числом периодов, как в Jira, — startOfMonth(-1),
// или длительностью — startOfDay(-1d), startOfMonth(-1M), endOfWeek(+2d).
func boundFn(p period, end bool) scalarFn {
	return func(v *Visitor, args ...any) (any, error) {
		t := p.start(v.now())
		var fixed time.Duration
		switch len(args) {
		case 0:
		case 1:
			switch shift := args[0].(type) {
			case int:
				t = p.add(t, shift)
			case Duration:
				// Месяцы сдвигают сам период: endOfMonth(-1M) — последний день прошлого месяца, а не 30-е число.
				t = t.AddDate(0, shift.Months, 0)
				fixed = shift.Fixed
			default:
				return nil, ErrShiftArg
			}
		default:
			return nil, ErrFuncArgs
		}
		if end {
			// Postgres хранит микросекунды: -1ns потерялся бы.
			t = p.add(t, 1).Add(-time.Microsecond)
		}
		return t.Add(fixed), nil
	}
}

// now — в часовом поясе пользователя: границы дня и недели у него свои.
func (v *Visitor) now() time.Time {
	if v.currentUser == nil {
		return time.Now().UTC()
	}
	return time.Now().In((*time.Location)(&v.currentUser.UserTimezone))
}
