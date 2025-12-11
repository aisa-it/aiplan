package tiptap

import (
	"encoding/hex"
	"encoding/json"
	"log/slog"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/editor/edtypes"
)

// Serialize сериализует editor.Document в TipTap JSON.
func Serialize(doc *edtypes.Document) ([]byte, error) {
	tipTapDoc := TipTapDocument{
		Type:    "doc",
		Content: make([]TipTapNode, 0, len(doc.Elements)),
	}

	for _, elem := range doc.Elements {
		node := serializeElement(elem)
		if node != nil {
			tipTapDoc.Content = append(tipTapDoc.Content, *node)
		}
	}

	return json.Marshal(tipTapDoc)
}

// serializeElement преобразует элемент editor в TipTap ноду.
func serializeElement(elem any) *TipTapNode {
	if elem == nil {
		return nil
	}

	switch e := elem.(type) {
	case *edtypes.Paragraph:
		return serializeParagraph(e)
	case *edtypes.Code:
		return serializeCode(e)
	case *edtypes.Quote:
		return serializeQuote(e)
	case *edtypes.List:
		return serializeList(e)
	case *edtypes.Image:
		return serializeImage(e)
	case *edtypes.Table:
		return serializeTable(e)
	case *edtypes.Spoiler:
		return serializeSpoiler(e)
	case *edtypes.InfoBlock:
		return serializeInfoBlock(e)
	case *edtypes.DateNode:
		return serializeDateNode(e)
	case *edtypes.IssueLinkMention:
		return serializeIssueLinkMention(e)
	default:
		slog.Warn("Unknown element type for serialization", "type", e)
		return nil
	}
}

// serializeParagraph преобразует Paragraph в TipTap ноду.
func serializeParagraph(p *edtypes.Paragraph) *TipTapNode {
	node := &TipTapNode{
		Type:    "paragraph",
		Content: make([]TipTapNode, 0, len(p.Content)),
		Attrs:   make(map[string]interface{}),
	}

	// Добавляем атрибуты если они не default
	if p.Indent > 0 {
		node.Attrs["indent"] = p.Indent
	}
	if p.Align != edtypes.LeftAlign {
		node.Attrs["textAlign"] = serializeTextAlign(p.Align)
	}

	// Сериализуем содержимое
	for _, content := range p.Content {
		if childNode := serializeParagraphContent(content); childNode != nil {
			node.Content = append(node.Content, *childNode)
		}
	}

	return node
}

// serializeParagraphContent преобразует содержимое параграфа.
func serializeParagraphContent(content any) *TipTapNode {
	switch c := content.(type) {
	case edtypes.Text:
		return serializeText(&c)
	case *edtypes.Image:
		return serializeImage(c)
	case *edtypes.DateNode:
		return serializeDateNode(c)
	case *edtypes.IssueLinkMention:
		return serializeIssueLinkMention(c)
	case *edtypes.Mention:
		return serializeMention(c)
	case *edtypes.HardBreak:
		return serializeHardBreak(c)
	default:
		slog.Warn("Unknown paragraph content type for serialization", "type", c)
		return nil
	}
}

// serializeText преобразует Text в TipTap текстовую ноду.
func serializeText(t *edtypes.Text) *TipTapNode {
	node := &TipTapNode{
		Type: "text",
		Text: t.Content,
	}

	// Добавляем marks (форматирование)
	marks := make([]TipTapMark, 0)

	if t.Strong {
		marks = append(marks, TipTapMark{Type: "bold"})
	}
	if t.Italic {
		marks = append(marks, TipTapMark{Type: "italic"})
	}
	if t.Underlined {
		marks = append(marks, TipTapMark{Type: "underline"})
	}
	if t.Strikethrough {
		marks = append(marks, TipTapMark{Type: "strike"})
	}
	if t.Sup {
		marks = append(marks, TipTapMark{Type: "superscript"})
	}
	if t.Sub {
		marks = append(marks, TipTapMark{Type: "subscript"})
	}

	// Цвет текста
	if t.Color != nil {
		marks = append(marks, TipTapMark{
			Type: "textStyle",
			Attrs: map[string]interface{}{
				"color": colorToHex(*t.Color),
			},
		})
	}

	// Цвет фона
	if t.BgColor != nil {
		marks = append(marks, TipTapMark{
			Type: "highlight",
			Attrs: map[string]interface{}{
				"color": colorToHex(*t.BgColor),
			},
		})
	}

	// Ссылка
	if t.URL != nil {
		marks = append(marks, TipTapMark{
			Type: "link",
			Attrs: map[string]interface{}{
				"href":   t.URL.String(),
				"target": "_blank",
			},
		})
	}

	if len(marks) > 0 {
		node.Marks = marks
	}

	return node
}

// serializeCode преобразует Code в TipTap codeBlock ноду.
func serializeCode(c *edtypes.Code) *TipTapNode {
	node := &TipTapNode{
		Type:    "codeBlock",
		Content: make([]TipTapNode, 1),
	}

	// Код хранится как текстовая нода внутри
	node.Content[0] = TipTapNode{
		Type: "text",
		Text: c.Content,
	}

	return node
}

// serializeQuote преобразует Quote в TipTap blockquote ноду.
func serializeQuote(q *edtypes.Quote) *TipTapNode {
	node := &TipTapNode{
		Type:    "blockquote",
		Content: make([]TipTapNode, 0, len(q.Content)),
	}

	for _, elem := range q.Content {
		if childNode := serializeElement(elem); childNode != nil {
			node.Content = append(node.Content, *childNode)
		}
	}

	return node
}

// serializeList преобразует List в TipTap bulletList или orderedList ноду.
func serializeList(l *edtypes.List) *TipTapNode {
	listType := "bulletList"
	if l.Numbered {
		listType = "orderedList"
	}
	if l.TaskList {
		listType = "taskList"
	}

	node := &TipTapNode{
		Type:    listType,
		Content: make([]TipTapNode, 0, len(l.Elements)),
	}

	for _, item := range l.Elements {
		itemNode := TipTapNode{
			Type:    "listItem",
			Content: make([]TipTapNode, 0, len(item.Content)),
		}

		if l.TaskList {
			itemNode.Attrs = map[string]interface{}{
				"checked": item.Checked,
			}
		}

		for _, elem := range item.Content {
			if childNode := serializeParagraph(&elem); childNode != nil {
				itemNode.Content = append(itemNode.Content, *childNode)
			}
		}

		node.Content = append(node.Content, itemNode)
	}

	return node
}

// serializeImage преобразует Image в TipTap image ноду.
func serializeImage(img *edtypes.Image) *TipTapNode {
	node := &TipTapNode{
		Type:  "image",
		Attrs: make(map[string]interface{}),
	}

	if img.Src != nil {
		node.Attrs["src"] = img.Src.String()
	}
	if img.Width > 0 {
		node.Attrs["width"] = img.Width
	}
	if img.Align != edtypes.LeftAlign {
		node.Attrs["textAlign"] = serializeTextAlign(img.Align)
	}

	return node
}

// serializeTable преобразует Table в TipTap table ноду.
func serializeTable(t *edtypes.Table) *TipTapNode {
	node := &TipTapNode{
		Type:    "table",
		Content: make([]TipTapNode, 0),
	}

	// Сериализуем строки
	for _, row := range t.Rows {
		rowNode := TipTapNode{
			Type:    "tableRow",
			Content: make([]TipTapNode, 0, len(row)),
		}

		for _, cell := range row {
			cellNode := TipTapNode{
				Type:    "tableCell",
				Attrs:   make(map[string]interface{}),
				Content: make([]TipTapNode, 0, len(cell.Content)),
			}

			if cell.Header {
				cellNode.Type = "tableHeader"
			}
			if cell.ColSpan > 1 {
				cellNode.Attrs["colspan"] = cell.ColSpan
			}
			if cell.RowSpan > 1 {
				cellNode.Attrs["rowspan"] = cell.RowSpan
			}

			for _, elem := range cell.Content {
				if childNode := serializeParagraph(&elem); childNode != nil {
					cellNode.Content = append(cellNode.Content, *childNode)
				}
			}

			rowNode.Content = append(rowNode.Content, cellNode)
		}

		node.Content = append(node.Content, rowNode)
	}

	return node
}

// serializeSpoiler преобразует Spoiler в TipTap spoiler ноду.
func serializeSpoiler(s *edtypes.Spoiler) *TipTapNode {
	node := &TipTapNode{
		Type:    "spoiler",
		Attrs:   make(map[string]interface{}),
		Content: make([]TipTapNode, 0, len(s.Content)),
	}

	if s.Title != "" {
		node.Attrs["title"] = s.Title
	}
	node.Attrs["collapsed"] = s.Collapsed
	node.Attrs["bgColor"] = colorToHex(s.BgColor)
	node.Attrs["color"] = colorToHex(s.Color)

	for _, elem := range s.Content {
		if childNode := serializeParagraph(&elem); childNode != nil {
			node.Content = append(node.Content, *childNode)
		}
	}

	return node
}

// serializeInfoBlock преобразует InfoBlock в TipTap info-block ноду.
func serializeInfoBlock(ib *edtypes.InfoBlock) *TipTapNode {
	node := &TipTapNode{
		Type:    "info-block",
		Attrs:   make(map[string]interface{}),
		Content: make([]TipTapNode, 0, len(ib.Content)),
	}

	if ib.Title != "" {
		node.Attrs["title"] = ib.Title
	}
	node.Attrs["color"] = colorToHex(ib.Color)

	for _, elem := range ib.Content {
		if childNode := serializeParagraph(&elem); childNode != nil {
			node.Content = append(node.Content, *childNode)
		}
	}

	return node
}

// serializeDateNode преобразует DateNode в TipTap date-node ноду.
func serializeDateNode(d *edtypes.DateNode) *TipTapNode {
	return &TipTapNode{
		Type: "date-node",
		Attrs: map[string]interface{}{
			"date": d.Date,
		},
	}
}

// serializeIssueLinkMention преобразует IssueLinkMention в TipTap issueLinkMention ноду.
func serializeIssueLinkMention(ilm *edtypes.IssueLinkMention) *TipTapNode {
	return &TipTapNode{
		Type: "issueLinkMention",
		Attrs: map[string]interface{}{
			"slug":              ilm.Slug,
			"projectIdentifier": ilm.ProjectIdentifier,
			"currentIssueId":    ilm.CurrentIssueId,
			"originalUrl":       ilm.OriginalUrl,
		},
	}
}

// serializeMention преобразует Mention в TipTap mention ноду.
func serializeMention(m *edtypes.Mention) *TipTapNode {
	return &TipTapNode{
		Type: "mention",
		Attrs: map[string]interface{}{
			"id":    m.ID,
			"label": m.Label,
		},
	}
}

// serializeHardBreak преобразует HardBreak в TipTap hardBreak ноду.
func serializeHardBreak(_ *edtypes.HardBreak) *TipTapNode {
	return &TipTapNode{
		Type: "hardBreak",
	}
}

// serializeTextAlign преобразует TextAlign в строку.
func serializeTextAlign(align edtypes.TextAlign) string {
	switch align {
	case edtypes.LeftAlign:
		return "left"
	case edtypes.CenterAlign:
		return "center"
	case edtypes.RightAlign:
		return "right"
	default:
		return "left"
	}
}

// colorToHex преобразует Color в hex строку формата #RRGGBBAA.
func colorToHex(c edtypes.Color) string {
	return "#" + hex.EncodeToString([]byte{c.R, c.G, c.B, c.A})
}
