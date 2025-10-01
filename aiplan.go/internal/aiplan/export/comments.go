// Экспортирует информацию из системы в PDF-формат.
//
// Основные возможности:
//   - Создание PDF-документа с заголовком и комментариями к задачам.
//   - Добавление комментариев с информацией об авторе, дате создания и HTML-описанием.
package export

import (
	"strings"

	"github.com/aisa-it/aiplan/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/internal/aiplan/editor"
)

func (w *pdfWriter) writeComments() {
	w.resetMargins()

	w.pdf.SetFontSize(14)
	w.write("Комментарии к задаче")
	w.pdf.Ln(8)

	for _, comment := range w.comments {
		w.writeComment(&comment)
	}
}

func (w *pdfWriter) writeComment(comment *dao.IssueComment) error {
	w.pdf.Ln(2)
	w.pdf.SetDrawColor(71, 74, 82)
	w.pdf.Line(w.pdf.GetX(), w.pdf.GetY(), 200, w.pdf.GetY())
	w.pdf.Ln(1)

	w.pdf.SetFont("Rubik", "", 8)
	w.pdf.SetTextColor(71, 74, 82)

	w.write(comment.Actor.GetName())

	_, s := w.pdf.GetFontSize()
	s += 0.1
	w.pdf.WriteAligned(0, s, comment.CreatedAt.Format("02.01.2006 15:04"), "R")
	w.pdf.Ln(5)

	doc, err := editor.ParseDocument(strings.NewReader(comment.CommentHtml.Body))
	if err != nil {
		return err
	}

	w.writeDescription(doc)

	w.pdf.Ln(-1)
	return nil
}
