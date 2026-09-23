package migration

import (
	"fmt"
	"log/slog"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/dao"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/types"
	"gorm.io/gorm"
)

// MigratePropertyTemplateRoles переводит флаг only_admin шаблонов кастомных
// полей в роли доступа reader_role/editor_role и удаляет старую колонку.
type MigratePropertyTemplateRoles struct {
	db *gorm.DB
}

func NewMigratePropertyTemplateRoles(db *gorm.DB) *MigratePropertyTemplateRoles {
	return &MigratePropertyTemplateRoles{db: db}
}

func (m *MigratePropertyTemplateRoles) Name() string {
	return "PropertyTemplateRoles"
}

// CheckMigrate: нужна, пока в таблице есть колонка only_admin. На свежей БД
// колонки ролей создаст AutoMigrate со значениями по умолчанию.
func (m *MigratePropertyTemplateRoles) CheckMigrate() (bool, error) {
	migrator := m.db.Migrator()
	tpl := &dao.ProjectPropertyTemplate{}
	return migrator.HasTable(tpl) && migrator.HasColumn(tpl, "only_admin"), nil
}

// Execute добавляет колонки ролей (миграции данных идут раньше AutoMigrate),
// переносит флаг и удаляет only_admin. Повторный запуск невозможен:
// CheckMigrate без only_admin вернёт false.
func (m *MigratePropertyTemplateRoles) Execute() error {
	migrator := m.db.Migrator()
	tpl := &dao.ProjectPropertyTemplate{}
	for _, field := range []string{"ReaderRole", "EditorRole"} {
		if migrator.HasColumn(tpl, field) {
			continue
		}
		if err := migrator.AddColumn(tpl, field); err != nil {
			return fmt.Errorf("MigratePropertyTemplateRoles add column %s: %w", field, err)
		}
	}

	res := m.db.Exec(`UPDATE project_property_templates
		SET reader_role = CASE WHEN only_admin THEN ? ELSE ? END,
		    editor_role = CASE WHEN only_admin THEN ? ELSE ? END`,
		types.AdminRole, types.GuestRole, types.AdminRole, types.GuestRole)
	if res.Error != nil {
		return fmt.Errorf("MigratePropertyTemplateRoles update: %w", res.Error)
	}

	if err := migrator.DropColumn(tpl, "only_admin"); err != nil {
		return fmt.Errorf("MigratePropertyTemplateRoles drop column: %w", err)
	}
	slog.Info("Property template roles migrated", "templates", res.RowsAffected)
	return nil
}
