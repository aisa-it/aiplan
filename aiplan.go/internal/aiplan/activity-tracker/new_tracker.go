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

func setEntityRefs(layer types.EntityLayer, event *dao.ActivityEvent, entity dao.IDaoAct) {
	if de, ok := any(entity).(dao.WorkspaceEntityI); ok && layer != types.LayerRoot {
		event.WorkspaceID = uuid.NullUUID{UUID: de.GetWorkspaceId(), Valid: true}
	}
	if de, ok := any(entity).(dao.DocEntityI); ok {
		event.DocID = uuid.NullUUID{UUID: de.GetDocId(), Valid: true}
	}
	if fe, ok := any(entity).(dao.FormEntityI); ok {
		event.FormID = uuid.NullUUID{UUID: fe.GetFormId(), Valid: true}
	}
	if se, ok := any(entity).(dao.SprintEntityI); ok {
		event.SprintID = uuid.NullUUID{UUID: se.GetSprintId(), Valid: true}
	}
	if pe, ok := any(entity).(dao.ProjectEntityI); ok && layer != types.LayerWorkspace {
		event.ProjectID = uuid.NullUUID{UUID: pe.GetProjectId(), Valid: true}
	}
	if ie, ok := any(entity).(dao.IssueEntityI); ok {
		event.IssueID = uuid.NullUUID{UUID: ie.GetIssueId(), Valid: true}
	}
	event.EntityType = layer
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
	event.OldValue = doc.Title
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
	event.OldValue = ""
}

func (t *SnapshotTracker) configureMoveActivity(event *dao.ActivityEvent, doc *dao.Doc, oldParent, newParent *dao.Doc) {
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

func (t *SnapshotTracker) TrackChanges(layer types.EntityLayer, oldSnapshot, newSnapshot SnapshotI, entity dao.IDaoAct, actor *dao.User, targetEntityID ...uuid.UUID) error {
	return t.TrackChangesWithVerb(layer, oldSnapshot, newSnapshot, entity, actor, "", targetEntityID...)
}

func (t *SnapshotTracker) TrackChangesWithVerb(layer types.EntityLayer, oldSnapshot, newSnapshot SnapshotI, entity dao.IDaoAct, actor *dao.User, verb string, targetEntityID ...uuid.UUID) error {
	targetID := uuid.Nil
	if len(targetEntityID) > 0 {
		targetID = targetEntityID[0]
	}
	if oldSnapshot == nil && newSnapshot != nil {
		return t.trackCreateWithVerb(layer, newSnapshot, entity, actor, verb, targetID)
	}
	if newSnapshot == nil && oldSnapshot != nil {
		return t.trackDelete(layer, oldSnapshot, entity, actor, targetID)
	}
	return t.continueUpdate(layer, oldSnapshot, newSnapshot, entity, actor, targetID)
}

func (t *SnapshotTracker) TrackVerb(layer types.EntityLayer, verb string, entity dao.IDaoAct, actor *dao.User, opts ...TrackOption) error {
	// Create TrackParams from opts
	params := &trackParams{}
	for _, opt := range opts {
		opt(params)
	}

	// Create activity event
	event := NewActivityEvent(verb, params.field, params.oldVal, params.newVal, params.newID, params.oldID, actor)

	// Set entity references based on layer
	//entityID := entity.GetId() //todo check

	setEntityRefs(layer, &event, entity)
	return t.saveAndNotifyActivity(&event)
}

func (t *SnapshotTracker) trackCreateWithVerb(layer types.EntityLayer, snapshot SnapshotI, entity dao.IDaoAct, actor *dao.User, verb string, targetEntityID uuid.UUID) error {
	if verb == "" {
		verb = actField.VerbCreated
	}

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

	ev := NewActivityEvent(
		verb,
		field,
		"",
		name,
		uuid.NullUUID{UUID: entityID, Valid: true},
		uuid.NullUUID{},
		actor,
	)

	setEntityRefs(layer, &ev, entity)
	return t.saveAndNotifyActivity(&ev)
}

func (t *SnapshotTracker) trackDelete(layer types.EntityLayer, snapshot SnapshotI, entity dao.IDaoAct, actor *dao.User, targetEntityID uuid.UUID) error {
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

	//entityID := entity.GetId()
	//if targetEntityID != uuid.Nil {
	//	entityID = targetEntityID
	//}

	ev := NewActivityEvent(
		actField.VerbDeleted,
		field,
		name,
		"",
		uuid.NullUUID{},
		uuid.NullUUID{},
		actor,
	)

	setEntityRefs(layer, &ev, entity)
	return t.saveAndNotifyActivity(&ev)
}

func (t *SnapshotTracker) continueUpdate(layer types.EntityLayer, oldSnapshot, newSnapshot any, entity dao.IDaoAct, actor *dao.User, targetEntityID uuid.UUID) error {
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
