package email

import (
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/gofrs/uuid"
)

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
