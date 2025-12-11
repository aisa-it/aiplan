package editor

import (
	"encoding/hex"
	"errors"
	"fmt"
	"image/color"
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

type Document struct {
	Elements []any
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
