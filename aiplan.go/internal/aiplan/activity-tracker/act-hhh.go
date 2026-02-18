package tracker

import (
	"fmt"

	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
)

const (
	updateScopeId = "updateScopeId"
	updateScope   = "updateScope"
)

func (a *ActivityCtx) getUpdateScope(field *actField.ActivityField) {
	if scope, ok := a.CurrentInstance[updateScope]; ok {
		field = utils.ToPtr(actField.ActivityField(fmt.Sprintf("%s_%s", scope, field)))
	} else if scope, ok := a.RequestedData[updateScope]; ok {
		field = utils.ToPtr(actField.ActivityField(fmt.Sprintf("%s_%s", scope, field)))
	}

	if fieldLog, ok := a.RequestedData[fmt.Sprintf("field_log")]; ok {
		field = utils.ToPtr(fieldLog.(actField.ActivityField))
	}

	if fieldLog, ok := a.RequestedData[fmt.Sprintf("%s_field_log", field)]; ok {
		field = utils.ToPtr(fieldLog.(actField.ActivityField))
	}
}

////

func (a *ActivityCtx) getIDFromSource(source map[string]interface{}, defaultValue uuid.NullUUID, field actField.ActivityField) uuid.NullUUID {
	if id, ok := source[updateScopeId].(uuid.UUID); ok {
		return uuid.NullUUID{UUID: id, Valid: true}
	}
	if id, ok := source[field.WithUpdateScopeId()].(uuid.UUID); ok {
		return uuid.NullUUID{UUID: id, Valid: true}
	}
	return defaultValue
}

func (a *ActivityCtx) getNewId(newId uuid.NullUUID, field actField.ActivityField) uuid.NullUUID {
	return a.getIDFromSource(a.RequestedData, newId, field)
}

func (a *ActivityCtx) getOldId(oldId uuid.NullUUID, field actField.ActivityField) uuid.NullUUID {
	return a.getIDFromSource(a.CurrentInstance, oldId, field)
}

////

func (a *ActivityCtx) getValueFromSource(source map[string]interface{}, field actField.ActivityField, handleNil bool) string {
	val := fmt.Sprint(source[field.String()])

	if activityVal, ok := source[field.WithActivityVal()]; ok {
		val = fmt.Sprint(activityVal)
	}

	if handleNil && val == "<nil>" {
		return ""
	}

	if f, ok := source[field.WithFuncStr()]; ok {
		if ff, ok := f.(func(str string) string); ok {
			val = ff(val)
		}
	}

	return val
}

func (a *ActivityCtx) getOldValue(field actField.ActivityField) string {
	return a.getValueFromSource(a.CurrentInstance, field, false)
}

func (a *ActivityCtx) getNewValue(field actField.ActivityField) string {
	return a.getValueFromSource(a.RequestedData, field, true)
}

//////
