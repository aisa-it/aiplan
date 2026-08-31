package tiptap

import (
	"strings"
	"testing"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/editor"
)

// TestParseDocAnchorInParagraph проверяет, что нода якоря документа АИДока
// внутри параграфа пропускается и не добавляет мусор в содержимое.
func TestParseDocAnchorInParagraph(t *testing.T) {
	const doc = `{"type":"doc","content":[{"type":"paragraph","content":[
		{"type":"text","text":"До "},
		{"type":"docAnchor","attrs":{"anchorId":"vvedenie","anchorTitle":"Введение"}},
		{"type":"text","text":"после"}
	]}]}`

	d, err := ParseJSON(strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}

	p, ok := d.Elements[0].(*editor.Paragraph)
	if !ok {
		t.Fatalf("ожидался *editor.Paragraph, получено %T", d.Elements[0])
	}

	if len(p.Content) != 2 {
		t.Fatalf("якорь оставил лишние элементы: %d вместо 2: %+v", len(p.Content), p.Content)
	}

	var sb strings.Builder
	for _, c := range p.Content {
		txt, ok := c.(editor.Text)
		if !ok {
			t.Fatalf("ожидался editor.Text, получено %T", c)
		}
		sb.WriteString(txt.Content)
	}

	if got, want := sb.String(), "До после"; got != want {
		t.Errorf("текст параграфа искажён якорем: %q, ожидалось %q", got, want)
	}
}

// TestParseDocAnchorTopLevel проверяет, что якорь на верхнем уровне документа
// не порождает элемент и не роняет парсер.
func TestParseDocAnchorTopLevel(t *testing.T) {
	const doc = `{"type":"doc","content":[
		{"type":"docAnchor","attrs":{"anchorId":"vvedenie","anchorTitle":"Введение"}},
		{"type":"paragraph","content":[{"type":"text","text":"текст"}]}
	]}`

	d, err := ParseJSON(strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}

	if len(d.Elements) != 1 {
		t.Fatalf("ожидался 1 элемент, получено %d: %+v", len(d.Elements), d.Elements)
	}
	if _, ok := d.Elements[0].(*editor.Paragraph); !ok {
		t.Fatalf("ожидался *editor.Paragraph, получено %T", d.Elements[0])
	}
}
