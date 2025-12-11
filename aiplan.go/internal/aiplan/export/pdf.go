// Пакет для экспорта данных в PDF формат.
// Предоставляет функциональность для создания PDF документов из данных, полученных из различных источников, таких как модели данных и HTML описание.
//
// Основные возможности:
//   - Генерация PDF из данных Issue.
//   - Вставка HTML описания Issue в PDF.
//   - Добавление комментариев к Issue в PDF.
//   - Вставка изображений (логотип, приоритеты) в PDF.
//   - Создание таблиц из данных Issue.
//   - Поддержка стилизации текста (жирный, курсив, подчеркнутый).
package export

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	_ "embed"

	"codeberg.org/go-pdf/fpdf"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/editor"
	_ "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/editor/tiptap" // Регистрация TipTap парсера и сериализатора
)

var (
	//go:embed Rubik/static/Rubik-Regular.ttf
	regularFont []byte
	//go:embed Rubik/static/Rubik-Italic.ttf
	italicFont []byte
	//go:embed Rubik/static/Rubik-Bold.ttf
	boldFont []byte
	//go:embed Rubik/static/Rubik-BoldItalic.ttf
	boldItalicFont []byte

	//go:embed images/aiplan_logo.png
	aiplanLogo []byte
)

type pdfWriter struct {
	pdf      *fpdf.Fpdf
	issue    *dao.Issue
	comments []dao.IssueComment
	webURL   *url.URL

	defaultMargins Margins
}

type Margins struct {
	Left   float64
	Top    float64
	Right  float64
	Bottom float64
}

func (m *Margins) GetMargins(pdf fpdf.Pdf) {
	m.Left, m.Top, m.Right, m.Bottom = pdf.GetMargins()
}

func IssueToFPDF(issue *dao.Issue, webURL *url.URL, out io.Writer, comments ...dao.IssueComment) error {
	pdf := fpdf.New("P", "mm", "A4", "Rubik/static") // 210*297 mm

	w := pdfWriter{
		pdf:      pdf,
		webURL:   webURL,
		issue:    issue,
		comments: comments,
	}

	w.defaultMargins.GetMargins(w.pdf)

	pdf.AddUTF8FontFromBytes("Rubik", "", regularFont)
	pdf.AddUTF8FontFromBytes("Rubik", "I", italicFont)
	pdf.AddUTF8FontFromBytes("Rubik", "B", boldFont)
	pdf.AddUTF8FontFromBytes("Rubik", "BI", boldItalicFont)

	// Register embed images
	pdf.RegisterImageOptionsReader("logo.png", fpdf.ImageOptions{ImageType: "png"}, bytes.NewReader(aiplanLogo))
	pdf.RegisterImageOptionsReader("urgent", fpdf.ImageOptions{ImageType: "png"}, bytes.NewReader(urgentPriorityIMG))
	pdf.RegisterImageOptionsReader("high", fpdf.ImageOptions{ImageType: "png"}, bytes.NewReader(highPriorityIMG))
	pdf.RegisterImageOptionsReader("medium", fpdf.ImageOptions{ImageType: "png"}, bytes.NewReader(mediumPriorityIMG))
	pdf.RegisterImageOptionsReader("low", fpdf.ImageOptions{ImageType: "png"}, bytes.NewReader(lowPriorityIMG))

	pdf.SetHeaderFunc(func() {
		pdf.SetRightMargin(30)
		pdf.SetFont("Rubik", "B", 25)
		pdf.Write(10, issue.String()+"\n")
		pdf.SetFont("Rubik", "", 15)
		w.write(issue.Name)
		w.pdf.Ln(8)

		w.writeIssueInfoTable()

		pdf.ImageOptions("logo.png", 179, 5, 25, 25, false, fpdf.ImageOptions{ReadDpi: true}, 0, "")

		pdf.SetY(pdf.GetY() + 2)

		if pdf.GetY() < 30 {
			pdf.SetY(30)
		}
		pdf.Line(pdf.GetX(), pdf.GetY(), 200, pdf.GetY())
		pdf.SetY(pdf.GetY() + 5)
		l, _, _, _ := pdf.GetMargins()
		pdf.SetRightMargin(l)
	})

	pdf.AddPage()
	pdf.Bookmark("Описание", 0, -1)

	doc, err := editor.ParseDocument(strings.NewReader(w.issue.DescriptionHtml))
	if err != nil {
		return err
	}

	w.writeDescription(doc)

	if len(comments) > 0 {
		pdf.AddPage()
		pdf.Bookmark("Комментарии", 0, -1)
		w.writeComments()
	}

	return pdf.Output(out)
}

func (w *pdfWriter) writeDescription(doc *editor.Document) error {
	for _, rawElement := range doc.Elements {
		switch el := rawElement.(type) {
		case editor.Paragraph:
			w.writeParagraph(el)
		case editor.Quote:
			w.pdf.Ln(2)
			y1 := w.pdf.GetY()
			w.pdf.SetLeftMargin(13)
			for _, p := range el.Content {
				w.writeParagraph(p)
			}
			w.pdf.SetLeftMargin(10)

			w.pdf.SetLineWidth(0.5)
			w.pdf.SetDrawColor(74, 71, 82)
			w.pdf.Line(11, y1, 11, w.pdf.GetY())
			w.pdf.Ln(2)
		case editor.List:
			w.pdf.SetLeftMargin(13)
			for i, e := range el.Elements {
				if el.Numbered {
					w.write(fmt.Sprintf("%d.", i+1))
				} else {
					w.write("•")
				}

				for _, p := range e.Content {
					w.pdf.SetX(17)
					w.writeParagraph(p)
				}
			}
			w.pdf.SetLeftMargin(10)
		case editor.Table:
			w.writeEditorTable(el)
		}
		w.resetMargins()
	}

	return nil
}

func (w *pdfWriter) writeParagraph(p editor.Paragraph) {
	for _, t := range p.Content {
		switch tt := t.(type) {
		case editor.Text:
			w.writeEditorText(tt)
		case *editor.Image:
			w.writeEditorImage(tt)
		}
	}
	w.pdf.Ln(-1)
}

func (w *pdfWriter) writeEditorText(t editor.Text) float64 {
	w.prepareEditorText(&t)
	_, s := w.pdf.GetFontSize()

	if t.BgColor != nil {
		x := w.pdf.GetX()
		w.pdf.SetX(x + w.pdf.GetCellMargin())
		w.pdf.CellFormat(w.pdf.GetStringWidth(t.Content), s+0.1, "", "", 0, "L", true, 0, "")
		w.pdf.SetX(x)
	}

	if t.URL != nil {
		return w.write(t.Content, t.URL.String())
	} else {
		return w.write(t.Content)
	}
}

func (w *pdfWriter) prepareEditorText(t *editor.Text) {
	t.Content = cleanUnsupportedSymbols(t.Content)

	styleStr := ""
	if t.Strong {
		styleStr += "B"
	}
	if t.Italic {
		styleStr += "I"
	}
	if t.Strikethrough {
		styleStr += "S"
	}
	if t.Underlined {
		styleStr += "U"
	}
	if t.Size == 0 {
		t.Size = 14
	}
	w.pdf.SetFont("Rubik", styleStr, w.PxToUnit(t.Size)*3)

	if t.Color != nil {
		w.pdf.SetTextColor(int(t.Color.R), int(t.Color.G), int(t.Color.B))
	} else {
		w.pdf.SetTextColor(0, 0, 0)
	}

	if t.BgColor != nil {
		w.pdf.SetFillColor(int(t.BgColor.R), int(t.BgColor.G), int(t.BgColor.B))
	}
}

func (w *pdfWriter) calcEditorText(t *editor.Text) float64 {
	w.prepareEditorText(t)
	return w.pdf.GetStringWidth(t.Content)
}

func (w *pdfWriter) write(text string, link ...string) float64 {
	_, s := w.pdf.GetFontSize()
	s += 0.1
	if len(link) > 0 {
		w.pdf.WriteLinkString(s, text, link[0])
		return 0
	}
	w.pdf.WriteLinkString(s, text, "")
	return w.pdf.GetStringWidth(text)
}

func cleanUnsupportedSymbols(text string) string {
	result := ""
	for _, s := range text {
		if s < 65536 {
			result += string(s)
		}
	}
	return result
}

func (w *pdfWriter) getEditorImageInfo(img *editor.Image) *fpdf.ImageInfoType {
	info := w.pdf.GetImageInfo(img.Src.Path)
	if info == nil {
		u := img.Src
		if img.Src.Host == "" {
			u = w.webURL.ResolveReference(u)
		}

		resp, err := http.Get(u.String())
		if err != nil {
			fmt.Println(err)
			return nil
		}
		defer resp.Body.Close()

		options := fpdf.ImageOptions{ImageType: w.pdf.ImageTypeFromMime(resp.Header.Get("Content-Type")), ReadDpi: true}

		// unsupported image type
		if options.ImageType == "" {
			w.pdf.ClearError()
			return nil
		}

		info = w.pdf.RegisterImageOptionsReader(
			img.Src.Path,
			options,
			resp.Body)
	}
	return info
}

func (w *pdfWriter) writeEditorImage(img *editor.Image) {
	u := img.Src
	if img.Src.Host == "" {
		u = w.webURL.ResolveReference(u)
	}

	w.getEditorImageInfo(img)

	maxX, _ := w.pdf.GetPageSize()
	left, _, _, _ := w.pdf.GetMargins()
	maxWidth := maxX - left - w.pdf.GetX()

	width := min(w.PxToUnit(img.Width), maxWidth)
	w.pdf.ImageOptions(img.Src.Path, -1, -1, width, 0, true, fpdf.ImageOptions{ReadDpi: true}, 0, u.String())
}

func (w *pdfWriter) writeEditorTable(table editor.Table) {
	const heightOffset = 2

	sizes := struct {
		colWidth  []float64
		rowHeight []float64
	}{
		colWidth:  w.getTableWidthUnits(table),
		rowHeight: make([]float64, len(table.Rows)),
	}

	for i, row := range table.Rows {
		for j, cell := range row {
			height := 0.0
			for _, p := range cell.Content {
				pHeight := 0.0
				contentWidth := 0.0
				for _, t := range p.Content {
					switch tt := t.(type) {
					case editor.Text:
						contentWidth += w.calcEditorText(&tt)
					case *editor.Image:
						info := w.getEditorImageInfo(tt)
						pHeight = (sizes.colWidth[j] - w.pdf.GetCellMargin()*2.0) * info.Height() / info.Width()
					}
				}

				lines := 1
				if contentWidth > sizes.colWidth[j] {
					lines = int(math.Ceil((contentWidth) / sizes.colWidth[j]))
				}
				_, fz := w.pdf.GetFontSize()
				height += fz*float64(lines) + pHeight
			}
			sizes.rowHeight[i] = max(sizes.rowHeight[i], height)
		}
	}

	for i, row := range table.Rows {
		for j, cell := range row {
			x, y := w.pdf.GetXY()

			w.SetHexFillColor("#e5edfa")
			w.pdf.CellFormat(sizes.colWidth[j], sizes.rowHeight[i]+heightOffset, "", "1", 0, "LM", cell.Header, 0, "")

			x1, y1 := w.pdf.GetXY()
			w.pdf.SetXY(x, y+heightOffset/2)

			l, _, r, _ := w.pdf.GetMargins()
			pW, _ := w.pdf.GetPageSize()
			w.pdf.SetRightMargin(pW - (w.pdf.GetX() + sizes.colWidth[j]))
			w.pdf.SetLeftMargin(w.pdf.GetX())

			for pI, p := range cell.Content {
				for _, t := range p.Content {
					switch tt := t.(type) {
					case editor.Text:
						if cell.Header {
							tt.Strong = true
						}
						w.writeEditorText(tt)
					case *editor.Image:
						u := tt.Src
						if tt.Src.Host == "" {
							u = w.webURL.ResolveReference(u)
						}
						w.getEditorImageInfo(tt)
						w.pdf.ImageOptions(tt.Src.Path, w.pdf.GetX()+w.pdf.GetCellMargin(), -1, sizes.colWidth[j]-w.pdf.GetCellMargin()*2.0, 0, true, fpdf.ImageOptions{ReadDpi: true}, 0, u.String())
					}
				}
				if pI != len(cell.Content)-1 {
					w.pdf.Ln(-1)
					w.pdf.SetX(x)
				}
			}
			w.pdf.SetRightMargin(r)
			w.pdf.SetLeftMargin(l)
			w.pdf.SetXY(x1, y1)
		}
		w.pdf.Ln(sizes.rowHeight[i] + heightOffset)
	}

	_, fz := w.pdf.GetFontSize()
	w.pdf.Ln(fz)
}

func (w *pdfWriter) PxToUnit(px int) float64 {
	return w.pdf.PointConvert(float64(px) * 0.75)
}

func (w *pdfWriter) SetHexFillColor(hex string) {
	hex = strings.TrimPrefix(hex, "#")
	values, err := strconv.ParseUint(string(hex), 16, 32)
	if err != nil {
		return
	}
	w.pdf.SetFillColor(
		int(uint8(values>>16)),
		int(uint8((values>>8)&0xFF)),
		int(uint8(values&0xFF)),
	)
}

func (w *pdfWriter) getTableWidthUnits(t editor.Table) []float64 {
	sum := 0
	autoColCount := 0
	for _, s := range t.ColWidth {
		sum += s
		if s == 0 {
			autoColCount++
		}
	}

	freeColSize := float64(t.MinWidth-sum) / float64(autoColCount)

	l, _, r, _ := w.pdf.GetMargins()
	pW, _ := w.pdf.GetPageSize()
	width := pW - l - r

	res := make([]float64, len(t.Rows[0]))
	for i, s := range t.ColWidth {
		if s == 0 {
			s = int(freeColSize)
		}
		res[i] = width / float64(t.MinWidth) * float64(s)
	}

	return res
}

func (w *pdfWriter) resetMargins() {
	w.pdf.SetMargins(w.defaultMargins.Left, w.defaultMargins.Top, w.defaultMargins.Right)
}
