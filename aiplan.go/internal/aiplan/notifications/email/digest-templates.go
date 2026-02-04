package email

import (
	"bytes"
	"log/slog"
	"text/template"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"gorm.io/gorm"
)

const (
	templateCollectAll = "v2_collect_all"
	templateCollectOne = "v2_collect_one"
	templateBody       = "v2_body"
	templateActivity   = "v2_activity"
)

type actValueCtx struct {
	Value *string
	Body  *string
}

func toValueCtx(value, body *string) *actValueCtx {
	if value == nil && body == nil {
		return nil
	}
	return &actValueCtx{
		Value: value,
		Body:  body,
	}
}

type collectOneCtx struct {
	Key string
	New *actValueCtx
	Old *actValueCtx
}

type collectAllCtx struct {
	Key   string
	Views []DigestView
}

type bodyCtx struct {
	Title string
	Body  string
}

type EmailTemplates struct {
	CollectAll *template.Template
	CollectOne *template.Template
	Body       *template.Template
	Activity   *template.Template
}

func LoadTemplates(tx *gorm.DB) EmailTemplates {
	names := []string{
		templateCollectAll,
		templateCollectOne,
		templateBody,
		templateActivity,
	}
	var templates []dao.Template
	if err := tx.Where("name in (?)", names).Find(&templates).Error; err != nil {
		return EmailTemplates{}
	}

	var res EmailTemplates
	for _, t := range templates {
		switch t.Name {
		case templateCollectAll:
			res.CollectAll = t.ParsedTemplate
		case templateCollectOne:
			res.CollectOne = t.ParsedTemplate
		case templateBody:
			res.Body = t.ParsedTemplate
		case templateActivity:
			res.Activity = t.ParsedTemplate
		}
	}
	return res
}

func (t *EmailTemplates) RenderCollectOne(c collectOneCtx) (string, int) {
	var buf bytes.Buffer
	if err := t.CollectOne.Execute(&buf, c); err != nil {
		return "", 0
	}
	return buf.String(), 1
}

func (t *EmailTemplates) RenderCollectAll(c collectAllCtx, count int) (string, int) {
	if count == 0 {
		return "", 0
	}

	var buf bytes.Buffer
	if err := t.CollectAll.Execute(&buf, c); err != nil {
		return "", 0
	}
	return buf.String(), count
}

func (t *EmailTemplates) RenderActivity(c bodyCtx) string {
	//if count == 0 {
	//  return "", 0
	//}

	var buf bytes.Buffer
	if err := t.Activity.Execute(&buf, c); err != nil {
		slog.Error("err", err.Error())
		return ""
	}
	return buf.String()
}

func (t *EmailTemplates) RenderBody(c bodyCtx) string {
	//if count == 0 {
	//  return "", 0
	//}

	var buf bytes.Buffer
	if err := t.Body.Execute(&buf, c); err != nil {
		slog.Error("err", err.Error())
		return ""
	}
	return buf.String()
}
