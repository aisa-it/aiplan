package tiptap

import (
	"log/slog"
	"net/url"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/editor"
)

// parseText преобразует текстовую ноду TipTap в editor.Text.
func parseText(node TipTapNode) editor.Text {
	text := editor.Text{
		Content: node.Text,
	}

	// Применить marks (форматирование)
	if len(node.Marks) > 0 {
		applyMarks(&text, node.Marks)
	}

	return text
}

// parseParagraph преобразует параграф TipTap в editor.Paragraph.
func parseParagraph(node TipTapNode) *editor.Paragraph {
	if node.Type != "paragraph" {
		return nil
	}

	p := &editor.Paragraph{
		Content: make([]any, 0),
		Indent:  getAttrInt(node.Attrs, "indent"),
		Align:   parseTextAlign(getAttrString(node.Attrs, "textAlign")),
	}

	// Обработать содержимое параграфа
	for _, child := range node.Content {
		switch child.Type {
		case "text":
			p.Content = append(p.Content, parseText(child))
		case "image":
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
		default:
			slog.Warn("Unknown paragraph child type", "type", child.Type)
		}
	}

	return p
}

// parseCodeBlock преобразует блок кода TipTap в editor.Code.
func parseCodeBlock(node TipTapNode) *editor.Code {
	if node.Type != "codeBlock" {
		return nil
	}

	var text string
	for _, child := range node.Content {
		if child.Type == "text" {
			text += child.Text
		}
	}

	return &editor.Code{
		Content: text,
	}
}

// parseBlockquote преобразует цитату TipTap в editor.Quote.
func parseBlockquote(node TipTapNode) *editor.Quote {
	if node.Type != "blockquote" {
		return nil
	}

	quote := &editor.Quote{
		Content: make([]editor.Paragraph, 0),
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

// parseImage преобразует изображение TipTap в editor.Image.
func parseImage(node TipTapNode) *editor.Image {
	if node.Type != "image" {
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

	img := &editor.Image{
		Src:   imgUrl,
		Width: getAttrInt(node.Attrs, "width"),
		Align: editor.LeftAlign,
	}

	// Парсить style для выравнивания
	style := getAttrString(node.Attrs, "style")
	if style != "" {
		styles := parseStyleAttr(style)
		if float, ok := styles["float"]; ok {
			switch float {
			case "left":
				img.Align = editor.LeftAlign
			case "right":
				img.Align = editor.RightAlign
			case "none", "":
				img.Align = editor.CenterAlign
			}
		}
	}

	return img
}

// parseDateNode преобразует date-node TipTap в editor.DateNode.
func parseDateNode(node TipTapNode) *editor.DateNode {
	if node.Type != "date-node" {
		return nil
	}

	return &editor.DateNode{
		Date: getAttrString(node.Attrs, "date"),
	}
}

// parseIssueLinkMention преобразует issueLinkMention TipTap в editor.IssueLinkMention.
func parseIssueLinkMention(node TipTapNode) *editor.IssueLinkMention {
	if node.Type != "issueLinkMention" {
		return nil
	}

	return &editor.IssueLinkMention{
		Slug:              getAttrString(node.Attrs, "slug"),
		ProjectIdentifier: getAttrString(node.Attrs, "projectIdentifier"),
		CurrentIssueId:    getAttrString(node.Attrs, "currentIssueId"),
		OriginalUrl:       getAttrString(node.Attrs, "originalUrl"),
	}
}

// parseSpoiler преобразует spoiler TipTap в editor.Spoiler.
func parseSpoiler(node TipTapNode) *editor.Spoiler {
	if node.Type != "spoiler" {
		return nil
	}

	spoiler := &editor.Spoiler{
		Title:     getAttrString(node.Attrs, "title"),
		Collapsed: getAttrBool(node.Attrs, "collapsed"),
		Content:   make([]editor.Paragraph, 0),
	}

	// Парсить style для цветов
	style := getAttrString(node.Attrs, "style")
	if style != "" {
		styles := parseStyleAttr(style)
		if color, ok := styles["color"]; ok && color != "" {
			c, _ := editor.ParseColor(color)
			spoiler.Color = c
		}
		if bgColor, ok := styles["background-color"]; ok && bgColor != "" {
			c, _ := editor.ParseColor(bgColor)
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

// parseInfoBlock преобразует info-block TipTap в editor.InfoBlock.
func parseInfoBlock(node TipTapNode) *editor.InfoBlock {
	if node.Type != "info-block" {
		return nil
	}

	block := &editor.InfoBlock{
		Title:   getAttrString(node.Attrs, "title"),
		Content: make([]editor.Paragraph, 0),
	}

	// Парсить цвет иконки
	iconColor := getAttrString(node.Attrs, "iconColor")
	if iconColor != "" {
		c, _ := editor.ParseColor(iconColor)
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

// parseList преобразует список TipTap в editor.List.
func parseList(node TipTapNode) *editor.List {
	list := &editor.List{
		Elements: make([]editor.ListElement, 0),
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

// parseListItem преобразует элемент списка TipTap в editor.ListElement.
func parseListItem(node TipTapNode) *editor.ListElement {
	elem := &editor.ListElement{
		Content: make([]editor.Paragraph, 0),
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

// parseTable преобразует таблицу TipTap в editor.Table.
func parseTable(node TipTapNode) *editor.Table {
	if node.Type != "table" {
		return nil
	}

	table := &editor.Table{
		Rows: make([][]editor.TableCell, 0),
	}

	// Обработать строки таблицы
	for _, rowNode := range node.Content {
		if rowNode.Type != "tableRow" {
			continue
		}

		row := make([]editor.TableCell, 0)

		// Обработать ячейки в строке
		for _, cellNode := range rowNode.Content {
			if cellNode.Type != "tableHeader" && cellNode.Type != "tableCell" {
				continue
			}

			cell := editor.TableCell{
				Header:  cellNode.Type == "tableHeader",
				ColSpan: getAttrInt(cellNode.Attrs, "colspan"),
				RowSpan: getAttrInt(cellNode.Attrs, "rowspan"),
				Content: make([]editor.Paragraph, 0),
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
