// Пакет предоставляет функциональность для перенаправления пользователя на файл, хранящийся в MinIO, по имени файла или ID.  Используется для интеграции с внешними сервисами, хранящими файлы в MinIO.
//
// Основные возможности:
//   - Перенаправление по имени файла или ID.
//   - Обработка ошибок, включая отсутствие файла и внутренние ошибки сервера.
//   - Использование gorm для работы с базой данных и поиска файлов.
package aiplan

import (
	"net/http"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/gofrs/uuid"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

// redirectToMinioFile godoc
// @id redirectToMinioFile
// @Summary Интеграции: перенаправление на файл в MinIO
// @Description Перенаправляет пользователя на файл, хранящийся в MinIO, по имени файла или ID
// @Tags Integrations
// @Security ApiKeyAuth
// @Accept */*
// @Produce */*
// @Param fileName path string true "Имя файла или ID файла"
// @Success 307 "Перенаправление на URL файла в MinIO"
// @Failure 404 {object} apierrors.DefinedError "Файл не найден"
// @Failure 500 {object} apierrors.DefinedError "Внутренняя ошибка сервера"
// @Router /api/file/{fileName} [get]
func (s *Services) redirectToMinioFile(c echo.Context) error {
	name := c.Param("fileName")

	query := s.db.
		Select("id", "content_type")

	if uuid, err := uuid.FromString(name); err == nil {
		query = query.Where("id = ?", uuid)
	} else {
		query = query.Where("name = ?", name)
	}

	var fileAsset dao.FileAsset
	if err := query.First(&fileAsset).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.NoContent(http.StatusNotFound)
		}
		return EError(c, err)
	}

	r, err := s.storage.LoadReader(fileAsset.Id)
	if err != nil {
		return EError(c, err)
	}

	return c.Stream(http.StatusOK, fileAsset.ContentType, r)
}
