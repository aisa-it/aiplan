// Пакет предоставляет базовые инструменты для обработки HTML-документов.
//
//	Включает парсинг HTML из файла и вывод структуры документа.
//	Также содержит пример парсинга RGB-цвета.
//	Дополнительно, демонстрирует вывод структуры различных HTML-элементов.
//
// Основные возможности:
//   - Парсинг HTML-документа из файла.
//   - Вывод структуры HTML-документа в консоль.
//   - Парсинг RGB-цвета.
//   - Вывод структуры различных HTML-элементов (Code, Paragraph, List, Quote, Table).
package editor

import (
	"bytes"
	"compress/gzip"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

func TestParsing(t *testing.T) {
	f, _ := os.Open("example2.html")
	d, err := ParseDocument(f)
	if err != nil {
		t.Fatal(err)
	}

	gob.Register(Document{})
	gob.Register(Paragraph{})
	gob.Register(Text{})
	gob.Register(ListElement{})
	gob.Register(List{})
	gob.Register(Quote{})
	gob.Register(Code{})
	gob.Register(Image{})
	gob.Register(Table{})
	gob.Register(TableCell{})
	gob.Register(Spoiler{})
	gob.Register(InfoBlock{})
	gob.Register(Color{})

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if err := gob.NewEncoder(gz).Encode(d); err != nil {
		t.Fatal(err)
	}
	gz.Flush()
	fmt.Println(buf.Len())

	/*dd := Document{}
	if err := gob.NewDecoder(&buf).Decode(&dd); err != nil {
		t.Fatal(err)
	}

	printDocument(&dd)*/
}

func TestParseRGB(t *testing.T) {
	c, err := ParseColor("#FFE4CC6B")
	fmt.Printf("%+v\n", c)
	fmt.Println(err)
}

func TestColorJSON(t *testing.T) {
	c := Color{R: 244, G: 244, B: 255, A: 100}
	d, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}

	var newC Color
	if err := json.Unmarshal(d, &newC); err != nil {
		t.Fatal(err)
	}
	if c.R != newC.R {
		t.Fatal("Not equal", c, newC)
	}
}

func printDocument(document *Document) {
	for _, el := range document.Elements {
		switch e := el.(type) {
		case Code:
			fmt.Println("Code", e.Content)
		case Paragraph:
			fmt.Println("Paragraph")
			for _, t := range e.Content {
				switch content := t.(type) {
				case Text:
					fmt.Printf(" Text: %s (%+v)\n", content.Content, content)
				case *Image:
					fmt.Printf(" Image: %s (%+v)\n", content.Src, content)
				}
			}
		case List:
			fmt.Printf("List Numbered: %t Task: %t \n", e.Numbered, e.TaskList)
			for _, li := range e.Elements {
				fmt.Printf(" Content: %v Checked: %t\n", li.Content, li.Checked)
			}
		case Quote:
			fmt.Println("Quote", e.Content)
		case Table:
			fmt.Printf("Table Sizes(%v)\n", e.ColWidth)
			fmt.Println(len(e.Rows), len(e.Rows[0]))
		case Spoiler:
			fmt.Println("Spoiler", e.Title, e.Content)
		case InfoBlock:
			fmt.Println("InfoBlock", e.Title, e.Content)
		}
	}
}
