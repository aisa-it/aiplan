// Пакет tiptap предоставляет инструменты для парсинга JSON-контента TipTap редактора.
// Преобразует JSON структуры TipTap в структуры данных пакета edtypes.
package tiptap

// TipTapDocument представляет корневой документ TipTap.
type TipTapDocument struct {
	Type    string       `json:"type"`
	Content []TipTapNode `json:"content,omitempty"`
}

// TipTapNode представляет узел в дереве документа TipTap.
// Используется универсальная структура с map для атрибутов для поддержки различных типов нод.
type TipTapNode struct {
	Type    string                 `json:"type"`
	Attrs   map[string]interface{} `json:"attrs,omitempty"`
	Content []TipTapNode           `json:"content,omitempty"`
	Marks   []TipTapMark           `json:"marks,omitempty"`
	Text    string                 `json:"text,omitempty"`
}

// TipTapMark представляет форматирование текста (bold, italic, link и т.д.).
type TipTapMark struct {
	Type  string                 `json:"type"`
	Attrs map[string]interface{} `json:"attrs,omitempty"`
}
