package tracker

import (
	"fmt"
	"log/slog"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

type ActHandler interface {
	Handle(activity dao.ActivityEvent) error
}

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

func NewActivityEvent(verb string, field actField.ActivityField, oldVal string, newVal string, newId, oldId uuid.NullUUID, actor *dao.User) dao.ActivityEvent {

	return dao.ActivityEvent{
		ID:            dao.GenUUID(),
		WorkspaceID:   uuid.NullUUID{},
		ProjectID:     uuid.NullUUID{},
		IssueID:       uuid.NullUUID{},
		DocID:         uuid.NullUUID{},
		FormID:        uuid.NullUUID{},
		SprintID:      uuid.NullUUID{},
		EntityType:    0,
		ActorID:       actor.ID,
		Actor:         actor,
		Notified:      false,
		Verb:          verb,
		Field:         field,
		OldValue:      oldVal,
		NewValue:      newVal,
		NewIdentifier: newId,
		OldIdentifier: oldId,
	}
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

func setTargetId(event *dao.ActivityEvent, layer types.EntityLayer, id uuid.UUID) {
	switch layer {
	case types.LayerWorkspace:
		event.WorkspaceID = uuid.NullUUID{UUID: id, Valid: true}
	case types.LayerProject:
		event.ProjectID = uuid.NullUUID{UUID: id, Valid: true}
	case types.LayerIssue:
		event.IssueID = uuid.NullUUID{UUID: id, Valid: true}
	case types.LayerSprint:
		event.SprintID = uuid.NullUUID{UUID: id, Valid: true}
	case types.LayerDoc:
		event.DocID = uuid.NullUUID{UUID: id, Valid: true}
	case types.LayerForm:
		event.FormID = uuid.NullUUID{UUID: id, Valid: true}
	default:
	}
}

func (t *SnapshotTracker) TrackChanges(layer types.EntityLayer, oldSnapshot, newSnapshot SnapshotI, entity dao.IDaoAct, actor *dao.User, targetEntityID ...uuid.UUID) error {
	if actor == nil {
		return fmt.Errorf("TrackChanges: actor is nil (layer %v)", layer)
	}
	targetID := uuid.Nil
	if len(targetEntityID) > 0 {
		targetID = targetEntityID[0]
	}
	if oldSnapshot == nil && newSnapshot != nil {
		return t.trackCreate(layer, newSnapshot, entity, actor, targetID)
	}
	if newSnapshot == nil && oldSnapshot != nil {
		return t.trackDelete(layer, oldSnapshot, entity, actor)
	}
	return t.continueUpdate(oldSnapshot, newSnapshot, entity, actor, targetID)
}

func (t *SnapshotTracker) TrackVerb(layer types.EntityLayer, verb string, entity dao.IDaoAct, actor *dao.User, opts ...TrackOption) error {
	params := &trackParams{}
	for _, opt := range opts {
		opt(params)
	}

	event := NewActivityEvent(verb, params.field, params.oldVal, params.newVal, params.newID, params.oldID, actor)

	event.SetEntityRefs(layer, entity)
	return t.saveAndNotifyActivity(&event)
}

func (t *SnapshotTracker) trackCreate(layer types.EntityLayer, snapshot SnapshotI, entity dao.IDaoAct, actor *dao.User, targetEntityID uuid.UUID) error {

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
	if targetEntityID != uuid.Nil {
		entityID = targetEntityID
	} else if entityID == uuid.Nil {
		entityID = entity.GetId()
	}
	if name == "" {
		name = entity.GetString()
	}

	ev := NewActivityEvent(actField.VerbCreated, field, "", name, uuid.NullUUID{UUID: entityID, Valid: true}, uuid.NullUUID{}, actor)

	ev.SetEntityRefs(layer, entity)
	return t.saveAndNotifyActivity(&ev)
}

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

	ev := NewActivityEvent(actField.VerbDeleted, field, name, "", uuid.NullUUID{}, uuid.NullUUID{}, actor)

	ev.SetEntityRefs(layer, entity)
	return t.saveAndNotifyActivity(&ev)
}

func (t *SnapshotTracker) continueUpdate(oldSnapshot, newSnapshot any, entity dao.IDaoAct, actor *dao.User, targetEntityID uuid.UUID) error {
	entityID := entity.GetId()
	if targetEntityID != uuid.Nil {
		entityID = targetEntityID
	}
	changes := Diff(oldSnapshot, newSnapshot, entityID, entity.GetString())
	if len(changes) == 0 {
		return nil
	}

	activityEvents := BuildActivityEvents(t.db, changes, entity, actor)

	for _, event := range activityEvents {
		if err := t.saveAndNotifyActivity(&event); err != nil {
			return err
		}
	}

	return nil
}
