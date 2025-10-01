// Экспортирует Issue в PDF-файл.
//
// Основные возможности:
//   - Преобразование Issue в PDF с использованием HTML-разметки.
//   - Поддержка указания URL для локального PDF-фреймворка.
//   - Добавление комментариев к Issue в PDF.
package export

import (
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/aisa-it/aiplan/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/internal/aiplan/types"
)

func TestFPDF(t *testing.T) {
	p := "urgent"
	issue := dao.Issue{
		SequenceId: 12,
		Priority:   &p,
		Project: &dao.Project{
			Identifier: "BAK",
		},
		CreatedAt: time.Now().Add(time.Minute * -5),
		UpdatedAt: time.Now(),
		Author: &dao.User{
			FirstName: "И",
			LastName:  "П",
		},
		State: &dao.State{
			Name:  "Новая",
			Color: "#26b5ce",
		},
		Assignees: &[]dao.User{
			{FirstName: "Павел", LastName: "Петров"},
			{FirstName: "Иван", LastName: "Иванов"},
		},
		Name:            "Удаление проекта, импортированного из Jira (жиры)",
		DescriptionHtml: `<table style="width: 623px"><colgroup><col style="width: 105px"><col style="width: 188px"><col style="width: 192px"><col style="width: 138px"></colgroup><tbody><tr><th colspan="1" rowspan="1" colwidth="105"><p>Заголовок</p></th><th colspan="1" rowspan="1" colwidth="188"><p>Заголовок 2 😋</p></th><th colspan="1" rowspan="1" colwidth="192"><p>Заголовок длинный ваще пипец</p></th><th colspan="1" rowspan="1" colwidth="138"><p>Мелкий заголовок</p></th></tr><tr><td colspan="1" rowspan="1" colwidth="105"><p>текст</p><p><mark data-color="rgb(0,245,123)" style="background-color: rgb(0,245,123); color: inherit">параграф</mark></p></td><td colspan="1" rowspan="1" colwidth="188"><p>ываыва</p></td><td colspan="1" rowspan="1" colwidth="192"><p><img src="/api/file/5ccf6647-6735-4137-a017-dcae0af1c994-0" alt="2" style="height: auto;" draggable="true"></p></td><td colspan="1" rowspan="1" colwidth="138"><p>ываыва</p></td></tr><tr><td colspan="1" rowspan="1" colwidth="105"><p>ываыва</p></td><td colspan="1" rowspan="1" colwidth="188"><p>ываыва</p></td><td colspan="1" rowspan="1" colwidth="192"><p></p></td><td colspan="1" rowspan="1" colwidth="138"><p>ываываыав</p></td></tr></tbody></table>`,
	}

	u, _ := url.Parse("http://localhost:9200")

	os.Remove("output.pdf")
	f, _ := os.Create("output.pdf")
	fmt.Println(IssueToFPDF(&issue, u, f, dao.IssueComment{
		Actor: &dao.User{
			FirstName: "И",
			LastName:  "П",
		},
		CreatedAt:   time.Now(),
		CommentHtml: types.RedactorHTML{Body: `<table style="width: 623px"><colgroup><col style="width: 105px"><col style="width: 188px"><col style="width: 192px"><col style="width: 138px"></colgroup><tbody><tr><th colspan="1" rowspan="1" colwidth="105"><p>Заголовок</p></th><th colspan="1" rowspan="1" colwidth="188"><p>Заголовок 2 😋</p></th><th colspan="1" rowspan="1" colwidth="192"><p>Заголовок длинный ваще пипец</p></th><th colspan="1" rowspan="1" colwidth="138"><p>Мелкий заголовок</p></th></tr><tr><td colspan="1" rowspan="1" colwidth="105"><p>текст</p><p><mark data-color="rgb(0,245,123)" style="background-color: rgb(0,245,123); color: inherit">параграф</mark></p></td><td colspan="1" rowspan="1" colwidth="188"><p>ываыва</p></td><td colspan="1" rowspan="1" colwidth="192"><p><img src="/api/file/5ccf6647-6735-4137-a017-dcae0af1c994-0" alt="2" style="height: auto;" draggable="true"></p></td><td colspan="1" rowspan="1" colwidth="138"><p>ываыва</p></td></tr><tr><td colspan="1" rowspan="1" colwidth="105"><p>ываыва</p></td><td colspan="1" rowspan="1" colwidth="188"><p>ываыва</p></td><td colspan="1" rowspan="1" colwidth="192"><p></p></td><td colspan="1" rowspan="1" colwidth="138"><p>ываываыав</p></td></tr></tbody></table>`},
	},
		dao.IssueComment{
			Actor: &dao.User{
				FirstName: "И",
				LastName:  "П",
			},
			CreatedAt:   time.Now(),
			CommentHtml: types.RedactorHTML{Body: `<p>Н<span style="font-size: 14px">а тесте установлено</span></p>`},
		}))
	f.Close()
}
