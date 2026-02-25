package tracker

import (
	"fmt"

	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/gofrs/uuid"
)

type DataEntity = map[string]interface{}

type Payload struct {
	data DataEntity
}

func NewPayload(d DataEntity) Payload {
	return Payload{data: d}
}

func (p Payload) GetValue(field actField.ActivityField) string {
	val := GetAsOrDefault[string](p.data, field.Only(), "")
	val = GetAsOrDefault[string](p.data, field.WithActivityVal(), val)

	if val == "<nil>" {
		return ""
	}

	if f, ok := GetAs[func(string) string](p.data, field.WithFunc()); ok {
		val = f(val)
	}

	return val
}

func (p Payload) GetUUID(field actField.ActivityField, def uuid.UUID) uuid.UUID {
	id := GetAsOrDefault[uuid.UUID](p.data, actField.UpdateScopeIdKey, def)
	id = GetAsOrDefault[uuid.UUID](p.data, field.WithUpdateScopeId(), id)
	return id
}

func (p Payload) Scope(field actField.ActivityField) actField.ActivityField {
	if scope, ok := GetAs[string](p.data, actField.UpdateScopeKey); ok {
		return actField.ActivityField(fmt.Sprintf("%s_%s", scope, field))
	}
	return field
}

func GetAs[T any](d DataEntity, key actField.FieldKey) (T, bool) {
	var zero T

	v, ok := GetField(d, key)
	if !ok || v == nil {
		return zero, false
	}

	if typed, ok := v.(T); ok {
		return typed, true
	}

	switch any(zero).(type) {

	case string:
		if s, ok := v.(actField.ActivityField); ok {
			return any(s.String()).(T), true
		}

	case actField.ActivityField:
		if s, ok := v.(string); ok {
			return any(actField.ActivityField(s)).(T), true
		}
	}

	return zero, false
}

func GetAsOrDefault[T any](d DataEntity, key actField.FieldKey, def T) T {
	if v, ok := GetAs[T](d, key); ok {
		return v
	}
	return def
}

func GetField(d DataEntity, key actField.FieldKey) (any, bool) {
	v, ok := d[key.String()]
	return v, ok
}

func SetField(d DataEntity, key actField.FieldKey, val any) {
	d[key.String()] = val
}
