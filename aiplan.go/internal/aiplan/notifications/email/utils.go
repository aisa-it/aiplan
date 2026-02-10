package email

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	member_role "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications/member-role"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
	"github.com/microcosm-cc/bluemonday"
)

const (
	targetDateTimeZ = "TargetDateTimeZ"
	targetDateZ     = "TargetDateZ"
	userMentioned   = "UserMentioned"
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
		"2006-01-02 15:04:05Z07:00",
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

func msgReplace(user member_role.MemberNotify, msg FieldPrerender) FieldPrerender {
	for k, v := range msg.Replace {
		key := k
		keys := strings.Split(k, "_")
		if len(keys) > 1 {
			key = keys[0]
		}
		switch key {
		case targetDateTimeZ:
			if strNeW, err := utils.FormatDateStr(v.(sql.NullTime).Time.String(), "02.01.2006 15:04", &user.GetUser().UserTimezone); err == nil {
				msg.Value = strings.ReplaceAll(msg.Value, strReplace(k), strNeW)
			} else {
				return FieldPrerender{}
			}
		case targetDateZ:
			if strNeW, err := utils.FormatDateStr(v.(sql.NullTime).Time.String(), "02.01.2006", &user.GetUser().UserTimezone); err == nil {
				msg.Value = strings.ReplaceAll(msg.Value, strReplace(k), strNeW)
			} else {
				return FieldPrerender{}
			}
		}
	}
	return msg
}

func collectDate(value *string, key string, replace map[string]any) *actValueCtx {
	if value == nil || *value == "" {
		return nil
	}

	replace[key] = utils.FormatDateToSqlNullTime(*value)
	return toValueCtx(utils.ToPtr(strReplace(key)), nil)
}

func uuidPtrFrom[T dao.IDaoAct](v *T) *uuid.UUID {
	if v == nil {
		return nil
	}
	return utils.ToPtr((*v).GetId())
}

func strReplace(in string) string {
	out := strings.Split(in, "_")
	return "$$$" + strings.Join(out, "$$$") + "$$$"
}
