package tracker

import (
	"fmt"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/gofrs/uuid"
)

type DataEntity = map[string]interface{}

func GetAs[T any](d DataEntity, key FieldKey) (T, bool) {
	var zero T

	v, ok := d[key.String()]
	if !ok || v == nil {
		return zero, false
	}

	if typed, ok := v.(T); ok {
		return typed, true
	}

	var t T
	switch any(t).(type) {
	case string, activities.ActivityField:
		if s, ok := v.(string); ok {
			return any(activities.ActivityField(s)).(T), true
		}
	}

	return zero, false
}

func GetAsOrDefault[T any](d DataEntity, key FieldKey, def T) T {
	if v, ok := GetAs[T](d, key); ok {
		return v
	}
	return def
}

type FieldKind int

const (
	KindValue FieldKind = iota
	KindActivityValue
	KindUpdateScopeID
	KindFieldLog
	KindFunc
	KindKey
)

type FieldKey struct {
	Field activities.ActivityField
	Kind  FieldKind
}

func (k FieldKey) String() string {
	base := k.Field.String()

	switch k.Kind {
	case KindValue:
		return base

	case KindActivityValue:
		return base + "_activity_val"

	case KindUpdateScopeID:
		return base + "_update_scope_id"

	case KindFieldLog:
		return base + "_field_log"

	case KindFunc:
		return base + "_func"
	case KindKey:
		return base + "_key"
	default:
		return base
	}
}

func GetField(d DataEntity, key FieldKey) (any, bool) {
	v, ok := d[key.String()]
	return v, ok
}

func SetField(d DataEntity, key FieldKey, val any) {
	d[key.String()] = val
}

func GetString(d DataEntity, key FieldKey) (string, bool) {
	v, ok := d[key.String()]
	if !ok || v == nil {
		return "", false
	}
	return fmt.Sprint(v), true
}

func GetUUID(d DataEntity, key FieldKey) (uuid.UUID, bool) {
	v, ok := d[key.String()]
	if !ok {
		return uuid.UUID{}, false
	}
	id, ok := v.(uuid.UUID)
	return id, ok
}

func ValueKey[E ~string](field E) FieldKey {
	return FieldKey{Field: activities.ActivityField(field), Kind: KindValue}
}

func FuncKey[E ~string](field E) FieldKey {
	return FieldKey{Field: activities.ActivityField(field), Kind: KindFunc}
}

func FieldLogKey[E ~string](field E) FieldKey {
	return FieldKey{Field: activities.ActivityField(field), Kind: KindFieldLog}
}

func ActivityValKey[E ~string](field E) FieldKey {
	return FieldKey{Field: activities.ActivityField(field), Kind: KindActivityValue}
}

func FieldWithKey[E ~string](field E) FieldKey {
	return FieldKey{Field: activities.ActivityField(field), Kind: KindKey}
}

func UpdateScopeIDKey[E ~string](field E) FieldKey {
	return FieldKey{Field: activities.ActivityField(field), Kind: KindUpdateScopeID}
}
