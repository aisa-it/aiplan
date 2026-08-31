package types

import (
	"strconv"
	"time"
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
	case "lookup":
		// Значение - id строки справочника (или null для сброса); существование
		// строки проверяется отдельным запросом в БД, схема проверяет только форму
		return map[string]any{"type": []any{"string", "null"}}
	case "date", "datetime":
		return genDateValueSchema(propType)
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
