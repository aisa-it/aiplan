// Пакет предоставляет middleware для обработки поисковых фильтров в приложении AIPlan.
// Он извлекает ID фильтра из параметров запроса, ищет соответствующий фильтр в базе данных и передает его в контекст обработчика.
//
// Основные возможности:
//   - Получение поискового фильтра по ID из URL.
//   - Поиск фильтра в базе данных.
//   - Передача фильтра в контекст обработчика для дальнейшего использования.
package aiplan

import (
	"crypto/md5"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/apierrors"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"gorm.io/gorm"
)

// Запрет методов, если включен демо-режим
func DemoMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if cfg.Demo {
			return EErrorDefined(c, apierrors.ErrDemo)
		}
		return next(c)
	}
}

type SearchFilterContext struct {
	AuthContext
	Filter dao.SearchFilter
}

func (s *Services) SearchFiltersMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		filterId := c.Param("filterId")

		var filter dao.SearchFilter
		if err := s.db.Where("id = ?", filterId).First(&filter).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return c.NoContent(http.StatusNotFound)
			}
			return EError(c, err)
		}

		return next(SearchFilterContext{c.(AuthContext), filter})
	}
}

func NewSPACacheMiddleware(config middleware.StaticConfig) func(echo.HandlerFunc) echo.HandlerFunc {
	formatRegexp := regexp.MustCompile(`\.(js|css|png|jpg|jpeg|gif|ico|svg|woff|woff2)`)

	indexHasher := md5.New()

	indexF, err := config.Filesystem.Open(filepath.Join(config.Root, "index.html"))
	if err != nil {
		slog.Error("Open SPA index file, cache disabled", "err", err)
	} else {
		if _, err := io.Copy(indexHasher, indexF); err != nil {
			slog.Error("Read SPA index file, cache disabled", "err", err)
			indexHasher = nil
		}
	}
	indexF.Close()

	indexHash := ""
	if indexHasher != nil {
		indexHash = hex.EncodeToString(indexHasher.Sum(nil))
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Assets
			if formatRegexp.MatchString(c.Request().URL.Path) {
				c.Response().Header().Set(echo.HeaderCacheControl, "public, max-age=31536000, immutable")
				return next(c)
			}

			// Index file
			if indexHash != "" && strings.Contains(c.Request().URL.Path, "index.html") {
				c.Response().Header().Set(echo.HeaderCacheControl, "no-cache")

				reqHash := c.Request().Header.Get("If-None-Match")
				if reqHash != indexHash {
					c.Response().Header().Set("ETag", indexHash)
					return next(c)
				}
				return c.NoContent(http.StatusNotModified)
			}

			return next(c)
		}
	}
}
