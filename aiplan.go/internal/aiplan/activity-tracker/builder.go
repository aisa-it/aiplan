package tracker

import (
	"fmt"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

func BuildActivityEvents(tx *gorm.DB, changes []FieldChange, entity dao.IDaoAct, actor *dao.User) []dao.ActivityEvent {
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

			changeLayer := determineLayerForEntity(entity)
			if change.IsLinked && change.Layer != types.LayerRoot { //todo!!!!
				changeLayer = change.Layer
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
				EntityType:    changeLayer,
			}

			if change.IsLinked {
				setTargetId(&event, changeLayer, targetEntityID)
			} else {
				event.SetEntityRefs(changeLayer, entity)
			}
			if err := event.ValidateAndSet(tx); err != nil {
				fmt.Println(err)
			}
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

func determineLinkedLayer(layer string) types.EntityLayer {
	switch layer {
	case types.LayerIssue.String():
		return types.LayerIssue
	case types.LayerProject.String():
		return types.LayerProject
	case types.LayerDoc.String():
		return types.LayerDoc
	case types.LayerWorkspace.String():
		return types.LayerWorkspace
	case types.LayerSprint.String():
		return types.LayerSprint
	case types.LayerForm.String():
		return types.LayerForm
	default:
		return types.LayerRoot
	}
}
