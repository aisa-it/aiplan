package tiptap

import (
	"encoding/json"
	"io"
	"log/slog"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/editor/edtypes"
)

func init() {
	// Регистрируем парсер и сериализатор TipTap в edtypes
	edtypes.TipTapParser = ParseJSON
	edtypes.TipTapSerializer = Serialize
}

// ParseJSON парсит JSON контент TipTap редактора в структуру edtypes.Document.
// Принимает io.Reader с JSON данными и возвращает распарсенный документ.
func ParseJSON(r io.Reader) (*edtypes.Document, error) {
	// Десериализовать JSON в TipTapDocument
	var tipTapDoc TipTapDocument
	if err := json.NewDecoder(r).Decode(&tipTapDoc); err != nil {
		return nil, err
	}

	// Создать результирующий документ
	doc := &edtypes.Document{
		Elements: make([]any, 0),
	}

	// Обработать каждую ноду верхнего уровня
	for _, node := range tipTapDoc.Content {
		elem := parseNode(node)
		if elem != nil {
			doc.Elements = append(doc.Elements, elem)
		}
	}

	return doc, nil
}

// parseNode парсит отдельную ноду TipTap и возвращает соответствующий элемент edtypes.
func parseNode(node TipTapNode) any {
	switch node.Type {
	case "paragraph":
		return parseParagraph(node)
	case "blockquote":
		return parseBlockquote(node)
	case "codeBlock":
		return parseCodeBlock(node)
	case "bulletList", "orderedList", "taskList":
		return parseList(node)
	case "table":
		return parseTable(node)
	case "spoiler":
		return parseSpoiler(node)
	case "info-block":
		return parseInfoBlock(node)
	case "image", "imageResize":
		return parseImage(node)
	case "date-node":
		return parseDateNode(node)
	case "issueLinkMention":
		return parseIssueLinkMention(node)
	case "mention":
		return parseMention(node)
	case "hardBreak":
		return parseHardBreak(node)
	default:
		slog.Debug("Unknown node type", "type", node.Type)
		return nil
	}
}
