package tiptap

import (
	"strings"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/editor"
)

// getAttrString безопасно извлекает строковый атрибут из map.
func getAttrString(attrs map[string]interface{}, key string) string {
	if attrs == nil {
		return ""
	}
	val, ok := attrs[key]
	if !ok {
		return ""
	}
	str, ok := val.(string)
	if !ok {
		return ""
	}
	return str
}

// getAttrInt безопасно извлекает целочисленный атрибут из map.
func getAttrInt(attrs map[string]interface{}, key string) int {
	if attrs == nil {
		return 0
	}
	val, ok := attrs[key]
	if !ok {
		return 0
	}

	// Может быть float64 из JSON
	if f, ok := val.(float64); ok {
		return int(f)
	}

	// Может быть int
	if i, ok := val.(int); ok {
		return i
	}

	return 0
}

// getAttrBool безопасно извлекает булевый атрибут из map.
func getAttrBool(attrs map[string]interface{}, key string) bool {
	if attrs == nil {
		return false
	}
	val, ok := attrs[key]
	if !ok {
		return false
	}
	b, ok := val.(bool)
	if !ok {
		return false
	}
	return b
}

// parseStyleAttr парсит CSS style строку в map key-value пар.
// Например: "background-color: red; color: blue;" -> {"background-color": "red", "color": "blue"}
func parseStyleAttr(style string) map[string]string {
	result := make(map[string]string)
	if style == "" {
		return result
	}

	parts := strings.Split(style, ";")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		kv := strings.SplitN(part, ":", 2)
		if len(kv) != 2 {
			continue
		}

		key := strings.TrimSpace(kv[0])
		value := strings.TrimSpace(kv[1])
		if key != "" && value != "" {
			result[key] = value
		}
	}

	return result
}

// parseTextAlign конвертирует строковое значение выравнивания в TextAlign.
func parseTextAlign(align string) editor.TextAlign {
	switch strings.TrimSpace(strings.ToLower(align)) {
	case "left":
		return editor.LeftAlign
	case "center":
		return editor.CenterAlign
	case "right":
		return editor.RightAlign
	default:
		return editor.LeftAlign
	}
}
