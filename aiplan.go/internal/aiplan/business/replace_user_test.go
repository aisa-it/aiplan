package business

import (
	"fmt"
	"testing"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/config"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/gofrs/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestPopulateFKs(t *testing.T) {
	cfg := config.ReadConfig()

	db, _ := gorm.Open(postgres.New(postgres.Config{
		DSN: cfg.DatabaseDSN,
	}), &gorm.Config{TranslateError: true})
	b := Business{db: db}

	if err := b.PopulateUserFKs(); err != nil {
		t.Fatal(err)
	}
	fmt.Println(userFKs)
}

func TestReplace(t *testing.T) {
	cfg := config.ReadConfig()

	db, _ := gorm.Open(postgres.New(postgres.Config{
		DSN: cfg.DatabaseDSN,
	}), &gorm.Config{TranslateError: true})
	//db = db.Debug()

	b := Business{db: db}

	if err := b.PopulateUserFKs(); err != nil {
		t.Fatal(err)
	}

	origUserId := uuid.Must(uuid.FromString("bb3828d2-1aa6-451d-9666-43a9f7aa0939"))
	newUserId := uuid.Must(uuid.FromString("44361aa5-b325-48bf-8c10-e9477615d219"))

	if err := b.ReplaceUser(b.db, origUserId, newUserId); err != nil {
		t.Fatal(err)
	}

	if err := db.Unscoped().Where("id = ?", origUserId).Delete(&dao.User{}).Error; err != nil {
		t.Fatal(err)
	}
}
