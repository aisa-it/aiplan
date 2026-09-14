package engine

import (
	tracker "github.com/aisa-it/aiplan/aiplan.go/pkg/activity-tracker"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/business"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/config"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/cronmanager"
	filestorage "github.com/aisa-it/aiplan/aiplan.go/pkg/file-storage"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

// Core — то, что ядро предоставляет движку.
//
// Методы Register* допустимо вызывать только из Engine.Init: после старта
// сервера регистрация игнорируется, чтобы не менять конфигурацию на ходу.
type Core interface {
	DB() *gorm.DB
	Config() *config.Config
	Version() string
	Storage() filestorage.FileStorage
	Business() *business.Business
	Tracker() *tracker.SnapshotTracker

	// RegisterModels добавляет модели движка в AutoMigrate.
	RegisterModels(models ...any)
	// RegisterActivityHandler подписывает обработчик на события активности.
	RegisterActivityHandler(h tracker.ActHandler)
	// RegisterCronJob добавляет периодическую задачу. Имя должно быть
	// уникальным: повторная регистрация под тем же именем заменяет задачу.
	RegisterCronJob(name string, job cronmanager.Job)
	// RegisterRoutes добавляет собственные роуты движка.
	// api — группа без авторизации, auth — с авторизацией.
	RegisterRoutes(fn func(api, auth *echo.Group))
}
