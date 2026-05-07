package tracker

import (
	"fmt"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/gofrs/uuid"
)

func (t *SnapshotTracker) TrackDocMove(doc *dao.Doc, oldParent, newParent *dao.Doc, actor *dao.User) error {
	fromType := types.LayerWorkspace.String()
	if oldParent != nil {
		fromType = types.LayerDoc.String()
	}
	toType := types.LayerWorkspace.String()
	if newParent != nil {
		toType = types.LayerDoc.String()
	}
	moveVerb := fmt.Sprintf("move_%s_to_%s", fromType, toType)

	if err := t.createDocActivityLike(moveVerb, doc, actor, oldParent, newParent, types.LayerDoc); err != nil {
		return err
	}

	addedLayer := types.LayerDoc
	if newParent == nil {
		addedLayer = types.LayerWorkspace
	}
	if err := t.createDocActivityLike(actField.VerbAdded, doc, actor, oldParent, newParent, addedLayer); err != nil {
		return err
	}

	removedLayer := types.LayerDoc
	if oldParent == nil {
		removedLayer = types.LayerWorkspace
	}
	if err := t.createDocActivityLike(actField.VerbRemoved, doc, actor, oldParent, newParent, removedLayer); err != nil {
		return err
	}

	return nil
}

func (t *SnapshotTracker) createDocActivityLike(verb string, doc *dao.Doc, actor *dao.User, oldParent, newParent *dao.Doc, entityLayer types.EntityLayer) error {
	event := dao.ActivityEvent{
		ID:          dao.GenUUID(),
		CreatedAt:   time.Now(),
		ActorID:     actor.ID,
		Actor:       actor,
		Verb:        verb,
		Field:       actField.Doc.Field,
		EntityType:  entityLayer,
		WorkspaceID: uuid.NullUUID{UUID: doc.WorkspaceId, Valid: true},
	}

	switch verb {
	case actField.VerbRemoved:
		t.docRemovedActivity(&event, doc, oldParent)
	case actField.VerbAdded:
		t.docAddedActivity(&event, doc, newParent)
	case actField.VerbMoveDocDoc, actField.VerbMoveWorkspaceDoc, actField.VerbMoveDocWorkspace:
		t.docMoveActivity(&event, doc, oldParent, newParent)
	}

	return t.saveAndNotifyActivity(&event)
}

func (t *SnapshotTracker) docRemovedActivity(event *dao.ActivityEvent, doc *dao.Doc, oldParent *dao.Doc) {
	if oldParent != nil {
		event.DocID = uuid.NullUUID{UUID: oldParent.ID, Valid: true}
	} else {
		event.WorkspaceID = uuid.NullUUID{UUID: doc.WorkspaceId, Valid: true}
	}

	event.NewValue = ""
	event.NewIdentifier = uuid.NullUUID{}
	event.OldValue = doc.Title
	event.OldIdentifier = uuid.NullUUID{UUID: doc.ID, Valid: true}
}

func (t *SnapshotTracker) docAddedActivity(event *dao.ActivityEvent, doc *dao.Doc, newParent *dao.Doc) {
	if newParent != nil {
		event.DocID = uuid.NullUUID{UUID: newParent.ID, Valid: true}
	} else {
		event.WorkspaceID = uuid.NullUUID{UUID: doc.WorkspaceId, Valid: true}
	}

	event.NewValue = doc.Title
	event.NewIdentifier = uuid.NullUUID{UUID: doc.ID, Valid: true}
	event.OldValue = ""
}

func (t *SnapshotTracker) docMoveActivity(event *dao.ActivityEvent, doc *dao.Doc, oldParent, newParent *dao.Doc) {
	event.DocID = uuid.NullUUID{UUID: doc.ID, Valid: true}

	if oldParent != nil {
		event.OldValue = oldParent.Title
		event.OldIdentifier = uuid.NullUUID{UUID: oldParent.ID, Valid: true}
	} else {
		if doc.Workspace != nil {
			event.OldValue = doc.Workspace.Name
		}
		event.OldIdentifier = uuid.NullUUID{UUID: doc.WorkspaceId, Valid: true}
	}

	if newParent != nil {
		event.NewValue = newParent.Title
		event.NewIdentifier = uuid.NullUUID{UUID: newParent.ID, Valid: true}
	} else {
		if doc.Workspace != nil {
			event.NewValue = doc.Workspace.Name
		}
		event.NewIdentifier = uuid.NullUUID{UUID: doc.WorkspaceId, Valid: true}
	}
}
