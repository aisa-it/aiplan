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

type activityBodyCtx struct {
	Title          string
	Body           string
	ActivityActors string
}

type finalBodyCtx struct {
	Title    string
	Changes  string
	HeadBody string
	Body     string
}

type headEntityCtx struct {
	WorkspaceName string
	Layer         string
	Identifier    string
	Title         string
	Url           string
	UrlText       string
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
	return FieldPrerender{
		Value:   t.ReplaceTxtToSvg(buf.String()),
		Count:   1,
		Start:   c.Start,
		End:     sql.NullTime{Time: c.Start.Time, Valid: c.Start.Valid},
		Authors: []dao.User{c.Author}}
}

func (t *EmailTemplates) RenderCollectAll(c collectAllCtx, count int) FieldPrerender {
	if count == 0 {
		return FieldPrerender{}
	}

	var buf bytes.Buffer
	if err := t.CollectAll.Execute(&buf, c); err != nil {
		return FieldPrerender{}
	}

	authors := utils.MapToSlice(c.Author, func(k uuid.UUID, t dao.User) dao.User { return t })
	return FieldPrerender{Value: buf.String(), Count: count, Start: c.Start, End: c.End, Authors: authors}
}

func (t *EmailTemplates) RenderActivity(c activityBodyCtx) string {
	var buf bytes.Buffer
	if err := t.Activity.Execute(&buf, c); err != nil {
		slog.Error("err", err.Error())
		return ""
	}
	return buf.String()
}

func (t *EmailTemplates) RenderChangesActivities(c ActivityActorView) string {
	var buf bytes.Buffer
	if err := t.ChangeCounter.Execute(&buf, c); err != nil {
		slog.Error("err", err.Error())
		return ""
	}
	return buf.String()
}

func (t *EmailTemplates) RenderHeadActivities(c headEntityCtx) string {
	var buf bytes.Buffer
	if err := t.HeadEntity.Execute(&buf, c); err != nil {
		slog.Error("err", err.Error())
		return ""
	}
	return buf.String()
}

func (t *EmailTemplates) RenderActivityAuthor(c ActivityActorView) string {
	var buf bytes.Buffer
	if err := t.AuthorActivity.Execute(&buf, c); err != nil {
		slog.Error("err", err.Error())
		return ""
	}
	return buf.String()
}

func (t *EmailTemplates) RenderBody(c finalBodyCtx) string {
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
