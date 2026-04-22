package tracker

import (
	"fmt"
	"reflect"

	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
)

func Diff(old, new any, entityID uuid.UUID, entityName string) []FieldChange {
	var changes []FieldChange
	var snapshotID uuid.UUID

	if oldSnap, ok := old.(SnapshotI); ok {
		snapshotID = oldSnap.GetID()
	} else if newSnap, ok := new.(SnapshotI); ok {
		snapshotID = newSnap.GetID()
	}

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
			changes = append(changes, diffCollection(spec, oldValue, newValue, entityID, entityName)...)
		default: // scalar
			changes = append(changes, diffScalar(spec, oldValue, newValue, oldSet, newSet, snapshotID, entityID, entityName)...)
		}
	}

	return changes
}

func diffScalar(spec ActivityFieldSpec, oldValue, newValue any, oldSet, newSet bool, snapshotID uuid.UUID, linkedEntityID uuid.UUID, entityName string) []FieldChange {
	oldStr := formatValueToString(oldValue)
	newStr := formatValueToString(newValue)

	if oldSet && newSet && oldStr == newStr {
		return nil
	}

	oldId := extractEntityRefID(oldValue)
	newId := extractEntityRefID(newValue)

	if spec.PreserveID && snapshotID != uuid.Nil && !oldId.Valid && !newId.Valid {
		oldId = uuid.NullUUID{UUID: snapshotID, Valid: true}
		newId = uuid.NullUUID{UUID: snapshotID, Valid: true}
	}

	changes := []FieldChange{}
	hasLinked := spec.LinkedField != "" && linkedEntityID != uuid.Nil

	if oldSet && newSet {
		changes = append(changes, makeUpdateChange(spec, oldStr, newStr, oldId, newId))
		if hasLinked {
			changes = appendLinkedChangesForScalar(changes, spec, oldId, newId, linkedEntityID, entityName)
		}
	} else if oldSet && !newSet {
		changes = append(changes, makeUpdateChange(spec, oldStr, "", oldId, newId))
		if hasLinked && oldId.Valid {
			changes = append(changes, makeLinkedRemovedChange(spec, oldId.UUID, linkedEntityID, entityName))
		}
	} else if !oldSet && newSet {
		changes = append(changes, makeUpdateChange(spec, "", newStr, oldId, newId))
		if hasLinked && newId.Valid {
			changes = append(changes, makeLinkedAddedChange(spec, newId.UUID, linkedEntityID, entityName))
		}
	}

	return changes
}

func diffCollection(spec ActivityFieldSpec, oldValue, newValue any, linkedEntityID uuid.UUID, entityName string) []FieldChange {
	oldSlice := toEntityRefSlice(oldValue, spec.Table)
	newSlice := toEntityRefSlice(newValue, spec.Table)
	if len(oldSlice) == 0 && len(newSlice) == 0 {
		return nil
	}

	oldMap := sliceToMap(oldSlice)
	newMap := sliceToMap(newSlice)

	oldIDs := entityRefsToIDs(oldSlice)
	newIDs := entityRefsToIDs(newSlice)

	changesList := utils.CalculateIDChanges(newIDs, oldIDs)
	if len(changesList.InvolvedIds) == 0 {
		return nil
	}

	hasLinked := spec.LinkedField != "" && linkedEntityID != uuid.Nil

	var result []FieldChange

	for _, id := range changesList.DelIds {
		if oldRef, exists := oldMap[id]; exists {
			result = append(result, makeRemovedChange(spec, oldRef.NameValue, id))
			if hasLinked {
				result = append(result, makeLinkedRemovedChange(spec, id, linkedEntityID, entityName))
			}
		}
	}

	for _, id := range changesList.AddIds {
		if newRef, exists := newMap[id]; exists {
			result = append(result, makeAddedChange(spec, newRef.NameValue, id))
			if hasLinked {
				result = append(result, makeLinkedAddedChange(spec, id, linkedEntityID, entityName))
			}
		}
	}

	return result
}

// diffScalar helpers

func makeUpdateChange(spec ActivityFieldSpec, oldVal, newVal string, oldID, newID uuid.NullUUID) FieldChange {
	return FieldChange{
		Verb:       "updated",
		Field:      actField.ActivityField(spec.Field),
		OldVal:     oldVal,
		NewVal:     newVal,
		OldID:      oldID,
		NewID:      newID,
		PreserveID: spec.PreserveID,
	}
}

func appendLinkedChangesForScalar(changes []FieldChange, spec ActivityFieldSpec, oldId, newId uuid.NullUUID, linkedEntityID uuid.UUID, entityName string) []FieldChange {
	if oldId.Valid {
		changes = append(changes, makeLinkedRemovedChange(spec, oldId.UUID, linkedEntityID, entityName))
	}
	if newId.Valid {
		changes = append(changes, makeLinkedAddedChange(spec, newId.UUID, linkedEntityID, entityName))
	}
	return changes
}

// diffCollection helpers

func makeRemovedChange(spec ActivityFieldSpec, name string, id uuid.UUID) FieldChange {
	oldID := uuid.NullUUID{}
	if id != uuid.Nil {
		oldID = uuid.NullUUID{UUID: id, Valid: true}
	}
	return FieldChange{
		Verb:       "removed",
		Field:      actField.ActivityField(spec.Field),
		OldVal:     name,
		OldID:      oldID,
		PreserveID: spec.PreserveID,
	}
}

func makeAddedChange(spec ActivityFieldSpec, name string, id uuid.UUID) FieldChange {
	newID := uuid.NullUUID{}
	if id != uuid.Nil {
		newID = uuid.NullUUID{UUID: id, Valid: true}
	}
	return FieldChange{
		Verb:       "added",
		Field:      actField.ActivityField(spec.Field),
		NewVal:     name,
		NewID:      newID,
		PreserveID: spec.PreserveID,
	}
}

// shared linked helpers

func makeLinkedRemovedChange(spec ActivityFieldSpec, targetID, linkedEntityID uuid.UUID, entityName string) FieldChange {
	//if entityName == "" {
	//	entityName = linkedEntityID.String()
	//}
	oldID := uuid.NullUUID{}
	if linkedEntityID != uuid.Nil {
		oldID = uuid.NullUUID{UUID: linkedEntityID, Valid: true}
	}
	return FieldChange{
		Verb:       "removed",
		Field:      actField.ActivityField(spec.LinkedField),
		OldVal:     entityName,
		NewVal:     "",
		OldID:      oldID,
		NewID:      uuid.NullUUID{},
		PreserveID: spec.PreserveID,
		EntityID:   targetID,
		IsLinked:   true,
	}
}

func makeLinkedAddedChange(spec ActivityFieldSpec, targetID, linkedEntityID uuid.UUID, entityName string) FieldChange {
	//if entityName == "" {
	//	entityName = linkedEntityID.String()
	//}
	newID := uuid.NullUUID{}
	if linkedEntityID != uuid.Nil {
		newID = uuid.NullUUID{UUID: linkedEntityID, Valid: true}
	}
	return FieldChange{
		Verb:       "added",
		Field:      actField.ActivityField(spec.LinkedField),
		OldVal:     "",
		NewVal:     entityName,
		OldID:      uuid.NullUUID{},
		NewID:      newID,
		PreserveID: spec.PreserveID,
		EntityID:   targetID,
		IsLinked:   true,
	}
}

// utility functions

func extractEntityRefID(value any) uuid.NullUUID {
	if entityRef, ok := value.(EntityRef); ok {
		if entityRef.ID != uuid.Nil {
			return uuid.NullUUID{UUID: entityRef.ID, Valid: true}
		}
	}
	return uuid.NullUUID{}
}

func sliceToMap(refs []EntityRef) map[uuid.UUID]EntityRef {
	result := make(map[uuid.UUID]EntityRef)
	for _, ref := range refs {
		result[ref.ID] = ref
	}
	return result
}

func entityRefsToIDs(refs []EntityRef) []any {
	ids := make([]any, 0, len(refs))
	for _, ref := range refs {
		ids = append(ids, ref.ID)
	}
	return ids
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
	if ptrStr, ok := v.(*string); ok {
		if ptrStr == nil {
			return ""
		}
		return *ptrStr
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
