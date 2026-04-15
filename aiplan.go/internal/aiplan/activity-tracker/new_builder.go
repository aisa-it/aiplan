package tracker

import (
	"fmt"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/gofrs/uuid"
)

func BuildActivityEvents(layer types.EntityLayer, changes []FieldChange, entity dao.IDaoAct, actor *dao.User) []dao.ActivityEvent {
	events := make([]dao.ActivityEvent, 0, len(changes))

	for _, change := range changes {
		if change.NewVal == "" && change.OldVal == "" {
			continue
		}

		event := dao.ActivityEvent{
			ID:            uuid.Must(uuid.NewV4()),
			CreatedAt:     time.Now(),
			ActorID:       actor.ID,
			Actor:         actor,
			Verb:          change.Verb,
			Field:         change.Field,
			OldValue:      &change.OldVal,
			NewValue:      change.NewVal,
			OldIdentifier: change.OldID,
			NewIdentifier: change.NewID,
			EntityType:    layer,
		}

		if layer == types.LayerDoc && change.Field == "parent" {
			event.Verb = "move"
		}

		if err := SetEntityRefs2(layer, &event, entity); err != nil {
			continue
		}

		events = append(events, event)
	}

	return events
}

func SetEntityRefs2[E dao.IDaoAct](layer types.EntityLayer, act *dao.ActivityEvent, entity E) error {
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
