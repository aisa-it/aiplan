package email

import (
	"log/slog"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	member_role "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications/member-role"
	"gorm.io/gorm"
)

type LayerPipeline[A dao.ActivityI, E dao.IDaoAct] struct {
	Plan *emailPlan

	Load            func(tx *gorm.DB) []A
	Group           func([]A) ActivityBuckets[A, E]
	BuildRecipients func(tx *gorm.DB, acts []A, entity E) []member_role.MemberNotify
	BuildDigest     func(tx *gorm.DB, acts []A, entity E) (map[string]fieldPrerender, int)

	Subject      func(entity E) string
	BuildMessage func(bucket *ActivityBucket[A, E], r Recipient) EmailMessage

	FilterEmpty bool
}

func ProcessLayer[A dao.ActivityI, E dao.IDaoAct](es *EmailService, p LayerPipeline[A, E]) {
	if es.sending {
		return
	}

	es.sending = true
	defer func() { es.sending = false }()

	buckets := RunLayerPipeline(es.db, p)
	if len(buckets) == 0 {
		return
	}

	messages := BuildEmailMessages(buckets, p)
	if len(messages) == 0 {
		return
	}

	for _, m := range messages {
		if err := es.Send(m); err != nil {
			slog.Error("send email", "to", m.To, "err", err)
		}
	}
}

func RunLayerPipeline[A dao.ActivityI, E dao.IDaoAct](tx *gorm.DB, p LayerPipeline[A, E]) ActivityBuckets[A, E] {

	acts := p.Load(tx)
	if len(acts) == 0 {
		return nil
	}

	buckets := p.Group(acts)

	for id, b := range buckets {
		b.MemberNotify = p.BuildRecipients(tx, b.Activities, b.Entity)
		prepared, changes := p.BuildDigest(tx, b.Activities, b.Entity)
		if p.FilterEmpty && changes == 0 {
			delete(buckets, id)
			continue
		}

		b.Prepared = prepared
	}

	return buckets
}

func BuildEmailMessages[A dao.ActivityI, E dao.IDaoAct](buckets ActivityBuckets[A, E], p LayerPipeline[A, E]) []EmailMessage {

	var res []EmailMessage

	for _, b := range buckets {
		subject := p.Subject(b.Entity)

		for _, m := range b.MemberNotify {
			if r, ok := buildRecipient(&m); ok {
				msg := p.BuildMessage(b, *r)
				if msg.To == "" {
					continue
				}
				msg.Subject = subject
				res = append(res, msg)
			}
		}
	}

	return res
}
