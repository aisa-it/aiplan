// Регистрация HTTP-роутов сервера.
package server

import (
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"

	apicontext "github.com/aisa-it/aiplan/aiplan.go/pkg/api-context"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/integrations"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/mcp"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/utils"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	echoSwagger "github.com/swaggo/echo-swagger"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
)

// registerRoutes регистрирует все группы роутов на echo-инстансе.
func (srv *Server) registerRoutes() {
	e := srv.e
	s := srv.services
	db := s.db
	bl := s.business
	version := s.version

	//services with auth
	apiGroup := e.Group("/api/",
		otelecho.Middleware("aiplan",
			otelecho.WithSkipper(func(c echo.Context) bool {
				p := c.Path()
				return strings.Contains(p, "_health") || strings.Contains(p, "/api/version") || strings.Contains(p, "/ws") || strings.Contains(p, "swagger")
			}),
		),
	)

	s.integrationsService = integrations.NewIntegrationService(apiGroup, db, s.notificationsService.Tg, s.storage, bl)

	authMiddleware := s.AuthMiddleware([]byte(cfg.SecretKey), nil)
	authGroup := apiGroup.Group("auth/", authMiddleware)

	apiGroup.Group("docs", middleware.StaticWithConfig(middleware.StaticConfig{
		Root:       "aiplan-help",
		Browse:     false,
		IgnoreBase: true,
		Skipper: func(c echo.Context) bool {
			ext := filepath.Ext(strings.TrimSuffix(c.Request().URL.Path, "/"))
			return !(ext == ".md" || ext == ".jpg" || ext == ".png" || ext == ".json")
		},
	}))
	apiGroup.GET("docsIndex/", NewHelpIndex("aiplan-help/"))

	s.AddAuthenticationServices(apiGroup, []byte(cfg.SecretKey))
	s.AddFormServices(authGroup)
	s.AddProjectServices(authGroup)
	s.AddWorkspaceServices(authGroup)
	s.AddUserServices(authGroup)
	s.AddIssueServices(authGroup)
	s.AddBackupServices(authGroup)
	s.AddAdminServices(authGroup)
	s.AddGitServices(authGroup)
	//AddProfileServices(e.Group("/"))
	s.AddIssueMigrationServices(authGroup)
	s.AddImportServices(authGroup)
	s.AddDocServices(authGroup)
	s.AddSprintServices(authGroup)

	// services without auth
	s.AddUserWithoutAuthServices(apiGroup)
	s.AddFormWithoutAuthServices(apiGroup)

	// Version endpoint
	apiGroup.GET("version/", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"version": version,
			"sign_up": cfg.SignUpEnable,
			"demo":    cfg.Demo,
			"ny":      cfg.NYEnable,
			"captcha": !cfg.CaptchaDisabled,
			"jitsi":   !cfg.JitsiDisabled,
		})
	})

	// Health endpoint
	apiGroup.GET("_health/", func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	})

	if cfg.SwaggerEnable {
		apiGroup.GET("swagger/*", echoSwagger.WrapHandler)
	}

	// Websocket notifications endpoint
	authGroup.GET("ws/notifications/", func(c echo.Context) error {
		s.notificationsService.Ws.Handle(apicontext.GetContext(c).GetUser().ID, c.Response(), c.Request())
		return nil
	})

	// Short urls
	e.GET("i/:slug/:projectIdent/:issueNum/", s.shortIssueURLRedirect)
	e.GET("i/:issue/", s.shortIssueURLRedirect)
	e.GET("d/:slug/:docNum/", s.shortDocURLRedirect)
	e.GET("sf/:id/", s.shortSearchFilterURLRedirect)

	// Get minio file
	authGroup.GET("file/:fileName/", s.assetsHandler)

	// Jitsi conf redirect
	if !cfg.JitsiDisabled {
		authGroup.GET("conf/:room/", s.redirectToJitsiConf, NewJitsiTokenLogMiddleware(db))
	}

	// MCP handler
	if cfg.MCPEnabled {
		opts := mcp.Options{DB: db, BL: s.business, Version: version, Policy: s.policy}
		if srv.core != nil {
			opts.ExtraTools = srv.core.mcpTools
			opts.ExtraResources = srv.core.mcpResources
			opts.ExtraPrompts = srv.core.mcpPrompts
		}
		e.Any("mcp/*", mcp.NewMCPServer(opts), authMiddleware)
	}

	// Роуты движка — до статики фронта: она перехватывает всё остальное.
	if srv.core != nil {
		for _, fn := range srv.core.routeFns {
			fn(apiGroup, authGroup)
		}
	}

	// Front handler
	if cfg.FrontFilesPath != "" || utils.CheckEmbedSPA(frontFS) {
		config := middleware.StaticConfig{
			Index: "index.html",
			Root:  "spa/",
			HTML5: true,
			Skipper: func(c echo.Context) bool {
				return strings.Contains(c.Path(), "api") ||
					strings.Contains(c.Path(), "tus") ||
					strings.Contains(c.Path(), "mcp") ||
					strings.Contains(c.Path(), "swagger")
			},
			Filesystem: http.FS(frontFS),
		}

		if !utils.CheckEmbedSPA(frontFS) {
			config.Root = "."
			config.Filesystem = http.Dir(cfg.FrontFilesPath)
		}

		slog.Info("Start front routing")
		e.Use(
			NewSPACacheMiddleware(config),
			middleware.StaticWithConfig(config),
		)
	}
}
