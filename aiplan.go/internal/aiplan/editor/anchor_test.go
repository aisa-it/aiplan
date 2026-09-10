package editor

import (
	"strings"
	"testing"
)

// collectParagraphText собирает весь текст параграфов документа в одну строку.
func collectParagraphText(doc *Document) string {
	var sb strings.Builder
	for _, el := range doc.Elements {
		p, ok := el.(Paragraph)
		if !ok {
			continue
		}
		for _, c := range p.Content {
			if t, ok := c.(Text); ok {
				sb.WriteString(t.Content)
			}
		}
	}
	return sb.String()
}

// TestParseDocumentDocAnchor проверяет, что якоря документа АИДока
// не ломают парсинг HTML и не оставляют мусора в экспортируемом содержимом.
func TestParseDocumentDocAnchor(t *testing.T) {
	const in = `<p>До <span class="doc-anchor" data-anchor-id="vvedenie" data-anchor-title="Введение"></span>после</p>`

	doc, err := ParseDocument(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}

	if len(doc.Elements) != 1 {
		t.Fatalf("ожидался 1 элемент, получено %d: %+v", len(doc.Elements), doc.Elements)
	}

	p, ok := doc.Elements[0].(Paragraph)
	if !ok {
		t.Fatalf("ожидался Paragraph, получено %T", doc.Elements[0])
	}

	// Якорь не должен превращаться в пустой Text: только "До " и "после"
	if len(p.Content) != 2 {
		t.Errorf("якорь оставил лишние элементы в параграфе: %d вместо 2: %+v", len(p.Content), p.Content)
	}

	if got, want := collectParagraphText(doc), "До после"; got != want {
		t.Errorf("текст параграфа искажён якорем: %q, ожидалось %q", got, want)
	}
}

// TestParseDocumentDocAnchorStandalone проверяет якорь в качестве единственного
// содержимого параграфа — параграф остаётся, но пустых Text в нём нет.
func TestParseDocumentDocAnchorStandalone(t *testing.T) {
	const in = `<p><span class="doc-anchor" data-anchor-id="a1" data-anchor-title="Раздел"></span></p>`

	doc, err := ParseDocument(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}

	p, ok := doc.Elements[0].(Paragraph)
	if !ok {
		t.Fatalf("ожидался Paragraph, получено %T", doc.Elements[0])
	}
	if len(p.Content) != 0 {
		t.Errorf("якорь попал в содержимое параграфа: %+v", p.Content)
	}
	if got := collectParagraphText(doc); got != "" {
		t.Errorf("якорь дал текст %q, ожидалась пустая строка", got)
	}
}

// TestParseDocumentAnchorLink проверяет, что ссылки на якорь сохраняют href.
func TestParseDocumentAnchorLink(t *testing.T) {
	cases := map[string]string{
		"локальный якорь": "#vvedenie",
		"другой документ": "/ws-slug/aidoc/2c0f6b1e-1111-2222-3333-444455556666#vvedenie",
	}

	for name, href := range cases {
		t.Run(name, func(t *testing.T) {
			doc, err := ParseDocument(strings.NewReader(`<p><a href="` + href + `">текст</a></p>`))
			if err != nil {
				t.Fatal(err)
			}

			p := doc.Elements[0].(Paragraph)
			txt, ok := p.Content[0].(Text)
			if !ok {
				t.Fatalf("ожидался Text, получено %T", p.Content[0])
			}
			if txt.URL == nil {
				t.Fatalf("ссылка потеряна: %+v", txt)
			}
			if txt.URL.String() != href {
				t.Errorf("href искажён: %q вместо %q", txt.URL.String(), href)
			}
			if txt.Content != "текст" {
				t.Errorf("текст ссылки искажён: %q", txt.Content)
			}
		})
	}
}

// TestParseDocumentHeadingWithID фиксирует поведение парсера HTML на заголовках
// с постоянным id. ParseDocument заголовки (h1-h6) не разбирает вообще —
// это давнее поведение, атрибут id его не меняет и парсинг не ломает.
func TestParseDocumentHeadingWithID(t *testing.T) {
	const in = `<h2 id="vvedenie">Введение</h2><p>текст</p>`

	doc, err := ParseDocument(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}

	if len(doc.Elements) != 1 {
		t.Fatalf("ожидался 1 элемент (заголовки не разбираются), получено %d: %+v",
			len(doc.Elements), doc.Elements)
	}
	if got, want := collectParagraphText(doc), "текст"; got != want {
		t.Errorf("содержимое искажено: %q, ожидалось %q", got, want)
	}
}
