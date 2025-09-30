package business

import (
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"sheff.online/aiplan/internal/aiplan/config"
	"sheff.online/aiplan/internal/aiplan/dao"
)

func TestReplace(t *testing.T) {
	cfg := config.ReadConfig()

	db, _ := gorm.Open(postgres.New(postgres.Config{
		DSN: cfg.DatabaseDSN,
	}), &gorm.Config{TranslateError: true})
	db = db.Debug()

	b := Business{db: db}

	if err := b.ReplaceUser("b004906a-009a-4ab0-b06c-fe3979f75295", "f2930ffe-a328-4164-a7ff-3e5de4c77755"); err != nil {
		t.Fatal(err)
	}

	if err := db.Unscoped().Where("id = ?", "b004906a-009a-4ab0-b06c-fe3979f75295").Delete(&dao.User{}).Error; err != nil {
		t.Fatal(err)
	}
}
