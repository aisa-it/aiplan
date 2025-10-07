// Обработка и отправка уведомлений пользователям.
//
// Основные возможности:
//   - Генерация HTML-сообщений для уведомлений.
//   - Интеграция с базой данных для получения данных о пользователях и событиях.
//   - Отправка уведомлений по электронной почте.
package notifications

import (
	"fmt"
	"os"
	"testing"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/config"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var es *EmailService

func setup() {
	cfg := config.ReadConfig()
	db, _ := gorm.Open(postgres.New(postgres.Config{
		DSN:                  cfg.DatabaseDSN,
		PreferSimpleProtocol: false,
	}))

	es = NewEmailService(cfg, db)
	es.Close()
}

func TestGenHTML(t *testing.T) {
	setup()

	var activities []dao.IssueActivity
	if err := es.db.Preload("Issue").
		Preload("Actor").
		Preload("Project").
		Preload("Issue.Workspace").
		Preload("Issue.Author").
		Preload("Issue.Assignees").
		Preload("Issue.State").
		Preload("Issue.Parent").
		Preload("Issue.Project").
		Preload("Issue.Parent.Project").
		Order("created_at").
		Where("issue_id = ?", "ba4abdb9-d1e4-49ed-a7ec-5914b671e7b8").
		Find(&activities).Error; err != nil {
		fmt.Println(err)
		t.Fail()
	}

	data, _, err := getIssueNotificationHTML(es.db, activities, activities[0].Actor)
	if err != nil {
		fmt.Println(err)
		t.Fail()
	}
	os.WriteFile("test.html", []byte(data), 0644)
}
