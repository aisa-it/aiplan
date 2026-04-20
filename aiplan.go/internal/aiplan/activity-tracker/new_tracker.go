package tracker

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

type SnapshotTracker struct {
	db       *gorm.DB
	handlers []ActHandler
}

func NewSnapshotTracker(db *gorm.DB) *SnapshotTracker {
	return &SnapshotTracker{db: db}
}

func (t *SnapshotTracker) RegisterHandler(handler ActHandler) {
	t.handlers = append(t.handlers, handler)
}

func (t *SnapshotTracker) GetDB() *gorm.DB {
	return t.db
}

func (t *SnapshotTracker) saveAndNotifyActivity(event *dao.ActivityEvent) error {
	if err := t.db.Create(event).Error; err != nil {
		return err
	}

	if err := event.AfterFind(t.db); err != nil {
		slog.Error("failed to execute AfterFind", "error", err)
		return err
	}
	for _, handler := range t.handlers {
		if err := handler.Handle(*event); err != nil {
			slog.Error("activity handler failed", "error", err)
		}
	}

	return nil
}

func (t *SnapshotTracker) TrackDocMove(doc *dao.Doc, oldParent, newParent *dao.Doc, actor *dao.User) error {
	fromType := "workspace"
	if oldParent != nil {
		fromType = "doc"
	}
	toType := "workspace"
	if newParent != nil {
		toType = "doc"
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
		ID:          uuid.Must(uuid.NewV4()),
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
		t.configureRemovedActivity(&event, doc, oldParent)
	case actField.VerbAdded:
		t.configureAddedActivity(&event, doc, newParent)
	case "move_doc_to_doc", "move_workspace_to_doc", "move_doc_to_workspace":
		t.configureMoveActivity(&event, doc, oldParent, newParent)
	}

	return t.saveAndNotifyActivity(&event)
}

func (t *SnapshotTracker) configureRemovedActivity(event *dao.ActivityEvent, doc *dao.Doc, oldParent *dao.Doc) {
	if oldParent != nil {
		event.DocID = uuid.NullUUID{UUID: oldParent.ID, Valid: true}
	} else {
		event.WorkspaceID = uuid.NullUUID{UUID: doc.WorkspaceId, Valid: true}
	}

	event.NewValue = ""
	event.NewIdentifier = uuid.NullUUID{}
	event.OldValue = &doc.Title
	event.OldIdentifier = uuid.NullUUID{UUID: doc.ID, Valid: true}
}

func (t *SnapshotTracker) configureAddedActivity(event *dao.ActivityEvent, doc *dao.Doc, newParent *dao.Doc) {
	if newParent != nil {
		event.DocID = uuid.NullUUID{UUID: newParent.ID, Valid: true}
	} else {
		event.WorkspaceID = uuid.NullUUID{UUID: doc.WorkspaceId, Valid: true}
	}

	event.NewValue = doc.Title
	event.NewIdentifier = uuid.NullUUID{UUID: doc.ID, Valid: true}
	event.OldValue = &[]string{""}[0]
}

func (t *SnapshotTracker) configureMoveActivity(event *dao.ActivityEvent, doc *dao.Doc, oldParent, newParent *dao.Doc) {
	event.DocID = uuid.NullUUID{UUID: doc.ID, Valid: true}

	if oldParent != nil {
		event.OldValue = &oldParent.Title
		event.OldIdentifier = uuid.NullUUID{UUID: oldParent.ID, Valid: true}
	} else {
		if doc.Workspace != nil {
			event.OldValue = &doc.Workspace.Name
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

func (t *SnapshotTracker) TrackChanges(layer types.EntityLayer, oldSnapshot, newSnapshot SnapshotI, entity dao.IDaoAct, actor *dao.User) error {
	if oldSnapshot == nil && newSnapshot != nil {
		return t.trackCreate(layer, newSnapshot, entity, actor)
	}

	if newSnapshot == nil && oldSnapshot != nil {
		return t.trackDelete(layer, oldSnapshot, entity, actor)
	}

	oldAny, newAny := any(oldSnapshot), any(newSnapshot)
	if oldAny == nil || newAny == nil {
		return nil
	}
	return t.continueUpdate(layer, oldAny, newAny, entity, actor)
}

func (t *SnapshotTracker) trackCreate(layer types.EntityLayer, snapshot SnapshotI, entity dao.IDaoAct, actor *dao.User) error {
	var name string
	var entityID uuid.UUID
	var field actField.ActivityField

	if snapshot != nil {
		name = snapshot.GetName()
		entityID = snapshot.GetID()
		field = snapshot.GetField()
	}
	if field == "" {
		field = entity.GetEntityType()
	}
	if entityID == uuid.Nil {
		entityID = entity.GetId()
	}
	if name == "" {
		name = entity.GetString()
	}

	ev := NewActivityEvent(
		actField.VerbCreated,
		field,
		nil,
		name,
		uuid.NullUUID{UUID: entityID, Valid: true},
		uuid.NullUUID{},
		actor,
	)
	if err := SetEntityRefs(layer, &ev, entity); err != nil {
		return fmt.Errorf("set entity refs create: %w", err)
	}

	return t.saveAndNotifyActivity(&ev)
}

// trackDelete logs deletion of an entity
func (t *SnapshotTracker) trackDelete(layer types.EntityLayer, snapshot SnapshotI, entity dao.IDaoAct, actor *dao.User) error {
	var name string
	var field actField.ActivityField

	if snapshot != nil {
		name = snapshot.GetName()
		field = snapshot.GetField()
	}
	if field == "" {
		field = entity.GetEntityType()
	}
	if name == "" {
		name = entity.GetString()
	}

	ev := NewActivityEvent(
		actField.VerbDeleted,
		field,
		&name,
		"",
		uuid.NullUUID{},
		uuid.NullUUID{},
		actor,
	)
	if err := SetEntityRefs(layer, &ev, entity); err != nil {
		return fmt.Errorf("set entity refs delete: %w", err)
	}

	return t.saveAndNotifyActivity(&ev)
}

// Continue with update logic
func (t *SnapshotTracker) continueUpdate(layer types.EntityLayer, oldSnapshot, newSnapshot any, entity dao.IDaoAct, actor *dao.User) error {
	changes := Diff(oldSnapshot, newSnapshot)
	if len(changes) == 0 {
		return nil
	}

	// BuildActivityEvents учитывает preserve_id в тэгах полей
	activityEvents := BuildActivityEvents(layer, changes, entity, actor)

	for _, event := range activityEvents {
		if err := t.saveAndNotifyActivity(&event); err != nil {
			return err
		}
	}

	return nil
}
