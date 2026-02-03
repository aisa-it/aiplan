package email

import (
	"bytes"
	"text/template"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"gorm.io/gorm"
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

type EmailTemplates struct {
	CollectAll *template.Template
	CollectOne *template.Template
}

func LoadTemplates(tx *gorm.DB) EmailTemplates {
	names := []string{
		"v2_collect_all",
		"v2_collect_one",
	}
	var templates []dao.Template
	if err := tx.Where("name in (?)", names).Find(&templates).Error; err != nil {
		return EmailTemplates{}
	}

	var res EmailTemplates
	for _, t := range templates {
		switch t.Name {
		case "v2_collect_all":
			res.CollectAll = t.ParsedTemplate
		case "v2_collect_one":
			res.CollectOne = t.ParsedTemplate
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
