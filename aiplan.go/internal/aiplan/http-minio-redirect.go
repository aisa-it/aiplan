// Пакет предоставляет функциональность для перенаправления пользователя на файл, хранящийся в MinIO, по имени файла или ID.  Используется для интеграции с внешними сервисами, хранящими файлы в MinIO.
//
// Основные возможности:
//   - Перенаправление по имени файла или ID.
//   - Обработка ошибок, включая отсутствие файла и внутренние ошибки сервера.
//   - Использование gorm для работы с базой данных и поиска файлов.
package aiplan

import (
	"fmt"
	"net/http"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/gofrs/uuid"
	"github.com/labstack/echo/v4"
	"github.com/minio/minio-go/v7"
)

const (
	selectFileWithPermissionCheck = `
SELECT
    f.id,
    f.content_type,
    (f.workspace_id IS NULL OR wm.role IS NOT NULL)
    AND (
        (f.comment_id IS NULL AND f.issue_id IS NULL AND f.doc_comment_id IS NULL)
        OR pm.role IS NOT NULL
    )
    AND (
        f.doc_id IS NULL
        OR wm.role >= d.reader_role
        OR dar.id IS NOT NULL
    ) AS allowed
FROM file_assets f
LEFT JOIN workspace_members wm
    ON wm.workspace_id = f.workspace_id
    AND wm.member_id = ?
LEFT JOIN project_members pm
    ON pm.workspace_id = f.workspace_id
    AND pm.member_id = ?
    AND (
        pm.project_id IN (SELECT project_id FROM issues WHERE id = f.issue_id)
        OR pm.project_id IN (SELECT project_id FROM issue_comments WHERE id = f.comment_id)
        OR pm.project_id IN (SELECT project_id FROM doc_comments WHERE id = f.doc_comment_id)
    )
LEFT JOIN docs d
    ON d.id = f.doc_id
LEFT JOIN doc_access_rules dar
    ON dar.doc_id = f.doc_id
    AND dar.member_id = ?
`
)

// assetsHandler godoc
// @id assetsHandler
// @Summary Получение файла
// @Description Эндпоинт для получения файла из MinIO хранилища. Проверяет права доступа пользователя к файлу и возвращает файл по его имени или идентификатору
// @Tags Integrations
// @Security ApiKeyAuth
// @Accept */*
// @Produce */*
// @Param fileName path string true "Имя файла или ID файла"
// @Success 200 "Успешный ответ с содержимым файла"
// @Failure 404 {object} apierrors.DefinedError "Файл не найден"
// @Failure 500 {object} apierrors.DefinedError "Внутренняя ошибка сервера"
// @Router /api/auth/file/{fileName} [get]
func (s *Services) assetsHandler(c echo.Context) error {
	user := c.(AuthContext).User
	name := c.Param("fileName")

	query := selectFileWithPermissionCheck
	if _, err := uuid.FromString(name); err == nil {
		query += " WHERE f.id = ?"
	} else {
		query += " WHERE f.name = ?"
	}

	var asset struct {
		dao.FileAsset
		Allowed bool
	}
	if err := s.db.Raw(query, user.ID, user.ID, user.ID, name).Find(&asset).Error; err != nil {
		return EError(c, err)
	}

	if asset.Id.IsNil() {
		return c.NoContent(http.StatusNotFound)
	}

	stats, err := s.storage.GetFileInfo(asset.Id)
	if err != nil {
		errResponse := minio.ToErrorResponse(err)
		if errResponse.Code == "NoSuchKey" {
			return c.NoContent(http.StatusNotFound)
		}
		return EError(c, err)
	}

	ifNoneMatchHeader := c.Request().Header.Get("If-None-Match")
	if ifNoneMatchHeader == stats.ETag {
		return c.NoContent(http.StatusNotModified)
	}

	c.Response().Header().Set("ETag", stats.ETag)
	c.Response().Header().Set("Content-Length", fmt.Sprint(stats.Size))

	r, err := s.storage.LoadReader(asset.Id)
	if err != nil {
		return EError(c, err)
	}
	defer r.Close()

	return c.Stream(http.StatusOK, asset.ContentType, r)
}
