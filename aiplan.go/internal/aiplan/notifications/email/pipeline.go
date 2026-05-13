package email

import (
	"log/slog"
	"strings"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	member_role "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications/member-role"
	policy "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/redactor-policy"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

type LayerPipeline[E dao.IDaoAct] struct {
	Plan *emailPlan

	Load            func(tx *gorm.DB) []dao.ActivityEvent
	Group           func([]dao.ActivityEvent) ActivityBuckets[E]
	BuildRecipients func(tx *gorm.DB, acts []dao.ActivityEvent, entity E) ([]member_role.MemberNotify, EmailContext)
	BuildDigest     func(tx *gorm.DB, acts []dao.ActivityEvent, entity E) (map[string]FieldPrerender, int)
	BuildHead       func(entity E) string

	Subject func(entity E) string

	FilterEmpty bool
}

func ProcessLayer[E dao.IDaoAct](es *EmailService, p LayerPipeline[E], template EmailTemplates) {
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

func RunLayerPipeline[E dao.IDaoAct](tx *gorm.DB, p LayerPipeline[E]) ActivityBuckets[E] {

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
		b.HeadBody = p.BuildHead(b.Entity)
	}

	return buckets
}

//func ddd[A dao.ActivityI, E dao.IDaoAct](acts []A) string {
//  res headEntityCtx{
//    WorkspaceName: "",
//    Layer:         "",
//    Identifier:    "",
//    Title:         "",
//    Url:           "",
//    UrlText:       "",
//  }
//}

func BuildEmailMessages[E dao.IDaoAct](
	buckets ActivityBuckets[E],
	p LayerPipeline[E],
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

func BuildEmailMessage[E dao.IDaoAct](
	b *ActivityBucket[E], r Recipient, ctx *EmailContext, template EmailTemplates,
) EmailMessage {

	var parts []string
	var cnt int

	var visible []FieldPrerender

	for field, html := range b.Prepared {

		if !r.MemberNotify.Allowed(field, html.Verb, ctx.Plan.EntityType, ctx.Plan.AuthorRole, &member_role.MemberSettings{Notify: ctx.Settings}, types.EmailCh) {
			continue
		}

		html = msgReplace(*r.MemberNotify, html)
		visible = append(visible, html)

		parts = append(parts, html.Value)
		cnt += html.Count
	}

	if len(parts) == 0 {
		return EmailMessage{}
	}

	actorView := BuildActivityActorView(visible, &r.MemberNotify.GetUser().UserTimezone)
	if actorView == nil {
		return EmailMessage{}
	}
	sss := template.RenderActivityAuthor(*actorView)
	ccc := template.RenderChangesActivities(*actorView)

	body := activityBodyCtx{
		Body:           strings.Join(parts, "\n"),
		ActivityActors: sss,
	}

	activity := template.RenderActivity(body)

	html := finalBodyCtx{
		Title:    "eeee",
		HeadBody: b.HeadBody,
		Body:     activity,
		Changes:  ccc,
	}

	msg := template.RenderBody(html)

	return EmailMessage{
		To:          r.Email,
		Content:     msg,
		TextContent: policy.StripTagsPolicy.Sanitize(msg),
	}
}

func updateNotified[E dao.IDaoAct](
	tx *gorm.DB, p LayerPipeline[E], buckets ActivityBuckets[E]) {
	var ids []uuid.UUID
	for _, e := range buckets {

		ids = append(ids,
			utils.SliceToSlice(utils.ToPtr((*e).Activities), func(t *dao.ActivityEvent) uuid.UUID { return (*t).ID })...)
	}

	if err := tx.Model(&dao.ActivityEvent{}).Where("id IN (?)", ids).Update("notified", true).Error; err != nil {
		slog.Error(err.Error())
	}
}
