// Пакет предоставляет функциональность для поиска и управления задачами в системе AIPlan.
//
// Основные возможности:
//   - Поиск задач по различным критериям (ключевые слова, автор, порядок).
//   - Получение списка задач с возможностью пагинации.
//   - Взаимодействие с базой данных задач.
package aiplan

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aisa-it/aiplan/internal/aiplan/config"
	"github.com/aisa-it/aiplan/internal/aiplan/dao"
	"github.com/labstack/echo/v4"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	e     *echo.Echo
	s     *Services
	admin *dao.User
)

func TestMain(m *testing.M) {
	cfg := config.ReadConfig()

	e = echo.New()

	db, _ := gorm.Open(postgres.New(postgres.Config{DSN: cfg.DatabaseDSN}), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	s = &Services{db: db}

	dao.Config = cfg
	db.Where("email = 'test@aiplan.ru'").First(&admin)

	m.Run()
}

func TestSearch(t *testing.T) {
	b, _ := json.Marshal(map[string]string{
		"search_query": "ускоритель",
	})

	req := httptest.NewRequest("GET", "/api/auth/issues/search/?order_by=author&light=true", bytes.NewBuffer(b))
	req.Header.Add("Content-type", "application/json")
	rec := httptest.NewRecorder()
	c := getAuthContext(req, rec)

	st := time.Now()
	if err := s.getIssueList(c); err != nil {
		t.Fatal(err)
	}
	fmt.Println(time.Since(st))

	var data map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &data)
	fmt.Println(data["count"], data["limit"], data["offset"], len(data["issues"].([]interface{})))
}

func getAuthContext(req *http.Request, rec http.ResponseWriter) echo.Context {
	c := e.NewContext(req, rec)
	return AuthContext{c, admin, nil, nil, false}
}
