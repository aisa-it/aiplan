// Пакет для подготовки HTML-сообщений для уведомлений.
// Преобразует HTML, удаляет лишние теги, заменяет изображения на текст и таблицы на текстовое описание.
// Также содержит функции для форматирования даты.
//
// Основные возможности:
//   - Очистка HTML от лишних тегов и пробелов.
//   - Замена изображений на текстовое описание с указанием alt-атрибута.
//   - Преобразование таблиц в текстовое описание с указанием размеров.
//   - Форматирование даты в различные форматы с учетом часового пояса.
package notifications

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/microcosm-cc/bluemonday"
)

func prepareHtmlBody(stripPolicy *bluemonday.Policy, html string) string {
	res := strings.ReplaceAll(html, "<p>", "\n")
	res = strings.ReplaceAll(res, "<li>", "\n")
	res = stripPolicy.Sanitize(res)
	res = strings.TrimSpace(res)
	return res
}

func prepareToMail(html string) string {
	return strings.ReplaceAll(html, "\n", "<br>")
}

func replaceImageToText(str string) string {
	imgRegex := regexp.MustCompile(`<img[^>]*alt="([^"]*)"[^>]*>`)

	result := imgRegex.ReplaceAllStringFunc(str, func(imgTag string) string {
		matches := imgRegex.FindStringSubmatch(imgTag)
		altText := "image"
		if len(matches) > 1 {
			altText = matches[1]
		}
		return fmt.Sprintf("%s: (alt: %s)", "image", altText)
	})
	return result
}

func replaceTablesToText(html string) string {
	tableRegex := regexp.MustCompile(`(?s)<table[^>]*>(.*?)</table>`)
	rowRegex := regexp.MustCompile(`(?s)<tr[^>]*>(.*?)</tr>`)
	cellRegex := regexp.MustCompile(`(?s)<td[^>]*>|<th[^>]*>`)

	result := tableRegex.ReplaceAllStringFunc(html, func(table string) string {
		rows := rowRegex.FindAllStringSubmatch(table, -1)
		numRows := len(rows)
		numCols := 0

		for _, row := range rows {
			cells := cellRegex.FindAllString(row[1], -1)
			if len(cells) > numCols {
				numCols = len(cells)
			}
		}

		sizeText := fmt.Sprintf("<p>table (size: %dx%d)</p>", numRows, numCols)
		return sizeText
	})

	return result
}

func FormatDate(dateStr, outFormat string, tz *types.TimeZone) (string, error) {
	if dateStr == "" {
		return "", nil
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
	return t.Format(outFormat), err
}
