package edtypes_test

import (
	"encoding/json"
	"testing"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/editor/edtypes"
	_ "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/editor/tiptap" // Регистрация парсера
)

func TestDocument_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name          string
		json          string
		wantElemCount int
		wantErr       bool
	}{
		{
			name: "simple paragraph",
			json: `{
				"type": "doc",
				"content": [
					{
						"type": "paragraph",
						"content": [
							{
								"type": "text",
								"text": "Hello World"
							}
						]
					}
				]
			}`,
			wantElemCount: 1,
			wantErr:       false,
		},
		{
			name: "multiple paragraphs",
			json: `{
				"type": "doc",
				"content": [
					{
						"type": "paragraph",
						"content": [
							{
								"type": "text",
								"text": "First"
							}
						]
					},
					{
						"type": "paragraph",
						"content": [
							{
								"type": "text",
								"text": "Second"
							}
						]
					}
				]
			}`,
			wantElemCount: 2,
			wantErr:       false,
		},
		{
			name: "empty document",
			json: `{
				"type": "doc",
				"content": []
			}`,
			wantElemCount: 0,
			wantErr:       false,
		},
		{
			name:          "invalid json",
			json:          `{"type": "doc", "content": [}`,
			wantElemCount: 0,
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var doc edtypes.Document
			err := json.Unmarshal([]byte(tt.json), &doc)

			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			if len(doc.Elements) != tt.wantElemCount {
				t.Errorf("Elements count = %v, want %v", len(doc.Elements), tt.wantElemCount)
			}
		})
	}
}

func TestDocument_UnmarshalJSON_Integration(t *testing.T) {
	// Тест для проверки что UnmarshalJSON работает в составе DTO структуры
	type IssueDTO struct {
		Title       string           `json:"title"`
		Description edtypes.Document `json:"description"`
	}

	jsonData := `{
		"title": "Test Issue",
		"description": {
			"type": "doc",
			"content": [
				{
					"type": "paragraph",
					"content": [
						{
							"type": "text",
							"text": "This is a test description"
						}
					]
				}
			]
		}
	}`

	var issue IssueDTO
	err := json.Unmarshal([]byte(jsonData), &issue)
	if err != nil {
		t.Fatalf("UnmarshalJSON() error = %v", err)
	}

	if issue.Title != "Test Issue" {
		t.Errorf("Title = %v, want %v", issue.Title, "Test Issue")
	}

	if len(issue.Description.Elements) != 1 {
		t.Errorf("Description.Elements count = %v, want %v", len(issue.Description.Elements), 1)
	}

	// Проверяем что первый элемент - это Paragraph
	if para, ok := issue.Description.Elements[0].(*edtypes.Paragraph); ok {
		if len(para.Content) != 1 {
			t.Errorf("Paragraph content count = %v, want %v", len(para.Content), 1)
		}
		if text, ok := para.Content[0].(edtypes.Text); ok {
			if text.Content != "This is a test description" {
				t.Errorf("Text content = %v, want %v", text.Content, "This is a test description")
			}
		} else {
			t.Errorf("First content element is not Text, got %T", para.Content[0])
		}
	} else {
		t.Error("First element is not *Paragraph")
	}
}
