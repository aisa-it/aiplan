// Глобальный обработчик ошибок и цепочка middleware сервера.
package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/apierrors"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/dao"
	"github.com/labstack/echo-contrib/echoprometheus"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// httpErrorHandler — единый обработчик ошибок echo.
func httpErrorHandler(err error, c echo.Context) {
	// Ignore brone pipe errors
	if err == nil || strings.Contains(err.Error(), "broken pipe") {
		return
	}

	code := http.StatusInternalServerError
	if he, ok := err.(*echo.HTTPError); ok {
		code = he.Code
	}

	// Ignore 404
	if code == http.StatusNotFound {
		_ = c.NoContent(http.StatusNotFound)
		return
	}
	ctx := c.Request().Context()
	if sc, ok := c.Get(spanCtxKey).(trace.SpanContext); ok {
		ctx = trace.ContextWithSpanContext(ctx, sc)
	}
	// Обрыв запроса (клиент отвалился) — не ошибка сервера: пишем в otel
	// root span запроса вместо лога.
	if errors.Is(err, context.Canceled) {
		span := trace.SpanFromContext(c.Request().Context())
		span.RecordError(err)
		span.SetStatus(codes.Error, "request canceled")
	} else {
		slog.ErrorContext(ctx, "API error",
			"err", err,
			"method", c.Request().Method,
			"url", c.Request().URL.String(),
		)
	}
	er := apierrors.ErrGeneric
	er.StatusCode = code
	if err != nil {
		er.Err = err.Error()
	}
	EErrorDefined(c, er)
}

// setupMiddlewares навешивает глобальные middleware и валидатор запросов.
func (srv *Server) setupMiddlewares() {
	e := srv.e

	// Global middlewares
	e.Use(ServerHeader)
	e.Use(PodHeader)
	e.Use(middleware.BodyLimitWithConfig(middleware.BodyLimitConfig{
		Limit: "5M",
		Skipper: func(c echo.Context) bool {
			return c.Path() == "/api/auth/workspaces/:workspaceSlug/logo/" ||
				c.Path() == "/api/auth/workspaces/:workspaceSlug/projects/:projectId/issues/:issueIdOrSeq/issue-attachments/" ||
				c.Path() == "/api/auth/workspaces/:workspaceSlug/doc/:docId/doc-attachments/" ||
				c.Path() == "/api/auth/workspaces/:workspaceSlug/doc/" ||
				c.Path() == "/api/auth/workspaces/:workspaceSlug/doc/:docId/" ||
				c.Path() == "/api/auth/workspaces/:workspaceSlug/projects/:projectId/issues/" ||
				c.Path() == "/api/auth/workspaces/:workspaceSlug/projects/:projectId/issues/:issueIdOrSeq/" ||
				c.Path() == "/api/auth/workspaces/:workspaceSlug/projects/:projectId/issues/:issueIdOrSeq/comments/" ||
				c.Path() == "/api/auth/workspaces/:workspaceSlug/projects/:projectId/issues/:issueIdOrSeq/comments/:commentId/" ||
				c.Path() == "/api/auth/forms/:formSlug/form-attachments/" ||
				c.Path() == "/api/auth/users/me/avatar/" ||
				strings.Contains(c.Path(), "/api/auth/issue-attachments/tus/") ||
				strings.Contains(c.Path(), "/api/auth/attachments/tus/")

		},
	}))
	e.Use(middleware.GzipWithConfig(middleware.GzipConfig{
		Level:     9,
		MinLength: 2048,
		Skipper: func(c echo.Context) bool {
			return c.Path() == "/api/auth/ws/notifications/" ||
				c.Path() == "/api/ws/notifications/" ||
				strings.HasPrefix(c.Path(), "/api/auth/file/") ||
				strings.HasPrefix(c.Path(), "/mcp/") ||
				strings.Contains(c.Request().URL.Path, "swagger")
		},
	}))
	e.Use(echoprometheus.NewMiddleware("aiplan"))
	e.Pre(
		middleware.ContextTimeout(requestTimeout),
		middleware.RateLimiterWithConfig(middleware.RateLimiterConfig{
			Skipper: func(c echo.Context) bool {
				return !strings.HasPrefix(c.Path(), "/api")
			},
			Store: middleware.NewRateLimiterMemoryStoreWithConfig(middleware.RateLimiterMemoryStoreConfig{
				Rate:      20,
				Burst:     50,
				ExpiresIn: time.Minute * 3,
			}),
			IdentifierExtractor: func(c echo.Context) (string, error) {
				if u := c.Get("user"); u != nil {
					return u.(*dao.User).ID.String(), nil
				}
				return c.RealIP() + c.Request().UserAgent(), nil
			},
			DenyHandler: func(c echo.Context, identifier string, err error) error {
				slog.Warn("Rate limit exceed", "identifier", identifier)
				return EErrorDefined(c, apierrors.ErrRateLimitExceed)
			},
		}),
		middleware.AddTrailingSlashWithConfig(middleware.TrailingSlashConfig{
			Skipper: func(c echo.Context) bool {
				return strings.Contains(c.Request().URL.Path, "swagger")
			},
		}),
	)
	e.Use(middleware.SecureWithConfig(middleware.SecureConfig{
		XFrameOptions:         "DENY",
		ContentTypeNosniff:    "nosniff",
		HSTSMaxAge:            31536000,
		HSTSExcludeSubdomains: false,
		ReferrerPolicy:        "strict-origin-when-cross-origin",
		Skipper: func(c echo.Context) bool {
			return c.Path() == "/api/auth/file/:fileName/"
		},
	}))
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if sc := trace.SpanContextFromContext(c.Request().Context()); sc.IsValid() {
				c.Set(spanCtxKey, sc)
			}
			return next(c)
		}
	})

	e.Validator = NewRequestValidator()
}
