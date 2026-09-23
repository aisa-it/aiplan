package types

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

type IssuePropertySchema struct {
	Schema               string           `json:"$schema"`
	Type                 string           `json:"type"`
	Required             []string         `json:"required"`
	Properties           SchemaProperties `json:"properties"`
	AdditionalProperties bool             `json:"additionalProperties"`
}

type SchemaType struct {
	Type  string `json:"type,omitempty"`
	Const string `json:"const,omitempty"`
}

type SchemaProperties struct {
	Name  SchemaType `json:"name"`
	Type  SchemaType `json:"type"`
	Value SchemaType `json:"value"`
}

// GenValueSchema создаёт JSON Schema для валидации значения по типу свойства
func GenValueSchema(propType string, options []string) map[string]any {
	switch propType {
	case "string":
		return map[string]any{"type": "string"}
	case "boolean":
		return map[string]any{"type": "boolean"}
	case "select":
		return genSelectValueSchema(options)
	case "multiselect":
		return genMultiselectValueSchema(options)
	case "lookup", "file":
		// Значение - id строки справочника (lookup) или id вложения задачи (file),
		// null - сброс; существование проверяется отдельным запросом в БД, схема
		// проверяет только форму
		return map[string]any{"type": []any{"string", "null"}}
	case "date", "datetime":
		return genDateValueSchema(propType)
	case "number":
		// JSON-число или числовая строка; null и пустая строка - сброс. Разбор и
		// отсев NaN/Inf/мусора - в NormalizeNumberValue, схема проверяет только форму
		return map[string]any{"type": []any{"number", "string", "null"}}
	case "link":
		return map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"oneOf": []any{
				map[string]any{
					"type":                 "object",
					"minProperties":        2,
					"additionalProperties": false,
					"properties": map[string]any{
						"name": map[string]any{"type": "string", "minLength": 1},
						"url":  map[string]any{"type": "string", "format": "uri"},
					},
				},
				map[string]any{"type": "null"},
			},
		}
	default:
		return map[string]any{}
	}
}

// genSelectValueSchema - строка из options или null (сброс значения)
func genSelectValueSchema(options []string) map[string]any {
	if len(options) == 0 {
		return map[string]any{"type": []any{"string", "null"}}
	}
	// Конвертируем []string в []any и добавляем nil для возможности сброса значения
	enumValues := make([]any, len(options)+1)
	for i, opt := range options {
		enumValues[i] = opt
	}
	enumValues[len(options)] = nil
	return map[string]any{"type": []any{"string", "null"}, "enum": enumValues}
}

// genMultiselectValueSchema - массив строк из options (пустой массив или null -
// сброс значения). Уникальность элементов проверяется отдельно (CheckUniqueValues):
// это настройка шаблона, а не форма значения
func genMultiselectValueSchema(options []string) map[string]any {
	items := map[string]any{"type": "string"}
	if len(options) > 0 {
		enumValues := make([]any, len(options))
		for i, opt := range options {
			enumValues[i] = opt
		}
		items["enum"] = enumValues
	}
	return map[string]any{"type": []any{"array", "null"}, "items": items}
}

// genDateValueSchema - схемы значений полей date/datetime; null или пустая строка -
// сброс значения. Семантика (несуществующая дата 2026-13-45, диапазон unix time) -
// в CheckDateValue, паттерн проверяет только форму
func genDateValueSchema(propType string) map[string]any {
	if propType == "date" {
		// Строка YYYY-MM-DD
		return map[string]any{"type": []any{"string", "null"}, "pattern": `^(\d{4}-\d{2}-\d{2})?$`}
	}
	// Строка из десятичных цифр - unix time в секундах
	return map[string]any{"type": []any{"string", "null"}, "pattern": `^\d*$`}
}

// maxDatetimeUnix - 9999-12-31T23:59:59Z, верхняя граница значения datetime-поля
const maxDatetimeUnix = 253402300799

// CheckDateValue семантически валидирует значение полей типа date/datetime:
// JSON Schema паттерном не поймать несуществующую дату (2026-13-45) или выход
// unix time за диапазон [0, 9999 год]. Для остальных типов, nil и пустой строки - true
func CheckDateValue(propType string, value any) bool {
	s, ok := value.(string)
	if !ok || s == "" {
		return true
	}
	switch propType {
	case "date":
		_, err := time.Parse("2006-01-02", s)
		return err == nil
	case "datetime":
		n, err := strconv.ParseInt(s, 10, 64)
		return err == nil && n >= 0 && n <= maxDatetimeUnix
	}
	return true
}

// NormalizeNumberValue приводит значение number-поля к канонической строке хранения
// (strconv.FormatFloat 'f', -1): принимает JSON-число (float64/json.Number) или
// числовую строку; nil и пустая строка - сброс (""). ok=false - не число (NaN, Inf, мусор)
func NormalizeNumberValue(value any) (string, bool) {
	if s, isString := value.(string); isString {
		value = strings.TrimSpace(s)
		if value == "" {
			value = nil
		}
	}
	if value == nil {
		return "", true
	}
	f, ok := numberValueToFloat(value)
	if !ok || math.IsNaN(f) || math.IsInf(f, 0) {
		return "", false
	}
	return strconv.FormatFloat(f, 'f', -1, 64), true
}

// numberValueToFloat - JSON-число (float64/json.Number) или числовая строка как float64
func numberValueToFloat(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case json.Number:
		f, err := v.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(v, 64)
		return f, err == nil
	}
	return 0, false
}

// ValidatePropertyValue проверяет значение кастомного поля по типу шаблона: форму -
// JSON Schema (GenValueSchema), семантику дат - CheckDateValue, число - NormalizeNumberValue.
// Единая проверка для HTTP- и MCP-каналов
func ValidatePropertyValue(propType string, options []string, value any) error {
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("schema.json", GenValueSchema(propType, options)); err != nil {
		return err
	}
	sch, err := compiler.Compile("schema.json")
	if err != nil {
		return err
	}
	if err := sch.Validate(value); err != nil {
		return err
	}
	if !CheckDateValue(propType, value) {
		return errors.New("invalid date value")
	}
	if propType == "number" {
		if _, ok := NormalizeNumberValue(value); !ok {
			return errors.New("invalid number value")
		}
	}
	return nil
}

// CheckUniqueValues проверяет настройку уникальности multiselect-поля: при
// uniqueValues элементы массива не должны повторяться. Для остальных типов,
// nil и не-массивов - true (форму значения проверяет JSON Schema)
func CheckUniqueValues(propType string, uniqueValues bool, value any) bool {
	if propType != "multiselect" || !uniqueValues {
		return true
	}
	items, ok := value.([]any)
	if !ok {
		return true
	}
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		key := fmt.Sprint(item)
		if _, dup := seen[key]; dup {
			return false
		}
		seen[key] = struct{}{}
	}
	return true
}
