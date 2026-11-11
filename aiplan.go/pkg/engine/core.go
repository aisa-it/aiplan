package engine

import (
	tracker "github.com/aisa-it/aiplan/aiplan.go/pkg/activity-tracker"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/business"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/config"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/cronmanager"
	filestorage "github.com/aisa-it/aiplan/aiplan.go/pkg/file-storage"
	"github.com/labstack/echo/v4"
	mcpserver "github.com/mark3labs/mcp-go/server"
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
	// Notifications — отправка писем и уведомлений средствами ядра.
	Notifications() Notifier

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

	// RegisterActionRoutes добавляет роуты движка внутри пространства,
	// проекта или задачи. В отличие от RegisterRoutes, такие роуты проходят
	// ту же цепочку, что и роуты ядра: загрузка и проверка сущностей пути,
	// затем проверка права на действие через движок.
	RegisterActionRoutes(routes ...ActionRoute)

	// RegisterNotificationType регистрирует тип уведомления для очереди ядра:
	// factory возвращает указатель на структуру с json-тегами, в неё
	// разбирается payload записи. Повторная регистрация имени заменяет фабрику.
	RegisterNotificationType(name string, factory func() NotificationRenderer)

	// RegisterActions добавляет действия движка в область: они попадут в
	// наборы прав, отдаваемые наружу (ActionsFor).
	RegisterActions(area Area, actions ...Action)

	// RegisterMCPTools, RegisterMCPResources, RegisterMCPPrompts добавляют
	// инструменты, ресурсы и промпты движка в MCP-сервер ядра.
	RegisterMCPTools(tools ...mcpserver.ServerTool)
	RegisterMCPResources(resources ...mcpserver.ServerResource)
	RegisterMCPPrompts(prompts ...mcpserver.ServerPrompt)
}

// ActionRoute — роут движка в области пространства, проекта или задачи.
type ActionRoute struct {
	// Area — AreaWorkspace, AreaProject или AreaIssue: от какого корня
	// считается путь и какие сущности ядро загрузит до обработчика.
	Area   Area
	Method string
	// Path — путь относительно корня области, с ведущим слэшем:
	// "/reports/" в AreaProject даёт
	// /api/auth/workspaces/:workspaceSlug/projects/:projectId/reports/.
	Path string
	// Action — действие, право на которое ядро проверит перед обработчиком.
	// Действие движка должно быть объявлено через RegisterActions.
	Action  Action
	Handler echo.HandlerFunc
}
