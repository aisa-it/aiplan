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

func BuildActivityEvents(db *gorm.DB, changes []FieldChange, entity dao.IDaoAct, actor *dao.User) []dao.ActivityEvent {
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

			// Determine correct layer for this change
			changeLayer := determineLayerForEntity(entity)
			if change.IsLinked {
				changeLayer = determineLayerForField(change.Field)
			}

			eventVerb := change.Verb

			event := dao.ActivityEvent{
				ID:            uuid.Must(uuid.NewV4()),
				CreatedAt:     time.Now(),
				ActorID:       actor.ID,
				Actor:         actor,
				Verb:          eventVerb,
				Field:         change.Field,
				OldValue:      change.OldVal,
				NewValue:      change.NewVal,
				OldIdentifier: change.OldID,
				NewIdentifier: change.NewID,
			}

			// Set entity references based on layer
			/*if err := */
			setEntityRefs(changeLayer, &event, entity, targetEntityID) //; err != nil {
			// Skip this event if refs can't be set
			//	continue
			//}
			events = append(events, event)
		}
	}

	return events
}

func determineLayerForEntity(entity dao.IDaoAct) types.EntityLayer {
	field := entity.GetEntityType()
	switch field {
	case actField.Sprint.Field:
		return types.LayerSprint
	case actField.Issue.Field:
		return types.LayerIssue
	case actField.Project.Field:
		return types.LayerProject
	case actField.Doc.Field:
		return types.LayerDoc
	case actField.Workspace.Field:
		return types.LayerWorkspace
	case actField.Template.Field:
		return types.LayerProject
	case actField.Form.Field:
		return types.LayerForm

	default:
		return types.LayerRoot
	}
}

func determineLayerForField(field actField.ActivityField) types.EntityLayer {
	switch field {
	case actField.Issue.Field:
		return types.LayerIssue
	case actField.Project.Field:
		return types.LayerProject
	case actField.Doc.Field:
		return types.LayerDoc
	case actField.Workspace.Field:
		return types.LayerWorkspace
	default:
		slog.Info("-------!!!! Unknown field", field)
		// For linked fields, determine layer from the field name
		switch field {
		case actField.Assignees.Field, actField.Watchers.Field, actField.Label.Field, actField.Parent.Field, actField.SubIssue.Field, actField.Blocking.Field, actField.Blocks.Field, actField.Linked.Field, actField.Sprint.Field:
			return types.LayerIssue
		case actField.Readers.Field, actField.Editors.Field:
			return types.LayerDoc
		case actField.Issues.Field:
			return types.LayerSprint
		case actField.TemplateName.Field, actField.TemplateTemplate.Field:
			return types.LayerProject
		default:
			return types.LayerRoot
		}
	}
}

func SetEntityRefs2[E dao.IDaoAct](db *gorm.DB, layer types.EntityLayer, act *dao.ActivityEvent, entity E, targetEntityID uuid.UUID) error {
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
		if targetEntityID != uuid.Nil {
			act.IssueID = uuid.NullUUID{UUID: targetEntityID, Valid: true}
			// For linked events, we need to get workspace from the main entity
			if e, ok := any(entity).(dao.WorkspaceEntityI); ok {
				act.WorkspaceID = uuid.NullUUID{UUID: e.GetWorkspaceId(), Valid: true}
			}
			// For linked events, load the issue to get project ID
			var issue dao.Issue
			if err := db.Where("id = ?", targetEntityID).First(&issue).Error; err == nil {
				act.ProjectID = uuid.NullUUID{UUID: issue.ProjectId, Valid: true}
			}
		} else {
			if e, ok := any(entity).(dao.IssueEntityI); ok {
				act.WorkspaceID = uuid.NullUUID{UUID: e.GetWorkspaceId(), Valid: true}
				act.ProjectID = uuid.NullUUID{UUID: e.GetProjectId(), Valid: true}
				act.IssueID = uuid.NullUUID{UUID: e.GetIssueId(), Valid: true}
			} else {
				return fmt.Errorf("entity is not IssueEntity")
			}
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
