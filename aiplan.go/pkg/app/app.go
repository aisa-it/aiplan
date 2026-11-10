// Пакет app — запуск приложения целиком: конфиг, миграция схемы, подключение
// к базе, первый пользователь, сборка и запуск сервера.
//
// Вынесен из cmd/aiplan, чтобы сборка с подключённым движком жила в своём
// модуле и запускала ядро одним вызовом Run, не копируя порядок запуска:
//
//	func main() {
//		opts := app.Options{Version: version, Engine: myengine.New()}
//		opts.BindFlags(flag.CommandLine)
//		flag.Parse()
//		if err := app.Run(context.Background(), opts); err != nil {
//			slog.Error("AIPlan fail", "err", err)
//			os.Exit(1)
//		}
//	}
package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/config"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/dao"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/engine"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/gormlogger"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/limiter"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/migration"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/server"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/utils"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// devVersion — версия сборки без -ldflags: цветной баннер и текстовый лог по умолчанию.
const devVersion = "DEV"

// Порог, после которого запрос к базе попадает в лог как медленный.
const slowQueryThreshold = 4 * time.Second

// Options — параметры запуска. Нулевое значение — штатный запуск ядра:
// конфиг из окружения, миграция включена, параметры запросов в логе скрыты.
type Options struct {
	// Version — версия сборки, обычно из -ldflags "-X main.version=...".
	// Пустая строка считается сборкой разработчика.
	Version string
	// ConfigPath — путь к json-конфигу; пусто — конфиг только из окружения.
	ConfigPath string
	// NoMigration отключает миграцию схемы перед стартом.
	NoMigration bool
	// NoTranslate отключает перевод ошибок базы в ошибки gorm.
	NoTranslate bool
	// ShowQueryParams пишет параметры запросов в лог открыто.
	// По умолчанию они скрыты.
	ShowQueryParams bool
	// Trace включает подробный лог и трассировку SQL.
	Trace bool

	// Engine — движок правил. nil — используется движок ядра.
	Engine engine.Engine
}

// BindFlags регистрирует штатные флаги командной строки ядра, чтобы у сборок
// с другим движком они назывались так же и подходили те же манифесты запуска.
func (o *Options) BindFlags(fs *flag.FlagSet) {
	fs.BoolVar(&o.NoTranslate, "noTranslate", o.NoTranslate, "Turn off BD errors translate")
	fs.BoolVar(&o.NoMigration, "noMigration", o.NoMigration, "Turn off DB migration")
	fs.StringVar(&o.ConfigPath, "configPath", o.ConfigPath, "Path of json config file, empty if disabled configoration via json file")
	fs.BoolVar(&o.Trace, "trace", o.Trace, "Verbose logs and sql trace")
	fs.BoolFunc("paramQueries", "Mask queries params in log (default true)", func(s string) error {
		mask, err := strconv.ParseBool(s)
		if err != nil {
			return err
		}
		o.ShowQueryParams = !mask
		return nil
	})
}

// Run запускает приложение и блокируется до остановки сервера.
func Run(ctx context.Context, opts Options) error {
	if opts.Version == "" {
		opts.Version = devVersion
	}

	PrintBanner(opts.Version)

	cfg := config.ReadConfig(opts.ConfigPath)
	dao.SetConfig(cfg)

	if opts.Trace {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}

	// Set prod log format
	if opts.Version != devVersion {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{})))
	}

	limiter.Init(cfg)

	slog.Info("AIPlan start.")

	// check default email config
	if cfg.DefaultUserEmail == "" {
		return errors.New("default email not preset")
	}

	db, err := prepareDB(cfg, opts)
	if err != nil {
		return err
	}

	//nolint:contextcheck // server.New контекст не принимает, остановка идёт через Run
	srv, err := server.New(server.Options{
		Config:       cfg,
		DB:           db,
		Version:      opts.Version,
		RunMigration: true,
		Engine:       opts.Engine,
	})
	if err != nil {
		return fmt.Errorf("init server: %w", err)
	}

	if err := srv.Run(ctx); err != nil {
		return fmt.Errorf("server: %w", err)
	}
	return nil
}

// prepareDB мигрирует схему, открывает основное подключение, ставит триггеры
// и создаёт первого пользователя.
func prepareDB(cfg *config.Config, opts Options) (*gorm.DB, error) {
	if !opts.NoMigration {
		if err := migrateSchema(cfg, opts); err != nil {
			return nil, err
		}
	}

	db, err := openDB(cfg, opts)
	if err != nil {
		return nil, err
	}

	if err := migration.CreateTriggers(db); err != nil {
		return nil, fmt.Errorf("create DB triggers: %w", err)
	}

	if err := ensureDefaultUser(db, cfg); err != nil {
		return nil, err
	}
	return db, nil
}

// gormConfig — общие настройки gorm для обоих подключений.
func gormConfig(opts Options) *gorm.Config {
	return &gorm.Config{
		TranslateError: !opts.NoTranslate,
		Logger:         gormlogger.NewGormLogger(slog.Default(), slowQueryThreshold, !opts.ShowQueryParams),
	}
}

// openDB создаёт основное подключение с prepared statements и настраивает пул.
func openDB(cfg *config.Config, opts Options) (*gorm.DB, error) {
	db, err := gorm.Open(utils.NewPostgresUUIDDialector(postgres.Config{
		DSN:                  cfg.DatabaseDSN,
		PreferSimpleProtocol: false, // используем prepared statements для производительности
	}), gormConfig(opts))
	if err != nil {
		return nil, fmt.Errorf("init DB connection: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("set settings to conn pool: %w", err)
	}
	sqlDB.SetMaxOpenConns(70)
	sqlDB.SetMaxIdleConns(30)
	sqlDB.SetConnMaxLifetime(time.Minute * 10)
	sqlDB.SetConnMaxIdleTime(time.Minute * 5)

	return db, nil
}

// ensureDefaultUser создаёт первого пользователя в пустой базе.
func ensureDefaultUser(db *gorm.DB, cfg *config.Config) error {
	var usersExist bool
	if err := db.Model(&dao.User{}).
		Select("EXISTS(?)",
			db.Model(&dao.User{}).Select("1"),
		).
		Find(&usersExist).Error; err != nil {
		return fmt.Errorf("count users in DB: %w", err)
	}

	if !usersExist {
		slog.Info("Creating default user", "email", cfg.DefaultUserEmail)
		dao.AddDefaultUser(db, cfg.DefaultUserEmail)
	}
	return nil
}

// PrintBanner выводит заголовок приложения с версией и ссылкой на сайт.
func PrintBanner(version string) {
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
	if version == devVersion {
		formattedVersion = colorYellow + version + colorReset
	}

	fmt.Printf(banner, formattedVersion, colorBlue+"https://aisa.ru"+colorReset)
}
