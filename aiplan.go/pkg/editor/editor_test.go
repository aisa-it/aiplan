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
