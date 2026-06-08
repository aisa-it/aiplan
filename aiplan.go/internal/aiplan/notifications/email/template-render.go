package email

import (
	"bytes"
	"database/sql"
	"log/slog"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
)

type ActivityActorView struct {
	Actors []dao.User

	AuthorsCount  int
	ActivityCount int
	CommentCount  int

	Start time.Time
	End   time.Time

	IsPeriod bool
}

func (t *EmailTemplates) RenderChangesActivities(c ActivityActorView) string {
	var buf bytes.Buffer
	if err := t.ChangeCounter.Execute(&buf, c); err != nil {
		slog.Error("RenderChangesActivities", "err", err.Error())
		return ""
	}
	return buf.String()
}

func (t *EmailTemplates) RenderActivityAuthor(c ActivityActorView) string {
	var buf bytes.Buffer
	if err := t.AuthorActivity.Execute(&buf, c); err != nil {
		slog.Error("RenderActivityAuthor", "err", err.Error())
		return ""
	}
	return buf.String()
}

type actValueCtx struct {
	Value *string
	Body  *string
}

type collectOneCtx struct {
	Key string
	New *actValueCtx
	Old *actValueCtx

	Start  sql.NullTime
	Author dao.User
}

func (c *collectOneCtx) Replace() {
	if c.Old != nil {
		c.Old.Body = htmlReplacer(c.Old.Body)
	}
	if c.New != nil {
		c.New.Body = htmlReplacer(c.New.Body)
	}
}

func (t *EmailTemplates) RenderCollectOne(c collectOneCtx) FieldPrerender {
	c.Replace()
	var buf bytes.Buffer
	if err := t.CollectOne.Execute(&buf, c); err != nil {
		slog.Error("Template execute failed", "template", "CollectOne", "error", err)
		return FieldPrerender{}
	}
	return FieldPrerender{
		Value:         t.ReplaceTxtToSvg(buf.String()),
		Count:         1,
		Start:         c.Start,
		End:           sql.NullTime{Time: c.Start.Time, Valid: c.Start.Valid},
		prerenderType: optPrerenderOne,
		Authors:       []dao.User{c.Author},
	}
}

type collectAllCtx struct {
	Key   string
	Views []DigestView

	Start sql.NullTime
	End   sql.NullTime

	Author map[uuid.UUID]dao.User
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
	return FieldPrerender{
		Value:         buf.String(),
		Count:         count,
		Start:         c.Start,
		End:           c.End,
		prerenderType: optPrerenderAll,
		Authors:       authors,
	}
}

type collectComplexCtx struct {
	Key   string
	Value string

	Start  sql.NullTime
	End    sql.NullTime
	Author map[uuid.UUID]dao.User
}

func (t *EmailTemplates) RenderCollectComplex(c collectComplexCtx, count int) FieldPrerender {
	var buf bytes.Buffer
	if err := t.CollectComplex.Execute(&buf, c); err != nil {
		slog.Error("Template execute failed", "template", "CollectComplex", "error", err)
		return FieldPrerender{}
	}
	return FieldPrerender{
		Value:         t.ReplaceTxtToSvg(buf.String()),
		Count:         count,
		Start:         c.Start,
		End:           c.End,
		prerenderType: optPrerenderComplex,
		Authors:       utils.MapToSlice(c.Author, func(k uuid.UUID, t dao.User) dao.User { return t }),
	}
}

func (t *EmailTemplates) RenderCollectValues(d DigestComplexView) FieldPrerender {
	var buf bytes.Buffer
	if err := t.CollectValues.Execute(&buf, d); err != nil {
		slog.Error("Template execute failed", "template", "CollectComplex", "error", err)
		return FieldPrerender{}
	}
	return FieldPrerender{
		Value: t.ReplaceTxtToSvg(buf.String()),
	}
}

// ----- email elements

type activityBodyCtx struct {
	Title          string
	Body           string
	ActivityActors string
}

func (t *EmailTemplates) RenderActivity(c activityBodyCtx) string {
	var buf bytes.Buffer
	if err := t.Activity.Execute(&buf, c); err != nil {
		slog.Error("RenderActivity", "err", err.Error())
		return ""
	}
	return buf.String()
}

type finalBodyCtx struct {
	Title    string
	Changes  string
	HeadBody string
	Body     string
}

func (t *EmailTemplates) RenderBody(c finalBodyCtx) string {
	var buf bytes.Buffer
	if err := t.Body.Execute(&buf, c); err != nil {
		slog.Error("RenderBody", "err", err.Error())
		return ""
	}
	return buf.String()
}

type headEntityCtx struct {
	WorkspaceName string
	Layer         string
	Identifier    string
	Title         string
	Url           string
	UrlText       string
}

func (t *EmailTemplates) RenderHead(ddd headEntityCtx) string {
	// plain-text поля заголовка экранируем — шаблон рендерится через text/template
	ddd.WorkspaceName = escapeText(ddd.WorkspaceName)
	ddd.Identifier = escapeText(ddd.Identifier)
	ddd.Title = escapeText(ddd.Title)
	var buf bytes.Buffer
	if err := t.HeadEntity.Execute(&buf, ddd); err != nil {
		slog.Error("RenderHead", "err", err.Error())
		return ""
	}
	return buf.String()
}
