package email

import (
	"bytes"
	"log/slog"
	"text/template"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	policy "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/redactor-policy"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"gorm.io/gorm"
)

const (
	templateCollectAll     = "v2_collect_all"
	templateCollectOne     = "v2_collect_one"
	templateBody           = "v2_body"
	templateActivity       = "v2_activity"
	templateAuthorActivity = "v2_author_activity"
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

type EmailTemplates struct {
	ReplaceTxtToSvg func(string) string
	CollectAll      *template.Template
	CollectOne      *template.Template
	Body            *template.Template
	Activity        *template.Template
	AuthorActivity  *template.Template
}

func LoadTemplates(tx *gorm.DB) EmailTemplates {
	names := []string{
		templateCollectAll,
		templateCollectOne,
		templateBody,
		templateActivity,
		templateAuthorActivity,
	}
	var templates []dao.Template
	if err := tx.Where("name in (?)", names).Find(&templates).Error; err != nil {
		return EmailTemplates{}
	}

	var res EmailTemplates
	res.ReplaceTxtToSvg = templates[0].ReplaceTxtToSvg
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
		case templateAuthorActivity:
			res.AuthorActivity = t.ParsedTemplate
		}
	}
	return res
}

type collectOneCtx struct {
  Key string
  New *actValueCtx
  Old *actValueCtx
}

func (c *collectOneCtx) Replace() {
  replacer := func(s *string) *string {
    if s == nil {
      return nil
    }
    tmp := replaceTablesToText(replaceImageToText(*s))
    tmp = policy.ProcessCustomHtmlTag(tmp)
    return utils.ToPtr(prepareToMail(prepareHtmlBody(policy.StripTagsPolicy, tmp)))
  }
  c.Old.Body = replacer(c.Old.Body)
  c.New.Body = replacer(c.New.Body)
}

func (t *EmailTemplates) RenderCollectOne(c collectOneCtx) FieldPrerender {
	c.Replace()
	var buf bytes.Buffer
	if err := t.CollectOne.Execute(&buf, c); err != nil {
		return FieldPrerender{}
	}
	return FieldPrerender{Value: t.ReplaceTxtToSvg(buf.String()), Count: 1}
}

type collectAllCtx struct {
  Key   string
  Views []DigestView
}

func (t *EmailTemplates) RenderCollectAll(c collectAllCtx, count int) FieldPrerender {
	if count == 0 {
		return FieldPrerender{}
	}

	var buf bytes.Buffer
	if err := t.CollectAll.Execute(&buf, c); err != nil {
    return FieldPrerender{}
	}
	return FieldPrerender{Value: buf.String(),Count: count}
}

type bodyCtx struct {
  Title string
  Body  string
}

func (t *EmailTemplates) RenderActivity(c bodyCtx) string {
	var buf bytes.Buffer
	if err := t.Activity.Execute(&buf, c); err != nil {
		slog.Error("err", err.Error())
		return ""
	}
	return buf.String()
}

func (t *EmailTemplates) RenderBody(c bodyCtx) string {
	var buf bytes.Buffer
	if err := t.Body.Execute(&buf, c); err != nil {
		slog.Error("err", err.Error())
		return ""
	}
	return buf.String()
}
