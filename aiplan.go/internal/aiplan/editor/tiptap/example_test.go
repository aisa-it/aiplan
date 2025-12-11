package tiptap_test

import (
	"fmt"
	"strings"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/editor/tiptap"
)

// ExampleParseJSON демонстрирует базовое использование парсера TipTap JSON.
func ExampleParseJSON() {
	// JSON контент от TipTap редактора
	jsonContent := `{
		"type": "doc",
		"content": [
			{
				"type": "paragraph",
				"attrs": {"textAlign": "left", "indent": null},
				"content": [
					{"type": "text", "marks": [{"type": "bold"}], "text": "Привет"},
					{"type": "text", "text": " "},
					{"type": "text", "marks": [{"type": "italic"}], "text": "мир"}
				]
			}
		]
	}`

	// Парсинг JSON
	doc, err := tiptap.ParseJSON(strings.NewReader(jsonContent))
	if err != nil {
		fmt.Printf("Ошибка парсинга: %v\n", err)
		return
	}

	// Вывод количества элементов
	fmt.Printf("Документ содержит %d элементов\n", len(doc.Elements))

	// Output:
	// Документ содержит 1 элементов
}
