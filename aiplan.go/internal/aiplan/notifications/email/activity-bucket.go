package email

import (
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	member_role "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications/member-role"
	"github.com/gofrs/uuid"
)

type ActivityBuckets[A dao.ActivityI, E dao.IDaoAct] map[uuid.UUID]*ActivityBucket[A, E]

type ActivityBucket[A dao.ActivityI, E dao.IDaoAct] struct {
	Entity     E
	Activities []A

	MemberNotify []member_role.MemberNotify

	HeadBody string
	Prepared map[string]FieldPrerender

	FirstAt time.Time
	LastAt  time.Time
	Ctx     EmailContext
}

func GroupActivitiesByLayer[A dao.ActivityI, E dao.IDaoAct](
	acts []A, layerID func(A) uuid.UUID, layer func(A) E,
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
