// Экспортирует информацию об issue в PDF-документ с использованием библиотеки go-pdf.  Включает таблицу с информацией об авторе, статусе, приоритете, датах создания/изменения, исполнителях и наблюдателях.  Также добавляет иконки, отображающие приоритет issue.
package export

import (
	_ "embed"
	"strings"

	"codeberg.org/go-pdf/fpdf"
)

var (
	//go:embed images/important.png
	urgentPriorityIMG []byte

	//go:embed images/priority_hight.png
	highPriorityIMG []byte

	//go:embed images/priority_medium.png
	mediumPriorityIMG []byte

	//go:embed images/priority_low.png
	lowPriorityIMG []byte
)

func (w *pdfWriter) writeIssueInfoTable() {

	table := [][][]string{
		{
			{"Автор:", w.issue.Author.GetName()},
			{"Статус:", "    " + w.issue.State.Name},
			{"Приоритет:", w.getPriority()},
		},
		{
			{"Создана:", w.issue.CreatedAt.Format("02.01.2006 15:04")},
			{"Изменена:", w.issue.UpdatedAt.Format("02.01.2006 15:04")},
			{"Срок исполнения:", w.getTargetDate()},
		},
		{
			{"Исполнители:", w.getAssigneesList()},
			{"Наблюдатели:", w.getWatchersList()},
		},
	}

	w.pdf.SetFont("Rubik", "", 8)
	w.pdf.SetTextColor(71, 74, 82)
	w.pdf.SetDrawColor(71, 74, 82)

	y := w.pdf.GetY()
	finalY := y
	for i, col := range table {
		w.pdf.SetXY(w.pdf.GetX(), y)

		var keyWidth, valWidth float64
		for _, cell := range col {
			keyWidth = max(keyWidth, w.pdf.GetStringWidth(cell[0])+3)
			valWidth = max(valWidth, w.pdf.GetStringWidth(cell[1])+3)
		}

		x := w.pdf.GetX()
		colX := x
		for _, cell := range col {
			k := cell[0]
			v := cell[1]

			w.pdf.CellFormat(keyWidth, 4, k, "", 0, "L", false, 0, "")

			switch k {
			case "Статус:":
				w.SetHexFillColor(w.issue.State.Color)
				w.pdf.Circle(w.pdf.GetX()+2, w.pdf.GetY()+1.9, 0.8, "F")
			case "Приоритет:":
				w.writePriorityImg()
			}

			border := ""
			if i != len(table)-1 {
				border = "R"
			}

			w.pdf.CellFormat(valWidth, 4, v, border, 0, "L", false, 0, "")

			x = max(x, w.pdf.GetX())
			w.pdf.Ln(-1)
			w.pdf.SetX(colX)
		}
		w.pdf.SetX(x + 2)
		finalY = max(finalY, w.pdf.GetY())
	}
	w.pdf.SetY(finalY)
}

func (w *pdfWriter) getAssigneesList() string {
	if w.issue.Assignees == nil {
		return ""
	}

	res := ""
	for i, assignee := range *w.issue.Assignees {
		res += assignee.GetName()
		if i != len(*w.issue.Assignees)-1 {
			res += ", "
		}
	}

	return res
}

func (w *pdfWriter) getWatchersList() string {
	if w.issue.Watchers == nil {
		return ""
	}

	res := ""
	for i, watcher := range *w.issue.Watchers {
		res += watcher.GetName()
		if i != len(*w.issue.Watchers)-1 {
			res += ", "
		}
	}

	return res
}

func (w *pdfWriter) getPriority() string {
	if w.issue.Priority == nil {
		return "не выбран"
	}

	imgPadding := 6

	switch *w.issue.Priority {
	case "urgent":
		return strings.Repeat(" ", imgPadding) + "Критический"
	case "high":
		return strings.Repeat(" ", imgPadding) + "Высокий"
	case "medium":
		return strings.Repeat(" ", imgPadding) + "Средний"
	case "low":
		return strings.Repeat(" ", imgPadding) + "Низкий"
	}
	return "не выбран"
}

func (w *pdfWriter) writePriorityImg() {
	if w.issue.Priority == nil {
		return
	}

	iconSize := 4.0
	w.pdf.ImageOptions(*w.issue.Priority, w.pdf.GetX()+0.5, w.pdf.GetY()-0.3, iconSize, iconSize, false, fpdf.ImageOptions{ReadDpi: true}, 0, "")
	//w.pdf.SetX(w.pdf.GetX() + iconSize + 0.5)
}

func (w *pdfWriter) getTargetDate() string {
	if w.issue.TargetDate == nil {
		return "не установлен"
	}
	return w.issue.TargetDate.Time.Format("02.01.2006 15:04")
}
