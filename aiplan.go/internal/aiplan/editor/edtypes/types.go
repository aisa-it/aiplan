package edtypes

import (
	"bytes"
	"database/sql/driver"
	"encoding/hex"
	"errors"
	"fmt"
	"image/color"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

type TextAlign int

const (
	LeftAlign TextAlign = iota
	CenterAlign
	RightAlign
)

var (
	colorReg = regexp.MustCompile(`[rgb()#\s"]`)
)

// TipTapParser - функция для парсинга TipTap JSON, устанавливается из tiptap пакета
var TipTapParser func(io.Reader) (*Document, error)

// TipTapSerializer - функция для сериализации Document в TipTap JSON, устанавливается из tiptap пакета
var TipTapSerializer func(*Document) ([]byte, error)

type Document struct {
	Elements []any
}

// UnmarshalJSON реализует кастомную десериализацию TipTap JSON в Document.
// Автоматически вызывает зарегистрированный TipTapParser.
func (d *Document) UnmarshalJSON(data []byte) error {
	if TipTapParser == nil {
		return errors.New("TipTapParser not registered, import tiptap package to enable TipTap JSON parsing")
	}

	// Вызываем парсер TipTap для получения документа
	doc, err := TipTapParser(bytes.NewReader(data))
	if err != nil {
		return err
	}

	d.Elements = doc.Elements
	return nil
}

// MarshalJSON реализует кастомную сериализацию Document в TipTap JSON.
// Автоматически вызывает зарегистрированный TipTapSerializer.
func (d *Document) MarshalJSON() ([]byte, error) {
	if TipTapSerializer == nil {
		return nil, errors.New("TipTapSerializer not registered, import tiptap package to enable TipTap JSON serialization")
	}

	return TipTapSerializer(d)
}

// Value реализует интерфейс driver.Valuer для сохранения Document в PostgreSQL JSONB.
// Использует существующий MarshalJSON который вызывает зарегистрированный TipTapSerializer.
func (d Document) Value() (driver.Value, error) {
	b, err := d.MarshalJSON()
	if err != nil {
		return nil, err
	}
	return b, nil
}

// Scan реализует интерфейс sql.Scanner для чтения Document из PostgreSQL JSONB.
// Использует существующий UnmarshalJSON который вызывает зарегистрированный TipTapParser.
func (d *Document) Scan(value interface{}) error {
	if value == nil {
		*d = Document{Elements: make([]any, 0)}
		return nil
	}

	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return errors.New(fmt.Sprint("Failed to unmarshal JSONB value:", value))
	}

	return d.UnmarshalJSON(bytes)
}

// GormDataType указывает GORM использовать тип JSONB для PostgreSQL колонок.
func (Document) GormDataType() string {
	return "jsonb"
}

type Paragraph struct {
	Content []any
	Indent  int       // для атрибута indent (0 = нет отступа)
	Align   TextAlign // для атрибута textAlign
}

type Text struct {
	Content string
	Size    int

	Strong        bool
	Italic        bool
	Underlined    bool
	Strikethrough bool
	Sup           bool
	Sub           bool

	Color   *Color
	BgColor *Color
	Align   TextAlign

	URL *url.URL
}

type ListElement struct {
	Content []Paragraph
	Checked bool
}

type List struct {
	Elements []ListElement
	Numbered bool
	TaskList bool
}

type Quote struct {
	Content []Paragraph
}

type Code struct {
	Content string
}

type Image struct {
	Src   *url.URL
	Width int
	Align TextAlign
}

type Table struct {
	MinWidth int
	ColWidth []int
	Rows     [][]TableCell
}

type TableCell struct {
	Content []Paragraph
	ColSpan int
	RowSpan int
	Header  bool
}

type Spoiler struct {
	Title     string
	Collapsed bool
	BgColor   Color
	Color     Color

	Content []Paragraph
}

type InfoBlock struct {
	Title string
	Color Color

	Content []Paragraph
}

type Color color.RGBA

func ParseColor(raw string) (Color, error) {
	isDecRGB := strings.Contains(raw, "rgb(")
	isHex := raw[0] == '#' || raw[1] == '#'
	raw = colorReg.ReplaceAllString(raw, "")
	if isDecRGB {
		c := Color{}
		for i, n := range strings.Split(raw, ",") {
			nn, err := strconv.ParseUint(n, 10, 8)
			if err != nil {
				return c, err
			}

			switch i {
			case 0:
				c.R = uint8(nn)
			case 1:
				c.G = uint8(nn)
			case 2:
				c.B = uint8(nn)
			case 3:
				c.A = uint8(nn)
			}
		}
		return c, nil
	} else if isHex {
		// HEX
		b, err := hex.DecodeString(raw)
		if err != nil {
			return Color{}, err
		}
		if len(b) < 3 {
			return Color{}, errors.New("unsupported color format")
		}
		c := Color{
			R: b[0],
			G: b[1],
			B: b[2],
		}
		if len(b) > 3 {
			c.A = b[3]
		}
		return c, nil
	}
	return Color{}, errors.New("unsupported color format")
}

func (c Color) MarshalJSON() ([]byte, error) {
	return fmt.Appendf(nil, "\"#%s\"", hex.EncodeToString([]byte{c.R, c.G, c.B, c.A})), nil
}

func (c *Color) UnmarshalJSON(data []byte) error {
	if string(data) == "null" || string(data) == `""` {
		return nil
	}

	cc, err := ParseColor(string(data))
	*c = cc

	return err
}

type DateNode struct {
	Date string
}

type IssueLinkMention struct {
	Slug              string
	ProjectIdentifier string
	CurrentIssueId    string
	OriginalUrl       string
}

type Mention struct {
	ID    string
	Label string
}

type HardBreak struct {
	// Пустая структура для представления переноса строки <br>
}
