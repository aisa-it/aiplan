package email

import (
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	memNotify "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications/member-role"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

type Recipient struct {
	Email        string
	MemberNotify *memNotify.MemberNotify
}

// ----------
func buildRecipient(m *memNotify.MemberNotify) (*Recipient, bool) {
	user := m.GetUser()
	if user == nil {
		return nil, false
	}
	if user.Email == "" {
		return nil, false
	}
	if user.Settings.EmailNotificationMute {
		return nil, false
	}

	return &Recipient{
		Email:        user.Email,
		MemberNotify: m,
	}, true
}

//----------

type activityFieldCollector[T dao.ActivityI] func(T, map[string][]T)

func collectOne[T dao.ActivityI](act T, m map[string][]T) {
	key := act.GetField()

	if v, ok := m[key]; ok && len(v) > 0 && !v[0].GetCreatedAt().Before(act.GetCreatedAt()) {
		return
	}

	m[key] = []T{act}
}

type collectOneCtx struct {
	Key string
	New *actValueCtx
	Old *actValueCtx
}

type actValueCtx struct {
	Value *string
	Body  *string
}

func collectAll[T dao.ActivityI](act T, m map[string][]T) {
	key := act.GetField()
	m[key] = append(m[key], act)
}

type collectAllCtx struct {
	Key   string
	Views []DigestView
}

type DigestView struct {
	Title  string
	IsNew  bool
	IsGone bool
}

func CollectActivitiesByField[T dao.ActivityI](
	acts []T,
	collectors map[actField.ActivityField]activityFieldCollector[T],
) map[string][]T {

	result := make(map[string][]T)

	for _, act := range acts {
		key := act.GetField()

		collector, ok := collectors[actField.ActivityField(key)]
		if !ok {
			continue
		}

		collector(act, result)
	}

	return result
}

type TransitionFlags struct {
	Added   bool
	Removed bool
}

type entitySpec[A dao.ActivityI, E dao.IDaoAct] struct {
	entityID    func(A) uuid.UUID
	isAdded     func(A) bool
	isRemoved   func(A) bool
	entityTitle func(E) string
	loadRemoved func(*gorm.DB, []uuid.UUID) map[uuid.UUID]string
}

func BuildEntityChangeDigest[A dao.ActivityI, E dao.IDaoAct](
	tx *gorm.DB, activities []A, currentEntities []E,
	f entitySpec[A, E],
	//entityID func(A) uuid.UUID,
	//isAdded func(A) bool,
	//isRemoved func(A) bool,
	//entityTitle func(E) string,
	//loadRemoved func(*gorm.DB, []uuid.UUID) map[uuid.UUID]string,
) (views []DigestView, changesCount int) {

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

		if f.isAdded(act) {
			t.Added = true
		}
		if f.isRemoved(act) {
			t.Removed = true
		}
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
