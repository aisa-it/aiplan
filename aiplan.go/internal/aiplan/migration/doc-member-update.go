package migration

import (
	"fmt"
	"log/slog"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"gorm.io/gorm"
)

type MigrateDocUpdateById struct {
	db *gorm.DB
}

func NewMigrateDocUpdateId(db *gorm.DB) *MigrateDocUpdateById {
	return &MigrateDocUpdateById{db: db}
}

func (m MigrateDocUpdateById) CheckMigrate() (bool, error) {
	var exists bool

	if err := m.db.Model(&dao.Doc{}).
		Select("EXISTS(?)",
			m.db.Model(&dao.Doc{}).
				Select("1").
				Joins("LEFT JOIN users u ON docs.updated_by_id = u.id").
				Where("docs.updated_by_id IS NOT NULL AND u.id IS NULL"),
		).
		Find(&exists).Error; err != nil {
		return false, fmt.Errorf("MigrateDocUpdateById checkMigrate: %s", err.Error())
	}

	return exists, nil
}

func (m MigrateDocUpdateById) Name() string {
	return "MigrateDocUpdateById"
}

func (m MigrateDocUpdateById) Execute() error {
	batchSize := 10
	var docs []dao.Doc

	if err := m.db.
		Model(&dao.Doc{}).
		Select("docs.id, docs.created_by_id").
		Joins("LEFT JOIN users u ON docs.updated_by_id = u.id").
		Where("docs.updated_by_id IS NOT NULL").
		Where("u.id IS NULL").
		Where("docs.created_by_id IS NOT NULL").
		FindInBatches(&docs, batchSize, func(tx *gorm.DB, batch int) error {
			for _, doc := range docs {
				if err := tx.Model(&dao.Doc{}).
					Where("id = ?", doc.ID).
					UpdateColumn("updated_by_id", doc.CreatedById).Error; err != nil {
					return fmt.Errorf("failed to update doc %d in batch %d: %s", doc.ID, batch, err)
				}
			}

			slog.Info("Migrate data ok", "name", m.Name(), "batch", batch, "count", len(docs))
			return nil
		}).Error; err != nil {
		return fmt.Errorf("failed to execute migration in batches: %w", err)
	}

	return nil
}
