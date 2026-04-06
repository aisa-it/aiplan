package tracker

import (
	"fmt"

	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/gofrs/uuid"
)

func (c *ActivityCtx) getUpdateScope(field actField.ActivityField) actField.ActivityField {
	if scope, ok := GetAs[string](c.CurrentInstance, actField.UpdateScopeKey); ok {
		field = actField.ActivityField(fmt.Sprintf("%s_%s", scope, field))
	} else if scope, ok := GetAs[string](c.RequestedData, actField.UpdateScopeKey); ok {
		field = actField.ActivityField(fmt.Sprintf("%s_%s", scope, field))
	}

	field = GetAsOrDefault[actField.ActivityField](c.RequestedData, actField.NewKey(actField.KindLogOverride), field)
	field = GetAsOrDefault[actField.ActivityField](c.RequestedData, field.LogAs(), field)

	return field
}

////

func (c *ActivityCtx) getIDFromSource(source map[string]interface{}, defaultValue uuid.NullUUID, field actField.ActivityField) uuid.UUID {
	id := GetAsOrDefault[uuid.UUID](source, actField.NewKey(actField.KindScopeID), defaultValue.UUID)
	id = GetAsOrDefault[uuid.UUID](source, field.WithScopeID(), id)
	return id
}

func (c *ActivityCtx) getNewId(newId uuid.NullUUID, field actField.ActivityField) uuid.NullUUID {
	return ToNullUUID(c.getIDFromSource(c.RequestedData, newId, field))
}

func (c *ActivityCtx) getOldId(oldId uuid.NullUUID, field actField.ActivityField) uuid.NullUUID {
	return ToNullUUID(c.getIDFromSource(c.CurrentInstance, oldId, field))
}

////

func (c *ActivityCtx) getValueFromSource(source DataEntity, field actField.ActivityField, handleNil bool) string {
	val := GetAsOrDefault[string](source, field.AsKey(), "")
	val = GetAsOrDefault[string](source, field.AsLogValue(), val)

	if handleNil && val == "<nil>" {
		return ""
	}

	f := GetAsOrDefault[func(string) string](source, field.WithFunc(), func(s string) string { return s })
	val = f(val)

	return val
}

func (c *ActivityCtx) getOldValue(field actField.ActivityField) string {
	return c.getValueFromSource(c.CurrentInstance, field, false)
}

func (c *ActivityCtx) getNewValue(field actField.ActivityField) string {
	return c.getValueFromSource(c.RequestedData, field, true)
}

// ////
func ToNullUUID(id uuid.UUID) uuid.NullUUID {
	if id.IsNil() {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: id, Valid: true}
}
