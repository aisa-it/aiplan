package email

import (
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	member_role "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications/member-role"
	"github.com/gofrs/uuid"
)

type ActivityBuckets[E dao.IDaoAct] map[uuid.UUID]*ActivityBucket[E]

type ActivityBucket[E dao.IDaoAct] struct {
	Entity     E
	Activities []dao.ActivityEvent

	MemberNotify []member_role.MemberNotify

	HeadBody string
	Prepared map[string]FieldPrerender

	FirstAt time.Time
	LastAt  time.Time
	Ctx     EmailContext
}

func GroupActivitiesByLayer[E dao.IDaoAct](
	acts []dao.ActivityEvent, layerID func(event dao.ActivityEvent) uuid.UUID, layer func(event dao.ActivityEvent) E,
) ActivityBuckets[E] {

	res := make(ActivityBuckets[E])

	for _, act := range acts {
		id := layerID(act)

		b, ok := res[id]
		if !ok {
			b = &ActivityBucket[E]{
				Entity:     layer(act),
				Activities: []dao.ActivityEvent{act},
				FirstAt:    act.CreatedAt,
				LastAt:     act.CreatedAt,
			}
			res[id] = b
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
