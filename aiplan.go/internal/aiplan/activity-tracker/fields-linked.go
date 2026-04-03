package tracker

import (
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
)

func actLinked[E dao.IDaoAct](field actField.FieldMapping, sourceFieldName, targetFieldName *actField.ActivityField, key *keyExtractor) func(c *ActivityCtx, entity E) ([]dao.ActivityEvent, error) {
	return func(c *ActivityCtx, entity E) ([]dao.ActivityEvent, error) {
		if key != nil {
			SetField(c.CurrentInstance, field.Field.AsKey(), key.extractUUIDList(c.CurrentInstance))
		}
		if sourceFieldName != nil {
			SetField(c.RequestedData, actField.New("source").LogAs(), sourceFieldName.String())
		}
		if targetFieldName != nil {
			SetField(c.RequestedData, actField.New("target").LogAs(), targetFieldName.String())
		}
		return fieldRelationsUpdate[E](c, field, entity)
	}
}

func fieldRelationsUpdate[E dao.IDaoAct](c *ActivityCtx, field actField.FieldMapping, entity E) ([]dao.ActivityEvent, error) {

	oldIds := GetAsOrDefault[[]interface{}](c.CurrentInstance, actField.New(field.Req).AsKey(), []interface{}{})
	newIds := GetAsOrDefault[[]interface{}](c.RequestedData, actField.New(field.Req).AsKey(), []interface{}{})

	changesList := utils.CalculateIDChanges(newIds, oldIds)

	involvedEntities, err := getEntities[E](c.Tracker.db, changesList)
	if err != nil {
		return nil, err
	}

	entityMap := utils.SliceToMap(&involvedEntities, func(a *E) uuid.UUID {
		return (*a).GetId()
	})

	sourceField := GetAsOrDefault[actField.ActivityField](c.RequestedData, actField.NewKey(actField.KindLogOverride), field.Field)
	targetField := sourceField

	sourceField = GetAsOrDefault[actField.ActivityField](c.RequestedData, actField.New("source").LogAs(), sourceField)
	targetField = GetAsOrDefault[actField.ActivityField](c.RequestedData, actField.New("target").LogAs(), targetField)

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

type keyExtractor struct {
	sourceKey string
	nestedKey string
}

func (k *keyExtractor) extractUUIDList(d DataEntity) []interface{} {
	if k == nil {
		return nil
	}

	res := GetAsOrDefault[[]interface{}](d, actField.New(k.sourceKey).AsKey(), nil)
	if res == nil {
		return nil
	}

	return utils.SliceToSlice(&res, func(t *interface{}) interface{} {
		m, ok := (*t).(map[string]interface{})
		if !ok {
			return nil
		}

		id, ok := m[k.nestedKey].(uuid.UUID)
		if !ok {
			return nil
		}

		return id
	})
}
