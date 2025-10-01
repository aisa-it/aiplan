// Пакет предоставляет middleware для обработки поисковых фильтров в приложении AIPlan.
// Он извлекает ID фильтра из параметров запроса, ищет соответствующий фильтр в базе данных и передает его в контекст обработчика.
//
// Основные возможности:
//   - Получение поискового фильтра по ID из URL.
//   - Поиск фильтра в базе данных.
//   - Передача фильтра в контекст обработчика для дальнейшего использования.
package aiplan

import (
	"net/http"

	"github.com/aisa-it/aiplan/internal/aiplan/apierrors"

	"github.com/aisa-it/aiplan/internal/aiplan/dao"
	"github.com/labstack/echo/v4"
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
