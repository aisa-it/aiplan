package editor

import (
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/editor/edtypes"
)

// Реэкспорт всех типов из edtypes для обратной совместимости
type (
	TextAlign        = edtypes.TextAlign
	Document         = edtypes.Document
	Paragraph        = edtypes.Paragraph
	Text             = edtypes.Text
	Code             = edtypes.Code
	ListElement      = edtypes.ListElement
	List             = edtypes.List
	Quote            = edtypes.Quote
	Image            = edtypes.Image
	Table            = edtypes.Table
	TableCell        = edtypes.TableCell
	Spoiler          = edtypes.Spoiler
	InfoBlock        = edtypes.InfoBlock
	Color            = edtypes.Color
	DateNode         = edtypes.DateNode
	IssueLinkMention = edtypes.IssueLinkMention
	Mention          = edtypes.Mention
	HardBreak        = edtypes.HardBreak
)

// Реэкспорт констант
const (
	LeftAlign   = edtypes.LeftAlign
	CenterAlign = edtypes.CenterAlign
	RightAlign  = edtypes.RightAlign
)

// Реэкспорт функций
var (
	ParseColor = edtypes.ParseColor
)
