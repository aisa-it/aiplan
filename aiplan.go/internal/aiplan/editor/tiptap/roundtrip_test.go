package tiptap

import (
	"os"
	"testing"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/editor"
)

func TestParseFullTipTapJSON(t *testing.T) {
	// Открыть файл tiptap.json
	f, err := os.Open("../tiptap.json")
	if err != nil {
		t.Fatalf("Failed to open tiptap.json: %v", err)
	}
	defer f.Close()

	// Распарсить документ
	doc, err := ParseJSON(f)
	if err != nil {
		t.Fatalf("ParseJSON failed: %v", err)
	}

	// Проверить что документ не пустой
	if len(doc.Elements) == 0 {
		t.Fatal("Document has no elements")
	}

	t.Logf("Parsed document with %d elements", len(doc.Elements))

	// Подсчитать типы элементов
	counts := make(map[string]int)
	for _, elem := range doc.Elements {
		switch elem.(type) {
		case *editor.Paragraph:
			counts["paragraph"]++
		case *editor.Code:
			counts["code"]++
		case *editor.List:
			counts["list"]++
		case *editor.Quote:
			counts["quote"]++
		case *editor.Table:
			counts["table"]++
		case *editor.Spoiler:
			counts["spoiler"]++
		case *editor.InfoBlock:
			counts["infoblock"]++
		default:
			counts["unknown"]++
		}
	}

	t.Logf("Element counts: %+v", counts)

	// Проверить что есть основные типы элементов
	if counts["paragraph"] == 0 {
		t.Error("No paragraphs found")
	}
	if counts["list"] == 0 {
		t.Error("No lists found")
	}
	if counts["table"] == 0 {
		t.Error("No tables found")
	}
	if counts["code"] == 0 {
		t.Error("No code blocks found")
	}

	// Найти и проверить параграф с форматированием
	foundBold := false
	foundItalic := false
	foundLink := false

	for _, elem := range doc.Elements {
		if p, ok := elem.(*editor.Paragraph); ok {
			for _, content := range p.Content {
				if text, ok := content.(editor.Text); ok {
					if text.Strong {
						foundBold = true
					}
					if text.Italic {
						foundItalic = true
					}
					if text.URL != nil {
						foundLink = true
					}
				}
			}
		}
	}

	if !foundBold {
		t.Error("No bold text found")
	}
	if !foundItalic {
		t.Error("No italic text found")
	}
	if !foundLink {
		t.Error("No links found")
	}

	// Проверить таблицу с colspan/rowspan
	foundTableWithSpan := false
	for _, elem := range doc.Elements {
		if table, ok := elem.(*editor.Table); ok {
			for _, row := range table.Rows {
				for _, cell := range row {
					if cell.ColSpan > 1 || cell.RowSpan > 1 {
						foundTableWithSpan = true
						t.Logf("Found table cell with colspan=%d, rowspan=%d", cell.ColSpan, cell.RowSpan)
					}
				}
			}
		}
	}

	if !foundTableWithSpan {
		t.Error("No table cells with colspan/rowspan found")
	}

	// Проверить список задач
	foundTaskList := false
	for _, elem := range doc.Elements {
		if list, ok := elem.(*editor.List); ok {
			if list.TaskList {
				foundTaskList = true
				t.Logf("Found task list with %d items", len(list.Elements))
			}
		}
	}

	if !foundTaskList {
		t.Error("No task lists found")
	}
}
