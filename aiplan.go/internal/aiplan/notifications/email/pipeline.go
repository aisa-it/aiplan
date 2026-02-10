package email

import (
	"log/slog"
	"strings"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	member_role "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications/member-role"
	policy "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/redactor-policy"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

type LayerPipeline[A dao.ActivityI, E dao.IDaoAct] struct {
	TableName string

	Plan *emailPlan

	Load            func(tx *gorm.DB) []A
	Group           func([]A) ActivityBuckets[A, E]
	BuildRecipients func(tx *gorm.DB, acts []A, entity E) ([]member_role.MemberNotify, EmailContext)
	BuildDigest     func(tx *gorm.DB, acts []A, entity E) (map[string]FieldPrerender, int)

	Subject func(entity E) string

	FilterEmpty bool
}

func ProcessLayer[A dao.ActivityI, E dao.IDaoAct](es *EmailService, p LayerPipeline[A, E], template EmailTemplates) {
	if es.sending {
		return
	}

	es.sending = true
	defer func() { es.sending = false }()

	buckets := RunLayerPipeline(es.db, p)
	if len(buckets) == 0 {
		return
	}

	messages := BuildEmailMessages(buckets, p, template)
	if len(messages) == 0 {
		return
	}

	for _, m := range messages {
		if err := es.Send(m); err != nil {
			slog.Error("send email", "to", m.To, "err", err)
		}
	}

	updateNotified(es.db, p, buckets)
}

func RunLayerPipeline[A dao.ActivityI, E dao.IDaoAct](tx *gorm.DB, p LayerPipeline[A, E]) ActivityBuckets[A, E] {

	acts := p.Load(tx)
	if len(acts) == 0 {
		return nil
	}

	buckets := p.Group(acts)

	for id, b := range buckets {

		b.MemberNotify, b.Ctx = p.BuildRecipients(tx, b.Activities, b.Entity)
		prepared, changes := p.BuildDigest(tx, b.Activities, b.Entity)
		if p.FilterEmpty && changes == 0 {
			delete(buckets, id)
			continue
		}

		b.Prepared = prepared
	}

	return buckets
}

func BuildEmailMessages[A dao.ActivityI, E dao.IDaoAct](
	buckets ActivityBuckets[A, E],
	p LayerPipeline[A, E],
	template EmailTemplates,
) []EmailMessage {

	var res []EmailMessage

	for _, b := range buckets {
		subject := p.Subject(b.Entity) // берем subject из pipeline

		for _, m := range b.MemberNotify {
			r, ok := buildRecipient(&m)
			if !ok {
				continue
			}

			msg := BuildEmailMessage(b, *r, &b.Ctx, template) // ctx из bucket для Allowed()
			if msg.To == "" {
				continue
			}

			msg.Subject = subject
			res = append(res, msg)
		}
	}

	return res
}

func BuildEmailMessage[A dao.ActivityI, E dao.IDaoAct](
	b *ActivityBucket[A, E], r Recipient, ctx *EmailContext, template EmailTemplates,
) EmailMessage {

	var parts []string
	var cnt int

	for field, html := range b.Prepared {
		if !r.MemberNotify.Allowed(field, html.Verb, ctx.Plan.Entity, ctx.Plan.AuthorRole, &ctx.Settings) {
			continue
		}
		html = msgReplace(*r.MemberNotify, html)
		parts = append(parts, html.Value)
		cnt += html.Count
	}

	if len(parts) == 0 {
		return EmailMessage{}
	}

	body := bodyCtx{
		Body: strings.Join(parts, "\n"),
	}

	activity := template.RenderActivity(body)

	html := bodyCtx{
		Title: "eeee",
		Body:  activity,
	}

	msg := template.RenderBody(html)

	return EmailMessage{
		To:   r.Email,
		HTML: msg,
		Text: policy.StripTagsPolicy.Sanitize(msg),
	}
}

func updateNotified[A dao.ActivityI, E dao.IDaoAct](
	tx *gorm.DB, p LayerPipeline[A, E], buckets ActivityBuckets[A, E]) {
	var ids []uuid.UUID
	for _, e := range buckets {

		ids = append(ids,
			utils.SliceToSlice(utils.ToPtr((*e).Activities), func(t *A) uuid.UUID { return (*t).GetId() })...)
	}

	if err := tx.Table(p.TableName).Where("id IN (?)", ids).Update("notified", true).Error; err != nil {
		slog.Error(err.Error())
	}
}
