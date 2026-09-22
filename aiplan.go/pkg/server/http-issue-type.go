// Ручки справочника типов задач проекта. Чтение — всем участникам проекта
// (селект в карточке задачи), управление — по отдельному действию.
package server

import (
	"errors"
	"net/http"
	"strings"

	apicontext "github.com/aisa-it/aiplan/aiplan.go/pkg/api-context"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/apierrors"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/dao"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/dto"
	"github.com/gofrs/uuid"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

// getIssueTypeList godoc
// @id getIssueTypeList
// @Summary Типы задач: получение списка типов проекта
// @Description Возвращает справочник типов задач проекта, отсортированный по названию.
// @Tags IssueTypes
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Success 200 {array} dto.IssueTypeLight "Список типов задач"
// @Failure 403 {object} apierrors.DefinedError "Нет доступа к проекту"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/issue-types/ [get]
func (s *Services) getIssueTypeList(c echo.Context) error {
	apiContext := apicontext.GetContext(c)
	project := apiContext.GetProject()
	if apiContext.Error() != nil {
		return EError(c, apiContext.Error())
	}

	var issueTypes []dao.IssueType
	if err := s.DB(c).Where("project_id = ?", project.ID).
		Order("name").
		Find(&issueTypes).Error; err != nil {
		return EError(c, err)
	}

	result := make([]dto.IssueTypeLight, 0, len(issueTypes))
	for i := range issueTypes {
		result = append(result, *issueTypes[i].ToLightDTO())
	}
	return c.JSON(http.StatusOK, result)
}

// getIssueType godoc
// @id getIssueType
// @Summary Типы задач: получение типа задачи
// @Description Возвращает тип задачи проекта по идентификатору.
// @Tags IssueTypes
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param issueTypeId path string true "ID типа задачи"
// @Success 200 {object} dto.IssueTypeLight "Тип задачи"
// @Failure 403 {object} apierrors.DefinedError "Нет доступа к проекту"
// @Failure 404 {object} apierrors.DefinedError "Тип задачи не найден"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/issue-types/{issueTypeId}/ [get]
func (s *Services) getIssueType(c echo.Context) error {
	apiContext := apicontext.GetContext(c)
	project := apiContext.GetProject()
	if apiContext.Error() != nil {
		return EError(c, apiContext.Error())
	}

	issueType, err := s.loadIssueType(c, project.ID)
	if err != nil {
		return EError(c, err)
	}

	return c.JSON(http.StatusOK, issueType.ToLightDTO())
}

// createIssueType godoc
// @id createIssueType
// @Summary Типы задач: создание типа задачи
// @Description Добавляет тип задачи в справочник проекта.
// @Tags IssueTypes
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param request body dto.CreateIssueTypeRequest true "Данные типа задачи"
// @Success 201 {object} dto.IssueTypeLight "Созданный тип задачи"
// @Failure 400 {object} apierrors.DefinedError "Некорректные данные"
// @Failure 403 {object} apierrors.DefinedError "Нет прав на создание"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/issue-types/ [post]
func (s *Services) createIssueType(c echo.Context) error {
	apiContext := apicontext.GetContext(c)
	project := apiContext.GetProject()
	if apiContext.Error() != nil {
		return EError(c, apiContext.Error())
	}
	user := apiContext.GetUser()

	var request dto.CreateIssueTypeRequest
	if err := c.Bind(&request); err != nil {
		return EError(c, err)
	}
	name := strings.TrimSpace(request.Name)
	if name == "" {
		return EErrorDefined(c, apierrors.ErrIssueTypeNameRequired)
	}

	userID := uuid.NullUUID{UUID: user.ID, Valid: true}
	issueType := dao.IssueType{
		Id:          dao.GenUUID(),
		ProjectId:   project.ID,
		WorkspaceId: project.WorkspaceId,
		Name:        name,
		Description: request.Description,
		Color:       request.Color,
		Default:     request.Default,
		CreatedById: userID,
		UpdatedById: userID,
	}

	if err := s.DB(c).Transaction(func(tx *gorm.DB) error {
		if issueType.Default {
			if err := resetDefaultIssueType(tx, project.ID, issueType.Id); err != nil {
				return err
			}
		}
		return tx.Create(&issueType).Error
	}); err != nil {
		return EError(c, err)
	}

	return c.JSON(http.StatusCreated, issueType.ToLightDTO())
}

// updateIssueType godoc
// @id updateIssueType
// @Summary Типы задач: обновление типа задачи
// @Description Обновляет название, описание, цвет или признак типа по умолчанию.
// @Tags IssueTypes
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param issueTypeId path string true "ID типа задачи"
// @Param request body dto.UpdateIssueTypeRequest true "Данные для обновления"
// @Success 200 {object} dto.IssueTypeLight "Обновленный тип задачи"
// @Failure 400 {object} apierrors.DefinedError "Некорректные данные"
// @Failure 403 {object} apierrors.DefinedError "Нет прав на обновление"
// @Failure 404 {object} apierrors.DefinedError "Тип задачи не найден"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/issue-types/{issueTypeId}/ [patch]
func (s *Services) updateIssueType(c echo.Context) error {
	apiContext := apicontext.GetContext(c)
	project := apiContext.GetProject()
	if apiContext.Error() != nil {
		return EError(c, apiContext.Error())
	}
	user := apiContext.GetUser()

	issueType, err := s.loadIssueType(c, project.ID)
	if err != nil {
		return EError(c, err)
	}

	var request dto.UpdateIssueTypeRequest
	if err := c.Bind(&request); err != nil {
		return EError(c, err)
	}

	if request.Name != nil {
		name := strings.TrimSpace(*request.Name)
		if name == "" {
			return EErrorDefined(c, apierrors.ErrIssueTypeNameRequired)
		}
		issueType.Name = name
	}
	if request.Description != nil {
		issueType.Description = *request.Description
	}
	if request.Color != nil {
		issueType.Color = *request.Color
	}
	if request.Default != nil {
		issueType.Default = *request.Default
	}
	issueType.UpdatedById = uuid.NullUUID{UUID: user.ID, Valid: true}

	if err := s.DB(c).Transaction(func(tx *gorm.DB) error {
		if issueType.Default {
			if err := resetDefaultIssueType(tx, project.ID, issueType.Id); err != nil {
				return err
			}
		}
		return tx.Save(issueType).Error
	}); err != nil {
		return EError(c, err)
	}

	return c.JSON(http.StatusOK, issueType.ToLightDTO())
}

// deleteIssueType godoc
// @id deleteIssueType
// @Summary Типы задач: удаление типа задачи
// @Description Удаляет тип задачи проекта. Тип, установленный хотя бы у одной задачи, удалить нельзя.
// @Tags IssueTypes
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param issueTypeId path string true "ID типа задачи"
// @Success 204 "Тип задачи успешно удален"
// @Failure 403 {object} apierrors.DefinedError "Нет прав на удаление"
// @Failure 404 {object} apierrors.DefinedError "Тип задачи не найден"
// @Failure 409 {object} apierrors.DefinedError "Тип задачи установлен у задач"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/issue-types/{issueTypeId}/ [delete]
func (s *Services) deleteIssueType(c echo.Context) error {
	apiContext := apicontext.GetContext(c)
	project := apiContext.GetProject()
	if apiContext.Error() != nil {
		return EError(c, apiContext.Error())
	}

	issueType, err := s.loadIssueType(c, project.ID)
	if err != nil {
		return EError(c, err)
	}

	used, err := dao.IsIssueTypeUsed(s.DB(c), issueType.Id)
	if err != nil {
		return EError(c, err)
	}
	if used {
		return EErrorDefined(c, apierrors.ErrIssueTypeInUse)
	}

	if err := s.DB(c).Delete(issueType).Error; err != nil {
		return EError(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}

// loadIssueType загружает тип задачи по параметру :issueTypeId с проверкой принадлежности проекту
func (s *Services) loadIssueType(c echo.Context, projectId uuid.UUID) (*dao.IssueType, error) {
	issueTypeId, err := uuid.FromString(c.Param("issueTypeId"))
	if err != nil {
		return nil, apierrors.ErrIssueTypeNotFound
	}

	var issueType dao.IssueType
	if err := s.DB(c).
		Where("id = ?", issueTypeId).
		Where("project_id = ?", projectId).
		First(&issueType).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apierrors.ErrIssueTypeNotFound
		}
		return nil, err
	}
	return &issueType, nil
}

// resetDefaultIssueType снимает признак типа по умолчанию с остальных типов проекта:
// тип по умолчанию в проекте один
func resetDefaultIssueType(tx *gorm.DB, projectId uuid.UUID, exceptId uuid.UUID) error {
	return tx.Model(&dao.IssueType{}).
		Where("project_id = ?", projectId).
		Where("id <> ?", exceptId).
		Update("default", false).Error
}
