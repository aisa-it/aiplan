package tracker

import (
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
)

func actSingle[E dao.IDaoAct](field actField.ActivityField) func(c *ActivityCtx, entity E) ([]dao.ActivityEvent, error) {
	return func(c *ActivityCtx, entity E) ([]dao.ActivityEvent, error) {
		return fieldUpdate(c, field, uuid.NullUUID{}, uuid.NullUUID{}, entity)
	}
}

func fieldUpdate[E dao.IDaoAct](c *ActivityCtx, field actField.ActivityField,
	newIdentifier uuid.NullUUID, oldIdentifier uuid.NullUUID, entity E) ([]dao.ActivityEvent, error) {
	result := make([]dao.ActivityEvent, 0)

	oldV := c.Current().GetValue(field)
	newV := c.Requested().GetValue(field)

	newIdentifier = ToNullUUID(c.Requested().GetUUID(field, newIdentifier.UUID))
	oldIdentifier = ToNullUUID(c.Current().GetUUID(field, oldIdentifier.UUID))
	resolvedField := c.ResolveField(field)

	if oldV == newV {
		return result, nil
	}

	change := activityChange[E]{
		verb:   actField.VerbUpdated,
		field:  resolvedField,
		oldVal: &oldV,
		newVal: newV,
		newID:  newIdentifier,
		oldID:  oldIdentifier,
		entity: entity,
	}

	return buildEvents(c, []activityChange[E]{change})
}

func actSeveral[E dao.IDaoAct, T dao.IDaoAct](field actField.FieldMapping) func(c *ActivityCtx, entity E) ([]dao.ActivityEvent, error) {
	return func(c *ActivityCtx, entity E) ([]dao.ActivityEvent, error) {
		return fieldListUpdate[E, T](c, field, entity)
	}
}

//

func fieldListUpdate[E dao.IDaoAct, T dao.IDaoAct](
	c *ActivityCtx, act actField.FieldMapping, entity E,
) ([]dao.ActivityEvent, error) {

	changesList := utils.CalculateIDChanges(c.getDiffData(act))
	involvedEntities, err := getEntities[T](c.Tracker.db, changesList)
	if err != nil {
		return nil, err
	}

	entityMap := utils.SliceToMap(&involvedEntities, func(a *T) uuid.UUID {
		return (*a).GetId()
	})

	act.Field = GetAsOrDefault[actField.ActivityField](c.RequestedData, actField.FieldLogKey, act.Field)

	var changes []activityChange[E]

	for _, id := range changesList.DelIds {
		if e, ok := entityMap[id]; ok {
			changes = append(changes, activityChange[E]{
				verb:   actField.VerbRemoved,
				field:  act.Field,
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
				field:  act.Field,
				newVal: e.GetString(),
				newID:  uuid.NullUUID{UUID: id, Valid: true},
				entity: entity,
			})
		}
	}

	return buildEvents(c, changes)
}

func actLinked[E dao.IDaoAct](field actField.FieldMapping) func(c *ActivityCtx, entity E) ([]dao.ActivityEvent, error) {
	return func(c *ActivityCtx, entity E) ([]dao.ActivityEvent, error) {
		return fieldRelationsUpdate[E](c, field, entity)
	}
}

func fieldRelationsUpdate[E dao.IDaoAct](c *ActivityCtx, field actField.FieldMapping, entity E) ([]dao.ActivityEvent, error) {

	oldIds := GetAsOrDefault[[]interface{}](c.CurrentInstance, actField.New(field.Req).Only(), []interface{}{})
	newIds := GetAsOrDefault[[]interface{}](c.RequestedData, actField.New(field.Req).Only(), []interface{}{})

	changesList := utils.CalculateIDChanges(newIds, oldIds)

	involvedEntities, err := getEntities[E](c.Tracker.db, changesList)
	if err != nil {
		return nil, err
	}

	entityMap := utils.SliceToMap(&involvedEntities, func(a *E) uuid.UUID {
		return (*a).GetId()
	})

	sourceField := GetAsOrDefault[actField.ActivityField](c.RequestedData, actField.FieldLogKey, field.Field)
	targetField := sourceField

	sourceField = GetAsOrDefault[actField.ActivityField](c.RequestedData, actField.New("source").WithFieldLog(), sourceField)
	targetField = GetAsOrDefault[actField.ActivityField](c.RequestedData, actField.New("target").WithFieldLog(), targetField)

	var changes []activityChange[E]

	selfString := entity.GetString()
	selfID := entity.GetId()

	for _, id := range changesList.DelIds {
		if related, ok := entityMap[id]; ok {
			oldStr := related.GetString()
			// событие для source
			changes = append(changes, activityChange[E]{
				verb: actField.VerbUpdated, field: sourceField, oldVal: &oldStr, oldID: uuid.NullUUID{UUID: id, Valid: true}, entity: entity,
			})
			// событие для target
			changes = append(changes, activityChange[E]{
				verb: actField.VerbUpdated, field: targetField, oldVal: &selfString, oldID: uuid.NullUUID{UUID: selfID, Valid: true}, entity: related})
		}
	}

	for _, id := range changesList.AddIds {
		if related, ok := entityMap[id]; ok {
			newStr := related.GetString()
			// событие для source
			changes = append(changes, activityChange[E]{
				verb: actField.VerbUpdated, field: sourceField, newVal: newStr, newID: uuid.NullUUID{UUID: id, Valid: true}, entity: entity})
			// событие для target
			changes = append(changes, activityChange[E]{
				verb: actField.VerbUpdated, field: targetField, newVal: selfString, newID: uuid.NullUUID{UUID: selfID, Valid: true}, entity: related})
		}
	}

	return buildEvents(c, changes)
}
