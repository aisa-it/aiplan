// Package export генерирует PDF-документы из задач AIPlan.
//
// Использует библиотеку fpdf для создания PDF с поддержкой Unicode (шрифт Rubik).
// Парсит описание задачи из TipTap JSON или HTML и рендерит все элементы:
//   - Параграфы с форматированием (жирный, курсив, цвет, размер)
//   - Списки (нумерованные, маркированные, чек-листы)
//   - Таблицы с автоподбором ширины колонок
//   - Изображения (загружаются по URL, масштабируются под страницу)
//   - Блоки кода, спойлеры, информационные блоки
//   - Кастомные элементы TipTap: даты, упоминания, ссылки на задачи
//
// PDF включает: заголовок задачи, метаинформацию, описание и комментарии.
package export

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

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

	// Использовать DescriptionJSON если он не пустой (содержит TipTap специальные типы),
	// иначе парсить DescriptionHtml (для обратной совместимости)
	var doc *editor.Document
	if len(w.issue.DescriptionJSON.Elements) > 0 {
		// Использовать TipTap JSON документ напрямую
		doc = &w.issue.DescriptionJSON
	} else {
		// Fallback на HTML парсер
		var err error
		doc, err = editor.ParseDocument(strings.NewReader(w.issue.DescriptionHtml))
		if err != nil {
			return err
		}
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
		// Значения (из HTML парсера)
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
		case editor.Code:
			w.writeCodeBlock(el)
		case editor.InfoBlock:
			w.writeInfoBlock(el)
		case editor.Spoiler:
			w.writeSpoiler(el)

		// Указатели (из TipTap парсера)
		case *editor.Paragraph:
			w.writeParagraph(*el)
		case *editor.Quote:
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
		case *editor.List:
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
		case *editor.Table:
			w.writeEditorTable(*el)
		case *editor.Code:
			w.writeCodeBlock(*el)
		case *editor.InfoBlock:
			w.writeInfoBlock(*el)
		case *editor.Spoiler:
			w.writeSpoiler(*el)
		}
		w.resetMargins()
	}

	return nil
}

func (w *pdfWriter) writeParagraph(p editor.Paragraph) {
	afterHardBreak := false
	for _, t := range p.Content {
		switch tt := t.(type) {
		case editor.Text:
			// Обрезать начальные пробелы после HardBreak для правильного выравнивания
			if afterHardBreak {
				tt.Content = strings.TrimLeft(tt.Content, " \t")
				afterHardBreak = false
			}
			w.writeEditorText(tt)
		case *editor.Image:
			w.writeEditorImage(tt)
			afterHardBreak = false
		case *editor.HardBreak:
			w.writeHardBreak()
			afterHardBreak = true
		case *editor.DateNode:
			w.writeDateNode(tt)
			afterHardBreak = false
		case *editor.Mention:
			w.writeMention(tt)
			afterHardBreak = false
		case *editor.IssueLinkMention:
			w.writeIssueLinkMention(tt)
			afterHardBreak = false
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

		// Записать текст сразу с фоном
		if t.URL != nil {
			w.pdf.CellFormat(w.pdf.GetStringWidth(t.Content), s+0.1, t.Content, "", 0, "LM", true, 0, t.URL.String())
		} else {
			w.pdf.CellFormat(w.pdf.GetStringWidth(t.Content), s+0.1, t.Content, "", 0, "LM", true, 0, "")
		}

		// Сбросить фон (установить белый)
		w.pdf.SetFillColor(255, 255, 255)
		return w.pdf.GetStringWidth(t.Content)
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
		if img.Src.Host == "" && img.Src.Scheme != "file" {
			u = w.webURL.ResolveReference(u)
		}

		// Обработка file:// схемы для локальных файлов
		if u.Scheme == "file" {
			file, err := os.Open(u.Path)
			if err != nil {
				fmt.Println(err)
				return nil
			}
			defer file.Close()

			// Определить тип изображения по расширению
			var imageType string
			if strings.HasSuffix(strings.ToLower(u.Path), ".png") {
				imageType = "png"
			} else if strings.HasSuffix(strings.ToLower(u.Path), ".jpg") || strings.HasSuffix(strings.ToLower(u.Path), ".jpeg") {
				imageType = "jpeg"
			} else if strings.HasSuffix(strings.ToLower(u.Path), ".gif") {
				imageType = "gif"
			}

			options := fpdf.ImageOptions{ImageType: imageType, ReadDpi: true}
			if options.ImageType == "" {
				w.pdf.ClearError()
				return nil
			}

			info = w.pdf.RegisterImageOptionsReader(img.Src.Path, options, file)
			return info
		}

		// Загрузка через HTTP/HTTPS
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

	// Попытка загрузить информацию об изображении
	info := w.getEditorImageInfo(img)
	if info == nil {
		// Изображение не удалось загрузить, пропускаем
		return
	}

	// Изображения требуют новой строки
	w.pdf.Ln(-1)

	// Вычислить максимальную ширину для изображения
	maxX, _ := w.pdf.GetPageSize()
	left, _, right, _ := w.pdf.GetMargins()
	maxWidth := maxX - left - right

	// Использовать указанную ширину или подогнать под размер страницы
	width := w.PxToUnit(img.Width)
	if width <= 0 || width > maxWidth {
		width = maxWidth
	}

	// Вставить изображение с явной позицией X (текущий X курсора)
	x := w.pdf.GetX()
	w.pdf.ImageOptions(img.Src.Path, x, -1, width, 0, true, fpdf.ImageOptions{ReadDpi: true}, 0, u.String())

	// Перейти на новую строку после изображения
	w.pdf.Ln(-1)
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
	// Определить количество колонок
	colCount := len(t.Rows[0])

	l, _, r, _ := w.pdf.GetMargins()
	pW, _ := w.pdf.GetPageSize()
	width := pW - l - r

	// Если ColWidth пустой или не задан (TipTap парсер не заполняет его),
	// распределить ширину равномерно между всеми колонками
	if len(t.ColWidth) == 0 || t.MinWidth == 0 {
		equalWidth := width / float64(colCount)
		res := make([]float64, colCount)
		for i := range res {
			res[i] = equalWidth
		}
		return res
	}

	// Логика для таблиц с заданными ширинами (из HTML парсера)
	sum := 0
	autoColCount := 0
	for _, s := range t.ColWidth {
		sum += s
		if s == 0 {
			autoColCount++
		}
	}

	freeColSize := float64(t.MinWidth-sum) / float64(autoColCount)

	res := make([]float64, colCount)
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

// formatDate форматирует дату из ISO в русский формат
func formatDate(isoDate string) string {
	t, err := time.Parse("2006-01-02", isoDate)
	if err != nil {
		return isoDate
	}
	return t.Format("02.01.2006")
}

// lightenColor создаёт более светлый оттенок цвета для фона
func lightenColor(c editor.Color, percent float64) (r, g, b int) {
	r = int(float64(c.R) + (255-float64(c.R))*percent)
	g = int(float64(c.G) + (255-float64(c.G))*percent)
	b = int(float64(c.B) + (255-float64(c.B))*percent)
	return
}

// getMaxContentWidth возвращает максимальную ширину контента
func (w *pdfWriter) getMaxContentWidth() float64 {
	pageWidth, _ := w.pdf.GetPageSize()
	leftMargin, _, rightMargin, _ := w.pdf.GetMargins()
	return pageWidth - leftMargin - rightMargin
}

// writeHardBreak записывает явный перенос строки в PDF
func (w *pdfWriter) writeHardBreak() {
	w.pdf.Ln(-1)
	// Сбросить X на левый margin после переноса строки
	leftMargin, _, _, _ := w.pdf.GetMargins()
	w.pdf.SetX(leftMargin)
}

// writeDateNode записывает дату внутри параграфа
func (w *pdfWriter) writeDateNode(dn *editor.DateNode) {
	// Добавить небольшой padding через пробелы
	text := " " + formatDate(dn.Date) + " "

	// Установить цвета
	w.pdf.SetFillColor(204, 204, 204) // #cccccc фон
	w.pdf.SetTextColor(0, 0, 0)       // Чёрный текст

	// Использовать CellFormat для одновременной отрисовки текста И фона
	_, fontSize := w.pdf.GetFontSize()
	w.pdf.CellFormat(
		w.pdf.GetStringWidth(text), // ширина = точная ширина текста
		fontSize+0.1,               // высота
		text,                       // текст для отображения
		"",                         // без рамки
		0,                          // ln=0 (продолжить на той же строке)
		"C",                        // выравнивание по центру
		true,                       // fill=true (использовать цвет фона)
		0,                          // ссылка
		"",                         // URL ссылки
	)

	// Сбросить цвета
	w.pdf.SetFillColor(255, 255, 255)
}

// writeMention записывает упоминание пользователя
func (w *pdfWriter) writeMention(m *editor.Mention) {
	// Добавить небольшой padding через пробелы
	text := " @" + m.Label + " "

	// Установить цвета
	w.pdf.SetFillColor(189, 189, 189) // #bdbdbd фон
	w.pdf.SetTextColor(71, 74, 82)    // #474a52 текст

	// Использовать CellFormat для одновременной отрисовки текста И фона
	_, fontSize := w.pdf.GetFontSize()
	w.pdf.CellFormat(
		w.pdf.GetStringWidth(text), // ширина = точная ширина текста
		fontSize+0.1,               // высота
		text,                       // текст для отображения
		"",                         // без рамки
		0,                          // ln=0 (продолжить на той же строке)
		"C",                        // выравнивание по центру
		true,                       // fill=true (использовать цвет фона)
		0,                          // ссылка
		"",                         // URL ссылки
	)

	// Сбросить цвета
	w.pdf.SetTextColor(0, 0, 0)
	w.pdf.SetFillColor(255, 255, 255)
}

// writeIssueLinkMention записывает ссылку на задачу
func (w *pdfWriter) writeIssueLinkMention(ilm *editor.IssueLinkMention) {
	// Сформировать текст ссылки
	text := ilm.ProjectIdentifier + "-" + ilm.Slug

	// Установить синий цвет и подчёркивание
	w.pdf.SetFont("Rubik", "U", 12) // U = underline
	w.pdf.SetTextColor(0, 102, 204) // Синий #0066cc

	// Определить URL
	linkURL := ilm.OriginalUrl
	if linkURL == "" {
		// Построить URL если не указан
		u := w.webURL.ResolveReference(&url.URL{Path: "/issues/" + text})
		linkURL = u.String()
	}

	// Записать ссылку
	w.write(text, linkURL)

	// Восстановить стиль и цвет
	w.pdf.SetFont("Rubik", "", 12)
	w.pdf.SetTextColor(0, 0, 0)
}

// writeCodeBlock записывает блок кода в PDF
func (w *pdfWriter) writeCodeBlock(code editor.Code) {
	w.pdf.Ln(2)

	// Установить моноширный шрифт для кода
	w.pdf.SetFont("Courier", "", 9)

	// Разбить код на строки
	lines := strings.Split(code.Content, "\n")

	maxWidth := w.getMaxContentWidth()
	lineHeight := 4.0
	paddingV := 2.0
	paddingH := 2.0

	// Вычислить высоту блока
	blockHeight := float64(len(lines))*lineHeight + paddingV*2

	x, y := w.pdf.GetXY()

	// Нарисовать фон и рамку (цвет из TipTap конфигурации)
	w.SetHexFillColor("#efeff6")
	w.pdf.SetDrawColor(204, 204, 204)
	w.pdf.SetLineWidth(0.3)
	w.pdf.Rect(x, y, maxWidth, blockHeight, "FD") // F=fill, D=draw border

	// Сместиться для padding
	w.pdf.SetXY(x+paddingH, y+paddingV)

	// Записать строки кода
	for _, line := range lines {
		w.pdf.SetX(x + paddingH)
		w.write(cleanUnsupportedSymbols(line))
		w.pdf.Ln(lineHeight)
	}

	// Восстановить позицию после блока
	w.pdf.SetXY(x, y+blockHeight)

	// Восстановить шрифт к стандартному
	w.pdf.SetFont("Rubik", "", 12)
	w.pdf.SetDrawColor(0, 0, 0) // Сброс цвета рамки

	w.pdf.Ln(2)
}

// writeInfoBlock записывает информационный блок в PDF
func (w *pdfWriter) writeInfoBlock(ib editor.InfoBlock) {
	w.pdf.Ln(3)

	startX, startY := w.pdf.GetXY()
	maxWidth := w.getMaxContentWidth()

	// Установить осветлённый фон
	r, g, b := lightenColor(ib.Color, 0.85)
	w.pdf.SetFillColor(r, g, b)

	// Записать заголовок с иконкой
	w.pdf.SetFont("Rubik", "B", 14)
	w.pdf.SetTextColor(int(ib.Color.R), int(ib.Color.G), int(ib.Color.B))

	// Записать заголовок на фоне
	w.pdf.CellFormat(maxWidth, 7, "", "", 0, "L", true, 0, "")
	w.pdf.SetXY(startX+2, startY+1)
	w.write("[!]  " + cleanUnsupportedSymbols(ib.Title))
	w.pdf.Ln(7)

	// Установить отступ слева для содержимого
	w.pdf.SetLeftMargin(startX + 8)
	w.pdf.SetX(startX + 8)

	// Записать содержимое
	w.pdf.SetFont("Rubik", "", 12)
	w.pdf.SetTextColor(0, 0, 0) // Чёрный цвет для содержимого
	for _, p := range ib.Content {
		w.writeParagraph(p)
	}

	endY := w.pdf.GetY()

	// Нарисовать левую вертикальную линию
	w.pdf.SetDrawColor(int(ib.Color.R), int(ib.Color.G), int(ib.Color.B))
	w.pdf.SetLineWidth(2)
	w.pdf.Line(startX, startY, startX, endY)

	// Восстановить отступ
	w.pdf.SetLeftMargin(startX)
	w.pdf.SetDrawColor(0, 0, 0) // Сброс цвета линии

	w.pdf.Ln(3)
}

// writeSpoiler записывает спойлер в PDF согласно стилям TipTap
// ВАЖНО: Всегда показывает содержимое развёрнутым (игнорирует флаг Collapsed)
func (w *pdfWriter) writeSpoiler(s editor.Spoiler) {
	w.pdf.Ln(3)

	startX, startY := w.pdf.GetXY()
	maxWidth := w.getMaxContentWidth()

	// Заголовок спойлера с фоном (border-radius: 6px в TipTap)
	// padding-left: 10px = ~3.5mm
	headerHeight := 7.0
	headerPaddingLeft := 3.5

	// Нарисовать фон заголовка
	w.pdf.SetFillColor(int(s.BgColor.R), int(s.BgColor.G), int(s.BgColor.B))
	w.pdf.Rect(startX, startY, maxWidth, headerHeight, "F")

	// Записать стрелку и заголовок
	w.pdf.SetXY(startX+headerPaddingLeft, startY+1.5)
	w.pdf.SetFont("Rubik", "B", 12)
	w.pdf.SetTextColor(int(s.Color.R), int(s.Color.G), int(s.Color.B))

	// Стрелка > повернутая на 90 градусов (v) как индикатор раскрытого спойлера
	indicator := ">"
	w.write(indicator + " " + cleanUnsupportedSymbols(s.Title))

	// Перейти после заголовка
	w.pdf.SetXY(startX, startY+headerHeight)
	w.pdf.Ln(1)

	// Всегда записываем содержимое (игнорируем s.Collapsed)
	// padding: 0px 8px 0px 20px = ~7mm слева (в TipTap)
	contentLeftPadding := 7.0
	w.pdf.SetLeftMargin(startX + contentLeftPadding)
	w.pdf.SetX(startX + contentLeftPadding)

	// Записать содержимое
	w.pdf.SetFont("Rubik", "", 12)
	w.pdf.SetTextColor(0, 0, 0) // Чёрный цвет для содержимого
	for _, p := range s.Content {
		w.writeParagraph(p)
	}

	// Восстановить отступ и стили
	w.pdf.SetLeftMargin(startX)
	w.pdf.SetTextColor(0, 0, 0)

	// margin-bottom: 10px = ~3.5mm
	w.pdf.Ln(3)
}
