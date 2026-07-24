package types

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

// fieldSchema возвращает JSON Schema для поля формы.
func fieldSchema(f FormFields) map[string]any {
	s := baseFieldSchema(f)
	if !f.Required {
		s = map[string]any{
			"anyOf": []any{
				map[string]any{"type": "null"},
				s,
			},
		}
	}
	return s
}

func baseFieldSchema(f FormFields) map[string]any {
	vr := f.Validate

	switch f.Type {
	case "input", "string", "textarea":
		return applyRules(vr, map[string]any{"type": "string"})
	case "checkbox":
		return map[string]any{"type": "boolean"}
	case "numeric":
		return applyRules(vr, map[string]any{"type": "number"})
	case "color":
		return map[string]any{
			"type":    "string",
			"pattern": `^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`,
		}
	case "date":
		return applyRules(vr, map[string]any{"type": "integer"})
	case "select":
		return selectSchema(vr)
	case "multiselect":
		return multiSelectSchema(vr, f.Required)
	case "attachment":
		return map[string]any{"type": "string", "format": "uuid"}
	default:
		return map[string]any{}
	}
}

func selectSchema(vr *ValidationRule) map[string]any {
	if vr != nil && len(vr.Opt) > 0 {
		return map[string]any{"type": "string", "enum": vr.Opt}
	}
	return map[string]any{"type": "string"}
}

func multiSelectSchema(vr *ValidationRule, required bool) map[string]any {
	items := map[string]any{"type": "string"}
	if vr != nil && len(vr.Opt) > 0 {
		items["enum"] = vr.Opt
	}
	s := map[string]any{"type": "array", "items": items, "uniqueItems": true}
	if required {
		s["minItems"] = 1
	}
	return s
}

func applyRules(vr *ValidationRule, schema map[string]any) map[string]any {
	if vr == nil || vr.ValidationType == "" {
		return schema
	}

	var off int
	for _, t := range strings.Fields(vr.ValidationType) {
		switch t {
		case "min_max":
			if off+1 < len(vr.Opt) {
				if v, ok := parseFloat(vr.Opt[off]); ok {
					schema["minimum"] = v
				}
				if v, ok := parseFloat(vr.Opt[off+1]); ok {
					schema["maximum"] = v
				}
			}
			off += 2

		case "len_str":
			if off+1 < len(vr.Opt) {
				if v, ok := parseFloat(vr.Opt[off]); ok {
					schema["minLength"] = int(math.Round(v))
				}
				if v, ok := parseFloat(vr.Opt[off+1]); ok {
					schema["maxLength"] = int(math.Round(v))
				}
			}
			off += 2

		case "only_integer":
			if schema["type"] == "number" {
				schema["type"] = "integer"
			}
		}
	}
	return schema
}

// buildConditions строит allOf из if/then/else для зависимых полей.
func buildConditions(fields FormFieldsSlice) []map[string]any {
	var out []map[string]any

	for i, f := range fields {
		if f.DependOn == nil {
			continue
		}
		if f.DependOn.FieldIndex < 0 || f.DependOn.FieldIndex >= i {
			continue
		}

		parent := fields[f.DependOn.FieldIndex]
		condition := parentCondition(parent, f.DependOn, len(fields))
		if condition == nil {
			continue
		}

		out = append(out, map[string]any{
			"if":   condition,
			"then": branchSchema(len(fields), i, fieldSchema(f)),
			"else": branchSchema(len(fields), i, map[string]any{"type": "null"}),
		})
	}

	return out
}

// parentCondition условие родительской зависимости
func parentCondition(parent FormFields, dep *FormFieldDependency, total int) map[string]any {
	items := emptyPrefixItems(total)

	if dep.OptionIndex == nil {
		items[dep.FieldIndex] = map[string]any{"const": dep.ExpectedValue}
		return map[string]any{"prefixItems": items}
	}

	if parent.Validate == nil || parent.Validate.Opt == nil || *dep.OptionIndex >= len(parent.Validate.Opt) {
		return nil
	}

	optVal := parent.Validate.Opt[*dep.OptionIndex]

	switch parent.Type {
	case "multiselect":
		cond := map[string]any{"contains": map[string]any{"const": optVal}}
		if !dep.ExpectedValue {
			cond = map[string]any{"not": cond}
		}
		items[dep.FieldIndex] = cond

	default:
		if dep.ExpectedValue {
			items[dep.FieldIndex] = map[string]any{"const": optVal}
		} else {
			items[dep.FieldIndex] = map[string]any{"not": map[string]any{"const": optVal}}
		}
	}

	return map[string]any{"prefixItems": items}
}

func emptyPrefixItems(n int) []map[string]any {
	items := make([]map[string]any, n)
	for i := range items {
		items[i] = map[string]any{}
	}
	return items
}

// branchSchema условия для текущего поля (then/else)
func branchSchema(total, idx int, schema map[string]any) map[string]any {
	items := emptyPrefixItems(total)
	items[idx] = schema
	return map[string]any{"prefixItems": items}
}

// parseFloat конвертирует any в float64 для числовых правил.
func parseFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case string:
		f, err := strconv.ParseFloat(n, 64)
		return f, err == nil
	}
	return 0, false
}

type FormAnswerResult struct {
	Type  string `json:"type"`
	Label string `json:"label"`
	Val   any    `json:"val"`
}

type FormAnswerValidationResult struct {
	Valid  bool              `json:"valid"`
	Errors []FormAnswerError `json:"errors,omitempty"`
}

type FormValidator struct {
	fields FormFieldsSlice
	schema *jsonschema.Schema
}

func NewFormValidator(fields FormFieldsSlice) (*FormValidator, error) {
	items := make([]map[string]any, len(fields))
	for i, f := range fields {
		if f.DependOn != nil {
			items[i] = map[string]any{} // allOf управляет валидацией
		} else {
			items[i] = fieldSchema(f)
		}
	}

	root := map[string]any{
		"$schema":     "https://json-schema.org/draft/2020-12/schema",
		"type":        "array",
		"prefixItems": items,
		"items":       false,
	}

	if cond := buildConditions(fields); len(cond) > 0 {
		root["allOf"] = cond
	}

	schema, err := compileSchema(root)
	if err != nil {
		return nil, err
	}

	return &FormValidator{fields: fields, schema: schema}, nil
}

func (v *FormValidator) Validate(answers []any) ([]FormAnswerResult, FormAnswerValidationResult) {
	if len(answers) != len(v.fields) {
		return nil, FormAnswerValidationResult{Valid: false, Errors: []FormAnswerError{{
			Index: 0, Message: fmt.Sprintf("expected %d answers, got %d", len(v.fields), len(answers)),
		}}}
	}

	if err := v.schema.Validate(answers); err != nil {
		return nil, FormAnswerValidationResult{Valid: false, Errors: collectErrors(err, v.fields)}
	}

	results := make([]FormAnswerResult, len(v.fields))
	for i, f := range v.fields {
		results[i] = FormAnswerResult{Type: f.Type, Label: f.Label, Val: answers[i]}
	}
	return results, FormAnswerValidationResult{Valid: true}
}

func compileSchema(root map[string]any) (*jsonschema.Schema, error) {
	raw, err := json.Marshal(root)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}

	c := jsonschema.NewCompiler()
	if err := c.AddResource("form.json", doc); err != nil {
		return nil, fmt.Errorf("add resource: %w", err)
	}
	s, err := c.Compile("form.json")
	if err != nil {
		return nil, fmt.Errorf("compile: %w", err)
	}
	return s, nil
}

// form answer error

type FormAnswerError struct {
	Index   int    `json:"index"`
	Label   string `json:"label,omitempty"`
	Message string `json:"message"`
}

func (e FormAnswerError) Error() string {
	if e.Label != "" {
		return fmt.Sprintf("[%d] %s: %s", e.Index, e.Label, e.Message)
	}
	return fmt.Sprintf("[%d]: %s", e.Index, e.Message)
}

func collectErrors(err error, fields FormFieldsSlice) []FormAnswerError {
	var ve *jsonschema.ValidationError
	ok := errors.As(err, &ve)
	if !ok {
		return []FormAnswerError{{Message: err.Error()}}
	}
	return extractErrors(ve, fields)
}

func extractErrors(ve *jsonschema.ValidationError, fields FormFieldsSlice) []FormAnswerError {
	if len(ve.Causes) > 0 {
		var out []FormAnswerError
		for _, cause := range ve.Causes {
			out = append(out, extractErrors(cause, fields)...)
		}
		return out
	}

	idx := 0
	if len(ve.InstanceLocation) > 0 {
		if ii, err := strconv.Atoi(ve.InstanceLocation[0]); err == nil {
			idx = ii
		}
	}

	msg := ve.Error()
	if i := strings.Index(msg, "] "); i != -1 {
		msg = msg[i+2:]
	}

	var label string
	if idx >= 0 && idx < len(fields) {
		label = fields[idx].Label
	}

	return []FormAnswerError{{Index: idx, Label: label, Message: msg}}
}
