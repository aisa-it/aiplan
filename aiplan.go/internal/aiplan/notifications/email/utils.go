package email

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"time"

  member_role "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications/member-role"
  "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/microcosm-cc/bluemonday"
)

const (
	targetDateTimeZ = "TargetDateTimeZ"
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

func strReplace(in string) string {
	out := strings.Split(in, "_")
	return "$$$" + strings.Join(out, "$$$") + "$$$"
}

func msgReplace(user member_role.MemberNotify, msg EmailMessage) EmailMessage {
	for k, v := range msg.replace {
		key := k
		keys := strings.Split(k, "_")
		if len(keys) > 1 {
			key = keys[0]
		}
		switch key {
		case targetDateTimeZ:
			if strNeW, err := utils.FormatDateStr(v.(sql.NullTime).Time.String(), "02.01.2006 15:04", &user.GetUser().UserTimezone); err == nil {
				msg.HTML = strings.ReplaceAll(msg.HTML, strReplace(k), strNeW)
			} else {
				return EmailMessage{}
			}
		//case userMentioned:
		//	if user.Has(member_role.CommentMentioned) {
		//		msg.title += Stelegramf("\n__%s__", "Вас упомянули в комментарии")
		//	}
		}
	}
	return msg
}
