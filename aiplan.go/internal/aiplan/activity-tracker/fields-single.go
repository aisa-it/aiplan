package tracker

import (
	"database/sql"
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

func fieldUpdate[E dao.IDaoAct](
	c *ActivityCtx, field actField.ActivityField, newIdentifier uuid.NullUUID, oldIdentifier uuid.NullUUID, entity E) (
	[]dao.ActivityEvent, error) {
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

func actSingleMappedField[E dao.IDaoAct](field actField.ActivityField, re actField.FieldMapping) func(c *ActivityCtx, entity E) ([]dao.ActivityEvent, error) {
	return func(c *ActivityCtx, entity E) ([]dao.ActivityEvent, error) {
		c.RequestedData[re.Field.LogAs().String()] = field
		c.RequestedData[re.Field.AsLogValue().String()] = c.RequestedData[re.Req]
		c.CurrentInstance[re.Field.AsLogValue().String()] = c.CurrentInstance[re.Req]
		return actSingle[E](re.Field)(c, entity)
	}
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

		SetField(c.RequestedData, cfg.Field.WithFunc(), format)
		SetField(c.CurrentInstance, cfg.Field.WithFunc(), format)

		if cfg.UnwrapTimeMap {
			normalizeDate(c.RequestedData, key)
			normalizeDate(c.CurrentInstance, key)
		}

		return actSingle[E](cfg.Field)(c, entity)
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
	case sql.NullTime:
		return t.Time.Format(time.RFC3339)
	case map[string]interface{}:
		if v, ok := t["Time"]; ok {
			return v.(time.Time).Format(time.RFC3339)
		}
		return nil
	case nil:
		return nil
	default:
		return v
	}
}
