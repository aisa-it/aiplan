package tracker

import (
	"fmt"

	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/gofrs/uuid"
)

func (a *ActivityCtx) getUpdateScope(field actField.ActivityField) actField.ActivityField {
	if scope, ok := GetAs[string](a.CurrentInstance, actField.UpdateScopeKey); ok {
		field = actField.ActivityField(fmt.Sprintf("%s_%s", scope, field))
	} else if scope, ok := GetAs[string](a.RequestedData, actField.UpdateScopeKey); ok {
		field = actField.ActivityField(fmt.Sprintf("%s_%s", scope, field))
	}

	field = GetAsOrDefault[actField.ActivityField](a.RequestedData, actField.FieldLogKey, field)
	field = GetAsOrDefault[actField.ActivityField](a.RequestedData, field.WithFieldLog(), field)

	return field
}

////

func (a *ActivityCtx) getIDFromSource(source map[string]interface{}, defaultValue uuid.NullUUID, field actField.ActivityField) uuid.UUID {
	id := GetAsOrDefault[uuid.UUID](source, actField.UpdateScopeIdKey, defaultValue.UUID)
	id = GetAsOrDefault[uuid.UUID](source, field.WithUpdateScopeId(), id)
	return id
}

func (a *ActivityCtx) getNewId(newId uuid.NullUUID, field actField.ActivityField) uuid.NullUUID {
	return ToNullUUID(a.getIDFromSource(a.RequestedData, newId, field))
}

func (a *ActivityCtx) getOldId(oldId uuid.NullUUID, field actField.ActivityField) uuid.NullUUID {
	return ToNullUUID(a.getIDFromSource(a.CurrentInstance, oldId, field))
}

////

func (a *ActivityCtx) getValueFromSource(source DataEntity, field actField.ActivityField, handleNil bool) string {
	val := GetAsOrDefault[string](source, field.Only(), "")
	val = GetAsOrDefault[string](source, field.WithActivityVal(), val)

	if handleNil && val == "<nil>" {
		return ""
	}

	f := GetAsOrDefault[func(string) string](source, field.WithFunc(), func(s string) string { return s })
	val = f(val)

	return val
}

func (a *ActivityCtx) getOldValue(field actField.ActivityField) string {
	return a.getValueFromSource(a.CurrentInstance, field, false)
}

func (a *ActivityCtx) getNewValue(field actField.ActivityField) string {
	return a.getValueFromSource(a.RequestedData, field, true)
}

// ////
func ToNullUUID(id uuid.UUID) uuid.NullUUID {
	if id.IsNil() {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: id, Valid: true}
}
