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

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"sheff.online/aiplan/internal/aiplan"
	"sheff.online/aiplan/internal/aiplan/config"
	"sheff.online/aiplan/internal/aiplan/dao"
	"sheff.online/aiplan/internal/aiplan/gormlogger"

	"ariga.io/atlas-go-sdk/atlasexec"
)

var version string = "DEV"

var models = []any{&dao.CommentReaction{}, &dao.DeferredNotifications{}, &dao.Doc{}, &dao.DocActivity{}, &dao.DocAttachment{}, &dao.DocComment{}, &dao.DocCommentReaction{}, &dao.DocEditor{}, &dao.DocFavorites{}, &dao.DocReader{}, &dao.DocWatcher{}, &dao.EntityActivity{}, &dao.Estimate{}, &dao.EstimatePoint{}, &dao.FileAsset{}, &dao.Form{}, &dao.FormActivity{}, &dao.FormAnswer{}, &dao.FormAttachment{}, &dao.ImportedProject{}, &dao.Issue{}, &dao.IssueActivity{}, &dao.IssueAssignee{}, &dao.IssueAttachment{}, &dao.IssueBlocker{}, &dao.IssueComment{}, &dao.IssueDescriptionLock{}, &dao.IssueLabel{}, &dao.IssueLink{}, &dao.IssueProperty{}, &dao.IssueTemplate{}, &dao.IssueWatcher{}, &dao.Label{}, &dao.LinkedIssues{}, &dao.Project{}, &dao.ProjectActivity{}, &dao.ProjectFavorites{}, &dao.ProjectMember{}, &dao.ReleaseNote{}, &dao.RootActivity{}, &dao.RulesLog{}, &dao.SearchFilter{}, &dao.SessionsReset{}, &dao.State{}, &dao.Tariffication{}, &dao.Team{}, &dao.TeamMembers{}, &dao.Template{}, &dao.User{}, &dao.UserFeedback{}, &dao.UserNotifications{}, &dao.Workspace{}, &dao.WorkspaceActivity{}, &dao.WorkspaceBackup{}, &dao.WorkspaceFavorites{}, &dao.WorkspaceMember{}}

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

	slog.Info("AIPlan start.")

	// check default email config
	if cfg.DefaultUserEmail == "" {
		slog.Error("Default email not preset")
		os.Exit(1)
	}

	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN:                  cfg.DatabaseDSN,
		PreferSimpleProtocol: false, // disables implicit prepared statement usage
	}), &gorm.Config{
		TranslateError: !*noTranslateFlag,
		Logger:         gormlogger.NewGormLogger(slog.Default(), time.Second, *paramQueries),
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

	if !*noMigration {
		slog.Info("Migrate models without relations")
		db.Config.DisableForeignKeyConstraintWhenMigrating = true
		if err := db.AutoMigrate(models...); err != nil {
			slog.Error("Migrate model error", "err", err)
		}
		db.Config.DisableForeignKeyConstraintWhenMigrating = false

		slog.Info("Migrate models with relations")
		if err := db.AutoMigrate(models...); err != nil {
			slog.Error("Migrate model error", "err", err)
		}
	}

	if err := CreateTriggers(db); err != nil {
		slog.Error("Fail create DB triggers", "err", err)
		os.Exit(1)
	}

	var usersExist bool
	if err := db.Model(&dao.User{}).Select("count(*) > 0").Find(&usersExist).Error; err != nil {
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
