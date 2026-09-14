// Сборка и доступ к общим зависимостям HTTP-слоя.
package server

import (
	"fmt"

	tracker "github.com/aisa-it/aiplan/aiplan.go/pkg/activity-tracker"
	authprovider "github.com/aisa-it/aiplan/aiplan.go/pkg/auth-provider"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/business"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/config"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/engine"
	filestorage "github.com/aisa-it/aiplan/aiplan.go/pkg/file-storage"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/integrations"
	issues_import "github.com/aisa-it/aiplan/aiplan.go/pkg/issues-import"
	jitsi_token "github.com/aisa-it/aiplan/aiplan.go/pkg/jitsi-token"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/notifications"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/notifications/email"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/policy"
	tokenscache "github.com/aisa-it/aiplan/aiplan.go/pkg/tokens-cache"

	mem "github.com/aisa-it/aiplan-mem/api"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

// Services — общие зависимости, доступные всем обработчикам.
// Поля неэкспортированы: снаружи доступны только NewServices и геттеры.
type Services struct {
	db                  *gorm.DB
	snapshotTracker     *tracker.SnapshotTracker
	storage             filestorage.FileStorage
	emailService        *email.EmailService
	memDB               *mem.AIPlanMemAPI
	integrationsService *integrations.IntegrationsService
	importService       *issues_import.ImportService
	jitsiTokenIss       *jitsi_token.JitsiTokenIssuer
	authProvider        *authprovider.LdapProvider

	notificationsService *notifications.Notification

	business    *business.Business
	tokensCache *tokenscache.TokensCache

	cfg     *config.Config
	version string

	// mappedRoutes — размеченные роуты и их действия; заполняется при
	// регистрации и очищается после стартовой проверки.
	mappedRoutes map[string]engine.Action

	// policy — применитель правил подключённого движка.
	policy *policy.Enforcer
}

// Deps — внешние зависимости для сборки Services.
// Обязательны DB и Config, остальное может быть nil (функциональность отключается).
type Deps struct {
	Config  *config.Config
	DB      *gorm.DB
	Version string

	SnapshotTracker      *tracker.SnapshotTracker
	Storage              filestorage.FileStorage
	EmailService         *email.EmailService
	MemDB                *mem.AIPlanMemAPI
	IntegrationsService  *integrations.IntegrationsService
	ImportService        *issues_import.ImportService
	JitsiTokenIssuer     *jitsi_token.JitsiTokenIssuer
	AuthProvider         *authprovider.LdapProvider
	NotificationsService *notifications.Notification
	Business             *business.Business
	TokensCache          *tokenscache.TokensCache

	// Policy — применитель правил движка. Обязателен.
	Policy *policy.Enforcer
}

// NewServices собирает набор зависимостей HTTP-слоя.
func NewServices(d Deps) (*Services, error) {
	if d.DB == nil {
		return nil, fmt.Errorf("services: DB is required")
	}
	if d.Config == nil {
		return nil, fmt.Errorf("services: Config is required")
	}
	if d.Policy == nil {
		return nil, fmt.Errorf("services: Policy is required")
	}
	if d.TokensCache == nil {
		d.TokensCache = tokenscache.NewTokensCache()
	}

	// Переходный костыль: часть кода пакета всё ещё читает пакетные cfg/appVersion.
	// Пока они есть, в процессе поддерживается ровно один инстанс сервера.
	cfg = d.Config
	appVersion = d.Version

	return &Services{
		policy:               d.Policy,
		db:                   d.DB,
		snapshotTracker:      d.SnapshotTracker,
		storage:              d.Storage,
		emailService:         d.EmailService,
		memDB:                d.MemDB,
		integrationsService:  d.IntegrationsService,
		importService:        d.ImportService,
		jitsiTokenIss:        d.JitsiTokenIssuer,
		authProvider:         d.AuthProvider,
		notificationsService: d.NotificationsService,
		business:             d.Business,
		tokensCache:          d.TokensCache,
		cfg:                  d.Config,
		version:              d.Version,
	}, nil
}

// DB возвращает *gorm.DB, привязанный к контексту HTTP-запроса.
// При cancel/timeout контекста pgx отправит pg_cancel_backend в Postgres,
// что не даёт зависшим запросам копиться в пуле.
func (s *Services) DB(c echo.Context) *gorm.DB {
	return s.db.WithContext(c.Request().Context())
}

// RawDB возвращает исходный *gorm.DB без привязки к запросу.
// Использовать только в фоновых задачах, кронах и не-HTTP-обработчиках.
func (s *Services) RawDB() *gorm.DB {
	return s.db
}

// Config возвращает конфигурацию сервера.
func (s *Services) Config() *config.Config { return s.cfg }

// Version возвращает версию сборки.
func (s *Services) Version() string { return s.version }

// Business возвращает слой бизнес-логики.
func (s *Services) Business() *business.Business { return s.business }

// Storage возвращает файловое хранилище.
func (s *Services) Storage() filestorage.FileStorage { return s.storage }

// Tracker возвращает трекер изменений сущностей.
func (s *Services) Tracker() *tracker.SnapshotTracker { return s.snapshotTracker }

// Notifications возвращает сервис уведомлений.
func (s *Services) Notifications() *notifications.Notification { return s.notificationsService }
