package tracker

import (
	"fmt"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/gofrs/uuid"
)

type activityChange[E dao.IDaoAct] struct {
	verb   string
	field  actField.ActivityField
	oldVal *string
	newVal string
	newID  uuid.NullUUID
	oldID  uuid.NullUUID
	entity E
}

func buildEvents[E dao.IDaoAct](c *ActivityCtx, changes []activityChange[E]) ([]dao.ActivityEvent, error) {
	result := make([]dao.ActivityEvent, 0, len(changes))
	for _, ch := range changes {
		ev := NewActivityEvent(ch.verb, ch.field, ch.oldVal, ch.newVal, ch.newID, ch.oldID, c.Actor)
		if err := SetEntityRefs(c.Layer, &ev, ch.entity); err != nil {
			return nil, fmt.Errorf("set entity refs failed: %w", err)
		}
		result = append(result, ev)
	}

	return result, nil
}

func NewActivityEvent(verb string, field actField.ActivityField, oldVal *string, newVal string, newId, oldId uuid.NullUUID, actor *dao.User) dao.ActivityEvent {

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

func SetEntityRefs[E dao.IDaoAct](layer types.EntityLayer, act *dao.ActivityEvent, entity E) error {
	act.EntityType = layer

	switch layer {
	case types.LayerRoot:
		return nil
	case types.LayerWorkspace:
		if e, ok := any(entity).(dao.WorkspaceEntityI); ok {
			act.WorkspaceID = uuid.NullUUID{UUID: e.GetWorkspaceId(), Valid: true}
		} else {
			return fmt.Errorf("entity is not WorkspaceEntity")
		}
	case types.LayerProject:
		if e, ok := any(entity).(dao.ProjectEntityI); ok {
			act.WorkspaceID = uuid.NullUUID{UUID: e.GetWorkspaceId(), Valid: true}
			act.ProjectID = uuid.NullUUID{UUID: e.GetProjectId(), Valid: true}
		} else {
			return fmt.Errorf("entity is not ProjectEntity")
		}
	case types.LayerIssue:
		if e, ok := any(entity).(dao.IssueEntityI); ok {
			act.WorkspaceID = uuid.NullUUID{UUID: e.GetWorkspaceId(), Valid: true}
			act.ProjectID = uuid.NullUUID{UUID: e.GetProjectId(), Valid: true}
			act.IssueID = uuid.NullUUID{UUID: e.GetIssueId(), Valid: true}
		} else {
			return fmt.Errorf("entity is not IssueEntity")
		}
	case types.LayerDoc:
		if e, ok := any(entity).(dao.DocEntityI); ok {
			act.WorkspaceID = uuid.NullUUID{UUID: e.GetWorkspaceId(), Valid: true}
			act.DocID = uuid.NullUUID{UUID: e.GetDocId(), Valid: true}
		} else {
			return fmt.Errorf("entity is not DocEntity")
		}
	case types.LayerForm:
		if e, ok := any(entity).(dao.FormEntityI); ok {
			act.WorkspaceID = uuid.NullUUID{UUID: e.GetWorkspaceId(), Valid: true}
			act.FormID = uuid.NullUUID{UUID: e.GetFormId(), Valid: true}
		} else {
			return fmt.Errorf("entity is not FormEntity")
		}
	case types.LayerSprint:
		if e, ok := any(entity).(dao.SprintEntityI); ok {
			act.WorkspaceID = uuid.NullUUID{UUID: e.GetWorkspaceId(), Valid: true}
			act.SprintID = uuid.NullUUID{UUID: e.GetSprintId(), Valid: true}
		} else {
			return fmt.Errorf("entity is not SprintEntity")
		}
	}
	return nil
}
