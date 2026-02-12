package tiptap

import (
	"log/slog"
	"net/url"
	"strconv"
	"strings"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/editor/edtypes"
)

// applyMarks применяет форматирование (marks) к текстовому элементу.
func applyMarks(text *edtypes.Text, marks []TipTapMark) {
	for _, mark := range marks {
		switch mark.Type {
		case "bold":
			text.Strong = true
		case "italic":
			text.Italic = true
		case "underline":
			text.Underlined = true
		case "strike":
			text.Strikethrough = true
		case "superscript":
			text.Sup = true
		case "subscript":
			text.Sub = true
		case "textStyle":
			applyTextStyle(text, mark.Attrs)
		case "link":
			applyLink(text, mark.Attrs)
		case "highlight":
			applyHighlight(text, mark.Attrs)
		default:
			slog.Debug("Unknown mark type", "type", mark.Type)
		}
	}
}

// applyTextStyle применяет стили текста (цвет, размер шрифта).
func applyTextStyle(text *edtypes.Text, attrs map[string]interface{}) {
	// Цвет текста
	if color := getAttrString(attrs, "color"); color != "" {
		c, err := edtypes.ParseColor(color)
		if err == nil {
			text.Color = &c
		}
	}

	// Размер шрифта
	if fontSize := getAttrString(attrs, "fontSize"); fontSize != "" {
		size, err := strconv.Atoi(strings.TrimSuffix(fontSize, "px"))
		if err == nil {
			text.Size = size
		}
	}
}

// applyLink применяет ссылку к тексту.
func applyLink(text *edtypes.Text, attrs map[string]interface{}) {
	href := getAttrString(attrs, "href")
	if href != "" {
		u, err := url.Parse(href)
		if err == nil {
			text.URL = u
		}
	}
}

// applyHighlight применяет подсветку фона к тексту.
func applyHighlight(text *edtypes.Text, attrs map[string]interface{}) {
	color := getAttrString(attrs, "color")
	if color != "" {
		c, err := edtypes.ParseColor(color)
		if err == nil {
			text.BgColor = &c
		}
	}
}
