package email

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	member_role "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications/member-role"
	policy "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/redactor-policy"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
	"github.com/microcosm-cc/bluemonday"
	"github.com/tdewolff/minify/v2"
	minhtml "github.com/tdewolff/minify/v2/html"
)

var emailMinifier *minify.M
var emailMinifierOnce sync.Once

func getEmailMinifier() *minify.M {
	emailMinifierOnce.Do(func() {
		m := minify.New()
		m.Add("text/html", &minhtml.Minifier{
			KeepDocumentTags:    true,
			KeepEndTags:         true,
			KeepSpecialComments: true,
			KeepDefaultAttrVals: true,
			KeepWhitespace:      false,
		})
		emailMinifier = m
	})
	return emailMinifier
}

var htmlStripPolicy *bluemonday.Policy = bluemonday.StrictPolicy()

const (
	targetDateTimeZ   = "TargetDateTimeZ"
	targetDateZ       = "TargetDateZ"
	complexActivities = "ComplexActivities"
	customIssueBody   = "CustomIssueBody"
)

func prepareHtmlBody(html string) string {
	p := bluemonday.StrictPolicy()
	p.AllowElements("br")

	res := strings.ReplaceAll(html, "<p>", "<br>")
	res = strings.ReplaceAll(res, "<li>", "<br>")
	res = strings.ReplaceAll(res, "<pre>", "<br>")
	res = p.Sanitize(res)

	parts := strings.Split(res, "<br>")

	result := make([]string, 0, len(parts))

	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			result = append(result, part)
		}
	}

	if len(result) == 0 {
		return ""
	}

	return strings.Join(result, "<br>")
}

func prepareToMail(html string) string {
	return strings.ReplaceAll(html, "\n", "<br>")
}

func processHtmlReplacements(html string) string {
	html = replaceImageToText(html)
	html = replaceTablesToText(html)
	return html
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

func msgReplace(user member_role.MemberNotify, msg FieldPrerender) FieldPrerender {
	if msg.Replace == nil {
		return msg
	}

	baseKey := func(k string) string {
		if keys := strings.Split(k, "_"); len(keys) > 1 {
			return keys[0]
		}
		return k
	}
	for k := range msg.Replace {
		switch baseKey(k) {
		case complexActivities:
			if msg.Has(optPrerenderComplex) {
				res := make(map[uuid.UUID]string)
				if !msg.Has(optCompositeFields) {
					msg.Count = 0
				}
				for u, s := range msg.ValueComplex {
					if msg.ActivityIds == nil || user.IsActNotify(append([]uuid.UUID{u}, msg.ActivityIds...)) {
						if !msg.Has(optCompositeFields) {
							msg.Count++
						}
						res[u] = s
					}
				}
				msg.ValueComplex = res
				msg.Value = strings.ReplaceAll(msg.Value, strReplace(k), msg.GetValue())
				msg.Remove(optPrerenderComplex)
				msg.Add(optPrerenderAll)
			}
		case customIssueBody:
			if msg.Has(optCustomBody) {
				res := make(map[uuid.UUID]string)
				msg.Count = 0
				for u, s := range msg.CustomBodyValue {
					if user.IsActNotify([]uuid.UUID{u}) {
						msg.Count++
						res[u] = s
					}
				}
				msg.CustomBodyValue = res
				msg.Value = strings.ReplaceAll(msg.Value, strReplace(k), msg.CustomBodyGetValue())
				msg.Remove(optCustomBody)
			}
		}
	}
	dateFormat := map[string]string{
		targetDateTimeZ: "02.01.2006 15:04",
		targetDateZ:     "02.01.2006",
	}
	for k, v := range msg.Replace {
		switch key := baseKey(k); key {
		case targetDateTimeZ, targetDateZ:
			strNew, err := utils.FormatDateStr(v.(sql.NullTime).Time.String(), dateFormat[key], &user.GetUser().UserTimezone)
			if err != nil {
				return FieldPrerender{}
			}
			msg.Value = strings.ReplaceAll(msg.Value, strReplace(k), strNew)
			for id, body := range msg.CustomBodyValue {
				msg.CustomBodyValue[id] = strings.ReplaceAll(body, strReplace(k), strNew)
			}
		}
	}

	return msg
}

func uuidPtrFrom[T dao.IDaoAct](v *T) *uuid.UUID {
	if v == nil {
		return nil
	}
	return utils.ToPtr((*v).GetId())
}

func uuidPtrFromNullUUID(u uuid.NullUUID) *uuid.UUID {
	if u.Valid {
		return &u.UUID
	}
	return nil
}

func strReplace(in string) string {
	out := strings.Split(in, "_")
	return "$$$" + strings.Join(out, "$$$") + "$$$"
}

func ApplyCollectDate(value *string, key string, replace map[string]any) *actValueCtx {
	if value == nil || *value == "" {
		return nil
	}

	replace[key] = utils.FormatDateToSqlNullTime(*value)
	return toValueCtx(utils.ToPtr(strReplace(key)), nil)
}

func ApplyCustomReplaceText(key string, replace map[string]any) string {
	replace[key] = struct{}{}
	return strReplace(key)
}

func htmlReplacer(s *string) *string {
	if s == nil {
		return nil
	}
	tmp := processHtmlReplacements(*s)
	tmp = policy.ProcessCustomHtmlTag(tmp)
	return utils.ToPtr(prepareToMail(prepareHtmlBody(tmp)))
}

type prerenderType int

const (
	optPrerenderOne prerenderType = 1 << iota
	optPrerenderAll
	optPrerenderComplex
	optComplexBlock
	optCompositeFields
	optCustomBody
)

func (fp *FieldPrerender) Has(t prerenderType) bool {
	return fp.prerenderType&t != 0
}

func (fp *FieldPrerender) Add(t prerenderType) {
	fp.prerenderType |= t
}

func (fp *FieldPrerender) Remove(t prerenderType) {
	fp.prerenderType &^= t
}

func (fp *FieldPrerender) GetValue() string {
	if fp.Has(optPrerenderComplex) {
		var sep string
		if fp.Has(optComplexBlock) {
			sep = "<br>"
		}
		return strings.Join(utils.MapToSlice(fp.ValueComplex, func(k uuid.UUID, t string) string {
			return t
		}), sep)
	}
	return fp.Value
}

func (fp *FieldPrerender) CustomBodyGetValue() string {
	return strings.Join(utils.MapToSlice(fp.CustomBodyValue, func(k uuid.UUID, t string) string {
		return t
	}), "\n")
}
