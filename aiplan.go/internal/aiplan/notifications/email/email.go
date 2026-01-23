package email

import (
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	memNotify "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications/member-role"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

//type NotifyMailPlan struct {
//	LayerName     string                   // спринт, задача, документ
//	Settings      memNotify.MemberSettings // project/workspace/doc settings
//	Entity        actField.ActivityField   // тип сущности для фильтрации
//	AuthorRole    memNotify.Role           // роль автора
//	PerLayerSteps []memNotify.UsersStep    // общие Step’ы для слоя
//}

//type LayerNotification struct {
//	LayerID    uuid.UUID
//	Recipients []Recipient
//	Title      string
//}

type Recipient struct {
	Email        string
	MemberNotify *memNotify.MemberNotify
}

// ----------
func buildRecipient(u *dao.User, m *memNotify.MemberNotify) (*Recipient, bool) {
	if u.Email == "" {
		return nil, false
	}
	if u.Settings.EmailNotificationMute {
		return nil, false
	}

	return &Recipient{
		Email:        u.Email,
		MemberNotify: m,
	}, true
}

func buildRecipients(users memNotify.UserRegistry) []Recipient {
	res := make([]Recipient, 0, len(users))
	for _, m := range users {
		if mail, ok := buildRecipient(m.GetUser(), m); ok {
			res = append(res, *mail)
		}
	}
	return res
}

//----------

type activityFieldCollector[T dao.ActivityI] func(T, map[string][]T)

func collectLast[T dao.ActivityI](act T, m map[string][]T) {
	key := act.GetField()

	if v, ok := m[key]; ok && len(v) > 0 && !v[0].GetCreatedAt().Before(act.GetCreatedAt()) {
		return
	}

	m[key] = []T{act}
}

func collectAll[T dao.ActivityI](act T, m map[string][]T) {
	key := act.GetField()
	m[key] = append(m[key], act)
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

type DigestView struct {
	Title  string
	IsNew  bool
	IsGone bool
}

type TransitionFlags struct {
	Added   bool
	Removed bool
}

func BuildEntityChangeDigest[A dao.ActivityI, E dao.IDaoAct](
	tx *gorm.DB, activities []A, currentEntities []E,
	entityID func(A) uuid.UUID,
	isAdded func(A) bool,
	isRemoved func(A) bool,
	entityTitle func(E) string,
	loadRemoved func(*gorm.DB, []uuid.UUID) map[uuid.UUID]string,
) (views []DigestView, changesCount int) {

	current := make(map[uuid.UUID]E, len(currentEntities))
	for _, e := range currentEntities {
		current[e.GetId()] = e
	}

	changes := make(map[uuid.UUID]*TransitionFlags)

	for _, act := range activities {
		t := changes[entityID(act)]
		if t == nil {
			t = &TransitionFlags{}
			changes[entityID(act)] = t
		}

		if isAdded(act) {
			t.Added = true
		}
		if isRemoved(act) {
			t.Removed = true
		}
	}

	views = make([]DigestView, 0, len(currentEntities))

	for _, e := range currentEntities {
		t := changes[e.GetId()]
		view := DigestView{
			Title: entityTitle(e),
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
		removedTitles := loadRemoved(tx, removedIDs)

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
