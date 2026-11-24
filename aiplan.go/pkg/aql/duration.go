package aql

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var durationPart = regexp.MustCompile(`^\s*(\d+(?:\.\d+)?)\s*(\p{L}+)`)

// Сутки — 24 часа, неделя — 7 суток: календарное время, не рабочее.
var minutesPerUnit = map[string]time.Duration{
	"w": 7 * 24 * 60, "нед": 7 * 24 * 60, "н": 7 * 24 * 60,
	"d": 24 * 60, "д": 24 * 60,
	"h": 60, "ч": 60,
	"m": 1, "мин": 1, "м": 1,
}

// "M" — месяцы, как в JQL; отличается от минут только регистром.
var monthsPerUnit = map[string]int{"y": 12, "г": 12, "мес": 1}

// Duration — длительность или сдвиг даты. Месяцы отдельно: их длина в часах не постоянна.
type Duration struct {
	Months int
	Fixed  time.Duration
}

// AddTo сдвигает t: месяцы по календарю, затем точная часть.
func (d Duration) AddTo(t time.Time) time.Time {
	return t.AddDate(0, d.Months, 0).Add(d.Fixed)
}

// Длительность или сдвиг даты: 2h 30m, 2ч30м, -7d, +1w 2d, -1M.
func parseDuration(raw string) (Duration, error) {
	s := strings.TrimSpace(raw)
	neg := false
	if rest, ok := strings.CutPrefix(s, "-"); ok {
		neg, s = true, rest
	} else {
		s = strings.TrimPrefix(s, "+")
	}
	if s == "" {
		return Duration{}, fmt.Errorf("duration %q: empty", raw)
	}

	var total Duration
	for rest := s; strings.TrimSpace(rest) != ""; {
		m := durationPart.FindStringSubmatch(rest)
		if m == nil {
			return Duration{}, fmt.Errorf("duration %q: unexpected %q", raw, strings.TrimSpace(rest))
		}
		rest = rest[len(m[0]):]

		part, err := durationPartValue(m[1], m[2])
		if err == nil && total.Fixed > math.MaxInt64-part.Fixed {
			err = ErrDurationRange
		}
		if err != nil {
			return Duration{}, fmt.Errorf("duration %q: %w", raw, err)
		}
		total.Months += part.Months
		total.Fixed += part.Fixed
	}

	if neg {
		total.Months, total.Fixed = -total.Months, -total.Fixed
	}
	return total, nil
}

func durationPartValue(num, unit string) (Duration, error) {
	if unit == "M" {
		return calendarPart(num, 1)
	}
	if k, ok := monthsPerUnit[strings.ToLower(unit)]; ok {
		return calendarPart(num, k)
	}
	k, ok := minutesPerUnit[strings.ToLower(unit)]
	if !ok {
		return Duration{}, fmt.Errorf("%w %q", ErrDurationUnit, unit)
	}
	minutes, err := time.ParseDuration(num + "m")
	if err != nil || minutes > math.MaxInt64/k {
		return Duration{}, ErrDurationRange
	}
	return Duration{Fixed: minutes * k}, nil
}

func calendarPart(num string, months int) (Duration, error) {
	if strings.Contains(num, ".") {
		return Duration{}, ErrDurationFraction
	}
	n, err := strconv.Atoi(num)
	if err != nil || n > math.MaxInt32 {
		return Duration{}, ErrDurationRange
	}
	return Duration{Months: n * months}, nil
}
