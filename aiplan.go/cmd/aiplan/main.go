// Основной пакет приложения AIPlan. Отвечает за запуск приложения, инициализацию базы данных, миграцию моделей, создание триггеров и запуск основного сервера приложения. Также содержит логику для работы с Atlas.
package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/config"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/gormlogger"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/limiter"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"ariga.io/atlas-go-sdk/atlasexec"
)

var version string = "DEV"

var models = []any{&dao.CommentReaction{}, &dao.DeferredNotifications{}, &dao.Doc{}, &dao.DocAccessRules{}, &dao.DocActivity{}, &dao.DocAttachment{}, &dao.DocComment{}, &dao.DocCommentReaction{}, &dao.DocFavorites{}, &dao.EntityActivity{}, &dao.Estimate{}, &dao.EstimatePoint{}, &dao.FileAsset{}, &dao.ForeignKey{}, &dao.Form{}, &dao.FormActivity{}, &dao.FormAnswer{}, &dao.FormAttachment{}, &dao.ImportedProject{}, &dao.Issue{}, &dao.IssueActivity{}, &dao.IssueAssignee{}, &dao.IssueAttachment{}, &dao.IssueBlocker{}, &dao.IssueComment{}, &dao.IssueDescriptionLock{}, &dao.IssueLabel{}, &dao.IssueLink{}, &dao.IssueProperty{}, &dao.IssueTemplate{}, &dao.IssueWatcher{}, &dao.Label{}, &dao.LinkedIssues{}, &dao.Project{}, &dao.ProjectActivity{}, &dao.ProjectFavorites{}, &dao.ProjectMember{}, &dao.ReleaseNote{}, &dao.RootActivity{}, &dao.RulesLog{}, &dao.SearchFilter{}, &dao.SessionsReset{}, &dao.Sprint{}, &dao.SprintActivity{}, &dao.SprintIssue{}, &dao.SprintViews{}, &dao.SprintWatcher{}, &dao.State{}, &dao.Team{}, &dao.TeamMembers{}, &dao.Template{}, &dao.User{}, &dao.UserFeedback{}, &dao.UserNotifications{}, &dao.Workspace{}, &dao.WorkspaceActivity{}, &dao.WorkspaceBackup{}, &dao.WorkspaceFavorites{}, &dao.WorkspaceMember{}}

//go:embed triggers.sql
var triggersSQL string

// main - Основная функция приложения, отвечающая за запуск приложения, инициализацию базы данных, миграцию моделей, создание триггеров и запуск основного сервера приложения. Также содержит логику для работы с Atlas.
// Функция принимает флаги командной строки для настройки поведения приложения.
// Возвращает:
//   - error: nil в случае успеха, ошибка в случае неудачи.
//
// Пример запуска: go run main.go --noTranslate --noMigration --trace
func main() {
	noTranslateFlag := flag.Bool("noTranslate", false, "Turn off BD errors translate")
	paramQueries := flag.Bool("paramQueries", true, "Mask queries params in log")
	noMigration := flag.Bool("noMigration", false, "Turn off DB migration")
	trace := flag.Bool("trace", false, "Verbose logs and sql trace")
	flag.Parse()

	PrintBanner()

	cfg := config.ReadConfig()
	dao.Config = cfg

	if *trace {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}

	// Set prod log format
	if version != "DEV" {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{})))
	}

	limiter.Init(cfg)

	slog.Info("AIPlan start.")

	// check default email config
	if cfg.DefaultUserEmail == "" {
		slog.Error("Default email not preset")
		os.Exit(1)
	}

	// Для миграций используем PreferSimpleProtocol: true чтобы избежать ошибок с кешированными планами
	// когда структура таблиц меняется (например, добавляются новые JSONB поля)
	dbForMigration, err := gorm.Open(utils.NewPostgresUUIDDialector(postgres.Config{
		DSN:                  cfg.DatabaseDSN,
		PreferSimpleProtocol: true, // отключаем prepared statements для миграции
	}), &gorm.Config{
		TranslateError: !*noTranslateFlag,
		Logger:         gormlogger.NewGormLogger(slog.Default(), time.Second*4, *paramQueries),
	})
	if err != nil {
		slog.Error("Fail init DB connection for migration", "err", err)
		os.Exit(1)
	}

	if !*noMigration {
		// Migrate all UUID fields from text to uuid type in a single transaction
		slog.Info("Starting UUID migration in single transaction")

		// Auto-generate UUID columns from DAO models
		allColumns := utils.GetUUIDColumnsFromModels(models)
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
		columnsToMigrate, err := utils.FilterMigratableColumns(dbForMigration, allColumns)
		if err != nil {
			slog.Error("Failed to filter migratable columns", "err", err)
			os.Exit(1)
		}
		slog.Info("Filtered migratable columns", "count", len(columnsToMigrate), "skipped", len(allColumns)-len(columnsToMigrate))

		// Пропускаем UUID миграцию если нет колонок для миграции
		if len(columnsToMigrate) > 0 {
			// Execute all migrations in a single transaction
			err = dbForMigration.Transaction(func(tx *gorm.DB) error {
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
				slog.Info("Cleaning invalid UUID values", "count", len(columnsToMigrate))
				for _, col := range columnsToMigrate {
					if err := dao.CleanInvalidUUIDs(tx, col.Table, col.Column); err != nil {
						return fmt.Errorf("failed to clean invalid UUIDs in %s.%s: %w", col.Table, col.Column, err)
					}
				}
				slog.Info("All invalid UUIDs cleaned")

				// Step 6: Replace column types
				slog.Info("Replacing column types", "count", len(columnsToMigrate))
				for _, col := range columnsToMigrate {
					slog.Info("Migrating column", "table", col.Table, "column", col.Column, "currentType", col.CurrentType)
					if err := utils.ReplaceColumnTypeWithCast(tx, col, "uuid"); err != nil {
						return fmt.Errorf("failed to replace column %s.%s (type %s): %w", col.Table, col.Column, col.CurrentType, err)
					}
				}
				slog.Info("All column types replaced successfully")

				// Step 7: Clean known self-referencing FK (они могли не существовать в БД)
				slog.Info("Cleaning known self-referencing foreign keys")
				selfRefFKs := []struct {
					Table            string
					Column           string
					ReferencedTable  string
					ReferencedColumn string
				}{
					{"issues", "parent_id", "issues", "id"},
					{"docs", "parent_id", "docs", "id"},
					{"doc_fields", "parent_id", "doc_fields", "id"},
				}
				for _, fk := range selfRefFKs {
					if err := dao.CleanOrphanedForeignKeys(tx, fk.Table, fk.Column, fk.ReferencedTable, fk.ReferencedColumn); err != nil {
						slog.Warn("Failed to clean self-referencing FK", "table", fk.Table, "column", fk.Column, "err", err)
					}
				}
				slog.Info("Self-referencing foreign keys cleaned")

				return nil
			})

			if err != nil {
				slog.Error("UUID migration failed", "err", err)
				os.Exit(1)
			}
			slog.Info("UUID migration completed successfully")
		} else {
			slog.Info("No UUID columns need migration, skipping UUID migration steps")
		}

		// Step 8: AutoMigrate models (в отдельной транзакции)
		err = dbForMigration.Transaction(func(tx *gorm.DB) error {
			slog.Info("Migrate models without relations")
			tx.DisableForeignKeyConstraintWhenMigrating = true
			if err := tx.AutoMigrate(models...); err != nil {
				return fmt.Errorf("failed to auto-migrate models without relations: %w", err)
			}
			tx.DisableForeignKeyConstraintWhenMigrating = false

			slog.Info("Migrate models with relations")
			if err := tx.AutoMigrate(models...); err != nil {
				return fmt.Errorf("failed to auto-migrate models with relations: %w", err)
			}
			slog.Info("All models migrated successfully")

			return nil
		})

		if err != nil {
			slog.Error("AutoMigrate failed", "err", err)
			os.Exit(1)
		}
		slog.Info("AutoMigrate completed successfully")

		// Закрываем соединение для миграции
		sqlDBMigration, err := dbForMigration.DB()
		if err != nil {
			slog.Error("Fail get SQL DB for migration", "err", err)
			os.Exit(1)
		}
		if err := sqlDBMigration.Close(); err != nil {
			slog.Error("Fail close migration DB connection", "err", err)
			os.Exit(1)
		}
		slog.Info("Migration database connection closed")
	}

	// Создаем основное подключение с prepared statements для production
	db, err := gorm.Open(utils.NewPostgresUUIDDialector(postgres.Config{
		DSN:                  cfg.DatabaseDSN,
		PreferSimpleProtocol: false, // используем prepared statements для производительности
	}), &gorm.Config{
		TranslateError: !*noTranslateFlag,
		Logger:         gormlogger.NewGormLogger(slog.Default(), time.Second*4, *paramQueries),
	})
	if err != nil {
		slog.Error("Fail init DB connection", "err", err)
		os.Exit(1)
	}

	sqlDB, err := db.DB()
	if err != nil {
		slog.Error("Fail set settings to conn pool", "err", err)
		os.Exit(1)
	}
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetMaxIdleConns(50)
	sqlDB.SetConnMaxLifetime(time.Hour)
	sqlDB.SetConnMaxIdleTime(time.Minute * 15)

	if err := CreateTriggers(db); err != nil {
		slog.Error("Fail create DB triggers", "err", err)
		os.Exit(1)
	}

	var usersExist bool
	if err := db.Model(&dao.User{}).
		Select("EXISTS(?)",
			db.Model(&dao.User{}).Select("1"),
		).
		Find(&usersExist).Error; err != nil {
		slog.Error("Fail count users in DB", "err", err)
		os.Exit(1)
	}

	if !usersExist {
		slog.Info("Creating default user", "email", cfg.DefaultUserEmail)
		dao.AddDefaultUser(db, cfg.DefaultUserEmail)
	}

	aiplan.Server(db, cfg, version)
}

// PrintBanner выводит заголовок приложения с версией и ссылкой на сайт. Использует переменные окружения и версию приложения для формирования текста заголовка. Не принимает параметров и не возвращает значений.
//
// Функция использует color codes для выделения версии и ссылки.
//
// Возвращает:
//   - error: nil в случае успеха, ошибка в случае неудачи (невозможно распечатать заголовок). Однако, в данном случае, функция не может вернуть ошибку, так как не выполняет никаких операций, кроме вывода текста на консоль.
//
// Примеры использования:
//   - В основном коде, функция вызывается в начале выполнения приложения для вывода информативного заголовка.
func PrintBanner() {
	banner := `
          _____ _____  _
    /\   |_   _|  __ \| |
   /  \    | | | |__) | | __ _ _ __
  / /\ \   | | |  ___/| |/ _  | '_ \
 / ____ \ _| |_| |    | | (_| | | | |
/_/    \_\_____|_|    |_|\__,_|_| |_| %s
High performance, minimalist project management tool
%s
----------------------------------------------------
`
	colorReset := "\033[0m"

	colorYellow := "\033[33m"
	colorBlue := "\033[34m"

	formattedVersion := version
	if version == "DEV" {
		formattedVersion = colorYellow + version + colorReset
	}

	fmt.Printf(banner, formattedVersion, colorBlue+"https://aisa.ru"+colorReset)
}

// AtlasMigration Применяет изменения схемы базы данных через Atlas.  Использует указанные параметры конфигурации для подключения к базе данных и применения изменений схемы.  Обрабатывает ошибки и выводит информацию об успешно примененных изменениях.  Необходимо, чтобы утилита 'atlas' была доступна в системе.
//
// Параметры:
//   - cfg: Конфигурация приложения, содержащая параметры подключения к базе данных.
//
// Возвращает:
//   - error: Ошибка, если при возникновении проблем с подключением к Atlas или при применении изменений схемы.
func AtlasMigration(cfg *config.Config) error {
	_, err := exec.LookPath("atlas")
	if err != nil {
		slog.Warn("Atlas cli exec not found in system, skip schema applying", "err", err)
		return nil
	}

	client, err := atlasexec.NewClient(".", "atlas")
	if err != nil {
		return err
	}
	res, err := client.SchemaApply(context.Background(), &atlasexec.SchemaApplyParams{
		URL: cfg.DatabaseDSN,
		//DevURL:      cfg.DatabaseEmptyDSN,
		To:          "file://schema.sql",
		AutoApprove: true,
	})

	if res != nil && res.Changes.Error != nil {
		fmt.Println("Error statement:")
		fmt.Printf("%s:\n%s\n", res.Changes.Error.Text, res.Changes.Error.Stmt)
	}

	if err != nil {
		return err
	}

	if len(res.Changes.Applied) > 0 {
		fmt.Println("Applied changes:")
		for _, change := range res.Changes.Applied {
			fmt.Println(change)
		}
	}

	return nil
}

// CreateTriggers Создает триггеры в базе данных на основе SQL-скрипта.
//
// Парамметры:
//   - db: Указатель на объект базы данных GORM.
//
// Возвращает:
//   - error: Ошибка, если возникли проблемы при выполнении SQL-скрипта.
func CreateTriggers(db *gorm.DB) error {
	slog.Info("Create DB triggers")
	return db.Exec(triggersSQL).Error
}
