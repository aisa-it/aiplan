package email

import (
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	member_role "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications/member-role"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

//type PipelineStep[A dao.ActivityI, E any] func(
//	tx *gorm.DB,
//	activities []A,
//) ([]LayerNotification, error)

//type PipelineConfig[A dao.ActivityI, E any] struct {
//	// Функции, специфичные для слоя
//	LoadActivities    func(tx *gorm.DB) ([]A, error)
//	GroupActivities   func(acts []A) map[uuid.UUID]sprintActivities // или generic
//	ResolveRecipients func(tx *gorm.DB, acts []A) []Recipient
//	BuildDigest       func(tx *gorm.DB, acts []A) (map[string]string, int)
//	FilterEmpty       bool
//}

func R() {

}

func BuildRecipientsFromActivities[A dao.ActivityI](
	tx *gorm.DB,
	acts []A,
	steps []member_role.UsersStep,
	actor func(A) *dao.User,
) []member_role.MemberNotify {

	users := make(member_role.UserRegistry)

	for _, step := range steps {
		if err := step(tx, users); err != nil {
			// лог тут
		}
	}

	for _, act := range acts {
		if u := actor(act); u != nil {
			users.AddUser(u, member_role.ActionAuthor)
		}
	}

	return utils.MapToSlice(users, func(k uuid.UUID, t *member_role.MemberNotify) member_role.MemberNotify {
		return *t
	})
}

func GroupActivitiesByLayer[A dao.ActivityI, E dao.IDaoAct](
	acts []A,
	layerID func(A) uuid.UUID,
	layer func(A) E,
) ActivityBuckets[A, E] {

	res := make(ActivityBuckets[A, E])

	for _, act := range acts {
		id := layerID(act)

		b, ok := res[id]
		if !ok {
			b = &ActivityBucket[A, E]{
				Entity:     layer(act),
				Activities: []A{act},
				FirstAt:    act.GetCreatedAt(),
				LastAt:     act.GetCreatedAt(),
			}
			res[id] = b
			continue
		}

		b.Activities = append(b.Activities, act)

		if act.GetCreatedAt().Before(b.FirstAt) {
			b.FirstAt = act.GetCreatedAt()
		}
		if act.GetCreatedAt().After(b.LastAt) {
			b.LastAt = act.GetCreatedAt()
		}
	}

	return res
}

type LayerPipeline[A dao.ActivityI, E dao.IDaoAct] struct {
	Load            func(tx *gorm.DB) []A
	Group           func([]A) ActivityBuckets[A, E]
	BuildRecipients func(tx *gorm.DB, acts []A, entity E) []member_role.MemberNotify
	BuildDigest     func(tx *gorm.DB, acts []A, entity E) (map[string]string, int)
	FilterEmpty     bool
}

func RunLayerPipeline[A dao.ActivityI, E dao.IDaoAct](
	tx *gorm.DB,
	p LayerPipeline[A, E],
) ActivityBuckets[A, E] {

	acts := p.Load(tx)
	if len(acts) == 0 {
		return nil
	}

	buckets := p.Group(acts)

	for id, b := range buckets {
		b.Recipients = p.BuildRecipients(tx, b.Activities, b.Entity)
		prepared, changes := p.BuildDigest(tx, b.Activities, b.Entity)
		if p.FilterEmpty && changes == 0 {
			delete(buckets, id)
			continue
		}

		b.Prepared = prepared
	}

	for _, b := range buckets {
		for _, r := range b.Recipients {
			//r.
		}
	}

	return buckets
}
