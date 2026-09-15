package server

import (
	"context"
	"fmt"
	"log/slog"

	tracker "github.com/aisa-it/aiplan/aiplan.go/pkg/activity-tracker"
	apicontext "github.com/aisa-it/aiplan/aiplan.go/pkg/api-context"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/business"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/config"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/cronmanager"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/engine"
	filestorage "github.com/aisa-it/aiplan/aiplan.go/pkg/file-storage"
	"github.com/labstack/echo/v4"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"gorm.io/gorm"
)

// coreAdapter отдаёт движку доступ к ядру и копит то, что движок
// регистрирует: модели, обработчики событий, cron-задачи и роуты.
// Накопленное применяется ядром на своих шагах сборки, поэтому Init
// вызывается до миграции схемы и до регистрации роутов.
type coreAdapter struct {
	db      *gorm.DB
	cfg     *config.Config
	version string
	storage filestorage.FileStorage
	bl      *business.Business
	st      *tracker.SnapshotTracker

	// closed запрещает регистрацию после Init: менять состав роутов или
	// задач на работающем сервере нельзя.
	closed bool

	models      []any
	actHandlers []tracker.ActHandler
	cronJobs    cronmanager.JobRegistry
	routeFns    []func(api, auth *echo.Group)

	mcpTools     []mcpserver.ServerTool
	mcpResources []mcpserver.ServerResource
	mcpPrompts   []mcpserver.ServerPrompt
}

func (c *coreAdapter) DB() *gorm.DB                      { return c.db }
func (c *coreAdapter) Config() *config.Config            { return c.cfg }
func (c *coreAdapter) Version() string                   { return c.version }
func (c *coreAdapter) Storage() filestorage.FileStorage  { return c.storage }
func (c *coreAdapter) Business() *business.Business      { return c.bl }
func (c *coreAdapter) Tracker() *tracker.SnapshotTracker { return c.st }

func (c *coreAdapter) RegisterModels(models ...any) {
	if c.rejectAfterInit("models") {
		return
	}
	c.models = append(c.models, models...)
}

func (c *coreAdapter) RegisterActivityHandler(h tracker.ActHandler) {
	if h == nil || c.rejectAfterInit("activity handler") {
		return
	}
	c.actHandlers = append(c.actHandlers, h)
}

func (c *coreAdapter) RegisterCronJob(name string, job cronmanager.Job) {
	if c.rejectAfterInit("cron job") {
		return
	}
	if c.cronJobs == nil {
		c.cronJobs = cronmanager.JobRegistry{}
	}
	c.cronJobs[name] = job
}

func (c *coreAdapter) RegisterRoutes(fn func(api, auth *echo.Group)) {
	if fn == nil || c.rejectAfterInit("routes") {
		return
	}
	c.routeFns = append(c.routeFns, fn)
}

func (c *coreAdapter) RegisterActions(area engine.Area, actions ...engine.Action) {
	if c.rejectAfterInit("actions") {
		return
	}
	engine.RegisterActions(area, actions...)
}

func (c *coreAdapter) RegisterMCPTools(tools ...mcpserver.ServerTool) {
	if c.rejectAfterInit("mcp tools") {
		return
	}
	c.mcpTools = append(c.mcpTools, tools...)
}

func (c *coreAdapter) RegisterMCPResources(resources ...mcpserver.ServerResource) {
	if c.rejectAfterInit("mcp resources") {
		return
	}
	c.mcpResources = append(c.mcpResources, resources...)
}

func (c *coreAdapter) RegisterMCPPrompts(prompts ...mcpserver.ServerPrompt) {
	if c.rejectAfterInit("mcp prompts") {
		return
	}
	c.mcpPrompts = append(c.mcpPrompts, prompts...)
}

// rejectAfterInit отклоняет и логирует регистрацию после Init.
func (c *coreAdapter) rejectAfterInit(what string) bool {
	if c.closed {
		slog.Warn("Engine registration after init ignored", "what", what)
	}
	return c.closed
}

// initEngine инициализирует движок и возвращает накопленные регистрации.
func initEngine(ctx context.Context, eng engine.Engine, core *coreAdapter) error {
	if err := eng.Init(ctx, core); err != nil {
		return fmt.Errorf("init engine %q: %w", eng.Name(), err)
	}
	core.closed = true
	slog.Info("Engine initialized", "name", eng.Name())
	return nil
}

var _ engine.Core = (*coreAdapter)(nil)

// Проверки контрактов на этапе компиляции: apicontext обслуживает движок,
// поэтому его сигнатуры не должны разойтись с engine.Subject незаметно.
var _ engine.Subject = (*apicontext.APIContext)(nil)
