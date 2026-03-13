package tracker

import (
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
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

func actLinked[E dao.IDaoAct](field actField.FieldMapping, sourceFieldName, targetFieldName *actField.ActivityField, key *keyExtractor) func(c *ActivityCtx, entity E) ([]dao.ActivityEvent, error) {
	return func(c *ActivityCtx, entity E) ([]dao.ActivityEvent, error) {
		if key != nil {
			SetField(c.CurrentInstance, field.Field.Only(), key.extractUUIDList(c.CurrentInstance))
		}
		if sourceFieldName != nil {
			SetField(c.RequestedData, actField.New("source").WithFieldLog(), sourceFieldName.String())
		}
		if targetFieldName != nil {
			SetField(c.RequestedData, actField.New("target").WithFieldLog(), targetFieldName.String())
		}
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

type keyExtractor struct {
	sourceKey string
	nestedKey string
}

func (k *keyExtractor) extractUUIDList(d DataEntity) []interface{} {
	if k == nil {
		return nil
	}

	res := GetAsOrDefault[[]interface{}](d, actField.New(k.sourceKey).Only(), nil)
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

type DateFieldConfig struct {
	Field         actField.ActivityField
	FormatLayout  string
	UnwrapTimeMap bool
	SkipIfNil     bool
}

func actDateField[E dao.IDaoAct](cfg DateFieldConfig) func(c *ActivityCtx, entity E) ([]dao.ActivityEvent, error) {
	return func(c *ActivityCtx, entity E) ([]dao.ActivityEvent, error) {
		key := cfg.Field.String()
		if cfg.SkipIfNil {
			if v, ok := c.RequestedData[key]; ok && v == nil {
				return []dao.ActivityEvent{}, nil
			}
		}

		format := func(str string) string {
			if str == "" {
				return ""
			}

			t, err := utils.FormatDate(str)
			if err != nil {
				return ""
			}

			return t.UTC().Format(cfg.FormatLayout)
		}

		//format := func(str string) string {
		//	if v, err := utils.FormatDateStr(str, cfg.FormatLayout, nil); err == nil {
		//		return v
		//	}
		//	return ""
		//}

		SetField(c.RequestedData, cfg.Field.WithFunc(), format)
		SetField(c.CurrentInstance, cfg.Field.WithFunc(), format)

		if cfg.UnwrapTimeMap {
			normalizeDate(c.RequestedData, key)
			normalizeDate(c.CurrentInstance, key)
		}

		return actSingle[E](cfg.Field)(c, entity)
	}
}

func actSingleMappedField[E dao.IDaoAct](field actField.ActivityField, re actField.FieldMapping) func(c *ActivityCtx, entity E) ([]dao.ActivityEvent, error) {
	return func(c *ActivityCtx, entity E) ([]dao.ActivityEvent, error) {
		c.RequestedData[re.Field.WithFieldLog().String()] = field
		c.RequestedData[re.Field.WithActivityVal().String()] = c.RequestedData[re.Req]
		c.CurrentInstance[re.Field.WithActivityVal().String()] = c.CurrentInstance[re.Req]
		return actSingle[E](re.Field)(c, entity)
	}
}

func normalizeDate(data map[string]interface{}, key string) {
	if v, exists := data[key]; exists {
		data[key] = normalizeDateValue(v)
	}
}

func normalizeDateValue(v interface{}) interface{} {
	switch t := v.(type) {
	case types.TimeValuer:
		return t.GetTime().Format(time.RFC3339)
	case time.Time:
		return t.Format(time.RFC3339)
	case nil:
		return nil
	default:
		return v
	}
}
