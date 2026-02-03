package email

import (
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

type entitySpec[A dao.ActivityI, E dao.IDaoAct] struct {
	entityID func(A) uuid.UUID
	//isAdded     func(A) bool
	//isRemoved   func(A) bool
	entityTitle func(E) string
	loadRemoved func(*gorm.DB, []uuid.UUID) map[uuid.UUID]string
}

func BuildEntityChangeDigest[A dao.ActivityI, E dao.IDaoAct](tx *gorm.DB, activities []A, currentEntities []E, f entitySpec[A, E]) (views []DigestView, changesCount int) {

	current := make(map[uuid.UUID]E, len(currentEntities))
	for _, e := range currentEntities {
		current[e.GetId()] = e
	}

	changes := make(map[uuid.UUID]*TransitionFlags)

	for _, act := range activities {
		t := changes[f.entityID(act)]
		if t == nil {
			t = &TransitionFlags{}
			changes[f.entityID(act)] = t
		}

		if act.GetVerb() == actField.VerbAdded {
			t.Added = true
		}

		if act.GetVerb() == actField.VerbRemoved {
			t.Removed = true
		}
		//if f.isRemoved(act) {
		//}
	}

	views = make([]DigestView, 0, len(currentEntities))

	for _, e := range currentEntities {
		t := changes[e.GetId()]
		view := DigestView{
			Title: f.entityTitle(e),
		}

		if t != nil && t.Added && !t.Removed {
			view.IsNew = true
			changesCount++
		}

		views = append(views, view)
	}

	removedIDs := make([]uuid.UUID, 0)

	for id, t := range changes {
		if _, exists := current[id]; exists {
			continue
		}
		if t.Removed && !t.Added {
			removedIDs = append(removedIDs, id)
		}
	}

	if len(removedIDs) > 0 {
		removedTitles := f.loadRemoved(tx, removedIDs)

		for _, id := range removedIDs {
			title, ok := removedTitles[id]
			if !ok {
				title = "unknown"
			}

			views = append(views, DigestView{
				Title:  title,
				IsGone: true,
			})

			changesCount++
		}
	}

	return views, changesCount
}
