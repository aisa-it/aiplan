package tracker

import (
	"fmt"
	"reflect"

	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
)

func Diff(old, new any) []FieldChange {
	var changes []FieldChange

	oldVal := reflect.ValueOf(old)
	newVal := reflect.ValueOf(new)

	if oldVal.Kind() == reflect.Ptr {
		oldVal = oldVal.Elem()
	}
	if newVal.Kind() == reflect.Ptr {
		newVal = newVal.Elem()
	}

	oldType := oldVal.Type()

	for i := 0; i < oldVal.NumField(); i++ {
		field := oldType.Field(i)
		tag := field.Tag.Get("act")
		if tag == "" {
			continue
		}

		spec, err := ParseActivityTag(tag)
		if err != nil || spec.Field == "" {
			continue
		}

		oldValue := getOptValueViaMethods(oldVal.Field(i))
		newValue := getOptValueViaMethods(newVal.Field(i))

		oldSet := isOptSetViaMethods(oldVal.Field(i))
		newSet := isOptSetViaMethods(newVal.Field(i))

		if !oldSet && !newSet {
			continue
		}

		switch spec.Kind {
		case "collection":
			changes = append(changes, diffCollection(spec, oldValue, newValue)...)
		default: // scalar
			changes = append(changes, diffScalar(spec, oldValue, newValue, oldSet, newSet)...)
		}
	}

	return changes
}

func diffScalar(spec ActivityFieldSpec, oldValue, newValue any, oldSet, newSet bool) []FieldChange {
	oldStr := formatValueToString(oldValue)
	newStr := formatValueToString(newValue)

	if oldSet && newSet && oldStr == newStr {
		return nil
	}

	var oldId, newId uuid.NullUUID
	if entityRef, ok := oldValue.(EntityRef); ok {
		oldId.UUID = entityRef.ID
		oldId.Valid = true
	}
	if entityRef, ok := newValue.(EntityRef); ok {
		newId.UUID = entityRef.ID
		newId.Valid = true
	}

	if oldSet && newSet {
		return []FieldChange{{
			Verb:   "updated",
			Field:  actField.ActivityField(spec.Field),
			OldVal: oldStr,
			NewVal: newStr,
			OldID:  oldId,
			NewID:  newId,
		}}
	}

	if oldSet && !newSet {
		return []FieldChange{{
			Verb:   "updated",
			Field:  actField.ActivityField(spec.Field),
			OldVal: oldStr,
			NewVal: "",
			OldID:  oldId,
			NewID:  newId,
		}}
	}

	if !oldSet && newSet {
		return []FieldChange{{
			Verb:   "updated",
			Field:  actField.ActivityField(spec.Field),
			OldVal: "",
			NewVal: newStr,
			OldID:  oldId,
			NewID:  newId,
		}}
	}

	return nil
}

func diffCollection(spec ActivityFieldSpec, oldValue, newValue any) []FieldChange {
	var changes []FieldChange

	oldSlice := toEntityRefSlice(oldValue, spec.Table)
	newSlice := toEntityRefSlice(newValue, spec.Table)
	if len(oldSlice) == 0 && len(newSlice) == 0 {
		return changes
	}

	oldMap := make(map[uuid.UUID]EntityRef)
	newMap := make(map[uuid.UUID]EntityRef)
	for _, ref := range oldSlice {
		oldMap[ref.ID] = ref
	}
	for _, ref := range newSlice {
		newMap[ref.ID] = ref
	}

	oldIDs := make([]any, 0, len(oldSlice))
	newIDs := make([]any, 0, len(newSlice))
	for _, ref := range oldSlice {
		oldIDs = append(oldIDs, ref.ID)
	}
	for _, ref := range newSlice {
		newIDs = append(newIDs, ref.ID)
	}

	changesList := utils.CalculateIDChanges(newIDs, oldIDs)
	if len(changesList.InvolvedIds) == 0 {
		return changes
	}

	for _, id := range changesList.DelIds {
		if oldRef, exists := oldMap[id]; exists {
			changes = append(changes, FieldChange{
				Verb:   "removed",
				Field:  actField.ActivityField(spec.Field),
				OldVal: oldRef.NameValue,
				OldID:  uuid.NullUUID{UUID: id, Valid: true},
			})
		}
	}

	for _, id := range changesList.AddIds {
		if newRef, exists := newMap[id]; exists {
			changes = append(changes, FieldChange{
				Verb:   "added",
				Field:  actField.ActivityField(spec.Field),
				NewVal: newRef.NameValue,
				NewID:  uuid.NullUUID{UUID: id, Valid: true},
			})
		}
	}

	return changes
}

func getOptValueViaMethods(field reflect.Value) interface{} {
	method := field.MethodByName("Value")
	if method.IsValid() {
		results := method.Call(nil)
		if len(results) > 0 {
			return results[0].Interface()
		}
	}
	return nil
}

func isOptSetViaMethods(field reflect.Value) bool {
	method := field.MethodByName("IsSet")
	if method.IsValid() {
		results := method.Call(nil)
		if len(results) > 0 {
			return results[0].Bool()
		}
	}
	return false
}

func formatValueToString(v interface{}) string {
	if v == nil {
		return ""
	}
	if entityRef, ok := v.(EntityRef); ok {
		return entityRef.NameValue
	}
	return fmt.Sprintf("%v", v)
}

func toEntityRefSlice(v interface{}, table string) []EntityRef {
	if v == nil {
		return []EntityRef{}
	}
	if slice, ok := v.([]EntityRef); ok {
		return slice
	}
	if slice, ok := v.([]uuid.UUID); ok {
		result := make([]EntityRef, len(slice))
		for i, id := range slice {
			result[i] = EntityRef{ID: id, NameField: table}
		}
		return result
	}
	return []EntityRef{}
}
