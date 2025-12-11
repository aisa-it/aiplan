// Пакет предоставляет функциональность для отслеживания изменений статуса блокировок задач (issues). Он записывает историю изменений, включая добавление и удаление блокировок, с указанием автора, времени и комментариев.
//
// Основные возможности:
//   - Добавление блокировок к задаче.
//   - Удаление блокировок с задачи.
//   - Логирование изменений блокировок с указанием автора, времени и комментария.
package tracker

import (
	"strings"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
)

// FormatDate преобразует строку даты в указанный формат.  Принимает строку даты, формат вывода и часовой пояс.  Пытается распарсить дату с использованием различных форматов, указанных в layouts.  Если парсинг успешен, форматирует дату в указанный формат и применяет часовой пояс, если он указан.  В случае ошибки парсинга возвращает пустую строку и ошибку.
//
// Парамметры:
//   - dateStr: Строка даты для преобразования.
//   - outFormat: Формат строки даты, в которую нужно преобразовать дату.
//   - tz: Часовой пояс для применения к дате.
//
// Возвращает:
//   - string: Отформатированная строка даты, или пустая строка в случае ошибки.
//   - error: Ошибка, произошедшая при парсинге или форматировании даты, или nil в случае успеха.
func FormatDate(dateStr, outFormat string, tz *types.TimeZone) (string, error) {
	if dateStr == "" {
		return "", nil
	}

	if idx := strings.Index(dateStr, " m="); idx != -1 {
		dateStr = dateStr[:idx]
	}

	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02T15:04:05.000Z",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04:05 -0700 MST",
		"2006-01-02",
		"02.01.2006 15:04 MST",
		"02.01.2006 15:04 -0700",
		"2006-01-02 15:04:05.999999 -0700 -07",
		"02.01.2006",
	}

	var t time.Time
	var err error
	for _, layout := range layouts {
		t, err = time.Parse(layout, dateStr)
		if err == nil {
			if tz != nil {
				t = t.In((*time.Location)(tz))
			}
			return t.Format(outFormat), nil
		}
	}
	return "", err
}

func confSkipper[A dao.Activity](act A, requestedData map[string]interface{}) A {
	var result A
	switch a := any(act).(type) {

	case dao.IssueActivity:
		if v, ok := requestedData["tg_sender"]; ok {
			if val, intOk := v.(int64); intOk {
				a.SenderTg = val
			}
		}
		result = any(a).(A)
	case dao.DocActivity:
		if v, ok := requestedData["tg_sender"]; ok {
			if val, intOk := v.(int64); intOk {
				a.SenderTg = val
			}
		}
		result = any(a).(A)
	default:
		result = act

	}
	return result
}
