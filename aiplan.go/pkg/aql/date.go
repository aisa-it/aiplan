package aql

import "time"

// Date — дата без времени. В сравнениях означает целые сутки.
type Date time.Time

// Форматы дат JQL; месяц и день можно без ведущего нуля, время — через пробел.
var dateLayouts = []string{"2006-1-2", "2006/1/2"}

// parseDate разбирает дату в часовом поясе пользователя: без времени — Date, со временем — time.Time.
func (v *Visitor) parseDate(s string) (any, error) {
	loc := v.now().Location()
	for _, layout := range dateLayouts {
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			return Date(t), nil
		}
		if t, err := time.ParseInLocation(layout+" 15:04", s, loc); err == nil {
			return t, nil
		}
	}
	return nil, ErrDate
}

// dateValue приводит значение из запроса к Date или time.Time.
func (v *Visitor) dateValue(value any) (any, error) {
	switch vv := value.(type) {
	case string:
		return v.parseDate(vv)
	case Duration:
		return vv.AddTo(v.now()), nil
	case Date, time.Time:
		return value, nil
	}
	return nil, ErrDate
}
