// Публичный запуск сервера: сборка инстанса (New) и его работа (Run).
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	tracker "github.com/aisa-it/aiplan/aiplan.go/pkg/activity-tracker"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/business"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/cache"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/config"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/cronmanager"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/dao"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/engine"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/engine/defaultengine"
	issues_import "github.com/aisa-it/aiplan/aiplan.go/pkg/issues-import"
	jitsi_token "github.com/aisa-it/aiplan/aiplan.go/pkg/jitsi-token"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/migration"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/notifications"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/notifications/email"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/policy"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/tracer"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

const (
	defaultHTTPAddr    = ":8080"
	defaultMetricsAddr = ":2112"

	shutdownTimeout = 10 * time.Second
)

// Options — параметры сборки сервера.
type Options struct {
	Config  *config.Config
	DB      *gorm.DB
	Version string

	HTTPAddr    string // "" => :8080
	MetricsAddr string // "" => :2112

	RunMigration bool

	// Engine — движок правил. nil — используется движок ядра.
	Engine engine.Engine
}

// Server — собранный, но ещё не запущенный сервер.
type Server struct {
	e        *echo.Echo
	services *Services
	opts     Options

	engine engine.Engine
	core   *coreAdapter

	cron       *cronmanager.CronManager
	stopTracer tracer.StopFn
	np         *notifications.NotificationProcessor
	es         *email.EmailService
	ns         *notifications.Notification
	sshServer  *SSHServer
}

// Echo возвращает echo-инстанс (для регистрации дополнительных роутов).
func (srv *Server) Echo() *echo.Echo { return srv.e }

// Engine возвращает подключённый движок правил.
func (srv *Server) Engine() engine.Engine { return srv.engine }

// Services возвращает набор зависимостей ядра.
func (srv *Server) Services() *Services { return srv.services }

// New собирает сервер: хранилище, зависимости, cron, middleware и роуты.
// Не блокирует и ничего не слушает — для запуска вызвать Run.
func New(opts Options) (*Server, error) {
	if opts.Config == nil {
		return nil, fmt.Errorf("server: Config is required")
	}
	if opts.DB == nil {
		return nil, fmt.Errorf("server: DB is required")
	}
	if opts.HTTPAddr == "" {
		opts.HTTPAddr = defaultHTTPAddr
	}
	if opts.MetricsAddr == "" {
		opts.MetricsAddr = defaultMetricsAddr
	}

	c := opts.Config
	db := opts.DB

	// Переходный костыль: пакетные cfg/appVersion ещё читаются кодом пакета,
	// поэтому ставятся до любой инициализации. Один инстанс на процесс.
	cfg = c
	appVersion = opts.Version

	// dao читает конфиг из пакетной переменной — ставим до первого обращения к dao.
	dao.SetConfig(c)

	e := echo.New()
	e.HideBanner = true
	e.HTTPErrorHandler = httpErrorHandler

	stopTracer, err := initTracer(context.Background(), c, db, opts.Version)
	if err != nil {
		return nil, err
	}

	storage, err := initStorage(c)
	if err != nil {
		return nil, err
	}
	// Хранилище кладётся в dao сразу после создания: вложения и аватары берут его
	// из пакетной переменной, nil здесь = паника на первом файле.
	dao.SetFileStorage(storage)

	memDB, err := initMemDB(c)
	if err != nil {
		return nil, err
	}

	es := email.NewEmailService(c, db)
	snapshotTracker := tracker.NewSnapshotTracker(db)
	bl, err := business.NewBL(db, snapshotTracker)
	if err != nil {
		return nil, fmt.Errorf("init business layer: %w", err)
	}
	ns := notifications.NewNotificationService(c, db, bl)
	np := notifications.NewNotificationProcessor(db, ns.Tg, es, ns.Ws)

	// Движок инициализируется до миграции и до роутов: он может добавить
	// свои модели, обработчики событий, задачи и ручки.
	eng := opts.Engine
	if eng == nil {
		eng = defaultengine.New()
	}
	core := &coreAdapter{
		db: db, cfg: c, version: opts.Version,
		storage: storage, bl: bl, st: snapshotTracker,
	}
	if err := initEngine(context.Background(), eng, core); err != nil {
		return nil, err
	}

	// Движок ядра — запасной: к нему уходят решения, которых подключённый
	// движок не принял, и все решения, если он правами не управляет.
	fallback := defaultengine.New()
	if err := fallback.Init(context.Background(), core); err != nil {
		return nil, err
	}
	primary, _ := eng.(engine.Authorizer)
	enforcer := policy.New(primary, fallback)

	cache.InitUsersCache(db)
	cache.InitWorkspaceSummaryCache(db)
	cache.InitWorkspaceMembersCache()

	ldapProvider, err := initLDAP(c)
	if err != nil {
		return nil, err
	}

	if opts.RunMigration {
		migration.New(db).Run()
		if len(core.models) > 0 {
			if err := db.AutoMigrate(core.models...); err != nil {
				return nil, fmt.Errorf("migrate engine models: %w", err)
			}
		}
	}

	jobs := defaultCronJobs(c, db, storage, np, es)
	for name, job := range core.cronJobs {
		jobs[name] = job
	}
	cronManager, err := initCron(jobs)
	if err != nil {
		return nil, err
	}

	services, err := NewServices(Deps{
		Config:               c,
		DB:                   db,
		Version:              opts.Version,
		SnapshotTracker:      snapshotTracker,
		Storage:              storage,
		EmailService:         es,
		MemDB:                memDB,
		ImportService:        issues_import.NewImportService(db, storage, es),
		JitsiTokenIssuer:     jitsi_token.NewJitsiTokenIssuer(c.JitsiJWTSecret, c.JitsiAppID),
		AuthProvider:         ldapProvider,
		NotificationsService: ns,
		Business:             bl,
		Policy:               enforcer,
	})
	if err != nil {
		return nil, err
	}

	srv := &Server{
		e:          e,
		services:   services,
		opts:       opts,
		engine:     eng,
		core:       core,
		cron:       cronManager,
		stopTracer: stopTracer,
		np:         np,
		es:         es,
		ns:         ns,
	}

	sendPasswordDefaultAdmin(db, es)

	snapshotTracker.RegisterHandler(notifications.NewEventNotificationService(ns))
	for _, h := range core.actHandlers {
		snapshotTracker.RegisterHandler(h)
	}

	srv.setupMiddlewares()
	srv.registerRoutes()

	// Разметка роутов проверяется до старта: неразмеченный роут иначе
	// молча получит отказ в доступе уже на проде.
	if err := services.checkRouteActions(e); err != nil {
		return nil, err
	}

	return srv, nil
}

// Run запускает cron, фоновые подписки и HTTP-сервер. Блокирует до остановки
// по отмене ctx или сигналу SIGINT/SIGTERM.
func (srv *Server) Run(ctx context.Context) error {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	srv.cron.Start()

	slog.Info("Start DB subscriptions")
	go dao.NotifiSubscription.Start(ctx, srv.services.cfg.DatabaseDSN)

	srv.startSSHServer(ctx)

	go func() {
		if err := startMetricsServer(srv.opts.MetricsAddr); err != nil {
			slog.Error("Metrics server fail", "err", err)
		}
	}()

	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		<-ctx.Done()
		srv.shutdown(ctx)
	}()

	err := srv.e.Start(srv.opts.HTTPAddr)
	if errors.Is(err, http.ErrServerClosed) {
		err = nil
	}
	// Если Start упал сам (занятый порт) — доводим остановку до конца.
	stop()
	<-stopped
	return err
}

// shutdown останавливает фоновые сервисы и HTTP-сервер.
// Контекст отвязывается от отмены: на остановку даётся свой таймаут.
func (srv *Server) shutdown(ctx context.Context) {
	slog.Info("Shutting down gracefully, press Ctrl+C again to force")
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()

	if srv.sshServer != nil {
		if err := srv.sshServer.Shutdown(shutdownCtx); err != nil {
			slog.Error("SSH server shutdown error", "err", err)
		}
	}

	if srv.stopTracer != nil {
		if err := srv.stopTracer(shutdownCtx); err != nil {
			slog.Error("Stop OTEL tracer", "err", err)
		}
	}

	srv.cron.Stop()
	srv.np.Stop()
	srv.es.Stop()
	srv.ns.Tg.Stop()
	if err := srv.e.Shutdown(shutdownCtx); err != nil {
		slog.Error("HTTP server shutdown error", "err", err)
	}
}

// LegacyRunner запускает сервер со стандартными параметрами и завершает
// процесс при ошибке.
//
// Deprecated: используйте New + Run.
func LegacyRunner(db *gorm.DB, c *config.Config, version string) {
	srv, err := New(Options{
		Config:       c,
		DB:           db,
		Version:      version,
		RunMigration: true,
	})
	if err != nil {
		slog.Error("Init server", "err", err)
		os.Exit(1)
	}

	if err := srv.Run(context.Background()); err != nil {
		slog.Error("Server fail", "err", err)
		os.Exit(1)
	}
}
