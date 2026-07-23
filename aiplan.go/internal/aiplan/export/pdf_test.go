// Экспортирует Issue в PDF-файл.
//
// Основные возможности:
//   - Преобразование Issue в PDF с использованием HTML-разметки.
//   - Поддержка указания URL для локального PDF-фреймворка.
//   - Добавление комментариев к Issue в PDF.
package export

import (
	"encoding/json"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/editor"
	_ "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/editor/tiptap"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
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
	err := IssueToFPDF(&issue, u, f, dao.IssueComment{
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
		})
	if err != nil {
		t.Fatalf("Failed to generate PDF: %v", err)
	}
	f.Close()
}

// validatePDF проверяет корректность созданного PDF файла
func validatePDF(t *testing.T, filepath string) {
	t.Helper()
	info, err := os.Stat(filepath)
	if err != nil {
		t.Fatalf("PDF file not created: %v", err)
	}
	if info.Size() == 0 {
		t.Error("PDF file is empty")
	}
}

// TestPDFExport_NewTypes тестирует экспорт новых типов контента в PDF
func TestPDFExport_NewTypes(t *testing.T) {
	u, _ := url.Parse("http://localhost:9200")

	baseIssue := dao.Issue{
		SequenceId: 1,
		Project: &dao.Project{
			Identifier: "TEST",
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Author: &dao.User{
			FirstName: "Тест",
			LastName:  "Тестов",
		},
		State: &dao.State{
			Name:  "В работе",
			Color: "#26b5ce",
		},
	}

	tests := []struct {
		name           string
		descriptionDoc editor.Document
		outputFile     string
	}{
		{
			name: "HardBreak",
			descriptionDoc: editor.Document{Elements: []any{
				editor.Paragraph{Content: []any{
					editor.Text{Content: "Первая строка"},
					&editor.HardBreak{},
					editor.Text{Content: "Вторая строка"},
					&editor.HardBreak{},
					editor.Text{Content: "Третья строка"},
				}},
			}},
			outputFile: "testdata/output/test_hardbreak.pdf",
		},
		{
			name: "CodeBlock",
			descriptionDoc: editor.Document{Elements: []any{
				editor.Code{Content: `function hello() {
    console.log("Hello, World!");
    return 42;
}`},
			}},
			outputFile: "testdata/output/test_codeblock.pdf",
		},
		{
			name: "Inline_Elements",
			descriptionDoc: editor.Document{Elements: []any{
				editor.Paragraph{Content: []any{
					editor.Text{Content: "Встреча назначена на "},
					&editor.DateNode{Date: "2024-12-15"},
					editor.Text{Content: " в офисе с "},
					&editor.Mention{ID: "user1", Label: "Иван Иванов"},
					editor.Text{Content: " по задаче "},
					&editor.IssueLinkMention{
						Slug:              "123",
						ProjectIdentifier: "TEST",
					},
				}},
			}},
			outputFile: "testdata/output/test_inline.pdf",
		},
		{
			name: "Combined",
			descriptionDoc: editor.Document{Elements: []any{
				editor.Paragraph{Content: []any{
					editor.Text{Content: "Обычный параграф с текстом"},
				}},
				editor.Code{Content: `const x = 42;
console.log(x);`},
				editor.Paragraph{Content: []any{
					editor.Text{Content: "Параграф с переносами:"},
					&editor.HardBreak{},
					editor.Text{Content: "Строка 1"},
					&editor.HardBreak{},
					editor.Text{Content: "Строка 2"},
				}},
				editor.Paragraph{Content: []any{
					editor.Text{Content: "Встреча "},
					&editor.DateNode{Date: "2024-12-20"},
					editor.Text{Content: " с "},
					&editor.Mention{ID: "user2", Label: "Петров П."},
					editor.Text{Content: " по "},
					&editor.IssueLinkMention{
						Slug:              "456",
						ProjectIdentifier: "TEST",
					},
				}},
			}},
			outputFile: "testdata/output/test_combined.pdf",
		},
		{
			name: "Long_Text_Wrapping",
			descriptionDoc: editor.Document{Elements: []any{
				editor.Paragraph{Content: []any{
					editor.Text{Content: strings.Repeat("Очень длинный текст для проверки переноса строк. ", 20)},
				}},
			}},
			outputFile: "testdata/output/test_long_text.pdf",
		},
		{
			name: "HardBreak_Long_Lines",
			descriptionDoc: editor.Document{Elements: []any{
				editor.Paragraph{Content: []any{
					editor.Text{Content: strings.Repeat("Первая длинная строка. ", 10)},
					&editor.HardBreak{},
					editor.Text{Content: strings.Repeat("Вторая длинная строка. ", 10)},
					&editor.HardBreak{},
					editor.Text{Content: strings.Repeat("Третья длинная строка. ", 10)},
				}},
			}},
			outputFile: "testdata/output/test_hardbreak_long.pdf",
		},
		{
			name: "Mentions_Many",
			descriptionDoc: editor.Document{Elements: []any{
				editor.Paragraph{Content: []any{
					editor.Text{Content: "Встреча с "},
					&editor.Mention{ID: "user1", Label: "Иван Иванов"},
					editor.Text{Content: ", "},
					&editor.Mention{ID: "user2", Label: "Петр Петров"},
					editor.Text{Content: ", "},
					&editor.Mention{ID: "user3", Label: "Сергей Сергеев"},
					editor.Text{Content: ", "},
					&editor.Mention{ID: "user4", Label: "Анна Смирнова"},
					editor.Text{Content: ", "},
					&editor.Mention{ID: "user5", Label: "Елена Кузнецова"},
					editor.Text{Content: " по важному вопросу проекта"},
				}},
			}},
			outputFile: "testdata/output/test_mentions_many.pdf",
		},
		{
			name: "Image_Inline",
			descriptionDoc: editor.Document{Elements: []any{
				editor.Paragraph{Content: []any{
					editor.Text{Content: "Текст перед изображением "},
					&editor.Image{
						Src:   &url.URL{Scheme: "file", Path: "/home/claude-user/aiplan-oss/aiplan.go/internal/aiplan/export/testdata/images/test.png"},
						Width: 150,
						Align: editor.LeftAlign,
					},
					editor.Text{Content: " текст после изображения"},
				}},
			}},
			outputFile: "testdata/output/test_image_inline.pdf",
		},
		{
			name: "Table_Simple",
			descriptionDoc: editor.Document{Elements: []any{
				editor.Paragraph{Content: []any{
					editor.Text{Content: "Таблица с данными:"},
				}},
				editor.Table{
					Rows: [][]editor.TableCell{
						{
							{Header: true, ColSpan: 1, RowSpan: 1, Content: []editor.Paragraph{
								{Content: []any{editor.Text{Content: "KLEIM"}}},
							}},
							{Header: true, ColSpan: 1, RowSpan: 1, Content: []editor.Paragraph{
								{Content: []any{editor.Text{Content: "DORID"}}},
							}},
							{Header: true, ColSpan: 1, RowSpan: 1, Content: []editor.Paragraph{
								{Content: []any{editor.Text{Content: "NAME"}}},
							}},
						},
						{
							{Header: false, ColSpan: 1, RowSpan: 1, Content: []editor.Paragraph{
								{Content: []any{editor.Text{Content: "4367"}}},
							}},
							{Header: false, ColSpan: 1, RowSpan: 1, Content: []editor.Paragraph{
								{Content: []any{editor.Text{Content: "94"}}},
							}},
							{Header: false, ColSpan: 1, RowSpan: 1, Content: []editor.Paragraph{
								{Content: []any{editor.Text{Content: "ВЧДэ-Могоча ТОР Сковородино"}}},
							}},
						},
					},
				},
			}},
			outputFile: "testdata/output/test_table_simple.pdf",
		},
		{
			name: "Spoiler",
			descriptionDoc: editor.Document{Elements: []any{
				editor.Paragraph{Content: []any{
					editor.Text{Content: "Параграф перед спойлером"},
				}},
				editor.Spoiler{
					Title:     "Скрытая информация",
					Collapsed: false,
					BgColor:   editor.Color{R: 230, G: 230, B: 230, A: 255}, // #e6e6e6
					Color:     editor.Color{R: 100, G: 100, B: 100, A: 255}, // #646464
					Content: []editor.Paragraph{
						{Content: []any{
							editor.Text{Content: "Это содержимое спойлера."},
						}},
						{Content: []any{
							editor.Text{Content: "Вторая строка в спойлере."},
						}},
					},
				},
				editor.Paragraph{Content: []any{
					editor.Text{Content: "Параграф после спойлера"},
				}},
			}},
			outputFile: "testdata/output/test_spoiler.pdf",
		},
		{
			name: "BulletList",
			descriptionDoc: editor.Document{Elements: []any{
				editor.Paragraph{Content: []any{
					editor.Text{Content: "Маркированный список:"},
				}},
				editor.List{
					Numbered: false,
					TaskList: false,
					Elements: []editor.ListElement{
						{Content: []editor.Paragraph{{
							Content: []any{editor.Text{Content: "Первый пункт"}},
						}}},
						{Content: []editor.Paragraph{{
							Content: []any{editor.Text{Content: "Второй пункт с форматированием"}},
						}}},
						{Content: []editor.Paragraph{{
							Content: []any{editor.Text{Content: "Третий пункт"}},
						}}},
					},
				},
			}},
			outputFile: "testdata/output/test_bullet_list.pdf",
		},
		{
			name: "NumberedList",
			descriptionDoc: editor.Document{Elements: []any{
				editor.Paragraph{Content: []any{
					editor.Text{Content: "Нумерованный список:"},
				}},
				editor.List{
					Numbered: true,
					TaskList: false,
					Elements: []editor.ListElement{
						{Content: []editor.Paragraph{{
							Content: []any{editor.Text{Content: "Первый"}},
						}}},
						{Content: []editor.Paragraph{{
							Content: []any{editor.Text{Content: "Второй"}},
						}}},
						{Content: []editor.Paragraph{{
							Content: []any{editor.Text{Content: "Третий"}},
						}}},
					},
				},
			}},
			outputFile: "testdata/output/test_numbered_list.pdf",
		},
		{
			name: "TaskList_Checked",
			descriptionDoc: editor.Document{Elements: []any{
				editor.Paragraph{Content: []any{
					editor.Text{Content: "Чек-лист (смешанный):"},
				}},
				editor.List{
					Numbered: false,
					TaskList: true,
					Elements: []editor.ListElement{
						{Checked: true, Content: []editor.Paragraph{
							{Content: []any{editor.Text{Content: "Выполненная задача"}}},
						}},
						{Checked: false, Content: []editor.Paragraph{
							{Content: []any{editor.Text{Content: "Невыполненная задача"}}},
						}},
						{Checked: true, Content: []editor.Paragraph{
							{Content: []any{editor.Text{Content: "Ещё одна выполненная"}}},
						}},
					},
				},
			}},
			outputFile: "testdata/output/test_tasklist_checked.pdf",
		},
		{
			name: "TaskList_AllUnchecked",
			descriptionDoc: editor.Document{Elements: []any{
				editor.Paragraph{Content: []any{
					editor.Text{Content: "Чек-лист (все невыполнены):"},
				}},
				editor.List{
					TaskList: true,
					Elements: []editor.ListElement{
						{Checked: false, Content: []editor.Paragraph{
							{Content: []any{editor.Text{Content: "Пункт 1"}}},
						}},
						{Checked: false, Content: []editor.Paragraph{
							{Content: []any{editor.Text{Content: "Пункт 2"}}},
						}},
						{Checked: false, Content: []editor.Paragraph{
							{Content: []any{editor.Text{Content: "Пункт 3"}}},
						}},
					},
				},
			}},
			outputFile: "testdata/output/test_tasklist_unchecked.pdf",
		},
		{
			name: "Paragraph_Align",
			descriptionDoc: editor.Document{Elements: []any{
				editor.Paragraph{Align: editor.LeftAlign, Content: []any{
					editor.Text{Content: "Этот параграф выровнен влево (Left)"},
				}},
				editor.Paragraph{Align: editor.CenterAlign, Content: []any{
					editor.Text{Content: "Этот параграф выровнен по центру (Center)"},
				}},
				editor.Paragraph{Align: editor.RightAlign, Content: []any{
					editor.Text{Content: "Этот параграф выровнен вправо (Right)"},
				}},
			}},
			outputFile: "testdata/output/test_paragraph_align.pdf",
		},
		{
			name: "Table_ColSpan",
			descriptionDoc: editor.Document{Elements: []any{
				editor.Paragraph{Content: []any{
					editor.Text{Content: "Таблица с colspan (пока не поддерживается):"},
				}},
				editor.Table{
					Rows: [][]editor.TableCell{
						{
							{Header: true, ColSpan: 2, RowSpan: 1, Content: []editor.Paragraph{
								{Content: []any{editor.Text{Content: "Заголовок A"}}},
							}},
							{Header: true, ColSpan: 1, RowSpan: 1, Content: []editor.Paragraph{
								{Content: []any{editor.Text{Content: "Заголовок B"}}},
							}},
						},
						{
							{Header: false, ColSpan: 1, RowSpan: 1, Content: []editor.Paragraph{
								{Content: []any{editor.Text{Content: "A1"}}},
							}},
							{Header: false, ColSpan: 2, RowSpan: 1, Content: []editor.Paragraph{
								{Content: []any{editor.Text{Content: "B1"}}},
							}},
						},
					},
				},
			}},
			outputFile: "testdata/output/test_table_colspan.pdf",
		},
		{
			name: "Table_RowSpan",
			descriptionDoc: editor.Document{Elements: []any{
				editor.Paragraph{Content: []any{
					editor.Text{Content: "Таблица с rowspan (пока не поддерживается):"},
				}},
				editor.Table{
					Rows: [][]editor.TableCell{
						{
							{Header: true, ColSpan: 1, RowSpan: 2, Content: []editor.Paragraph{
								{Content: []any{editor.Text{Content: "A1"}}},
							}},
							{Header: true, ColSpan: 1, RowSpan: 1, Content: []editor.Paragraph{
								{Content: []any{editor.Text{Content: "Заголовок 2"}}},
							}},
						},
						{
							{Header: false, ColSpan: 1, RowSpan: 1, Content: []editor.Paragraph{
								{Content: []any{editor.Text{Content: "B1"}}},
							}},
							{Header: false, ColSpan: 1, RowSpan: 1, Content: []editor.Paragraph{
								{Content: []any{editor.Text{Content: "B2"}}},
							}},
						},
					},
				},
			}},
			outputFile: "testdata/output/test_table_rowspan.pdf",
		},
		{
			name: "Drawio",
			descriptionDoc: editor.Document{Elements: []any{
				editor.Paragraph{Content: []any{
					editor.Text{Content: "Параграф с drawio-диаграммой ниже:"},
				}},
				editor.Paragraph{Content: []any{
					&editor.Drawio{
						Src:   &url.URL{Path: "testdata/images/test.png"},
						Width: 200,
						XML:   "<mxGraphModel></mxGraphModel>",
					},
				}},
				editor.Paragraph{Content: []any{
					editor.Text{Content: "Параграф после drawio"},
				}},
			}},
			outputFile: "testdata/output/test_drawio.pdf",
		},
		{
			name: "Superscript_Subscript",
			descriptionDoc: editor.Document{Elements: []any{
				editor.Paragraph{Content: []any{
					editor.Text{Content: "Обычный текст, "},
					editor.Text{Content: "верхний индекс", Sup: true},
					editor.Text{Content: ", "},
					editor.Text{Content: "нижний индекс", Sub: true},
					editor.Text{Content: " и снова обычный текст."},
				}},
			}},
			outputFile: "testdata/output/test_sup_sub.pdf",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issue := baseIssue
			issue.Name = "Тест: " + tt.name
			issue.DescriptionJSON = tt.descriptionDoc

			// Удалить старый файл если существует
			os.Remove(tt.outputFile)

			// Создать новый файл
			f, err := os.Create(tt.outputFile)
			if err != nil {
				t.Fatalf("Failed to create output file: %v", err)
			}
			defer f.Close()

			// Экспортировать в PDF
			err = IssueToFPDF(&issue, u, f)
			if err != nil {
				t.Fatalf("Failed to export PDF: %v", err)
			}

			// Валидировать результат
			validatePDF(t, tt.outputFile)
		})
	}
}

// TestPDFExport_TipTapJSON тестирует полный цикл десериализации TipTap JSON в PDF
func TestPDFExport_TipTapJSON(t *testing.T) {
	u, _ := url.Parse("http://localhost:9200")

	// TipTap JSON с таблицей (как из API)
	tipTapJSON := `{
		"type": "doc",
		"content": [
			{
				"type": "paragraph",
				"content": [
					{"type": "text", "text": "Необходимо внести в БД данные о следующих депо:"},
					{"type": "hardBreak"},
					{"type": "text", "text": "В таблицу 'REM_PRED':"}
				]
			},
			{
				"type": "table",
				"content": [
					{
						"type": "tableRow",
						"content": [
							{"type": "tableHeader", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "KLEIM"}]}]},
							{"type": "tableHeader", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "DORID"}]}]},
							{"type": "tableHeader", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "NAME"}]}]}
						]
					},
					{
						"type": "tableRow",
						"content": [
							{"type": "tableCell", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "4367"}]}]},
							{"type": "tableCell", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "94"}]}]},
							{"type": "tableCell", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "ВЧДэ-Могоча ТОР Сковородино"}]}]}
						]
					}
				]
			}
		]
	}`

	// Десериализовать JSON в Document
	var doc editor.Document
	err := json.Unmarshal([]byte(tipTapJSON), &doc)
	if err != nil {
		t.Fatalf("Failed to unmarshal TipTap JSON: %v", err)
	}

	// Проверить что таблица распарсилась
	if len(doc.Elements) != 2 {
		t.Fatalf("Expected 2 elements, got %d", len(doc.Elements))
	}

	// Второй элемент должен быть таблицей
	table, ok := doc.Elements[1].(*editor.Table)
	if !ok {
		t.Fatalf("Second element is not a table, got %T", doc.Elements[1])
	}

	if len(table.Rows) != 2 {
		t.Fatalf("Expected 2 rows in table, got %d", len(table.Rows))
	}

	// Создать Issue с этим Document
	issue := dao.Issue{
		SequenceId: 528,
		Project: &dao.Project{
			Identifier: "IIT",
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Author: &dao.User{
			FirstName: "Егор",
			LastName:  "Федин",
		},
		State: &dao.State{
			Name:  "Установлено на пром",
			Color: "#26b5ce",
		},
		Name:            "CR_23_02_Добавление нового клейма депо",
		DescriptionJSON: doc,
	}

	// Экспортировать в PDF
	outputFile := "testdata/output/test_tiptap_json_table.pdf"
	os.Remove(outputFile)
	f, err := os.Create(outputFile)
	if err != nil {
		t.Fatalf("Failed to create output file: %v", err)
	}
	defer f.Close()

	err = IssueToFPDF(&issue, u, f)
	if err != nil {
		t.Fatalf("Failed to export PDF: %v", err)
	}

	// Валидировать результат
	validatePDF(t, outputFile)
}

// TestPDFExport_HTMLWithBrTags тестирует переносы строк через <br/> в HTML
func TestPDFExport_HTMLWithBrTags(t *testing.T) {
	u, _ := url.Parse("http://localhost:9200")

	// HTML с <br/> тегами (как из API) - точная копия из вашего примера
	htmlContent := `<p>Необходимо узнать про возможность кастомизировать веб-интерфейс Minio:<br/> 1) Главный экран (авторизация) <br/> 2) Тема самой графаны (Значки, меню)<br/>   <span class="mention" data-type="mention" data-id="ivan.zakharov" data-label="ivan.zakharov@aisa.ru">@ivan.zakharov</span> приложит скриншоты тех компонентов, которые нужно поменять. <br/> После передачи фронтам\дизайнерам\итд ИМ комментариях нужно написать что можно поменять.<br/> После согласования поменять на нашу тему<br/>3) название самого "Minio" поменять на "Централизованное файловое хранилище" </p>`

	issue := dao.Issue{
		SequenceId: 501,
		Project: &dao.Project{
			Identifier: "ANICOMDEV",
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Author: &dao.User{
			FirstName: "Иван",
			LastName:  "Захаров",
		},
		State: &dao.State{
			Name:  "Закрыто",
			Color: "#26b5ce",
		},
		Name:            "Замена интерфейса Minio",
		DescriptionHtml: htmlContent,
	}

	// Экспортировать в PDF
	outputFile := "testdata/output/test_html_br_tags.pdf"
	os.Remove(outputFile)
	f, err := os.Create(outputFile)
	if err != nil {
		t.Fatalf("Failed to create output file: %v", err)
	}
	defer f.Close()

	err = IssueToFPDF(&issue, u, f)
	if err != nil {
		t.Fatalf("Failed to export PDF: %v", err)
	}

	// Валидировать результат
	validatePDF(t, outputFile)
}

// TestPDFExport_HTMLWithBrTagsNoSpaces тестирует переносы строк БЕЗ пробелов после <br/>
func TestPDFExport_HTMLWithBrTagsNoSpaces(t *testing.T) {
	u, _ := url.Parse("http://localhost:9200")

	// HTML БЕЗ пробелов после <br/>
	htmlContent := `<p>Необходимо узнать про возможность кастомизировать веб-интерфейс Minio:<br/>1) Главный экран (авторизация)<br/>2) Тема самой графаны (Значки, меню)<br/>После передачи фронтам нужно написать что можно поменять.<br/>После согласования поменять на нашу тему<br/>3) название самого "Minio" поменять на "Централизованное файловое хранилище"</p>`

	issue := dao.Issue{
		SequenceId: 502,
		Project: &dao.Project{
			Identifier: "TEST",
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Author: &dao.User{
			FirstName: "Тест",
			LastName:  "Тестов",
		},
		State: &dao.State{
			Name:  "Новая",
			Color: "#26b5ce",
		},
		Name:            "Тест без пробелов после br",
		DescriptionHtml: htmlContent,
	}

	// Экспортировать в PDF
	outputFile := "testdata/output/test_html_br_no_spaces.pdf"
	os.Remove(outputFile)
	f, err := os.Create(outputFile)
	if err != nil {
		t.Fatalf("Failed to create output file: %v", err)
	}
	defer f.Close()

	err = IssueToFPDF(&issue, u, f)
	if err != nil {
		t.Fatalf("Failed to export PDF: %v", err)
	}

	// Валидировать результат
	validatePDF(t, outputFile)
}
