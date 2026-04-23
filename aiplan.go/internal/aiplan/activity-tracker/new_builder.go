package tracker

import (
	"fmt"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

func BuildActivityEvents(layer types.EntityLayer, changes []FieldChange, entity dao.IDaoAct, actor *dao.User, db *gorm.DB) []dao.ActivityEvent {
	changesByEntity := make(map[uuid.UUID][]FieldChange)
	for _, change := range changes {
		entityID := entity.GetId()
		if change.IsLinked {
			entityID = change.EntityID
		}
		changesByEntity[entityID] = append(changesByEntity[entityID], change)
	}

	events := make([]dao.ActivityEvent, 0, len(changes))

	for targetEntityID, entityChanges := range changesByEntity {
		if targetEntityID == uuid.Nil {
			continue
		}

		for _, change := range entityChanges {
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
				OldValue:      change.OldVal,
				NewValue:      change.NewVal,
				OldIdentifier: change.OldID,
				NewIdentifier: change.NewID,
				EntityType:    layer,
			}

			if err := SetEntityRefs2(layer, &event, entity, targetEntityID); err != nil {
				continue
			}
			events = append(events, event)
		}
	}

	return events
}

func SetEntityRefs2[E dao.IDaoAct](layer types.EntityLayer, act *dao.ActivityEvent, entity E, targetEntityID uuid.UUID) error {
	act.EntityType = layer

	switch layer {
	case types.LayerRoot:
		return nil
	case types.LayerWorkspace:
		if targetEntityID != uuid.Nil {
			act.WorkspaceID = uuid.NullUUID{UUID: targetEntityID, Valid: true}
		} else if e, ok := any(entity).(dao.WorkspaceEntityI); ok {
			act.WorkspaceID = uuid.NullUUID{UUID: e.GetWorkspaceId(), Valid: true}
		} else {
			return fmt.Errorf("entity is not WorkspaceEntity")
		}
	case types.LayerProject:
		if targetEntityID != uuid.Nil {
			act.ProjectID = uuid.NullUUID{UUID: targetEntityID, Valid: true}
			if e, ok := any(entity).(dao.ProjectEntityI); ok {
				act.WorkspaceID = uuid.NullUUID{UUID: e.GetWorkspaceId(), Valid: true}
			}
		} else if e, ok := any(entity).(dao.ProjectEntityI); ok {
			act.WorkspaceID = uuid.NullUUID{UUID: e.GetWorkspaceId(), Valid: true}
			act.ProjectID = uuid.NullUUID{UUID: e.GetProjectId(), Valid: true}
		} else {
			return fmt.Errorf("entity is not ProjectEntity")
		}
	case types.LayerIssue:
		if act.WorkspaceID.Valid == false || act.ProjectID.Valid == false {
			if e, ok := any(entity).(dao.IssueEntityI); ok {
				act.WorkspaceID = uuid.NullUUID{UUID: e.GetWorkspaceId(), Valid: true}
				act.ProjectID = uuid.NullUUID{UUID: e.GetProjectId(), Valid: true}
			}
		}
		if targetEntityID != uuid.Nil {
			act.IssueID = uuid.NullUUID{UUID: targetEntityID, Valid: true}
		} else if e, ok := any(entity).(dao.IssueEntityI); ok {
			act.IssueID = uuid.NullUUID{UUID: e.GetIssueId(), Valid: true}
		} else {
			return fmt.Errorf("entity is not IssueEntity")
		}
	case types.LayerDoc:
		if act.WorkspaceID.Valid == false {
			if e, ok := any(entity).(dao.DocEntityI); ok {
				act.WorkspaceID = uuid.NullUUID{UUID: e.GetWorkspaceId(), Valid: true}
			}
		}
		if targetEntityID != uuid.Nil {
			act.DocID = uuid.NullUUID{UUID: targetEntityID, Valid: true}
		} else if e, ok := any(entity).(dao.DocEntityI); ok {
			act.DocID = uuid.NullUUID{UUID: e.GetDocId(), Valid: true}
		} else {
			return fmt.Errorf("entity is not DocEntity")
		}
	case types.LayerForm:
		if act.WorkspaceID.Valid == false {
			if e, ok := any(entity).(dao.FormEntityI); ok {
				act.WorkspaceID = uuid.NullUUID{UUID: e.GetWorkspaceId(), Valid: true}
			}
		}
		if targetEntityID != uuid.Nil {
			act.FormID = uuid.NullUUID{UUID: targetEntityID, Valid: true}
		} else if e, ok := any(entity).(dao.FormEntityI); ok {
			act.FormID = uuid.NullUUID{UUID: e.GetFormId(), Valid: true}
		} else {
			return fmt.Errorf("entity is not FormEntity")
		}
	case types.LayerSprint:
		if act.WorkspaceID.Valid == false {
			if e, ok := any(entity).(dao.SprintEntityI); ok {
				act.WorkspaceID = uuid.NullUUID{UUID: e.GetWorkspaceId(), Valid: true}
			}
		}
		if targetEntityID != uuid.Nil {
			act.SprintID = uuid.NullUUID{UUID: targetEntityID, Valid: true}
		} else if e, ok := any(entity).(dao.SprintEntityI); ok {
			act.SprintID = uuid.NullUUID{UUID: e.GetSprintId(), Valid: true}
		} else {
			return fmt.Errorf("entity is not SprintEntity")
		}
	}
	return nil
}
