// Пакет dao для логирования SQL запросов и предоставления сводной статистики.
//
// Основные возможности:
//   - Логирование каждого SQL запроса, выполненного через gorm.
//   - Сбор статистики по количеству выполнения каждого запроса.
//   - Предоставление HTML отчета с результатами статистики.
package dao

import (
	"html/template"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

const (
	recordsTemplate = `<html><body><table><tr><th>Query</th><th>Count</th></tr>{{ range $i, $r := .Records}}<tr><td>{{$r}}</td><td>{{index $.Map $r}}</td><tr>{{end}}</table></body></html>`
)

// -migration
type QueryLogger struct {
	mu      sync.RWMutex
	Records map[string]int
	tmpl    *template.Template
}

// NewQueryLogger создает новый экземпляр QueryLogger с заданным шаблоном HTML для отчета.  Шаблон используется для отображения статистики по SQL запросам.
//
// Возвращает:
//   - *QueryLogger: новый экземпляр QueryLogger.
//
// Параметры:
//   - Нет параметров.
func NewQueryLogger() *QueryLogger {
	tmpl, err := template.New("records").Parse(recordsTemplate)
	if err != nil {
		slog.Error("Parse query template", "err", err)
	}
	return &QueryLogger{Records: make(map[string]int), tmpl: tmpl}
}

// CountEndpoint отображает статистику по количеству выполненных SQL запросов.  Получает контекст Echo и выполняет рендеринг HTML отчета со статистикой.
//
// Параметры:
//   - c: Контекст Echo, используемый для выполнения рендеринга HTML отчета.
//
// Возвращает:
//   - error: Возвращает ошибку, если произошла ошибка при рендеринге HTML отчета.
func (ql *QueryLogger) CountEndpoint(c echo.Context) error {
	ql.mu.RLock()
	defer ql.mu.RUnlock()
	keys := []string{}
	total := 0
	for k, v := range ql.Records {
		keys = append(keys, strings.ReplaceAll(k, "\n", ""))
		total += v
	}

	sort.SliceStable(keys, func(i, j int) bool {
		return ql.Records[keys[i]] > ql.Records[keys[j]]
	})

	if err := ql.tmpl.Execute(c.Response(), struct {
		Records []string
		Map     map[string]int
	}{keys, ql.Records}); err != nil {
		return c.HTML(http.StatusInternalServerError, err.Error())
	}

	return c.NoContent(http.StatusOK)
}

// QueryCallback регистрирует каждый выполненный SQL запрос.  Принимает объект базы данных gorm.DB и увеличивает счетчик для соответствующего SQL запроса в словаре Records.  Счетчик используется для формирования статистики по запросам.
func (ql *QueryLogger) QueryCallback(scope *gorm.DB) {
	ql.mu.Lock()
	defer ql.mu.Unlock()
	ql.Records[scope.Statement.SQL.String()] += 1
}
