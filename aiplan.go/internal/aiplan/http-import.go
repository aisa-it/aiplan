// Пакет, предоставляющий API для интеграции с Jira.  Позволяет запускать, отслеживать и отменять импорты данных из Jira в систему AiPlan.
//
// Основные возможности:
//   - Запуск импорта проекта из Jira.
//   - Получение статуса импорта Jira.
//   - Отслеживание списка импортов, инициированных пользователем.
//   - Отмена запущенного импорта Jira.
package aiplan

import (
	"net/http"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/apierrors"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/issues-import/entity"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/issues-import/errors"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

func (s *Services) AddImportServices(g *echo.Group) {
	jiraGroup := g.Group("import/jira/", DemoMiddleware)

	jiraGroup.POST("info/", s.getJiraInfo)
	jiraGroup.POST("start/:projectKey/", s.startJiraImport)
	jiraGroup.GET("status/", s.getMyImportList)
	jiraGroup.GET("status/:importId/", s.getJiraImportStatus)
	jiraGroup.POST("cancel/:importId/", s.cancelJiraImport)
}

type JiraInfoRequest struct {
	JiraURL  string `json:"jira_url"`
	Username string `json:"username"`
	Token    string `json:"token"`

	TargetWorkspaceID string                   `json:"target_workspace_id"`
	BlockLinkID       string                   `json:"block_link_id"`
	RelatesLinkID     []string                 `json:"relates_link_id"`
	PrioritiesMapping entity.PrioritiesMapping `json:"priorities_mapping"`
}

// getJiraInfo godoc
// @id getJiraInfo
// @Summary Интеграции (Jira): получение информации о Jira
// @Description Возвращает информацию о Jira на основе предоставленных учетных данных и URL
// @Tags Integrations
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body JiraInfoRequest true "Данные для получения информации о Jira"
// @Success 200 {object} entity.JiraInfo "Информация о Jira"
// @Failure 400 {object} apierrors.DefinedError "Некорректные данные запроса"
// @Failure 401 {object} apierrors.DefinedError "Неавторизованный доступ"
// @Failure 500 {object} apierrors.DefinedError "Внутренняя ошибка сервера"
// @Router /api/auth/import/jira/info [post]
func (s *Services) getJiraInfo(c echo.Context) error {
	var req JiraInfoRequest
	if err := c.Bind(&req); err != nil {
		return EError(c, err)
	}

	info, err := s.importService.GetJiraInfo(req.Username, req.Token, req.JiraURL)
	if err != nil {

		if err == errors.ErrJiraUnauthorized {
			return EErrorDefined(c, apierrors.ErrJiraInvalidCredentials)
		}
		return EError(c, err)
	}
	return c.JSON(http.StatusOK, info)
}

// startJiraImport godoc
// @id startJiraImport
// @Summary Интеграции (Jira): начало импорта проекта из Jira
// @Description Запускает процесс импорта проекта из Jira в указанное рабочее пространство
// @Tags Integrations
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param projectKey path string true "Ключ проекта в Jira"
// @Param data body JiraInfoRequest true "Данные для импорта проекта из Jira"
// @Success 200 {object} map[string]string "ID импорта и сообщение о запуске"
// @Failure 400 {object} apierrors.DefinedError "Некорректные данные запроса или конфликт проекта"
// @Failure 401 {object} apierrors.DefinedError "Неавторизованный доступ"
// @Failure 403 {object} apierrors.DefinedError "Отсутствие прав для выполнения импорта"
// @Failure 500 {object} apierrors.DefinedError "Внутренняя ошибка сервера"
// @Router /api/auth/import/jira/start/{projectKey} [post]
func (s *Services) startJiraImport(c echo.Context) error {
	user := c.(AuthContext).User
	projectKey := c.Param("projectKey")

	if !s.importService.CanStartImport(user.ID) {
		return EErrorDefined(c, apierrors.ErrAlreadyImportingProject)
	}

	var req JiraInfoRequest
	if err := c.Bind(&req); err != nil {
		return EError(c, err)
	}

	var workspaceExist bool
	if err := s.db.Model(&dao.WorkspaceMember{}).
		Select("EXISTS(?)",
			s.db.Model(&dao.WorkspaceMember{}).
				Select("1").
				Where("workspace_id = ?", req.TargetWorkspaceID).
				Where("member_id = ?", user.ID).
				Where("role = 15"),
		).
		Find(&workspaceExist).Error; err != nil {
		return EError(c, err)
	}
	if !workspaceExist {
		return EErrorDefined(c, apierrors.ErrTargetWorkspaceNotFoundOrNotAdmin)
	}

	var projectExist bool
	if err := s.db.Model(&dao.Project{}).
		Select("EXISTS(?)",
			s.db.Model(&dao.Project{}).
				Select("1").
				Where("workspace_id = ?", req.TargetWorkspaceID).
				Where("identifier = ?", projectKey),
		).
		Find(&projectExist).Error; err != nil {
		return EError(c, err)
	}
	if projectExist {
		return EErrorDefined(c, apierrors.ErrProjectConflict)
	}

	context, err := s.importService.StartJiraProjectImport(
		cfg.WebURL,
		*user,
		req.Username, req.Token, req.JiraURL,
		projectKey,
		req.TargetWorkspaceID,
		req.BlockLinkID,
		entity.NewLinkMapper(req.RelatesLinkID...),
		req.PrioritiesMapping,
	)
	if err != nil {
		if err == errors.ErrJiraUnauthorized {
			return EErrorDefined(c, apierrors.ErrJiraInvalidCredentials)
		}
		return EError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]string{
		"id":      context.ID.String(),
		"message": "import started",
	})
}

// getMyImportList godoc
// @id getMyImportList
// @Summary Интеграции (Jira): получение моих импортов
// @Description Возвращает список всех импортов, инициированных текущим пользователем
// @Tags Integrations
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{} "Список импортов пользователя"
// @Failure 401 {object} apierrors.DefinedError "Неавторизованный доступ"
// @Failure 500 {object} apierrors.DefinedError "Внутренняя ошибка сервера"
// @Router /api/auth/import/jira/status [get]
func (s *Services) getMyImportList(c echo.Context) error {
	user := c.(AuthContext).User

	statuses, err := s.importService.GetUserImports(user.ID)
	if err != nil {
		return EError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]interface{}{
		"imports": statuses,
	})
}

// getJiraImportStatus godoc
// @id getJiraImportStatus
// @Summary Интеграции (Jira): получение статуса импорта из Jira
// @Description Возвращает статус конкретного импорта по его ID
// @Tags Integrations
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param importId path string true "ID импорта"
// @Success 200 {object} issues_import.ImportStatus "Статус импорта"
// @Failure 400 {object} apierrors.DefinedError "Некорректный ID импорта"
// @Failure 401 {object} apierrors.DefinedError "Неавторизованный доступ"
// @Failure 404 {object} apierrors.DefinedError "Импорт не найден"
// @Failure 500 {object} apierrors.DefinedError "Внутренняя ошибка сервера"
// @Router /api/auth/import/jira/status/{importId} [get]
func (s *Services) getJiraImportStatus(c echo.Context) error {
	user := c.(AuthContext).User
	importId := c.Param("importId")

	if importId == "" {
		return EErrorDefined(c, apierrors.ErrImportIDRequired)
	}

	status, err := s.importService.GetUserImportStatus(importId, user.ID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.NoContent(http.StatusNotFound)
		}
		return EError(c, err)
	}

	return c.JSON(http.StatusOK, status)
}

// cancelJiraImport godoc
// @id cancelJiraImport
// @Summary Интеграции (Jira): отмена запущенного импорта
// @Description Отменяет запущенный импорт
// @Tags Integrations
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param importId path string true "ID импорта"
// @Success 204 {object} nil "Успешная отмена импорта"
// @Failure 400 {object} apierrors.DefinedError "Некорректный ID импорта"
// @Failure 401 {object} apierrors.DefinedError "Неавторизованный доступ"
// @Failure 404 {object} apierrors.DefinedError "Импорт не найден"
// @Failure 500 {object} apierrors.DefinedError "Внутренняя ошибка сервера"
// @Router /api/auth/import/jira/cancel/{importId} [post]
func (s *Services) cancelJiraImport(c echo.Context) error {
	user := c.(AuthContext).User
	importId := c.Param("importId")

	if importId == "" {
		return EErrorDefined(c, apierrors.ErrImportIDRequired)
	}

	if err := s.importService.CancelImport(importId, user.ID.String()); err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.NoContent(http.StatusNotFound)
		}
		return EError(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}
