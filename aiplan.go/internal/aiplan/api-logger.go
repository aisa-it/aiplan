// API error handling utilities for the aiplan package.
// Provides functions for returning errors with appropriate HTTP status codes and logging.
//
// Key features:
//   - Standardized error response formatting.
//   - Logging of API errors with context (method, URL, user).
//   - Support for custom error types with status codes.
//   - Handles common error scenarios like entity too large and generic API errors.
package aiplan

import (
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"runtime"

	"sheff.online/aiplan/internal/aiplan/apierrors"

	"github.com/labstack/echo/v4"
	"sheff.online/aiplan/internal/aiplan/dao"
)

// Возврат ошибки 400 с универсальным сообщением
func EError(c echo.Context, err error) error {
	if customErr, ok := err.(apierrors.DefinedError); ok {
		return EErrorDefined(c, customErr)
	}
	var user *dao.User
	if ctx, ok := c.(AuthContext); ok {
		user = ctx.User
	}
	if err == nil {
		slog.Error("Unknown API error",
			"method", c.Request().Method,
			"url", c.Request().URL,
			"user", user,
			getCallerFile(),
		)
	} else {
		slog.Error("API error",
			"err", err,
			"method", c.Request().Method,
			"url", c.Request().URL,
			"user", user,
			getCallerFile(),
		)
	}
	return EErrorDefined(c, apierrors.ErrGeneric)
}

// Возврат ошибки <status> с сообщением ошибки(403 код с пустой ошибкой не логируется)
func EErrorMsgStatus(c echo.Context, err error, status int) error {
	var user *dao.User
	if ctx, ok := c.(AuthContext); ok {
		user = ctx.User
	}
	if status == http.StatusRequestEntityTooLarge {
		return EErrorDefined(c, apierrors.ErrEntityToLarge)
	}

	if err == nil {
		if status != http.StatusForbidden {
			slog.Error("Unknown API error",
				"method", c.Request().Method,
				slog.Int("status", status),
				"url", c.Request().URL,
				"user", user,
				getCallerFile(),
			)
		}
		er := apierrors.ErrGeneric
		er.StatusCode = status
		return EErrorDefined(c, er)
	} else {
		// Ignore log 404 error
		if status != http.StatusNotFound {
			slog.Error("API error",
				"err", err,
				"method", c.Request().Method,
				slog.Int("status", status),
				"url", c.Request().URL,
				"user", user,
				getCallerFile(),
			)
		}
		er := apierrors.ErrGeneric
		er.StatusCode = status
		er.Err = err.Error()
		return EErrorDefined(c, er)
	}
}

// Возврат ошибки 400 с сообщением ошибки
func EErrorMsg(c echo.Context, err error) error {
	var user *dao.User
	if ctx, ok := c.(AuthContext); ok {
		user = ctx.User
	}
	if err == nil {
		slog.Error("Unknown API error",
			"method", c.Request().Method,
			"url", c.Request().URL,
			"user", user,
			getCallerFile(),
		)
		return EErrorDefined(c, apierrors.ErrGeneric)
	} else {
		slog.Error("API error",
			"err", err,
			"method", c.Request().Method,
			"url", c.Request().URL,
			"user", user,
			getCallerFile(),
		)
		er := apierrors.ErrGeneric
		er.Err = err.Error()
		return EErrorDefined(c, er)
	}
}

// EErrorDefined возвращает JSON-ответ с кодом статуса и сообщением об ошибке.  Если код статуса не определен, используется 400 Bad Request.
//
// Параметры:
//   - c: Context Echo, используемый для отправки JSON-ответа.
//   - err:  Объект DefinedError, содержащий код статуса и сообщение об ошибке.
//
// Возвращает:
//   - error:  Ошибка, если произошла ошибка при формировании ответа.  В противном случае nil.
func EErrorDefined(c echo.Context, err apierrors.DefinedError) error {
	// If unknown code use 400 Bad Request
	if http.StatusText(err.StatusCode) == "" {
		err.StatusCode = http.StatusBadRequest
	}
	return c.JSON(err.StatusCode, err)
}

// getCallerFile возвращает строку с именем файла и номером строки, из которых была вызвана функция.  Используется для улучшения отладки логов API.
//
// Возвращаемые значения:
//   - slog.Attr: Атрибут для логирования, содержащий имя файла и номер строки вызова.
//
// При неудачном получении информации о вызывающем коде возвращает пустой атрибут.
func getCallerFile() slog.Attr {
	_, path, no, ok := runtime.Caller(2)
	if !ok {
		return slog.Attr{}
	}
	_, file := filepath.Split(path)
	return slog.String("caller", fmt.Sprintf("%s:%d", file, no))
}
