package email

import (
	"bytes"
	"database/sql"
	"log/slog"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	policy "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/redactor-policy"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
)

type collectOneCtx struct {
	Key string
	New *actValueCtx
	Old *actValueCtx

	Start  sql.NullTime
	Author dao.User
}

type collectAllCtx struct {
	Key   string
	Views []DigestView

	Start sql.NullTime
	End   sql.NullTime

	Author map[uuid.UUID]dao.User
}

type bodyCtx struct {
	Title string
	Body  string
}

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

func (t *EmailTemplates) RenderCollectOne(c collectOneCtx) FieldPrerender {
	c.Replace()
	var buf bytes.Buffer
	if err := t.CollectOne.Execute(&buf, c); err != nil {
		return FieldPrerender{}
	}
	return FieldPrerender{Value: t.ReplaceTxtToSvg(buf.String()), Count: 1}
}

func (t *EmailTemplates) RenderCollectAll(c collectAllCtx, count int) FieldPrerender {
	if count == 0 {
		return FieldPrerender{}
	}

	var buf bytes.Buffer
	if err := t.CollectAll.Execute(&buf, c); err != nil {
		return FieldPrerender{}
	}
	return FieldPrerender{Value: buf.String(), Count: count}
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

func (c *collectOneCtx) Replace() {
	replacer := func(s *string) *string {
		if s == nil {
			return nil
		}
		tmp := replaceTablesToText(replaceImageToText(*s))
		tmp = policy.ProcessCustomHtmlTag(tmp)
		return utils.ToPtr(prepareToMail(prepareHtmlBody(policy.StripTagsPolicy, tmp)))
	}
	if c.Old != nil {
		c.Old.Body = replacer(c.Old.Body)
	}
	if c.New != nil {
		c.New.Body = replacer(c.New.Body)
	}
}
