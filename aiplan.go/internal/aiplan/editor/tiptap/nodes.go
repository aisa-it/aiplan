package tiptap

import (
	"log/slog"
	"net/url"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/editor/edtypes"
)

// parseText преобразует текстовую ноду TipTap в edtypes.Text.
func parseText(node TipTapNode) edtypes.Text {
	text := edtypes.Text{
		Content: node.Text,
	}

	// Применить marks (форматирование)
	if len(node.Marks) > 0 {
		applyMarks(&text, node.Marks)
	}

	return text
}

// parseParagraph преобразует параграф TipTap в edtypes.Paragraph.
func parseParagraph(node TipTapNode) *edtypes.Paragraph {
	if node.Type != "paragraph" {
		return nil
	}

	p := &edtypes.Paragraph{
		Content: make([]any, 0),
		Indent:  getAttrInt(node.Attrs, "indent"),
		Align:   parseTextAlign(getAttrString(node.Attrs, "textAlign")),
	}

	// Обработать содержимое параграфа
	for _, child := range node.Content {
		switch child.Type {
		case "text":
			p.Content = append(p.Content, parseText(child))
		case "image", "imageResize":
			if img := parseImage(child); img != nil {
				p.Content = append(p.Content, img)
			}
		case "date-node":
			if dn := parseDateNode(child); dn != nil {
				p.Content = append(p.Content, dn)
			}
		case "issueLinkMention":
			if ilm := parseIssueLinkMention(child); ilm != nil {
				p.Content = append(p.Content, ilm)
			}
		case "mention":
			if m := parseMention(child); m != nil {
				p.Content = append(p.Content, m)
			}
		case "hardBreak":
			if hb := parseHardBreak(child); hb != nil {
				p.Content = append(p.Content, hb)
			}
		default:
			slog.Warn("Unknown paragraph child type", "type", child.Type)
		}
	}

	return p
}

// parseCodeBlock преобразует блок кода TipTap в edtypes.Code.
func parseCodeBlock(node TipTapNode) *edtypes.Code {
	if node.Type != "codeBlock" {
		return nil
	}

	var text string
	for _, child := range node.Content {
		if child.Type == "text" {
			text += child.Text
		}
	}

	return &edtypes.Code{
		Content: text,
	}
}

// parseBlockquote преобразует цитату TipTap в edtypes.Quote.
func parseBlockquote(node TipTapNode) *edtypes.Quote {
	if node.Type != "blockquote" {
		return nil
	}

	quote := &edtypes.Quote{
		Content: make([]edtypes.Paragraph, 0),
	}

	// Обработать содержимое цитаты (может включать параграфы и spoiler)
	for _, child := range node.Content {
		switch child.Type {
		case "paragraph":
			if p := parseParagraph(child); p != nil {
				quote.Content = append(quote.Content, *p)
			}
		case "spoiler":
			// Spoiler внутри blockquote - обрабатываем его параграфы
			if spoiler := parseSpoiler(child); spoiler != nil {
				quote.Content = append(quote.Content, spoiler.Content...)
			}
		default:
			slog.Warn("Unknown blockquote child type", "type", child.Type)
		}
	}

	return quote
}

// parseImage преобразует изображение TipTap в edtypes.Image.
// Поддерживает как "image", так и "imageResize" типы.
func parseImage(node TipTapNode) *edtypes.Image {
	if node.Type != "image" && node.Type != "imageResize" {
		return nil
	}

	src := getAttrString(node.Attrs, "src")
	if src == "" {
		return nil
	}

	imgUrl, err := url.Parse(src)
	if err != nil {
		slog.Warn("Failed to parse image URL", "src", src, "err", err)
		return nil
	}

	img := &edtypes.Image{
		Src:   imgUrl,
		Width: getAttrInt(node.Attrs, "width"),
		Align: edtypes.LeftAlign,
	}

	// Парсить style для выравнивания
	style := getAttrString(node.Attrs, "style")
	if style != "" {
		styles := parseStyleAttr(style)
		if float, ok := styles["float"]; ok {
			switch float {
			case "left":
				img.Align = edtypes.LeftAlign
			case "right":
				img.Align = edtypes.RightAlign
			case "none", "":
				img.Align = edtypes.CenterAlign
			}
		}
	}

	return img
}

// parseDateNode преобразует date-node TipTap в edtypes.DateNode.
func parseDateNode(node TipTapNode) *edtypes.DateNode {
	if node.Type != "date-node" {
		return nil
	}

	return &edtypes.DateNode{
		Date: getAttrString(node.Attrs, "date"),
	}
}

// parseIssueLinkMention преобразует issueLinkMention TipTap в edtypes.IssueLinkMention.
func parseIssueLinkMention(node TipTapNode) *edtypes.IssueLinkMention {
	if node.Type != "issueLinkMention" {
		return nil
	}

	return &edtypes.IssueLinkMention{
		Slug:              getAttrString(node.Attrs, "slug"),
		ProjectIdentifier: getAttrString(node.Attrs, "projectIdentifier"),
		CurrentIssueId:    getAttrString(node.Attrs, "currentIssueId"),
		OriginalUrl:       getAttrString(node.Attrs, "originalUrl"),
	}
}

// parseMention преобразует mention TipTap в edtypes.Mention.
func parseMention(node TipTapNode) *edtypes.Mention {
	if node.Type != "mention" {
		return nil
	}

	return &edtypes.Mention{
		ID:    getAttrString(node.Attrs, "id"),
		Label: getAttrString(node.Attrs, "label"),
	}
}

// parseHardBreak преобразует hardBreak TipTap в edtypes.HardBreak.
func parseHardBreak(node TipTapNode) *edtypes.HardBreak {
	if node.Type != "hardBreak" {
		return nil
	}

	return &edtypes.HardBreak{}
}

// parseSpoiler преобразует spoiler TipTap в edtypes.Spoiler.
func parseSpoiler(node TipTapNode) *edtypes.Spoiler {
	if node.Type != "spoiler" {
		return nil
	}

	spoiler := &edtypes.Spoiler{
		Title:     getAttrString(node.Attrs, "title"),
		Collapsed: getAttrBool(node.Attrs, "collapsed"),
		Content:   make([]edtypes.Paragraph, 0),
	}

	// Парсить style для цветов
	style := getAttrString(node.Attrs, "style")
	if style != "" {
		styles := parseStyleAttr(style)
		if color, ok := styles["color"]; ok && color != "" {
			c, _ := edtypes.ParseColor(color)
			spoiler.Color = c
		}
		if bgColor, ok := styles["background-color"]; ok && bgColor != "" {
			c, _ := edtypes.ParseColor(bgColor)
			spoiler.BgColor = c
		}
	}

	// Обработать содержимое
	for _, child := range node.Content {
		if child.Type == "paragraph" {
			if p := parseParagraph(child); p != nil {
				spoiler.Content = append(spoiler.Content, *p)
			}
		}
	}

	return spoiler
}

// parseInfoBlock преобразует info-block TipTap в edtypes.InfoBlock.
func parseInfoBlock(node TipTapNode) *edtypes.InfoBlock {
	if node.Type != "info-block" {
		return nil
	}

	block := &edtypes.InfoBlock{
		Title:   getAttrString(node.Attrs, "title"),
		Content: make([]edtypes.Paragraph, 0),
	}

	// Парсить цвет иконки
	iconColor := getAttrString(node.Attrs, "iconColor")
	if iconColor != "" {
		c, _ := edtypes.ParseColor(iconColor)
		block.Color = c
	}

	// Обработать содержимое
	for _, child := range node.Content {
		if child.Type == "paragraph" {
			if p := parseParagraph(child); p != nil {
				block.Content = append(block.Content, *p)
			}
		}
	}

	return block
}

// parseList преобразует список TipTap в edtypes.List.
func parseList(node TipTapNode) *edtypes.List {
	list := &edtypes.List{
		Elements: make([]edtypes.ListElement, 0),
	}

	// Определить тип списка
	switch node.Type {
	case "bulletList":
		list.Numbered = false
		list.TaskList = false
	case "orderedList":
		list.Numbered = true
		list.TaskList = false
	case "taskList":
		list.Numbered = false
		list.TaskList = true
	default:
		return nil
	}

	// Обработать элементы списка
	for _, child := range node.Content {
		if child.Type == "listItem" || child.Type == "taskItem" {
			if elem := parseListItem(child); elem != nil {
				list.Elements = append(list.Elements, *elem)
			}
		}
	}

	return list
}

// parseListItem преобразует элемент списка TipTap в edtypes.ListElement.
func parseListItem(node TipTapNode) *edtypes.ListElement {
	elem := &edtypes.ListElement{
		Content: make([]edtypes.Paragraph, 0),
		Checked: false,
	}

	// Для taskItem извлечь статус checked
	if node.Type == "taskItem" {
		elem.Checked = getAttrBool(node.Attrs, "checked")
	}

	// Обработать содержимое элемента списка
	for _, child := range node.Content {
		if child.Type == "paragraph" {
			if p := parseParagraph(child); p != nil {
				elem.Content = append(elem.Content, *p)
			}
		}
	}

	return elem
}

// parseTable преобразует таблицу TipTap в edtypes.Table.
func parseTable(node TipTapNode) *edtypes.Table {
	if node.Type != "table" {
		return nil
	}

	table := &edtypes.Table{
		Rows: make([][]edtypes.TableCell, 0),
	}

	// Обработать строки таблицы
	for _, rowNode := range node.Content {
		if rowNode.Type != "tableRow" {
			continue
		}

		row := make([]edtypes.TableCell, 0)

		// Обработать ячейки в строке
		for _, cellNode := range rowNode.Content {
			if cellNode.Type != "tableHeader" && cellNode.Type != "tableCell" {
				continue
			}

			cell := edtypes.TableCell{
				Header:  cellNode.Type == "tableHeader",
				ColSpan: getAttrInt(cellNode.Attrs, "colspan"),
				RowSpan: getAttrInt(cellNode.Attrs, "rowspan"),
				Content: make([]edtypes.Paragraph, 0),
			}

			// Если ColSpan/RowSpan не указаны, по умолчанию 1
			if cell.ColSpan == 0 {
				cell.ColSpan = 1
			}
			if cell.RowSpan == 0 {
				cell.RowSpan = 1
			}

			// Обработать содержимое ячейки
			for _, contentNode := range cellNode.Content {
				if contentNode.Type == "paragraph" {
					if p := parseParagraph(contentNode); p != nil {
						cell.Content = append(cell.Content, *p)
					}
				}
			}

			row = append(row, cell)
		}

		if len(row) > 0 {
			table.Rows = append(table.Rows, row)
		}
	}

	return table
}
