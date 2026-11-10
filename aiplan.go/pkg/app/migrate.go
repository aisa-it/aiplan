package app

import (
	"fmt"
	"log/slog"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/config"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/dao"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/migration"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/utils"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// migrateSchema приводит схему базы к моделям ядра на отдельном подключении:
// перевод текстовых идентификаторов в uuid, дедупликация, AutoMigrate.
// Модели движка мигрирует сервер — они известны только после Engine.Init.
func migrateSchema(cfg *config.Config, opts Options) error {
	// Для миграций используем PreferSimpleProtocol: true чтобы избежать ошибок с кешированными планами
	// когда структура таблиц меняется (например, добавляются новые JSONB поля)
	db, err := gorm.Open(utils.NewPostgresUUIDDialector(postgres.Config{
		DSN:                  cfg.DatabaseDSN,
		PreferSimpleProtocol: true, // отключаем prepared statements для миграции
	}), gormConfig(opts))
	if err != nil {
		return fmt.Errorf("init DB connection for migration: %w", err)
	}

	if err := migrateUUIDColumns(db); err != nil {
		return fmt.Errorf("UUID migration: %w", err)
	}

	// Deduplicate sprint_folders before adding unique index
	if err := migration.DeduplicateSprintFolders(db); err != nil {
		return fmt.Errorf("sprint folder deduplication: %w", err)
	}

	if err := autoMigrateModels(db); err != nil {
		return fmt.Errorf("AutoMigrate: %w", err)
	}
	slog.Info("AutoMigrate completed successfully")

	// Закрываем соединение для миграции
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("get SQL DB for migration: %w", err)
	}
	if err := sqlDB.Close(); err != nil {
		return fmt.Errorf("close migration DB connection: %w", err)
	}
	slog.Info("Migration database connection closed")
	return nil
}

// migrateUUIDColumns переводит все UUID-поля из text в uuid одной транзакцией.
func migrateUUIDColumns(db *gorm.DB) error {
	slog.Info("Starting UUID migration in single transaction")

	// Auto-generate UUID columns from DAO models
	allColumns := utils.GetUUIDColumnsFromModels(dao.Models())
	slog.Info("Auto-detected UUID columns from models", "count", len(allColumns))

	// Add many2many fields migration
	allColumns = append(allColumns,
		utils.ColumnInfo{
			Table:       "user_search_filters",
			Column:      "user_id",
			CurrentType: "text",
		},
		utils.ColumnInfo{
			Table:       "user_search_filters",
			Column:      "search_filter_id",
			CurrentType: "text",
		},
	)

	// Filter columns that need migration (exclude uuid, bytea, non-existent)
	columnsToMigrate, err := utils.FilterMigratableColumns(db, allColumns)
	if err != nil {
		return fmt.Errorf("failed to filter migratable columns: %w", err)
	}
	slog.Info("Filtered migratable columns", "count", len(columnsToMigrate), "skipped", len(allColumns)-len(columnsToMigrate))

	// Пропускаем UUID миграцию если нет колонок для миграции
	if len(columnsToMigrate) == 0 {
		slog.Info("No UUID columns need migration, skipping UUID migration steps")
		return nil
	}

	// Execute all migrations in a single transaction
	err = db.Transaction(func(tx *gorm.DB) error {
		return replaceColumnTypes(tx, columnsToMigrate)
	})
	if err != nil {
		return err
	}
	slog.Info("UUID migration completed successfully")
	return nil
}

// replaceColumnTypes — шаги перевода колонок в uuid внутри транзакции.
func replaceColumnTypes(tx *gorm.DB, columns []utils.ColumnInfo) error {
	// Step 1: Drop all generated columns (they block type changes)
	slog.Info("Dropping all generated columns")
	if err := dao.DropAllGeneratedColumns(tx); err != nil {
		return fmt.Errorf("failed to drop generated columns: %w", err)
	}
	slog.Info("All generated columns dropped")

	// Step 2: Clean orphaned foreign keys (пока FK ещё существуют в БД)
	slog.Info("Cleaning orphaned foreign keys")
	if err := dao.CleanAllOrphanedForeignKeys(tx); err != nil {
		return fmt.Errorf("failed to clean all orphaned foreign keys: %w", err)
	}
	slog.Info("All orphaned foreign keys cleaned")

	// Step 3: Drop all FK constraints
	slog.Info("Dropping all foreign key constraints")
	if err := dao.DropAllForeignKeys(tx); err != nil {
		return fmt.Errorf("failed to drop foreign keys: %w", err)
	}
	slog.Info("All foreign key constraints dropped")

	// Step 4: Drop all CHECK constraints (including linked_issues check:id1<id2)
	slog.Info("Dropping all check constraints")
	if err := dao.DropAllCheckConstraints(tx); err != nil {
		return fmt.Errorf("failed to drop check constraints: %w", err)
	}
	slog.Info("All check constraints dropped")

	// Step 5: Clean invalid UUIDs (set to NULL)
	slog.Info("Cleaning invalid UUID values", "count", len(columns))
	for _, col := range columns {
		if err := dao.CleanInvalidUUIDs(tx, col.Table, col.Column); err != nil {
			return fmt.Errorf("failed to clean invalid UUIDs in %s.%s: %w", col.Table, col.Column, err)
		}
	}
	slog.Info("All invalid UUIDs cleaned")

	// Step 6: Replace column types
	slog.Info("Replacing column types", "count", len(columns))
	for _, col := range columns {
		slog.Info("Migrating column", "table", col.Table, "column", col.Column, "currentType", col.CurrentType)
		if err := utils.ReplaceColumnTypeWithCast(tx, col, "uuid"); err != nil {
			return fmt.Errorf("failed to replace column %s.%s (type %s): %w", col.Table, col.Column, col.CurrentType, err)
		}
	}
	slog.Info("All column types replaced successfully")

	cleanSelfReferencingFKs(tx)

	return nil
}

// cleanSelfReferencingFKs — шаг 7: чистит известные ссылки таблицы на саму себя
// (они могли не существовать в БД). Ошибки не прерывают миграцию.
func cleanSelfReferencingFKs(tx *gorm.DB) {
	slog.Info("Cleaning known self-referencing foreign keys")
	selfRefFKs := []struct {
		Table            string
		Column           string
		ReferencedTable  string
		ReferencedColumn string
	}{
		{"issues", "parent_id", "issues", "id"},
		{"docs", "parent_doc_id", "docs", "id"},
		{"doc_fields", "parent_id", "doc_fields", "id"},
	}
	for _, fk := range selfRefFKs {
		if err := dao.CleanOrphanedForeignKeys(tx, fk.Table, fk.Column, fk.ReferencedTable, fk.ReferencedColumn); err != nil {
			slog.Warn("Failed to clean self-referencing FK", "table", fk.Table, "column", fk.Column, "err", err)
		}
	}
	slog.Info("Self-referencing foreign keys cleaned")
}

// autoMigrateModels создаёт таблицы моделей ядра: сначала без связей, затем со связями.
func autoMigrateModels(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		slog.Info("Migrate models without relations")
		tx.DisableForeignKeyConstraintWhenMigrating = true
		if err := tx.AutoMigrate(dao.Models()...); err != nil {
			return fmt.Errorf("failed to auto-migrate models without relations: %w", err)
		}
		tx.DisableForeignKeyConstraintWhenMigrating = false

		slog.Info("Migrate models with relations")
		if err := tx.AutoMigrate(dao.Models()...); err != nil {
			return fmt.Errorf("failed to auto-migrate models with relations: %w", err)
		}
		slog.Info("All models migrated successfully")

		return nil
	})
}
