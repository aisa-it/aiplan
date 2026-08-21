// Ручки справочников проекта (Dictionary/DictionaryRow) — именованные наборы
// строк с атрибутами для полей типа "lookup". Управление — админ проекта,
// чтение строк — все участники (для селектов в карточке задачи).
package aiplan

import (
	"errors"
	"net/http"
	"strings"

	apicontext "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/api-context"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/apierrors"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dto"
	"github.com/gofrs/uuid"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

// dictionaryImportLimit - максимальное количество строк в одном батч-импорте
const dictionaryImportLimit = 10_000

// getDictionaryList godoc
// @id getDictionaryList
// @Summary Справочники: получение списка справочников проекта
// @Description Возвращает все справочники проекта с количеством строк в каждом.
// @Tags Dictionaries
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Success 200 {array} dto.Dictionary "Список справочников"
// @Failure 403 {object} apierrors.DefinedError "Нет доступа к проекту"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/dictionaries/ [get]
func (s *Services) getDictionaryList(c echo.Context) error {
	apiContext := apicontext.GetContext(c)
	project := apiContext.GetProject()
	if apiContext.Error() != nil {
		return EError(c, apiContext.Error())
	}

	var dictionaries []dao.Dictionary
	if err := s.DB(c).Where("project_id = ?", project.ID).
		Order("name, created_at").
		Find(&dictionaries).Error; err != nil {
		return EError(c, err)
	}

	// Количество строк по справочникам одним запросом
	type rowsCount struct {
		DictionaryId uuid.UUID
		Count        int64
	}
	var counts []rowsCount
	if err := s.DB(c).Model(&dao.DictionaryRow{}).
		Select("dictionary_id, count(*) as count").
		Where("project_id = ?", project.ID).
		Group("dictionary_id").
		Find(&counts).Error; err != nil {
		return EError(c, err)
	}
	countsMap := make(map[uuid.UUID]int64, len(counts))
	for _, cnt := range counts {
		countsMap[cnt.DictionaryId] = cnt.Count
	}

	result := make([]dto.Dictionary, 0, len(dictionaries))
	for _, dictionary := range dictionaries {
		item := *dictionary.ToDTO()
		item.RowsCount = countsMap[dictionary.Id]
		result = append(result, item)
	}
	return c.JSON(http.StatusOK, result)
}

// createDictionary godoc
// @id createDictionary
// @Summary Справочники: создание справочника
// @Description Создает новый справочник проекта. Доступно только для админов проекта.
// @Tags Dictionaries
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param request body dto.CreateDictionaryRequest true "Данные справочника"
// @Success 201 {object} dto.Dictionary "Созданный справочник"
// @Failure 400 {object} apierrors.DefinedError "Некорректные данные"
// @Failure 403 {object} apierrors.DefinedError "Нет прав на создание"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/dictionaries/ [post]
func (s *Services) createDictionary(c echo.Context) error {
	apiContext := apicontext.GetContext(c)
	project := apiContext.GetProject()
	if apiContext.Error() != nil {
		return EError(c, apiContext.Error())
	}
	user := apiContext.GetUser()

	var request dto.CreateDictionaryRequest
	if err := c.Bind(&request); err != nil {
		return EError(c, err)
	}
	if strings.TrimSpace(request.Name) == "" {
		return EErrorDefined(c, apierrors.ErrDictionaryNameRequired)
	}

	dictionary := dao.Dictionary{
		Id:          dao.GenUUID(),
		ProjectId:   project.ID,
		WorkspaceId: project.WorkspaceId,
		Name:        strings.TrimSpace(request.Name),
		CreatedById: uuid.NullUUID{UUID: user.ID, Valid: true},
		UpdatedById: uuid.NullUUID{UUID: user.ID, Valid: true},
	}
	if err := s.DB(c).Create(&dictionary).Error; err != nil {
		return EError(c, err)
	}

	return c.JSON(http.StatusCreated, dictionary.ToDTO())
}

// updateDictionary godoc
// @id updateDictionary
// @Summary Справочники: обновление справочника
// @Description Обновляет справочник проекта. Доступно только для админов проекта.
// @Tags Dictionaries
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param dictionaryId path string true "ID справочника"
// @Param request body dto.UpdateDictionaryRequest true "Данные для обновления"
// @Success 200 {object} dto.Dictionary "Обновленный справочник"
// @Failure 400 {object} apierrors.DefinedError "Некорректные данные"
// @Failure 403 {object} apierrors.DefinedError "Нет прав на обновление"
// @Failure 404 {object} apierrors.DefinedError "Справочник не найден"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/dictionaries/{dictionaryId}/ [patch]
func (s *Services) updateDictionary(c echo.Context) error {
	apiContext := apicontext.GetContext(c)
	dictionary := apiContext.GetDictionary()
	if apiContext.Error() != nil {
		return EError(c, apiContext.Error())
	}
	user := apiContext.GetUser()

	var request dto.UpdateDictionaryRequest
	if err := c.Bind(&request); err != nil {
		return EError(c, err)
	}

	if request.Name != nil {
		name := strings.TrimSpace(*request.Name)
		if name == "" {
			return EErrorDefined(c, apierrors.ErrDictionaryNameRequired)
		}
		dictionary.Name = name
		dictionary.UpdatedById = uuid.NullUUID{UUID: user.ID, Valid: true}
		if err := s.DB(c).Save(dictionary).Error; err != nil {
			return EError(c, err)
		}
	}

	return c.JSON(http.StatusOK, dictionary.ToDTO())
}

// deleteDictionary godoc
// @id deleteDictionary
// @Summary Справочники: удаление справочника
// @Description Удаляет справочник проекта вместе со строками. Справочник, на который ссылаются шаблоны полей, удалить нельзя. Доступно только для админов проекта.
// @Tags Dictionaries
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param dictionaryId path string true "ID справочника"
// @Success 204 "Справочник успешно удален"
// @Failure 403 {object} apierrors.DefinedError "Нет прав на удаление"
// @Failure 404 {object} apierrors.DefinedError "Справочник не найден"
// @Failure 409 {object} apierrors.DefinedError "Справочник используется полями проекта"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/dictionaries/{dictionaryId}/ [delete]
func (s *Services) deleteDictionary(c echo.Context) error {
	apiContext := apicontext.GetContext(c)
	dictionary := apiContext.GetDictionary()
	if apiContext.Error() != nil {
		return EError(c, apiContext.Error())
	}

	var used bool
	if err := s.DB(c).Model(&dao.ProjectPropertyTemplate{}).
		Select("count(*) > 0").
		Where("dictionary_id = ?", dictionary.Id).
		Find(&used).Error; err != nil {
		return EError(c, err)
	}
	if used {
		return EErrorDefined(c, apierrors.ErrDictionaryInUse)
	}

	if err := s.DB(c).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("dictionary_id = ?", dictionary.Id).Delete(&dao.DictionaryRow{}).Error; err != nil {
			return err
		}
		return tx.Delete(dictionary).Error
	}); err != nil {
		return EError(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}

// getDictionaryRows godoc
// @id getDictionaryRows
// @Summary Справочники: получение строк справочника
// @Description Возвращает строки справочника с пагинацией и поиском по отображаемому значению. Архивные строки по умолчанию не возвращаются.
// @Tags Dictionaries
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param dictionaryId path string true "ID справочника"
// @Param offset query int false "Смещение (по умолчанию 0)"
// @Param limit query int false "Количество строк (по умолчанию 100, максимум 1000)"
// @Param search_query query string false "Поиск по отображаемому значению"
// @Param include_archived query bool false "Включить архивные строки (по умолчанию false)"
// @Success 200 {object} dao.PaginationResponse{result=[]dto.DictionaryRow} "Строки справочника"
// @Failure 403 {object} apierrors.DefinedError "Нет доступа к проекту"
// @Failure 404 {object} apierrors.DefinedError "Справочник не найден"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/dictionaries/{dictionaryId}/rows/ [get]
func (s *Services) getDictionaryRows(c echo.Context) error {
	apiContext := apicontext.GetContext(c)
	dictionary := apiContext.GetDictionary()
	if apiContext.Error() != nil {
		return EError(c, apiContext.Error())
	}

	offset := 0
	limit := 100
	var searchQuery string
	var includeArchived bool
	if err := echo.QueryParamsBinder(c).
		Int("offset", &offset).
		Int("limit", &limit).
		String("search_query", &searchQuery).
		Bool("include_archived", &includeArchived).
		BindError(); err != nil {
		return EError(c, err)
	}
	if limit < 1 || limit > 1000 {
		limit = 100
	}

	query := s.DB(c).Where("dictionary_id = ?", dictionary.Id).Order("value, created_at")
	if !includeArchived {
		query = query.Where("archived = false")
	}
	if searchQuery != "" {
		query = query.Where("value ILIKE ?", "%"+searchQuery+"%")
	}

	var rows []dao.DictionaryRow
	resp, err := dao.PaginationRequest(offset, limit, query, &rows)
	if err != nil {
		return EError(c, err)
	}

	result := make([]dto.DictionaryRow, 0, len(rows))
	for _, row := range rows {
		result = append(result, *row.ToDTO())
	}
	resp.Result = result

	return c.JSON(http.StatusOK, resp)
}

// createDictionaryRow godoc
// @id createDictionaryRow
// @Summary Справочники: создание строки справочника
// @Description Добавляет строку в справочник. Доступно только для админов проекта.
// @Tags Dictionaries
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param dictionaryId path string true "ID справочника"
// @Param request body dto.CreateDictionaryRowRequest true "Данные строки"
// @Success 201 {object} dto.DictionaryRow "Созданная строка"
// @Failure 400 {object} apierrors.DefinedError "Некорректные данные"
// @Failure 403 {object} apierrors.DefinedError "Нет прав на создание"
// @Failure 404 {object} apierrors.DefinedError "Справочник не найден"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/dictionaries/{dictionaryId}/rows/ [post]
func (s *Services) createDictionaryRow(c echo.Context) error {
	apiContext := apicontext.GetContext(c)
	dictionary := apiContext.GetDictionary()
	if apiContext.Error() != nil {
		return EError(c, apiContext.Error())
	}
	user := apiContext.GetUser()

	var request dto.CreateDictionaryRowRequest
	if err := c.Bind(&request); err != nil {
		return EError(c, err)
	}
	if strings.TrimSpace(request.Value) == "" {
		return EErrorDefined(c, apierrors.ErrDictionaryRowValueRequired)
	}

	row := dao.DictionaryRow{
		Id:           dao.GenUUID(),
		ProjectId:    dictionary.ProjectId,
		WorkspaceId:  dictionary.WorkspaceId,
		DictionaryId: dictionary.Id,
		Value:        strings.TrimSpace(request.Value),
		Attrs:        request.Attrs,
		CreatedById:  uuid.NullUUID{UUID: user.ID, Valid: true},
		UpdatedById:  uuid.NullUUID{UUID: user.ID, Valid: true},
	}
	if err := s.DB(c).Create(&row).Error; err != nil {
		return EError(c, err)
	}

	return c.JSON(http.StatusCreated, row.ToDTO())
}

// importDictionaryRows godoc
// @id importDictionaryRows
// @Summary Справочники: батч-импорт строк
// @Description Импортирует строки справочника одним запросом (до 10000 строк). При replace=true существующие строки без ссылок из задач удаляются, строки со ссылками архивируются. Доступно только для админов проекта.
// @Tags Dictionaries
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param dictionaryId path string true "ID справочника"
// @Param request body dto.ImportDictionaryRowsRequest true "Строки для импорта"
// @Success 200 {object} dto.ImportDictionaryRowsResult "Результат импорта"
// @Failure 400 {object} apierrors.DefinedError "Некорректные данные или превышен лимит"
// @Failure 403 {object} apierrors.DefinedError "Нет прав на импорт"
// @Failure 404 {object} apierrors.DefinedError "Справочник не найден"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/dictionaries/{dictionaryId}/rows/import/ [post]
func (s *Services) importDictionaryRows(c echo.Context) error {
	apiContext := apicontext.GetContext(c)
	dictionary := apiContext.GetDictionary()
	if apiContext.Error() != nil {
		return EError(c, apiContext.Error())
	}
	user := apiContext.GetUser()

	var request dto.ImportDictionaryRowsRequest
	if err := c.Bind(&request); err != nil {
		return EError(c, err)
	}
	if len(request.Rows) == 0 {
		return EErrorDefined(c, apierrors.ErrDictionaryRowValueRequired)
	}
	if len(request.Rows) > dictionaryImportLimit {
		return EErrorDefined(c, apierrors.ErrDictionaryImportTooLarge.WithFormattedMessage(dictionaryImportLimit))
	}

	userID := uuid.NullUUID{UUID: user.ID, Valid: true}
	newRows := make([]dao.DictionaryRow, 0, len(request.Rows))
	for _, reqRow := range request.Rows {
		if strings.TrimSpace(reqRow.Value) == "" {
			return EErrorDefined(c, apierrors.ErrDictionaryRowValueRequired)
		}
		newRows = append(newRows, dao.DictionaryRow{
			Id:           dao.GenUUID(),
			ProjectId:    dictionary.ProjectId,
			WorkspaceId:  dictionary.WorkspaceId,
			DictionaryId: dictionary.Id,
			Value:        strings.TrimSpace(reqRow.Value),
			Attrs:        reqRow.Attrs,
			CreatedById:  userID,
			UpdatedById:  userID,
		})
	}

	result := dto.ImportDictionaryRowsResult{Created: len(newRows)}
	if err := s.DB(c).Transaction(func(tx *gorm.DB) error {
		if request.Replace {
			deleted, archived, err := replaceDictionaryRows(tx, dictionary.Id)
			if err != nil {
				return err
			}
			result.Deleted = deleted
			result.Archived = archived
		}
		return tx.CreateInBatches(&newRows, 500).Error
	}); err != nil {
		return EError(c, err)
	}

	return c.JSON(http.StatusOK, result)
}

// replaceDictionaryRows подготавливает справочник к перезаливке: строки, на которые
// ссылаются значения задач, архивируются, остальные удаляются
func replaceDictionaryRows(tx *gorm.DB, dictionaryId uuid.UUID) (deleted int, archived int, err error) {
	usedSub := tx.Model(&dao.IssueProperty{}).
		Select("issue_properties.value").
		Joins("JOIN project_property_templates ppt ON ppt.id = issue_properties.template_id").
		Where("ppt.type = 'lookup' AND ppt.dictionary_id = ?", dictionaryId).
		Where("issue_properties.value <> ''")

	archiveRes := tx.Model(&dao.DictionaryRow{}).
		Where("dictionary_id = ?", dictionaryId).
		Where("id::text IN (?)", usedSub).
		Update("archived", true)
	if archiveRes.Error != nil {
		return 0, 0, archiveRes.Error
	}

	deleteRes := tx.Where("dictionary_id = ?", dictionaryId).
		Where("id::text NOT IN (?)", usedSub).
		Delete(&dao.DictionaryRow{})
	if deleteRes.Error != nil {
		return 0, 0, deleteRes.Error
	}

	return int(deleteRes.RowsAffected), int(archiveRes.RowsAffected), nil
}

// updateDictionaryRow godoc
// @id updateDictionaryRow
// @Summary Справочники: обновление строки справочника
// @Description Обновляет отображаемое значение, атрибуты или признак архивности строки. Доступно только для админов проекта.
// @Tags Dictionaries
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param dictionaryId path string true "ID справочника"
// @Param rowId path string true "ID строки"
// @Param request body dto.UpdateDictionaryRowRequest true "Данные для обновления"
// @Success 200 {object} dto.DictionaryRow "Обновленная строка"
// @Failure 400 {object} apierrors.DefinedError "Некорректные данные"
// @Failure 403 {object} apierrors.DefinedError "Нет прав на обновление"
// @Failure 404 {object} apierrors.DefinedError "Справочник или строка не найдены"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/dictionaries/{dictionaryId}/rows/{rowId}/ [patch]
func (s *Services) updateDictionaryRow(c echo.Context) error {
	apiContext := apicontext.GetContext(c)
	dictionary := apiContext.GetDictionary()
	if apiContext.Error() != nil {
		return EError(c, apiContext.Error())
	}
	user := apiContext.GetUser()

	row, err := s.loadDictionaryRow(c, dictionary.Id)
	if err != nil {
		return EError(c, err)
	}

	var request dto.UpdateDictionaryRowRequest
	if err := c.Bind(&request); err != nil {
		return EError(c, err)
	}

	updated := false
	if request.Value != nil {
		value := strings.TrimSpace(*request.Value)
		if value == "" {
			return EErrorDefined(c, apierrors.ErrDictionaryRowValueRequired)
		}
		row.Value = value
		updated = true
	}
	if request.Attrs != nil {
		row.Attrs = *request.Attrs
		updated = true
	}
	if request.Archived != nil {
		row.Archived = *request.Archived
		updated = true
	}

	if updated {
		row.UpdatedById = uuid.NullUUID{UUID: user.ID, Valid: true}
		if err := s.DB(c).Save(row).Error; err != nil {
			return EError(c, err)
		}
	}

	return c.JSON(http.StatusOK, row.ToDTO())
}

// deleteDictionaryRow godoc
// @id deleteDictionaryRow
// @Summary Справочники: удаление строки справочника
// @Description Удаляет строку справочника. Строку, на которую ссылаются значения в задачах, удалить нельзя — её следует заархивировать. Доступно только для админов проекта.
// @Tags Dictionaries
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param dictionaryId path string true "ID справочника"
// @Param rowId path string true "ID строки"
// @Success 204 "Строка успешно удалена"
// @Failure 403 {object} apierrors.DefinedError "Нет прав на удаление"
// @Failure 404 {object} apierrors.DefinedError "Справочник или строка не найдены"
// @Failure 409 {object} apierrors.DefinedError "На строку ссылаются значения в задачах"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/dictionaries/{dictionaryId}/rows/{rowId}/ [delete]
func (s *Services) deleteDictionaryRow(c echo.Context) error {
	apiContext := apicontext.GetContext(c)
	dictionary := apiContext.GetDictionary()
	if apiContext.Error() != nil {
		return EError(c, apiContext.Error())
	}

	row, err := s.loadDictionaryRow(c, dictionary.Id)
	if err != nil {
		return EError(c, err)
	}

	used, err := dao.IsDictionaryRowUsed(s.DB(c), row.Id)
	if err != nil {
		return EError(c, err)
	}
	if used {
		return EErrorDefined(c, apierrors.ErrDictionaryRowInUse)
	}

	if err := s.DB(c).Delete(row).Error; err != nil {
		return EError(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}

// loadDictionaryRow загружает строку справочника по параметру :rowId
func (s *Services) loadDictionaryRow(c echo.Context, dictionaryId uuid.UUID) (*dao.DictionaryRow, error) {
	rowUUID, err := uuid.FromString(c.Param("rowId"))
	if err != nil {
		return nil, apierrors.ErrDictionaryRowNotFound
	}

	row, err := dao.GetDictionaryRow(s.DB(c), dictionaryId, rowUUID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apierrors.ErrDictionaryRowNotFound
		}
		return nil, err
	}
	return row, nil
}
