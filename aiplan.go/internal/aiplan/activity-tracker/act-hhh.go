package tracker

import (
	"fmt"

	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/gofrs/uuid"
)

const (
	updateScopeId = "updateScopeId"
	updateScope   = "updateScope"
)

func (a *ActivityCtx) getUpdateScope(field actField.ActivityField) actField.ActivityField {
	if scope, ok := GetAs[string](a.CurrentInstance, ValueKey(updateScope)); ok {
		field = actField.ActivityField(fmt.Sprintf("%s_%s", scope, field))
	} else if scope, ok := GetAs[string](a.RequestedData, ValueKey(updateScope)); ok {
		field = actField.ActivityField(fmt.Sprintf("%s_%s", scope, field))
	}

	field = GetAsOrDefault[actField.ActivityField](a.RequestedData, ValueKey("field_log"), field)
	field = GetAsOrDefault[actField.ActivityField](a.RequestedData, FieldLogKey(field), field)

	return field
}

////

func (a *ActivityCtx) getIDFromSource(source map[string]interface{}, defaultValue uuid.NullUUID, field actField.ActivityField) uuid.UUID {
	id := GetAsOrDefault[uuid.UUID](source, ValueKey(updateScopeId), defaultValue.UUID)
	id = GetAsOrDefault[uuid.UUID](source, UpdateScopeIDKey(field), id)
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
	val := GetAsOrDefault[string](source, ValueKey(field), "")
	val = GetAsOrDefault[string](source, ActivityValKey(field), val)

	if handleNil && val == "<nil>" {
		return ""
	}

	f := GetAsOrDefault[func(string) string](source, FuncKey(field), func(s string) string { return s })
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
