// Пакет aiplan предоставляет функциональность для управления формами и ответами в системе планирования. Он включает в себя создание, редактирование, получение и обработку форм, а также управление ответами на них. Пакет также предоставляет инструменты для работы с вложениями и валидации данных форм.
//
// Основные возможности:
//   - Управление формами: создание, редактирование, удаление, получение списка.
//   - Управление ответами: создание, получение, удаление, обработка ответов на формы.
//   - Работа с вложениями: загрузка, удаление, управление вложениями к ответам.
//   - Валидация данных форм: проверка корректности введенных данных.
//   - Авторизация и права доступа: проверка прав доступа пользователей к формам и ответам.
//   - Интеграция с другими сервисами: возможность интеграции с другими сервисами, такими как уведомления и хранилище файлов.
package aiplan

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"

	apicontext "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/api-context"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/apierrors"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/business"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/limiter"
	"github.com/gofrs/uuid"

	tracker "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/activity-tracker"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dto"
	filestorage "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/file-storage"
	errStack "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/stack-error"
	types2 "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/labstack/echo/v4"
	"github.com/sethvargo/go-password/password"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"strconv"
	"strings"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications"
)

func (s *Services) FormMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		exists, err := dao.IsFormExists(s.DB(c), c.Param("formSlug"))
		if err != nil {
			return EError(c, err)
		}
		if !exists {
			return EErrorDefined(c, apierrors.ErrFormNotFound)
		}
		return next(c)
	}
}

func (s *Services) AnswerFormAuthMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		exists, err := dao.IsFormExists(s.DB(c), c.Param("formSlug"))
		if err != nil {
			return EError(c, err)
		}
		if !exists {
			return EErrorDefined(c, apierrors.ErrFormNotFound)
		}
		return next(c)
	}
}

func (s *Services) AnswerFormNoAuthMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if apicontext.GetContext(c) == nil {
			apicontext.SetContext(c, s.db, &apicontext.UserMeta{})
		}
		apiContext := apicontext.GetContext(c)
		form := apiContext.GetForm(apicontext.WithFormAll())
		if apiContext.Error() != nil {
			return EError(c, apiContext.Error())
		}
		if form.AuthRequire {
			return EErrorDefined(c, apierrors.ErrFormAnswerForbidden)
		}
		return next(c)
	}
}

func (s *Services) AddFormServices(g *echo.Group) {
	workspaceGroup := g.Group("workspaces/:workspaceSlug", s.WorkspaceMiddleware)
	workspaceGroup.Use(s.WorkspacePermissionMiddleware)

	answerGroup := g.Group("forms/:formSlug", s.AnswerFormAuthMiddleware)

	formGroup := workspaceGroup.Group("/forms/:formSlug", s.FormMiddleware)
	formGroup.Use(s.FormPermissionMiddleware)

	workspaceGroup.GET("/forms/", s.getFormList)
	workspaceGroup.POST("/forms/", s.createForm)

	answerGroup.GET("/", s.getFormAuth)
	answerGroup.POST("/answer/", s.createAnswerAuth)
	answerGroup.POST("/form-attachments/", s.createFormAttachments)
	answerGroup.DELETE("/form-attachments/:attachmentId/", s.deleteFormAttachment)

	formGroup.PATCH("/", s.updateForm)
	formGroup.DELETE("/", s.deleteForm)

	formGroup.GET("/answers/", s.getAnswers)
	formGroup.GET("/answers/:answerSeq", s.getAnswer)

}

func (s *Services) AddFormWithoutAuthServices(g *echo.Group) {
	formNoAuthGroup := g.Group("forms/:formSlug", s.AnswerFormNoAuthMiddleware)
	formNoAuthGroup.GET("/", s.getFormNoAuth)
	formNoAuthGroup.POST("/answer/", s.createAnswerNoAuth)
	formNoAuthGroup.POST("/form-attachments/", s.createFormAttachmentsNoAuth)
}

// getFormList godoc
// @id getFormList
// @Summary формы: список по пространству
// @Description Возвращает список форм для указанного рабочего пространства.
// @Tags Forms
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Success 200 {array} dto.FormLight "Список форм"
// @Failure 403 {object} apierrors.DefinedError "Ошибка: доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Рабочее пространство не найдено"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/forms/ [get]
func (s *Services) getFormList(c echo.Context) error {
	apiContext := apicontext.GetContext(c)
	workspace := apiContext.GetWorkspace()
	if apiContext.Error() != nil {
		return EError(c, apiContext.Error())
	}

	var forms []dao.Form
	query := s.DB(c).Preload("Workspace")

	if err := query.Where("workspace_id", workspace.ID).Order("lower(title)").Find(&forms).Error; err != nil {
		return EErrorDefined(c, apierrors.ErrGeneric)
	}

	return c.JSON(
		http.StatusOK,
		utils.SliceToSlice(&forms, func(f *dao.Form) dto.FormLight { return *f.ToLightDTO() }),
	)
}

// createForm godoc
// @id createForm
// @Summary формы: создать форму
// @Description Создает новую форму в рабочем пространстве.
// @Tags Forms
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param form body reqForm true "Данные формы"
// @Success 201 {object} dto.Form "Созданная форма"
// @Failure 400 {object} apierrors.DefinedError "Ошибка валидации данных формы"
// @Failure 403 {object} apierrors.DefinedError "Недостаточно прав для создания формы"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/forms/ [post]
func (s *Services) createForm(c echo.Context) error {
	apiContext := apicontext.GetContext(c)
	workspace := apiContext.GetWorkspace()
	if apiContext.Error() != nil {
		return EError(c, apiContext.Error())
	}
	user := apiContext.GetUser()

	req, err := bindAndValidate[reqForm](c)
	if err != nil {
		return EError(c, err)
	}

	form, err := req.toDao(nil, nil)
	if err != nil {
		return EErrorDefined(c, apierrors.ErrFormBadConvertRequest.WithFormattedMessage(err.Error()))
	}

	form.Author = user
	form.Workspace = workspace

	if err := validateFormEndDate(form.EndDate); err != nil {
		return EErrorDefined(c, apierrors.ErrFormEndDate)
	}

	if err := validateForm(&form.Fields); err != nil {
		return EErrorDefined(c, apierrors.ErrFormCheckFields.WithFormattedMessage(err.Error()))
	}

	if err := s.DB(c).Create(&form).Error; err != nil {
		return EErrorDefined(c, apierrors.ErrGeneric)
	}

	newSnap := tracker.FormToSnapshot(form)
	if err := s.snapshotTracker.TrackChanges(types2.LayerWorkspace, nil, &newSnap, workspace, user); err != nil {
		errStack.GetError(c, err)
	}

	return c.JSON(http.StatusOK, form.ToDTO())
}

// updateForm godoc
// @id updateForm
// @Summary формы: обновить форму
// @Description Обновляет данные формы.
// @Tags Forms
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param formSlug path string true "Slug формы"
// @Param form body reqForm true "Новые данные формы"
// @Success 200 {object} dto.Form "Обновленная форма"
// @Failure 400 {object} apierrors.DefinedError "Ошибка валидации данных"
// @Failure 403 {object} apierrors.DefinedError "Недостаточно прав для обновления формы"
// @Failure 404 {object} apierrors.DefinedError "Форма не найдена"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/forms/{formSlug}/ [patch]
func (s *Services) updateForm(c echo.Context) error {
	apiContext := apicontext.GetContext(c)
	workspace := apiContext.GetWorkspace()
	if apiContext.Error() != nil {
		return EError(c, apiContext.Error())
	}
	user := apiContext.GetUser()
	formPtr := apiContext.GetForm(apicontext.WithFormAll())
	if apiContext.Error() != nil {
		return EError(c, apiContext.Error())
	}
	form := *formPtr

	oldSnap := tracker.FormToSnapshot(&form)

	req, err := bindAndValidate[reqForm](c)
	if err != nil {
		return EError(c, err)
	}

	updateMap := StructToJSONMap(*req)
	if req.EndDate != nil {
		updateMap["end_date"] = req.EndDate.Time
	} else {
		updateMap["end_date"] = nil
	}
	if _, ok := updateMap["fields"]; ok {
		updateMap["fields"] = req.Fields
	}

	newForm, err := req.toDao(&form, updateMap)
	if err != nil {
		return EErrorDefined(c, apierrors.ErrFormBadConvertRequest.WithFormattedMessage(err.Error()))
	}

	if err := validateFormEndDate(newForm.EndDate); err != nil {
		return EErrorDefined(c, apierrors.ErrFormEndDate)
	}

	newForm.UpdatedById = uuid.NullUUID{UUID: user.ID, Valid: true}
	newForm.Workspace = workspace

	if err := validateForm(&form.Fields); err != nil {
		return EErrorDefined(c, apierrors.ErrFormCheckFields.WithFormattedMessage(err.Error()))
	}

	updateFields := []string{"updated_by", "workspace_detail"}
	for k := range updateMap {
		updateFields = append(updateFields, k)
	}
	if err := s.DB(c).Select(updateFields).Updates(&newForm).Error; err != nil {
		return EErrorDefined(c, apierrors.ErrGeneric)
	}

	newForm = apiContext.CleanForm().GetForm(apicontext.WithFormAll())
	newSnap := tracker.FormToSnapshot(newForm)
	if err := s.snapshotTracker.TrackChanges(types2.LayerForm, &oldSnap, &newSnap, &form, user); err != nil {
		errStack.GetError(c, err)
	}

	return c.JSON(http.StatusOK, newForm.ToDTO())
}

// deleteForm godoc
// @id deleteForm
// @Summary формы: удалить форму
// @Description Удаляет форму в рабочем пространстве.
// @Tags Forms
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param formSlug path string true "Slug формы"
// @Success 200 "Форма успешно удалена"
// @Failure 403 {object} apierrors.DefinedError "Недостаточно прав для удаления формы"
// @Failure 404 {object} apierrors.DefinedError "Форма не найдена"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/forms/{formSlug}/ [delete]
func (s *Services) deleteForm(c echo.Context) error {
	apiCtx := apicontext.GetContext(c)
	formPtr := apiCtx.GetForm(apicontext.WithFormAll())
	if apiCtx.Error() != nil {
		return EError(c, apiCtx.Error())
	}

	form := *formPtr
	user := apiCtx.GetUser()
	workspace := apiCtx.GetWorkspace()

	oldSnap := tracker.FormToSnapshot(&form)

	if err := s.DB(c).Omit(clause.Associations).Delete(&form).Error; err != nil {
		return EErrorDefined(c, apierrors.ErrGeneric)
	}
	err := s.snapshotTracker.TrackChanges(types2.LayerWorkspace, &oldSnap, nil, workspace, user)
	if err != nil {
		errStack.GetError(c, err)
	}
	return c.NoContent(http.StatusOK)
}

// getFormAuth godoc
// @id getFormAuth
// @Summary формы: получить форму (аутентификация)
// @Description Получает информацию о форме, требующей аутентификации для ответов.
// @Tags Forms
// @Produce json
// @Security ApiKeyAuth
// @Param formSlug path string true "Slug формы"
// @Success 200 {object} dto.Form "Информация о форме"
// @Failure 403 {object} apierrors.DefinedError "Требуется аутентификация"
// @Failure 404 {object} apierrors.DefinedError "Форма не найдена"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/forms/{formSlug}/ [get]
func (s *Services) getFormAuth(c echo.Context) error {
	apiContext := apicontext.GetContext(c)
	form := apiContext.GetForm(apicontext.WithFormAll())
	if apiContext.Error() != nil {
		return EError(c, apiContext.Error())
	}
	return c.JSON(http.StatusOK, form.ToDTO())
}

// getFormNoAuth godoc
// @id getFormNoAuth
// @Summary формы: получить форму
// @Description Получает информацию о форме, без аутентификации
// @Tags Forms
// @Produce json
// @Param formSlug path string true "Slug формы"
// @Success 200 {object} dto.Form "Информация о форме"
// @Failure 403 {object} apierrors.DefinedError "форма доступна только с аутентификацией"
// @Failure 404 {object} apierrors.DefinedError "Форма не найдена"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/forms/{formSlug}/ [get]
func (s *Services) getFormNoAuth(c echo.Context) error {
	return s.getFormAuth(c)
}

// getAnswers godoc
// @id getAnswers
// @Summary ответы: Получить ответы
// @Description Возвращает список всех ответов на указанную форму.
// @Tags Forms
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param formSlug path string true "Slug формы"
// @Param offset query int false "Смещение для пагинации" default(0)
// @Param limit query int false "Количество результатов на странице" default(100)
// @Success 200 {array} dao.PaginationResponse{result=[]dto.FormAnswer}  "Список ответов"
// @Failure 403 {object} apierrors.DefinedError "Ошибка: доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Форма не найдена"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/forms/{formSlug}/answers/ [get]
func (s *Services) getAnswers(c echo.Context) error {
	apiContext := apicontext.GetContext(c)
	workspace := apiContext.GetWorkspace()
	form := apiContext.GetForm(apicontext.WithFormAll())
	if apiContext.Error() != nil {
		return EError(c, apiContext.Error())
	}

	offset := 0
	limit := 100

	if err := echo.QueryParamsBinder(c).
		Int("offset", &offset).
		Int("limit", &limit).
		BindError(); err != nil {
		return EErrorDefined(c, apierrors.ErrFormBadRequest)
	}

	if limit > 100 {
		limit = 100
	}

	var answers []dao.FormAnswer

	query := s.DB(c).
		Joins("Form").
		Joins("Responder").
		Preload("Attachments.Asset").
		Where("form_answers.workspace_id = ?", workspace.ID).
		Where("form_answers.form_id = ?", form.ID)

	resp, err := dao.PaginationRequest(
		offset,
		limit,
		query,
		&answers,
	)
	if err != nil {
		return EErrorDefined(c, apierrors.ErrGeneric)
	}

	resp.Result = utils.SliceToSlice(resp.Result.(*[]dao.FormAnswer), func(fa *dao.FormAnswer) dto.FormAnswer { return *fa.ToDTO() })

	return c.JSON(http.StatusOK, resp)
}

// getAnswer godoc
// @id getAnswer
// @Summary ответы: Получить ответ
// @Description Возвращает информацию о конкретном ответе по ID.
// @Tags Forms
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param formSlug path string true "Slug формы"
// @Param answerSeq path string true "Порядковый номер ответа"
// @Success 200 {object} dto.FormAnswer "Информация об ответе"
// @Failure 403 {object} apierrors.DefinedError "Ошибка: доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Ответ не найден"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/forms/{formSlug}/answers/{answerSeq}/ [get]
func (s *Services) getAnswer(c echo.Context) error {
	apiContext := apicontext.GetContext(c)
	workspace := apiContext.GetWorkspace()
	form := apiContext.GetForm(apicontext.WithFormAll())
	if apiContext.Error() != nil {
		return EError(c, apiContext.Error())
	}
	rawAnswerSeq := strings.TrimSuffix(c.Param("answerSeq"), "/")
	answerSeq, err := strconv.ParseInt(rawAnswerSeq, 10, 64)
	if err != nil {
		return EErrorDefined(c, apierrors.ErrFormAnswerNotFound)
	}
	var answer dao.FormAnswer

	if err := s.DB(c).
		Preload("Responder").
		Preload("Attachment.Asset").
		Preload("Form").
		Preload("Attachments.Asset").
		Where("workspace_id = ?", workspace.ID).
		Where("form_id = ?", form.ID).
		Where("seq_id = ?", answerSeq).
		First(&answer).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return EErrorDefined(c, apierrors.ErrFormAnswerNotFound)
		}
		return EErrorDefined(c, apierrors.ErrGeneric)
	}

	return c.JSON(http.StatusOK, answer.ToDTO())
}

// createAnswerAuth godoc
// @id createAnswerAuth
// @Summary ответы: Отправить ответ (аутентифицированный)
// @Description Отправляет ответ на форму, требующую аутентификации.
// @Tags Forms
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param formSlug path string true "Slug формы"
// @Param answer body []dto.RequestAnswer true "Данные ответа"
// @Success 201 {object} dto.ResponseAnswers "Отправленный ответ"
// @Failure 400 {object} apierrors.DefinedError "Ошибка валидации ответа"
// @Failure 403 {object} apierrors.DefinedError "Требуется аутентификация"
// @Failure 404 {object} apierrors.DefinedError "Форма не найдена"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/forms/{formSlug}/answer/ [post]
func (s *Services) createAnswerAuth(c echo.Context) error {
	apiContext := apicontext.GetContext(c)
	formPtr := apiContext.GetForm(apicontext.WithFormAll())
	if apiContext.Error() != nil {
		return EError(c, apiContext.Error())
	}
	form := *formPtr
	user := apiContext.GetUser()

	if !form.Active {
		return EErrorDefined(c, apierrors.ErrFormAnswerEnd)
	}

	var userAnswer types2.FormFieldsSlice
	if err := c.Bind(&userAnswer); err != nil {
		return EErrorDefined(c, apierrors.ErrFormBadRequest)
	}

	if len(form.Fields) != len(userAnswer) {
		return EErrorDefined(c, apierrors.ErrLenAnswers)
	}

	validator, err := types2.NewFormValidator(form.Fields)
	if err != nil {
		return EErrorDefined(c, apierrors.ErrFormCheckFields.WithFormattedMessage("schema compile: "+err.Error()))
	}

	resultAnswers, errs := validateAnswers(validator, form.Fields, userAnswer)
	if len(errs) > 0 {
		for _, e := range errs {
			slog.Debug("form validation error", "index", e.Index, "err", e.Message)
		}
		if strings.Contains(errs[0].Error(), "null") {
			return EErrorDefined(c, apierrors.ErrFormDependOn)
		}
		return EErrorDefined(c, apierrors.ErrFormCheckAnswers.WithFormattedMessage(errs[0].Error()))
	}

	if len(resultAnswers) == 0 {
		return EErrorDefined(c, apierrors.ErrFormEmptyAnswers)
	}

	var attachmentUUIDs []string
	for _, field := range resultAnswers {
		if field.Type == "attachment" && field.Val != nil {
			attachmentUUIDs = append(attachmentUUIDs, fmt.Sprint(field.Val))
		}
	}

	var answer dao.FormAnswer
	if err := s.DB(c).Transaction(func(tx *gorm.DB) error {
		var lastId sql.NullInt64
		row := tx.Model(&dao.FormAnswer{}).
			Select("max(seq_id)").Unscoped().
			Where("form_id = ?", form.ID).Row()
		if err := row.Scan(&lastId); err != nil {
			return err
		}
		seqId := 1
		if lastId.Valid {
			seqId = int(lastId.Int64) + 1
		}

		answer.ID = dao.GenUUID()
		answer.Fields = resultAnswers
		answer.Form = &form
		answer.Workspace = form.Workspace
		answer.Responder = user
		answer.SeqId = seqId
		answer.FormDate = form.UpdatedAt

		if err := tx.Model(&dao.FormAnswer{}).Create(&answer).Error; err != nil {
			return err
		}
		if len(attachmentUUIDs) > 0 {
			if err := tx.Model(&dao.FormAttachment{}).
				Where("workspace_id = ?", form.WorkspaceId).
				Where("form_id = ?", form.ID).
				Where("id IN (?)", attachmentUUIDs).
				Update("answer_id", answer.ID).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return EError(c, err)
	}

	if user == nil {
		var sysUser dao.User
		if err := s.db.Where("username = ?", "no_auth_user").First(&sysUser).Error; err != nil {
			user = dao.GetSystemUser(s.db)
		} else {
			user = &sysUser
		}
	}

	if len(attachmentUUIDs) > 0 {
		if err := s.DB(c).Preload("Asset").Where("answer_id = ?", answer.ID).Find(&answer.Attachments).Error; err != nil {
			return EError(c, err)
		}
	}

	newSnap := tracker.FormAnswerToSnapshot(&answer)
	actor := user
	if actor == nil {
		actor = dao.GetSystemUser(s.db)
	}
	if err := s.snapshotTracker.TrackChanges(types2.LayerForm, nil, &newSnap, &answer, actor); err != nil {
		errStack.GetError(c, err)
	}

	// callbacks
	if form.TargetProjectId.Valid {
		go func(f *dao.Form, a *dao.FormAnswer, u *dao.User) {
			if err := s.createAnswerIssue(c, f, a, u); err != nil {
				slog.ErrorContext(c.Request().Context(), "Create answer issue", "formId", f.ID, "err", err)
			}
		}(&form, &answer, user)
	}
	if form.NotificationChannels.Email && !form.Author.Settings.EmailNotificationMute {
		s.emailService.FormAnswerNotify(&form, &answer, user)
	}
	if form.NotificationChannels.Telegram && !form.Author.Settings.TgNotificationMute && form.Author.TelegramId != nil {
		s.notificationsService.Tg.SendFormAnswer(*form.Author.TelegramId, form, &answer, user)
	}

	result := dto.ResponseAnswers{
		Form:   *form.ToLightDTO(),
		Fields: resultAnswers,
	}
	return c.JSON(http.StatusOK, result)
}

// createAnswerNoAuth godoc
// @id createAnswerNoAuth
// @Summary ответы: Отправить ответ
// @Description Отправляет ответ на форму.
// @Tags Forms
// @Accept json
// @Produce json
// @Param formSlug path string true "Slug формы"
// @Param answer body []dto.RequestAnswer true "Данные ответа"
// @Success 201 {object} dto.ResponseAnswers "Отправленный ответ"
// @Failure 400 {object} apierrors.DefinedError "Ошибка валидации ответа"
// @Failure 403 {object} apierrors.DefinedError "Требуется аутентификация"
// @Failure 404 {object} apierrors.DefinedError "Форма не найдена"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/forms/{formSlug}/answer/ [post]
func (s *Services) createAnswerNoAuth(c echo.Context) error {
	return s.createAnswerAuth(c)
}

// createFormAttachmentsNoAuth godoc
// @id createFormAttachmentsNoAuth
// @Summary вложения: загрузка вложения в ответ формы (без аутентификации)
// @Description Загружает новое вложение в ответ формы, доступной без аутентификации
// @Tags Forms
// @Accept multipart/form-data
// @Produce json
// @Param formSlug path string true "Slug формы"
// @Param asset formData file true "Файл для загрузки"
// @Success 201 {object} dto.FormAttachmentLight "Созданное вложение"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 403 {object} apierrors.DefinedError "Форма доступна только с аутентификацией"
// @Failure 404 {object} apierrors.DefinedError "Форма не найдена"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/forms/{formSlug}/form-attachments/ [post]
func (s *Services) createFormAttachmentsNoAuth(c echo.Context) error {
	return s.createFormAttachments(c)
}

func (s *Services) createAnswerIssue(c echo.Context, form *dao.Form, answer *dao.FormAnswer, user *dao.User) error {
	res, err := business.GenBodyAnswer(answer, user)
	if err != nil {
		return err
	}

	var defaultAssignees []uuid.UUID
	if err := s.RawDB().Select("member_id").
		Model(&dao.ProjectMember{}).
		Where("project_id = ? and is_default_assignee = true", form.TargetProjectId.UUID).
		Find(&defaultAssignees).Error; err != nil {
		return err
	}

	systemUser := dao.GetSystemUser(s.db)

	issue := &dao.Issue{
		ID:              dao.GenUUID(),
		Name:            fmt.Sprintf("Ответ №%d формы \"%s\"", answer.SeqId, form.Title),
		CreatedById:     systemUser.ID,
		ProjectId:       form.TargetProjectId.UUID,
		Project:         form.TargetProject,
		WorkspaceId:     form.WorkspaceId,
		DescriptionHtml: res,
		//DescriptionStripped: issue.DescriptionStripped,
	}

	for i, field := range form.Fields {
		if field.IssueNameField {
			issue.Name = fmt.Sprint(answer.Fields[i].Val)
		}
	}

	var createWatcher bool
	if user != nil {
		if err := s.RawDB().Raw("select exists(select 1 from project_members where member_id = ? and project_id = ?)", user.ID, form.TargetProjectId).Find(&createWatcher).Error; err != nil {
			return err
		}
	}

	var formAttachments []dao.FormAttachment
	if err := s.RawDB().Where("form_id = ?", form.ID).
		Where("answer_id = ?", answer.ID).
		Find(&formAttachments).Error; err != nil {
		return err
	}

	if err := s.RawDB().Transaction(func(tx *gorm.DB) error {
		if err := dao.CreateIssue(tx, issue); err != nil {
			return err
		}
		systemUserID := uuid.NullUUID{UUID: systemUser.ID, Valid: true}
		newAssignees := make([]dao.IssueAssignee, 0, len(defaultAssignees))
		for _, assignee := range defaultAssignees {
			newAssignees = append(newAssignees, dao.IssueAssignee{
				Id:          dao.GenUUID(),
				AssigneeId:  assignee,
				IssueId:     issue.ID,
				ProjectId:   issue.ProjectId,
				WorkspaceId: issue.WorkspaceId,
				CreatedById: systemUserID,
				UpdatedById: systemUserID,
			})
		}

		if createWatcher {
			if err := tx.Create(&dao.IssueWatcher{
				Id:          dao.GenUUID(),
				WatcherId:   user.ID,
				IssueId:     issue.ID,
				ProjectId:   issue.ProjectId,
				WorkspaceId: issue.WorkspaceId,
				CreatedById: systemUserID,
				UpdatedById: systemUserID,
			}).Error; err != nil {
				return err
			}
		}

		for _, formAttachment := range formAttachments {
			if err := tx.Create(&dao.IssueAttachment{
				Id:          dao.GenUUID(),
				AssetId:     formAttachment.AssetId,
				IssueId:     issue.ID,
				ProjectId:   issue.ProjectId,
				WorkspaceId: issue.WorkspaceId,
				CreatedById: systemUserID,
				UpdatedById: systemUserID,
			}).Error; err != nil {
				return err
			}
		}

		return tx.CreateInBatches(&newAssignees, 10).Error
	}); err != nil {
		return err
	}

	err = s.snapshotTracker.TrackChanges(types2.LayerProject, nil, tracker.IssueToSnapshot(*issue), issue.Project, user)
	if err != nil {
		errStack.GetError(nil, err)
	}

	return nil
}

// createFormAttachments godoc
// @id createFormAttachments
// @Summary вложения: загрузка вложения в ответ формы
// @Description Загружает новое вложение в ответ формы
// @Tags Forms
// @Security ApiKeyAuth
// @Accept multipart/form-data
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param formSlug path string true "Slug формы"
// @Param asset formData file true "Файл для загрузки"
// @Success 201 {object} dto.FormAttachmentLight "Созданное вложение"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/forms/{formSlug}/form-attachments/ [post]
func (s *Services) createFormAttachments(c echo.Context) error {
	apiContext := apicontext.GetContext(c)
	form := apiContext.GetForm(apicontext.WithFormAll())
	if apiContext.Error() != nil {
		return EError(c, apiContext.Error())
	}
	user := apiContext.GetUser()

	if !limiter.Limiter.CanAddAttachment(form.WorkspaceId) {
		return EErrorDefined(c, apierrors.ErrAssetsLimitExceed)
	}
	const maxFileSize = 50 * 1024 * 1024 // 50 МБ

	asset, err := c.FormFile("asset")
	if err != nil {
		return EError(c, err)
	}

	if asset.Size > maxFileSize {
		return EErrorDefined(c, apierrors.ErrFileTooLarge)
	}

	assetSrc, err := asset.Open()
	if err != nil {
		return EError(c, err)
	}

	fileName := asset.Filename

	if decodedFilename, err := url.QueryUnescape(asset.Filename); err == nil {
		fileName = decodedFilename
	}

	assetId := dao.GenUUID()
	contentType := utils.ResolveContentType(fileName, asset.Header.Get("Content-Type"))

	if err := s.storage.SaveReader(
		assetSrc,
		asset.Size,
		assetId,
		contentType,
		&filestorage.Metadata{
			WorkspaceId: form.WorkspaceId.String(),
			FormId:      form.ID.String(),
		},
	); err != nil {
		return EError(c, err)
	}

	var userID uuid.NullUUID
	if user != nil {
		userID = uuid.NullUUID{UUID: user.ID, Valid: true}
	}
	formAttachment := dao.FormAttachment{
		Id:          dao.GenUUID(),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		CreatedById: userID,
		UpdatedById: userID,
		AssetId:     assetId,
		FormId:      form.ID,
		WorkspaceId: form.WorkspaceId,
	}

	fa := dao.FileAsset{
		Id:          assetId,
		CreatedById: userID,
		WorkspaceId: uuid.NullUUID{UUID: form.WorkspaceId, Valid: true},
		Name:        fileName,
		ContentType: contentType,
		FileSize:    int(asset.Size),
	}

	if err := s.DB(c).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&fa).Error; err != nil {
			return err
		}

		if err := tx.Create(&formAttachment).Error; err != nil {
			return err
		}
		formAttachment.Asset = &fa
		return nil
	}); err != nil {
		return EError(c, err)
	}

	return c.JSON(http.StatusCreated, formAttachment.ToDTO())
}

// deleteFormAttachment godoc
// @id deleteFormAttachment
// @Summary Doc (вложения): удаление вложения из документа
// @Description Удаляет указанное вложение, прикрепленное к документу
// @Tags Docs
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param formSlug path string true "Slug формы"
// @Param attachmentId path string true "ID вложения"
// @Success 200 "Вложение успешно удалено"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/forms/{formSlug}/form-attachments/{attachmentId} [delete]
func (s *Services) deleteFormAttachment(c echo.Context) error {
	apiCtx := apicontext.GetContext(c)
	form := apiCtx.GetForm(apicontext.WithFormAll(), apicontext.WithFormCurrentMember())
	if apiCtx.Error() != nil {
		return EError(c, apiCtx.Error())
	}
	workspaceId := form.WorkspaceId
	formId := form.ID
	isAdmin := form.CurrentWorkspaceMember != nil && form.CurrentWorkspaceMember.Role == types2.AdminRole
	userId := apiCtx.GetUser().ID

	attachmentId := c.Param("attachmentId")

	var attachment dao.FormAttachment
	if err := s.DB(c).
		Preload("Asset").
		Where("workspace_id = ?", workspaceId).
		Where("form_id = ?", formId).
		Where("id = ?", attachmentId).
		First(&attachment).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return EErrorDefined(c, apierrors.ErrFormAttachmentNotFound)
		}
		return EError(c, err)
	}

	if !isAdmin || (attachment.CreatedById.Valid && userId != attachment.CreatedById.UUID) {
		return EErrorDefined(c, apierrors.ErrFormForbidden)
	}

	if err := s.DB(c).Omit(clause.Associations).
		Delete(&attachment).Error; err != nil {
		if errors.Is(err, gorm.ErrForeignKeyViolated) {
			return EErrorDefined(c, apierrors.ErrAttachmentInUse)
		}
		return EError(c, err)
	}

	return c.NoContent(http.StatusOK)
}

func bindAndValidate[T any](c echo.Context) (*T, error) {
	var req T
	if err := c.Bind(&req); err != nil {
		return nil, apierrors.ErrFormBadRequest
	}
	if err := c.Validate(req); err != nil {
		return nil, apierrors.ErrFormRequestValidate
	}
	return &req, nil
}

// ***** REQUEST *****

type reqForm struct {
	Title                string                  `json:"title,omitempty" validate:"required"`
	Description          types2.RedactorHTML     `json:"description,omitempty"`
	AuthRequire          bool                    `json:"auth_require,omitempty"`
	EndDate              *types2.TargetDate      `json:"end_date,omitempty" extensions:"x-nullable"`
	TargetProjectId      *string                 `json:"target_project_id" extensions:"x-nullable"`
	Fields               types2.FormFieldsSlice  `json:"fields,omitempty"`
	NotificationChannels types2.FormAnswerNotify `json:"notification_channels"`
}

func (rf *reqForm) toDao(form *dao.Form, updFields map[string]interface{}) (*dao.Form, error) {
	allowedForm := []string{"title", "description", "auth_require", "end_date", "fields", "target_project_id", "notification_channels"}

	if form == nil {
		form = &dao.Form{}
		form.ID = dao.GenUUID()
		form.Slug = password.MustGenerate(6, 3, 0, false, true)
		form.Title = rf.Title
		form.Description = rf.Description
		form.AuthRequire = rf.AuthRequire
		if rf.EndDate != nil {
			date, err := notifications.FormatDate(rf.EndDate.Time.String(), "2006-01-02", nil)
			if err != nil {
				return nil, fmt.Errorf("end_date")
			}
			form.EndDate = &types2.TargetDate{}
			err = form.EndDate.UnmarshalJSON([]byte(`"` + date + `"`))
			if err != nil {
				return nil, fmt.Errorf("end_date")
			}
		}
		if rf.TargetProjectId != nil {
			projectUUID, _ := uuid.FromString(*rf.TargetProjectId)
			form.TargetProjectId = uuid.NullUUID{Valid: true, UUID: projectUUID}
		}
		form.Fields = rf.Fields
		form.NotificationChannels = rf.NotificationChannels
	} else {
		for _, field := range allowedForm {
			if value, ok := updFields[field]; ok {
				switch field {
				case "title":
					if title, ok := value.(string); ok {
						form.Title = title
					} else {
						return nil, fmt.Errorf("title")
					}
				case "description":
					if description, ok := value.(types2.RedactorHTML); ok {
						form.Description = description
					} else {
						return nil, fmt.Errorf("description")
					}
				case "auth_require":
					if authRequire, ok := value.(bool); ok {
						form.AuthRequire = authRequire
					} else {
						return nil, fmt.Errorf("auth_require")
					}
				case "end_date":
					if rawValue, ok := value.(time.Time); ok {
						endDate := &types2.TargetDate{}
						if err := endDate.Scan(rawValue); err != nil {
							return nil, fmt.Errorf("end_date")
						}
						form.EndDate = endDate
					} else if targetDate, ok := value.(*types2.TargetDate); ok {

						form.EndDate = targetDate
					} else if strValue, ok := value.(string); ok {
						date, err := notifications.FormatDate(strValue, "2006-01-02", nil)
						if err != nil {
							return nil, fmt.Errorf("end_date")
						}
						form.EndDate = &types2.TargetDate{}
						err = form.EndDate.UnmarshalJSON([]byte(`"` + date + `"`))
						if err != nil {
							return nil, fmt.Errorf("end_date")
						}
					} else if value == nil {
						form.EndDate = nil
					} else {
						return nil, fmt.Errorf("end_date")
					}
				case "fields":
					if fields, ok := value.(types2.FormFieldsSlice); ok {
						form.Fields = fields
					} else {
						return nil, fmt.Errorf("fields")
					}
				case "target_project_id":
					if value == nil {
						form.TargetProjectId = uuid.NullUUID{Valid: false}
						continue
					}
					if id, ok := value.(string); ok {
						projectUUID, _ := uuid.FromString(id)
						form.TargetProjectId = uuid.NullUUID{Valid: true, UUID: projectUUID}
					} else {
						return nil, fmt.Errorf("target_project_id")
					}
				case "notification_channels":
					if channels, ok := value.(types2.FormAnswerNotify); ok {
						form.NotificationChannels = channels
					} else {
						return nil, fmt.Errorf("notification_channels")
					}
				}
			}
		}
	}

	return form, nil
}
