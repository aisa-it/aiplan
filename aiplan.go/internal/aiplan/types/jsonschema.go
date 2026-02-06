package types

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
			return map[string]any{"type": "string"}
		}
		// Конвертируем []string в []any для корректной работы с jsonschema
		enumValues := make([]any, len(options))
		for i, opt := range options {
			enumValues[i] = opt
		}
		return map[string]any{"type": "string", "enum": enumValues}
	default:
		return map[string]any{}
	}
}
