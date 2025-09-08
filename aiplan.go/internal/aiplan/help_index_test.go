// Пакет aiplan предоставляет инструменты для работы с планами и задачами, вероятно, с использованием AI.  Включает функции для добавления элементов в слайсы и генерации индексов для документации.
//
// Основные возможности:
//   - Добавление элементов в слайсы по индексу.
//   - Генерация индексов для документации, возможно, на основе анализа кода или других данных.
package aiplan

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"sheff.online/aiplan/internal/aiplan/config"
)

func TestSliceInsert(t *testing.T) {
	test := make([]int, 4)
	insertInIndexSlice(test, 3, 2)
	insertInIndexSlice(test, 6, 3)
}

func TestIndexGen(t *testing.T) {
	cfg = config.ReadConfig()

	e := echo.New()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	c := e.NewContext(req, rec)
	if err := NewHelpIndex("../../../aiplan-doc")(c); err != nil {
		t.Fatal(err)
	}

	var resp interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	b, _ := json.MarshalIndent(resp, "", "    ")
	fmt.Println(string(b))
}
