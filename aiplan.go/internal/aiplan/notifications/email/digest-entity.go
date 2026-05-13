package email

import (
	"database/sql"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

type entitySpec[E dao.IDaoAct] struct {
	entityID    func(dao.ActivityEvent) uuid.UUID
	entityTitle func(E) string
	loadRemoved func(*gorm.DB, []uuid.UUID) map[uuid.UUID]string
	//getAuthor   func(dao.ActivityEvent) *dao.User
}

type EntityChangeMeta struct {
	Authors map[uuid.UUID]dao.User
	Start   sql.NullTime
	End     sql.NullTime
}

type transitionFlags struct {
	Added   bool
	Removed bool
}

func BuildEntityChangeDigest[E dao.IDaoAct](
	tx *gorm.DB,
	activities []dao.ActivityEvent,
	current []E,
	spec entitySpec[E],
) (
	views []DigestView,
	meta EntityChangeMeta,
	count int,
) {

	// ---------- CURRENT ----------
	currentMap := make(map[uuid.UUID]E, len(current))
	for _, e := range current {
		currentMap[e.GetId()] = e
	}

	// ---------- TRANSITIONS ----------
	changes := make(map[uuid.UUID]*transitionFlags)

	// ---------- META ----------
	authors := make(map[uuid.UUID]dao.User)

	var (
		start   time.Time
		end     time.Time
		hasTime bool
	)

	for _, act := range activities {
		id := spec.entityID(act)

		t := changes[id]
		if t == nil {
			t = &transitionFlags{}
			changes[id] = t
		}

		switch act.Verb {
		case actField.VerbAdded:
			t.Added = true
		case actField.VerbRemoved:
			t.Removed = true
		}

		// AUTHOR
		if u := act.Actor; u != nil {
			authors[u.ID] = *u
		}

		// TIME
		at := act.CreatedAt // time.Time
		if !hasTime {
			start = at
			end = at
			hasTime = true
		} else {
			if at.Before(start) {
				start = at
			}
			if at.After(end) {
				end = at
			}
		}
	}

	// ---------- CURRENT VIEWS ----------
	for _, e := range current {
		t := changes[e.GetId()]

		view := DigestView{
			Title: spec.entityTitle(e),
		}

		if t != nil && t.Added && !t.Removed {
			view.IsNew = true
			count++
		}

		views = append(views, view)
	}

	// ---------- REMOVED ----------
	var removedIDs []uuid.UUID
	for id, t := range changes {
		if _, ok := currentMap[id]; ok {
			continue
		}
		if t.Removed && !t.Added {
			removedIDs = append(removedIDs, id)
		}
	}

	if len(removedIDs) > 0 {
		titles := spec.loadRemoved(tx, removedIDs)

		for _, id := range removedIDs {
			title := titles[id]
			if title == "" {
				title = "unknown"
			}

			views = append(views, DigestView{
				Title:  title,
				IsGone: true,
			})

			count++
		}
	}

	// ---------- META FINAL ----------
	meta.Authors = authors

	if hasTime {
		meta.Start = sql.NullTime{Time: start, Valid: true}
		meta.End = sql.NullTime{Time: end, Valid: true}
	}

	return views, meta, count
}
