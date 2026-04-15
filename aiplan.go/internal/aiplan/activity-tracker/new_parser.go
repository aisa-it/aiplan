package tracker

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/opt"
	"github.com/gofrs/uuid"
)

func ParseActivityTag(tag string) (ActivityFieldSpec, error) {
	spec := ActivityFieldSpec{Kind: "scalar"} // default

	for _, param := range strings.Split(tag, ";") {
		parts := strings.SplitN(param, ":", 2)
		if len(parts) != 2 {
			continue
		}

		switch parts[0] {
		case "req":
			spec.Req = parts[1]
		case "field":
			spec.Field = parts[1]
		case "kind":
			spec.Kind = parts[1]
		case "transform":
			spec.Transform = parts[1]
		case "table":
			spec.Table = parts[1]
		}
	}

	return spec, nil
}

func UpdateSnapshotFromMap[T any](base T, data map[string]interface{}) T {
	result := base // todo check
	v := reflect.ValueOf(&result).Elem()
	t := v.Type()

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		tag := field.Tag.Get("act")
		if tag == "" {
			continue
		}

		spec, err := ParseActivityTag(tag)
		if err != nil || spec.Req == "" {
			continue
		}

		if value, exists := data[spec.Req]; exists && value != nil {
			var convertedValue interface{}

			switch spec.Kind {
			case "scalar":
				convertedValue = convertScalarValue(value, spec, field.Type)
			case "collection":
				convertedValue = convertCollectionValue(value, spec, field.Type)
			}

			if convertedValue != nil {
				fieldValue := v.Field(i)
				fieldValue.Set(reflect.ValueOf(convertedValue))
			}
		}
	}

	return result
}

func SnapshotFromMap[T any](data map[string]interface{}) T {
	var result T
	v := reflect.ValueOf(&result).Elem()
	t := v.Type()

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		tag := field.Tag.Get("act")
		if tag == "" {
			continue
		}

		spec, err := ParseActivityTag(tag)
		if err != nil || spec.Req == "" {
			continue
		}

		if value, exists := data[spec.Req]; exists && value != nil {
			var convertedValue interface{}

			switch spec.Kind {
			case "scalar":
				convertedValue = convertScalarValue(value, spec, field.Type)
			case "collection":
				convertedValue = convertCollectionValue(value, spec, field.Type)
			}

			if convertedValue != nil {
				fieldValue := v.Field(i)
				fieldValue.Set(reflect.ValueOf(convertedValue))
			}
		}
	}

	return result
}

func tryParseUUID(value interface{}) (uuid.UUID, bool) {
	if uuidVal, ok := value.(uuid.UUID); ok {
		return uuidVal, true
	}

	if str, ok := value.(string); ok {
		if id, err := uuid.FromString(str); err == nil {
			return id, true
		}
	}

	if obj, ok := value.(map[string]interface{}); ok {
		if idStr, exists := obj["id"]; exists {
			if str, ok := idStr.(string); ok {
				if id, err := uuid.FromString(str); err == nil {
					return id, true
				}
			}
		}
	}

	str := fmt.Sprintf("%v", value)
	if id, err := uuid.FromString(str); err == nil {
		return id, true
	}

	return uuid.Nil, false
}

func convertScalarValue(value interface{}, spec ActivityFieldSpec, fieldType reflect.Type) interface{} {
	switch fieldType {
	case reflect.TypeOf(opt.Field[string]{}):
		if str, ok := value.(string); ok {
			return opt.Some(str)
		}
	case reflect.TypeOf(opt.Field[uuid.UUID]{}):
		if id, ok := tryParseUUID(value); ok {
			return opt.Some(id)
		}
	case reflect.TypeOf(opt.Field[EntityRef]{}):
		if spec.Transform == "uuid" {
			if id, ok := tryParseUUID(value); ok {
				entityRef := EntityRef{ID: id, NameField: spec.Table}
				return opt.Some(entityRef)
			}
		} else {
			if obj, ok := value.(map[string]interface{}); ok {
				if idStr, exists := obj["id"]; exists {
					if str, ok := idStr.(string); ok {
						if id, err := uuid.FromString(str); err == nil {
							entityRef := EntityRef{ID: id, NameField: spec.Table}
							return opt.Some(entityRef)
						}
					}
				}
			}
		}
	}
	return nil
}

func convertCollectionValue(value interface{}, spec ActivityFieldSpec, fieldType reflect.Type) interface{} {
	if spec.Transform != "uuid" {
		return nil
	}

	var ids []uuid.UUID

	switch slice := value.(type) {
	case []uuid.UUID:
		ids = slice
	case []string:
		for _, str := range slice {
			if id, err := uuid.FromString(str); err == nil {
				ids = append(ids, id)
			}
		}
	case []interface{}:
		for _, item := range slice {
			if id, ok := tryParseUUID(item); ok {
				ids = append(ids, id)
			}
		}
	}

	if len(ids) == 0 {
		return nil
	}

	switch fieldType {
	case reflect.TypeOf(opt.Field[[]uuid.UUID]{}):
		return opt.Some(ids)
	case reflect.TypeOf(opt.Field[[]EntityRef]{}):
		entityRefs := make([]EntityRef, len(ids))
		for i, id := range ids {
			entityRefs[i] = EntityRef{ID: id, NameField: spec.Table}
		}
		return opt.Some(entityRefs)
	}

	return nil
}
