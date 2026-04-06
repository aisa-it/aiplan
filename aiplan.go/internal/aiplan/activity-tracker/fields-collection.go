package tracker

import (
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
)

func actSeveral[E dao.IDaoAct, T dao.IDaoAct](field actField.FieldMapping) func(c *ActivityCtx, entity E) ([]dao.ActivityEvent, error) {
	return func(c *ActivityCtx, entity E) ([]dao.ActivityEvent, error) {
		changesList := utils.CalculateIDChanges(c.getDiffData(field))
		if changesList.InvolvedIds == nil {
			return nil, nil
		}
		involvedEntities, err := getEntities[T](c.Tracker.db, changesList)
		if err != nil {
			return nil, err
		}

		entityMap := utils.SliceToMap(&involvedEntities, func(a *T) uuid.UUID {
			return (*a).GetId()
		})

		field.Field = GetAsOrDefault[actField.ActivityField](c.RequestedData, actField.NewKey(actField.KindLogOverride), field.Field)

		var changes []activityChange[E]

		for _, id := range changesList.DelIds {
			if e, ok := entityMap[id]; ok {
				changes = append(changes, activityChange[E]{
					verb:   actField.VerbRemoved,
					field:  field.Field,
					oldVal: utils.ToPtr(e.GetString()),
					oldID:  uuid.NullUUID{UUID: id, Valid: true},
					entity: entity,
				})
			}
		}

		for _, id := range changesList.AddIds {
			if e, ok := entityMap[id]; ok {
				changes = append(changes, activityChange[E]{
					verb:   actField.VerbAdded,
					field:  field.Field,
					newVal: e.GetString(),
					newID:  uuid.NullUUID{UUID: id, Valid: true},
					entity: entity,
				})
			}
		}

		return buildEvents(c, changes)
	}
}
