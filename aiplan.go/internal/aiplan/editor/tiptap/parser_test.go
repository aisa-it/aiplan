package tiptap

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/editor"
)

func TestParseParagraph(t *testing.T) {
	tests := []struct {
		name       string
		json       string
		wantText   string
		wantIndent int
		wantAlign  editor.TextAlign
	}{
		{
			name:       "simple paragraph",
			json:       `{"type":"paragraph","attrs":{"textAlign":"left","indent":null},"content":[{"type":"text","text":"Hello"}]}`,
			wantText:   "Hello",
			wantIndent: 0,
			wantAlign:  editor.LeftAlign,
		},
		{
			name:       "paragraph with indent",
			json:       `{"type":"paragraph","attrs":{"textAlign":"left","indent":1},"content":[{"type":"text","text":"Indented"}]}`,
			wantText:   "Indented",
			wantIndent: 1,
			wantAlign:  editor.LeftAlign,
		},
		{
			name:       "paragraph with center align",
			json:       `{"type":"paragraph","attrs":{"textAlign":"center","indent":null},"content":[{"type":"text","text":"Centered"}]}`,
			wantText:   "Centered",
			wantIndent: 0,
			wantAlign:  editor.CenterAlign,
		},
		{
			name:       "paragraph with right align",
			json:       `{"type":"paragraph","attrs":{"textAlign":"right","indent":null},"content":[{"type":"text","text":"Right"}]}`,
			wantText:   "Right",
			wantIndent: 0,
			wantAlign:  editor.RightAlign,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var node TipTapNode
			if err := json.Unmarshal([]byte(tt.json), &node); err != nil {
				t.Fatalf("Failed to unmarshal JSON: %v", err)
			}

			p := parseParagraph(node)
			if p == nil {
				t.Fatal("parseParagraph returned nil")
			}

			if p.Indent != tt.wantIndent {
				t.Errorf("Indent = %d, want %d", p.Indent, tt.wantIndent)
			}

			if p.Align != tt.wantAlign {
				t.Errorf("Align = %v, want %v", p.Align, tt.wantAlign)
			}

			if len(p.Content) == 0 {
				t.Fatal("Content is empty")
			}

			text, ok := p.Content[0].(editor.Text)
			if !ok {
				t.Fatalf("Content[0] is not Text, got %T", p.Content[0])
			}

			if text.Content != tt.wantText {
				t.Errorf("Text.Content = %q, want %q", text.Content, tt.wantText)
			}
		})
	}
}

func TestParseTextMarks(t *testing.T) {
	tests := []struct {
		name           string
		json           string
		wantStrong     bool
		wantItalic     bool
		wantUnderlined bool
		wantStrike     bool
	}{
		{
			name:       "bold text",
			json:       `{"type":"text","marks":[{"type":"bold"}],"text":"Bold"}`,
			wantStrong: true,
		},
		{
			name:       "italic text",
			json:       `{"type":"text","marks":[{"type":"italic"}],"text":"Italic"}`,
			wantItalic: true,
		},
		{
			name:           "underline text",
			json:           `{"type":"text","marks":[{"type":"underline"}],"text":"Under"}`,
			wantUnderlined: true,
		},
		{
			name:       "strike text",
			json:       `{"type":"text","marks":[{"type":"strike"}],"text":"Strike"}`,
			wantStrike: true,
		},
		{
			name:       "bold and italic",
			json:       `{"type":"text","marks":[{"type":"bold"},{"type":"italic"}],"text":"Both"}`,
			wantStrong: true,
			wantItalic: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var node TipTapNode
			if err := json.Unmarshal([]byte(tt.json), &node); err != nil {
				t.Fatalf("Failed to unmarshal JSON: %v", err)
			}

			text := parseText(node)

			if text.Strong != tt.wantStrong {
				t.Errorf("Strong = %v, want %v", text.Strong, tt.wantStrong)
			}
			if text.Italic != tt.wantItalic {
				t.Errorf("Italic = %v, want %v", text.Italic, tt.wantItalic)
			}
			if text.Underlined != tt.wantUnderlined {
				t.Errorf("Underlined = %v, want %v", text.Underlined, tt.wantUnderlined)
			}
			if text.Strikethrough != tt.wantStrike {
				t.Errorf("Strikethrough = %v, want %v", text.Strikethrough, tt.wantStrike)
			}
		})
	}
}

func TestParseList(t *testing.T) {
	tests := []struct {
		name         string
		json         string
		wantNumbered bool
		wantTaskList bool
		wantCount    int
	}{
		{
			name:         "bullet list",
			json:         `{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","attrs":{"textAlign":"left","indent":null},"content":[{"type":"text","text":"Item 1"}]}]}]}`,
			wantNumbered: false,
			wantTaskList: false,
			wantCount:    1,
		},
		{
			name:         "ordered list",
			json:         `{"type":"orderedList","attrs":{"start":1},"content":[{"type":"listItem","content":[{"type":"paragraph","attrs":{"textAlign":"left","indent":null},"content":[{"type":"text","text":"Item 1"}]}]}]}`,
			wantNumbered: true,
			wantTaskList: false,
			wantCount:    1,
		},
		{
			name:         "task list",
			json:         `{"type":"taskList","content":[{"type":"taskItem","attrs":{"checked":false},"content":[{"type":"paragraph","attrs":{"textAlign":"left","indent":null},"content":[{"type":"text","text":"Task 1"}]}]}]}`,
			wantNumbered: false,
			wantTaskList: true,
			wantCount:    1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var node TipTapNode
			if err := json.Unmarshal([]byte(tt.json), &node); err != nil {
				t.Fatalf("Failed to unmarshal JSON: %v", err)
			}

			list := parseList(node)
			if list == nil {
				t.Fatal("parseList returned nil")
			}

			if list.Numbered != tt.wantNumbered {
				t.Errorf("Numbered = %v, want %v", list.Numbered, tt.wantNumbered)
			}
			if list.TaskList != tt.wantTaskList {
				t.Errorf("TaskList = %v, want %v", list.TaskList, tt.wantTaskList)
			}
			if len(list.Elements) != tt.wantCount {
				t.Errorf("Elements count = %d, want %d", len(list.Elements), tt.wantCount)
			}
		})
	}
}

func TestParseCodeBlock(t *testing.T) {
	jsonStr := `{"type":"codeBlock","attrs":{"language":null},"content":[{"type":"text","text":"const x = 1;"}]}`

	var node TipTapNode
	if err := json.Unmarshal([]byte(jsonStr), &node); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	code := parseCodeBlock(node)
	if code == nil {
		t.Fatal("parseCodeBlock returned nil")
	}

	want := "const x = 1;"
	if code.Content != want {
		t.Errorf("Content = %q, want %q", code.Content, want)
	}
}

func TestParseJSON(t *testing.T) {
	jsonStr := `{
		"type": "doc",
		"content": [
			{
				"type": "paragraph",
				"attrs": {"textAlign": "left", "indent": null},
				"content": [{"type": "text", "text": "Hello World"}]
			}
		]
	}`

	doc, err := ParseJSON(strings.NewReader(jsonStr))
	if err != nil {
		t.Fatalf("ParseJSON failed: %v", err)
	}

	if len(doc.Elements) != 1 {
		t.Fatalf("Elements count = %d, want 1", len(doc.Elements))
	}

	p, ok := doc.Elements[0].(*editor.Paragraph)
	if !ok {
		t.Fatalf("Elements[0] is not *Paragraph, got %T", doc.Elements[0])
	}

	if len(p.Content) == 0 {
		t.Fatal("Paragraph content is empty")
	}

	text, ok := p.Content[0].(editor.Text)
	if !ok {
		t.Fatalf("Paragraph content[0] is not Text, got %T", p.Content[0])
	}

	if text.Content != "Hello World" {
		t.Errorf("Text content = %q, want %q", text.Content, "Hello World")
	}
}
