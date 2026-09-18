package migration

import (
	"fmt"
	"log/slog"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/dao"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

// MigrateDocSlug заполняет docs.slug у документов, созданных до появления
// адресов по названию.
type MigrateDocSlug struct {
	db *gorm.DB
}

func NewMigrateDocSlug(db *gorm.DB) *MigrateDocSlug {
	return &MigrateDocSlug{db: db}
}

func (m *MigrateDocSlug) Name() string {
	return "DocSlug"
}

func (m *MigrateDocSlug) CheckMigrate() (bool, error) {
	migrator := m.db.Migrator()
	if !migrator.HasTable(&dao.Doc{}) {
		return false, nil
	}
	// Миграции данных идут раньше AutoMigrate: колонку добавляем сами,
	// чтобы заполнить её тем же запуском.
	if !migrator.HasColumn(&dao.Doc{}, "Slug") {
		if err := migrator.AddColumn(&dao.Doc{}, "Slug"); err != nil {
			return false, fmt.Errorf("MigrateDocSlug add column: %w", err)
		}
	}
	var exists bool
	if err := m.db.Raw(`SELECT EXISTS (SELECT 1 FROM docs WHERE slug IS NULL OR slug = '')`).Scan(&exists).Error; err != nil {
		return false, fmt.Errorf("MigrateDocSlug checkMigrate: %w", err)
	}
	return exists, nil
}

// Execute заполняет адреса от корня вниз: потомок ждёт, пока адрес
// получит родитель.
func (m *MigrateDocSlug) Execute() error {
	type row struct {
		ID          uuid.UUID
		Title       string
		WorkspaceId uuid.UUID
		ParentSlug  string
	}
	total := 0
	for {
		var rows []row
		if err := m.db.Raw(`
			SELECT d.id, d.title, d.workspace_id, COALESCE(p.slug, '') AS parent_slug
			FROM docs d
			LEFT JOIN docs p ON p.id = d.parent_doc_id
			WHERE (d.slug IS NULL OR d.slug = '')
			  AND (d.parent_doc_id IS NULL OR (p.slug IS NOT NULL AND p.slug <> ''))
			LIMIT 500`).Scan(&rows).Error; err != nil {
			return fmt.Errorf("MigrateDocSlug select: %w", err)
		}
		if len(rows) == 0 {
			break
		}
		for _, r := range rows {
			slug, err := dao.UniqueDocSlug(m.db, r.WorkspaceId, r.ParentSlug, r.Title, r.ID)
			if err != nil {
				return fmt.Errorf("MigrateDocSlug slug: %w", err)
			}
			if err := m.db.Model(&dao.Doc{}).Where("id = ?", r.ID).UpdateColumn("slug", slug).Error; err != nil {
				return fmt.Errorf("MigrateDocSlug update: %w", err)
			}
		}
		total += len(rows)
	}
	slog.Info("Doc slugs filled", "docs", total)
	return nil
}
