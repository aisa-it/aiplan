package email

import (
	"log/slog"
	"strings"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	member_role "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications/member-role"
	policy "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/redactor-policy"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

type EmailProcessor interface {
	LoadActivities(tx *gorm.DB) []dao.ActivityEvent
	GroupActivities(acts []dao.ActivityEvent) ActivityBuckets
	BuildRecipients(tx *gorm.DB, acts []dao.ActivityEvent, entity dao.IDaoAct) ([]member_role.MemberNotify, EmailContext)
	BuildDigest(tx *gorm.DB, templates *EmailTemplates, acts []dao.ActivityEvent, entity dao.IDaoAct) (map[string]FieldPrerender, int)
	BuildSubject(entity dao.IDaoAct) string
	BuildHead(templates *EmailTemplates, entity dao.IDaoAct) string
	FullLoad(tx *gorm.DB, entity dao.IDaoAct) dao.IDaoAct
}

type ActivityBuckets map[uuid.UUID]*ActivityBucket

type ActivityBucket struct {
	Entity     dao.IDaoAct
	Activities []dao.ActivityEvent

	MemberNotify []member_role.MemberNotify

	HeadBody string
	Prepared map[string]FieldPrerender

	FirstAt time.Time
	LastAt  time.Time
	Ctx     EmailContext
}

type emailPlan struct {
	EntityType types.EntityLayer
	AuthorRole member_role.Role
}

type EmailContext struct {
	Settings       member_role.IsNotifyFunc
	Steps          []member_role.UsersStep
	Plan           *emailPlan
	CustomRoleFunc []func(act dao.ActivityEvent) []member_role.UsersStep
}

func ProcessLayer(es *EmailService, p EmailProcessor, templates EmailTemplates) {
	es.sendingMutex.Lock()
	if es.sending {
		es.sendingMutex.Unlock()
		return
	}
	es.sending = true
	es.sendingMutex.Unlock()

	defer func() {
		es.sendingMutex.Lock()
		es.sending = false
		es.sendingMutex.Unlock()
	}()

	buckets, delBuckets := RunLayerPipeline(es.db, p, &templates)
	updateNotified(es.db, delBuckets)
	if len(buckets) == 0 {
		return
	}

	defer updateNotified(es.db, buckets)

	messages := BuildEmailMessages(buckets, p, templates)
	if len(messages) == 0 {
		return
	}

	for _, m := range messages {
		if err := es.Send(m); err != nil {
			slog.Error("send email", "to", m.To, "err", EmailError{Type: "send", User: m.To, Err: err})
		}
	}
}

func RunLayerPipeline(tx *gorm.DB, p EmailProcessor, templates *EmailTemplates) (ActivityBuckets, ActivityBuckets) {

	acts := p.LoadActivities(tx)
	if len(acts) == 0 {
		return nil, nil
	}

	deleteBuckets := make(ActivityBuckets)

	buckets := p.GroupActivities(acts)

	for id, b := range buckets {
		b.Entity = p.FullLoad(tx, b.Entity)
		b.MemberNotify, b.Ctx = p.BuildRecipients(tx, b.Activities, b.Entity)

		prepared, changes := p.BuildDigest(tx, templates, b.Activities, b.Entity)
		if changes == 0 {
			deleteBuckets[id] = b
			delete(buckets, id)
			continue
		}

		b.Prepared = prepared
		b.HeadBody = p.BuildHead(templates, b.Entity)
	}

	return buckets, deleteBuckets
}

func BuildEmailMessages(buckets ActivityBuckets, p EmailProcessor, template EmailTemplates) []EmailMessage {

	var res []EmailMessage

	for _, b := range buckets {
		subject := p.BuildSubject(b.Entity.(dao.IDaoAct))

		for _, m := range b.MemberNotify {
			r, ok := buildRecipient(&m)
			if !ok {
				continue
			}

			msg := BuildEmailMessage(b, *r, &b.Ctx, template)
			if msg.To == "" {
				continue
			}

			msg.Subject = subject
			res = append(res, msg)
		}
	}

	return res
}

func filterVisibleFields(b *ActivityBucket, r Recipient, ctx *EmailContext) ([]FieldPrerender, []string) {
	visible := make([]FieldPrerender, 0, len(b.Prepared))
	parts := make([]string, 0, len(b.Prepared))

	for field, html := range b.Prepared { //todo переписать активиди идс
		needActionAuthor := ctx.Plan.AuthorRole == member_role.ActionAuthor &&
			!isUserInAuthors(html.Authors, r.MemberNotify.GetUser().Email)

		if needActionAuthor {
			r.MemberNotify.Toggle(member_role.ActionAuthor)
		}

		allowed := r.MemberNotify.Allowed(field, html.Verb, ctx.Plan.EntityType, ctx.Plan.AuthorRole, &member_role.MemberSettings{Notify: ctx.Settings}, types.EmailCh)

		if needActionAuthor {
			r.MemberNotify.Toggle(member_role.ActionAuthor)
		}

		if !allowed {
			continue
		}

		if !r.MemberNotify.IsActNotify(html.ActivityIds) {
			continue
		}

		html = msgReplace(*r.MemberNotify, html)
		visible = append(visible, html)
		parts = append(parts, html.GetValue())
	}

	return visible, parts
}

func buildActorView(acts []FieldPrerender, tz *types.TimeZone) *ActivityActorView {
	authors := make(map[uuid.UUID]dao.User)
	var (
		start        time.Time
		end          time.Time
		init         bool
		count        int
		commentCount int
	)

	for _, fp := range acts {
		if fp.Count == 0 {
			continue
		}

		if fp.Field == actField.Comment.Field {
			commentCount += fp.Count
		} else {
			count += fp.Count
		}

		for _, author := range fp.Authors {
			if author.ID != uuid.Nil {
				authors[author.ID] = author
			}
		}

		if fp.Start.Valid {
			t := fp.Start.Time
			if !init {
				start, end = t, t
				init = true
			} else {
				if t.Before(start) {
					start = t
				}
				if t.After(end) {
					end = t
				}
			}
		}

		if fp.End.Valid {
			t := fp.End.Time
			if !init {
				start, end = t, t
				init = true
			} else {
				if t.Before(start) {
					start = t
				}
				if t.After(end) {
					end = t
				}
			}
		}
	}

	if count == 0 && commentCount == 0 {
		return nil
	}

	actors := make([]dao.User, 0, len(authors))
	for _, u := range authors {
		actors = append(actors, u)
	}

	loc := (*time.Location)(tz)
	start = start.In(loc)
	end = end.In(loc)

	// период (>= 3 секунд)
	isPeriod := false
	if !start.IsZero() && !end.IsZero() {
		isPeriod = end.Sub(start) >= 3*time.Second
	}

	return &ActivityActorView{
		Actors:        actors,
		AuthorsCount:  len(actors),
		ActivityCount: count,
		CommentCount:  commentCount,
		Start:         start,
		End:           end,
		IsPeriod:      isPeriod,
	}
}

func renderBody(actorView *ActivityActorView, parts []string, template EmailTemplates) (string, string) {
	sss := template.RenderActivityAuthor(*actorView)
	ccc := template.RenderChangesActivities(*actorView)

	body := activityBodyCtx{
		Body:           strings.Join(parts, "\n"),
		ActivityActors: sss,
	}

	activity := template.RenderActivity(body)

	return activity, ccc
}

func finalRender(activity, changes, headBody string, actorView *ActivityActorView, template EmailTemplates) (string, string) {
	from := actorView.Actors[0].GetName()
	if actorView.AuthorsCount > 1 {
		from += " и др."
	}

	html := finalBodyCtx{
		Title:    "",
		HeadBody: headBody,
		Body:     activity,
		Changes:  changes,
	}

	msg := template.RenderBody(html)

	return msg, from
}

func BuildEmailMessage(b *ActivityBucket, r Recipient, ctx *EmailContext, template EmailTemplates) EmailMessage {

	visible, parts := filterVisibleFields(b, r, ctx)

	if len(parts) == 0 {
		return EmailMessage{}
	}

	actorView := buildActorView(visible, &r.MemberNotify.GetUser().UserTimezone)
	if actorView == nil {
		return EmailMessage{}
	}

	activity, changes := renderBody(actorView, parts, template)
	msg, from := finalRender(activity, changes, b.HeadBody, actorView, template)

	return EmailMessage{
		Actor:       utils.ToPtr(from),
		To:          r.Email,
		Content:     msg,
		TextContent: policy.StripTagsPolicy.Sanitize(msg),
	}
}

func GroupActivitiesByLayer(acts []dao.ActivityEvent, getLayer func(event dao.ActivityEvent) dao.IDaoAct) ActivityBuckets {
	res := make(ActivityBuckets)

	for _, act := range acts {
		entity := getLayer(act)

		b, ok := res[entity.GetId()]
		if !ok {
			b = &ActivityBucket{
				Entity:     entity,
				Activities: []dao.ActivityEvent{act},
				FirstAt:    act.CreatedAt,
				LastAt:     act.CreatedAt,
			}
			res[entity.GetId()] = b
			continue
		}

		b.Activities = append(b.Activities, act)

		if act.CreatedAt.Before(b.FirstAt) {
			b.FirstAt = act.CreatedAt
		}

		if act.CreatedAt.After(b.LastAt) {
			b.LastAt = act.CreatedAt
		}
	}

	return res
}

func updateNotified(tx *gorm.DB, buckets ActivityBuckets) {
	var ids []uuid.UUID
	for _, e := range buckets {
		ids = append(ids, utils.SliceToSlice(utils.ToPtr(e.Activities), func(t *dao.ActivityEvent) uuid.UUID { return (*t).ID })...)
	}

	if err := tx.Model(&dao.ActivityEvent{}).Where("id IN (?)", ids).Update("notified", true).Error; err != nil {
		slog.Error(err.Error())
	}
}

func isUserInAuthors(authors []dao.User, userEmail string) bool {
	for _, author := range authors {
		if author.Email == userEmail {
			return true
		}
	}
	return false
}
