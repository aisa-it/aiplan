// Пакет предоставляет инструменты для парсинга HTML-документов и извлечения информации о структуре и содержимом.
// Он предназначен для работы с различными элементами, такими как параграфы, списки, таблицы и другие, а также для извлечения стилей и атрибутов.
//
// Основные возможности:
//   - Парсинг HTML-документов из io.Reader.
//   - Извлечение текста, стилей, атрибутов и других данных из HTML-элементов.
//   - Поддержка различных типов элементов, включая параграфы, списки, таблицы, изображения и ссылки.
//   - Предоставление удобных типов данных для представления различных элементов и атрибутов HTML.
package editor

import (
	"io"
	"log/slog"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

func ParseDocument(r io.Reader) (*Document, error) {
	rootNode, err := html.Parse(r)
	if err != nil {
		return nil, err
	}

	var document Document

	for el := getBody(rootNode).FirstChild; el != nil; el = el.NextSibling {
		if el.Type != html.ElementNode {
			continue
		}

		switch el.Data {
		case "pre":
			document.Elements = append(document.Elements, parseCode(el))
		case "p":
			p := parseParagraph(el)
			if p != nil {
				document.Elements = append(document.Elements, *p)
			}
		case "ul", "ol":
			list := parseList(el)
			if list != nil {
				document.Elements = append(document.Elements, *list)
			}
		case "blockquote":
			var quote Quote
			iterNodes(el, func(child *html.Node) bool {
				if p := parseParagraph(child); p != nil {
					quote.Content = append(quote.Content, *p)
					return true
				}
				return false
			})
			document.Elements = append(document.Elements, quote)
		case "table":
			t := parseTable(el)
			if t != nil {
				document.Elements = append(document.Elements, *t)
			}
		case "div":
			if attrExists("data-spoiler", el.Attr) {
				document.Elements = append(document.Elements, parseSpoiler(el))
			} else if attrExists("data-info-block", el.Attr) {
				document.Elements = append(document.Elements, parseInfoBlock(el))
			}
		}
	}

	return &document, nil
}

func parseParagraph(root *html.Node) *Paragraph {
	if root.Type != html.ElementNode || root.Data != "p" {
		return nil
	}
	var p Paragraph

	for el := root.FirstChild; el != nil; el = el.NextSibling {
		// Обработка <br> тегов для переноса строки
		if el.Type == html.ElementNode && el.Data == "br" {
			p.Content = append(p.Content, &HardBreak{})
			continue
		}

		image := getImage(el)
		if image != nil {
			p.Content = append(p.Content, image)
		} else {
			p.Content = append(p.Content, getText(el))
		}
	}

	return &p
}

func parseList(root *html.Node) *List {
	var list List
	if root.Type != html.ElementNode || (root.Data != "ul" && root.Data != "ol") {
		return nil
	}
	list.Numbered = root.Data == "ol"
	list.TaskList = getAttrValue("data-type", root.Attr) == "taskList"

	for li := root.FirstChild; li != nil; li = li.NextSibling {
		if le := parseListElement(li); le != nil {
			list.Elements = append(list.Elements, *le)
		}
	}

	return &list
}

func parseListElement(li *html.Node) *ListElement {
	if li.Type != html.ElementNode || li.Data != "li" {
		return nil
	}

	var listElement ListElement

	listElement.Checked = getAttrValue("data-checked", li.Attr) == "true"

	iterNodes(li, func(p *html.Node) bool {
		paragraph := parseParagraph(p)
		if paragraph != nil {
			listElement.Content = append(listElement.Content, *paragraph)
			return true
		}
		return false
	})
	return &listElement
}

func parseCode(root *html.Node) Code {
	var text string
	iterNodes(root, func(child *html.Node) bool {
		if child.Type != html.TextNode {
			return false
		}
		text += child.Data
		return false
	})
	return Code{text}
}

func getText(root *html.Node) Text {
	var text Text

	iterNodes(root, func(el *html.Node) bool {
		if el.Type == html.TextNode {
			text.Content = el.Data
			return true
		}
		switch el.Data {
		case "em":
			text.Italic = true
		case "u":
			text.Underlined = true
		case "s":
			text.Strikethrough = true
		case "strong":
			text.Strong = true
		case "sub":
			text.Sub = true
		case "sup":
			text.Sup = true
		case "span", "mark":
			parseTextStyles(el, &text)
		case "a":
			if u, err := url.Parse(getAttrValue("href", el.Attr)); err == nil {
				text.URL = u
			}
		}

		return false
	})

	return text
}

func parseTextStyles(node *html.Node, text *Text) {
	for _, attr := range node.Attr {
		if attr.Key == "style" {
			for styleRaw := range strings.SplitSeq(attr.Val, ";") {
				arr := strings.Split(styleRaw, ":")
				style := html.Attribute{
					Key: strings.TrimSpace(arr[0]),
					Val: strings.TrimSpace(arr[1]),
				}

				if style.Val == "inherit" {
					continue
				}

				switch style.Key {
				case "font-size":
					size, err := strconv.Atoi(strings.TrimSuffix(style.Val, "px"))
					if err == nil {
						text.Size = size
					} else {
						slog.Error("Parse font size", "input", style.Val, "err", err)
					}
				case "color":
					rgb, _ := ParseColor(style.Val)
					text.Color = &rgb
				case "background-color":
					rgb, _ := ParseColor(style.Val)
					text.BgColor = &rgb
				case "text-align":
					text.Align = toTextAlign(style.Val)
				}
			}
		}
	}
}

func parseSpoiler(root *html.Node) Spoiler {
	spoiler := Spoiler{
		Title:     getAttrValue("data-title", root.Attr),
		Collapsed: getAttrValue("data-collapsed", root.Attr) == "true",
	}

	for _, style := range parseStyles(strings.Split(getAttrValue("style", root.Attr), ";")) {
		switch style.Key {
		case "color":
			spoiler.Color, _ = ParseColor(style.Val)
		case "background-color":
			spoiler.BgColor, _ = ParseColor(style.Val)
		}
	}

	iterNodes(root, func(child *html.Node) bool {
		if p := parseParagraph(child); p != nil {
			spoiler.Content = append(spoiler.Content, *p)
			return true
		}
		return false
	})

	return spoiler
}

func parseInfoBlock(root *html.Node) InfoBlock {
	block := InfoBlock{
		Title: getAttrValue("data-title", root.Attr),
	}

	block.Color, _ = ParseColor(getAttrValue("data-icon-color", root.Attr))

	iterNodes(root, func(child *html.Node) bool {
		if p := parseParagraph(child); p != nil {
			block.Content = append(block.Content, *p)
			return true
		}
		return false
	})

	return block
}

func findElementByTagName(rootNode *html.Node, tagName string) *html.Node {
	var el *html.Node
	iterNodes(rootNode, func(child *html.Node) bool {
		if child.Type == html.ElementNode && child.Data == tagName {
			el = child
			return true
		}
		return false
	})
	return el
}

func getBody(rootNode *html.Node) *html.Node {
	return findElementByTagName(rootNode, "body")
}

func iterNodes(node *html.Node, f func(child *html.Node) bool) {
	if f(node) {
		return
	}
	for p := node.FirstChild; p != nil; p = p.NextSibling {
		iterNodes(p, f)
	}
}

func getAttrValue(key string, attrs []html.Attribute) string {
	for _, attr := range attrs {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

func attrExists(key string, attrs []html.Attribute) bool {
	return slices.ContainsFunc(attrs, func(attr html.Attribute) bool {
		return attr.Key == key
	})
}

func getImage(el *html.Node) *Image {
	if el.Type != html.ElementNode || el.Data != "img" {
		return nil
	}

	i := &Image{}

	imgUrl, err := url.Parse(getAttrValue("src", el.Attr))
	if err != nil {
		return nil
	}
	i.Src = imgUrl

	for _, styleRaw := range strings.Split(getAttrValue("style", el.Attr), ";") {
		if !strings.Contains(styleRaw, ":") {
			continue
		}
		arr := strings.Split(styleRaw, ":")
		style := html.Attribute{
			Key: strings.TrimSpace(arr[0]),
			Val: strings.TrimSpace(arr[1]),
		}

		switch style.Key {
		case "width":
			i.Width, _ = strconv.Atoi(strings.TrimSuffix(style.Val, "px"))
		case "float":
			switch style.Val {
			case "left":
				i.Align = LeftAlign
			case "":
				i.Align = CenterAlign
			case "right":
				i.Align = RightAlign
			}
		}
	}

	return i
}

func parseTable(root *html.Node) *Table {
	table := new(Table)

	for _, style := range parseStyles(strings.Split(getAttrValue("style", root.Attr), ";")) {
		if style.Key == "min-width" || style.Key == "width" {
			table.MinWidth, _ = strconv.Atoi(strings.TrimSuffix(style.Val, "px"))
		}
	}

	table.ColWidth = parseColGroup(findElementByTagName(root, "colgroup"))

	tbody := findElementByTagName(root, "tbody")
	for tr := tbody.FirstChild; tr != nil; tr = tr.NextSibling {
		if tr.Type != html.ElementNode {
			continue
		}
		var row []TableCell

		for td := tr.FirstChild; td != nil; td = td.NextSibling {
			if td.Type != html.ElementNode {
				continue
			}

			var cell TableCell

			cell.ColSpan, _ = strconv.Atoi(getAttrValue("colspan", td.Attr))
			cell.RowSpan, _ = strconv.Atoi(getAttrValue("rowspan", td.Attr))
			cell.Header = td.Data == "th"

			for p := td.FirstChild; p != nil; p = p.NextSibling {
				if p := parseParagraph(p); p != nil {
					cell.Content = append(cell.Content, *p)
				}
			}

			row = append(row, cell)
		}

		table.Rows = append(table.Rows, row)

	}

	return table
}

func parseColGroup(root *html.Node) []int {
	var res []int
	iterNodes(root, func(child *html.Node) bool {
		if child.Type != html.ElementNode || child.Data != "col" {
			return false
		}
		stylesRaw := strings.Split(getAttrValue("style", child.Attr), ";")
		for _, style := range parseStyles(stylesRaw) {
			if style.Key == "width" {
				res = append(res, sizeToInt(style.Val))
				return false
			}
		}
		res = append(res, 0)

		return false
	})
	return res
}

func toTextAlign(raw string) TextAlign {
	switch strings.TrimSpace(raw) {
	case "left":
		return LeftAlign
	case "center":
		return CenterAlign
	case "right":
		return RightAlign
	}
	return LeftAlign
}

func parseStyles(rawStyles []string) []html.Attribute {
	res := make([]html.Attribute, len(rawStyles))
	for i, styleRaw := range rawStyles {
		arr := strings.Split(styleRaw, ":")
		if len(arr) < 2 {
			continue
		}
		style := html.Attribute{
			Key: strings.TrimSpace(arr[0]),
			Val: strings.TrimSpace(arr[1]),
		}
		res[i] = style
	}
	return res
}

func sizeToInt(raw string) int {
	i, _ := strconv.Atoi(strings.TrimSuffix(raw, "px"))
	return i
}
