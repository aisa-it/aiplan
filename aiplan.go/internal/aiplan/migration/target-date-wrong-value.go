package migration

import (
	"fmt"
	"log/slog"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

type MigrateActivityTargetDateUpdate struct {
	db *gorm.DB
}

func NewMigrateActivityTargetDateUpdate(db *gorm.DB) *MigrateActivityTargetDateUpdate {
	return &MigrateActivityTargetDateUpdate{
		db: db}
}

func (m *MigrateActivityTargetDateUpdate) CheckMigrate() (bool, error) {
	var exist bool

	if err := m.db.Model(&dao.IssueActivity{}).
		Select("EXISTS(?)",
			m.db.Model(&dao.IssueActivity{}).
				Select("1").
				Where("field = ? and old_value = ?", "target_date", "<nil>"),
		).
		Find(&exist).Error; err != nil {
		return false, fmt.Errorf("ActivityTargetDateUpdate checkMigrate: %s", err.Error())
	}
	return exist, nil
}

func (m *MigrateActivityTargetDateUpdate) Name() string {
	return "ActivityTargetDateUpdate"
}

func (m *MigrateActivityTargetDateUpdate) Execute() error {
	var activities []dao.IssueActivity

	if err := m.db.Where("field = ? and old_value = ?", "target_date", "<nil>").FindInBatches(&activities, 30, func(tx *gorm.DB, batch int) error {
		result := tx.Model(&dao.IssueActivity{}).Where("id IN ?", utils.SliceToSlice(&activities, func(t *dao.IssueActivity) uuid.UUID {
			return t.Id
		})).Update("old_value", nil)
		if result.Error != nil {
			return result.Error
		}
		slog.Info("ActivityTargetDateUpdate", "batch", batch, "rows", result.RowsAffected)
		return nil
	}).Error; err != nil {
		slog.Error("ActivityTargetDateUpdate", "error", err.Error())
	}
	return nil
}
