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
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"go/types"
	"html/template"
	"log/slog"
	"math"
	"net/http"
	"net/url"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/apierrors"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
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

type FormContext struct {
	WorkspaceContext
	Form dao.Form
}

type AnswerFormContext struct {
	AuthContext
	Form             dao.Form
	IsAdminWorkspace bool
}

func (s *Services) FormMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		formSlug := c.Param("formSlug")
		workspace := c.(WorkspaceContext).Workspace

		var form dao.Form
		if err := s.db.
			Preload("Author").
			Preload("Workspace").
			Where("forms.workspace_id = ?", workspace.ID).
			Where("slug = ?", formSlug).
			First(&form).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return EErrorDefined(c, apierrors.ErrFormNotFound)

			}
			return EErrorDefined(c, apierrors.ErrGeneric)
		}

		return next(FormContext{c.(WorkspaceContext), form})
	}
}

func (s *Services) AnswerFormAuthMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		formSlug := c.Param("formSlug")
		userId := c.(AuthContext).User.ID
		var form dao.Form
		if err := s.db.
			Set("userId", userId).
			Preload("Author").
			Preload("Workspace").
			Where("slug = ?", formSlug).
			First(&form).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return EErrorDefined(c, apierrors.ErrFormNotFound)
			}
			return EErrorDefined(c, apierrors.ErrGeneric)
		}

		isAdmin := form.CurrentWorkspaceMember != nil && form.CurrentWorkspaceMember.Role == types2.AdminRole

		return next(AnswerFormContext{c.(AuthContext), form, isAdmin})
	}
}

func (s *Services) AnswerFormNoAuthMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		formSlug := c.Param("formSlug")

		var form dao.Form
		if err := s.db.
			Preload("Author").
			Preload("Workspace").
			Where("slug = ?", formSlug).
			First(&form).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return EErrorDefined(c, apierrors.ErrFormNotFound)
			}
			return EErrorDefined(c, apierrors.ErrGeneric)
		}

		if form.AuthRequire {
			return EErrorDefined(c, apierrors.ErrFormAnswerForbidden)
		}

		return next(AnswerFormContext{AuthContext{
			Context:      c,
			User:         nil,
			AccessToken:  nil,
			RefreshToken: nil,
			TokenAuth:    false,
		}, form,
			false})
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
	workspace := c.(WorkspaceContext).Workspace

	var forms []dao.Form
	query := s.db.Preload("Workspace")

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
	user := c.(WorkspaceContext).User
	workspace := c.(WorkspaceContext).Workspace

	var req reqForm
	if err := c.Bind(&req); err != nil {
		return EErrorDefined(c, apierrors.ErrFormBadRequest)
	}

	if err := c.Validate(req); err != nil {
		return EErrorDefined(c, apierrors.ErrFormRequestValidate)
	}

	form, err := req.toDao(nil, nil)
	if err != nil {
		return EErrorDefined(c, apierrors.ErrFormBadConvertRequest.WithFormattedMessage(err.Error()))
	}

	if form.EndDate != nil && !form.EndDate.Time.After(time.Now().Truncate(24*time.Hour).UTC().Add(-time.Millisecond)) {
		return EErrorDefined(c, apierrors.ErrFormEndDate)
	}

	form.Author = user
	form.Workspace = &workspace

	if err := checkFormFields(&form.Fields); err != nil {
		return EErrorDefined(c, apierrors.ErrFormCheckFields.WithFormattedMessage(err.Error()))
	}

	if err := s.db.Create(&form).Error; err != nil {
		return EErrorDefined(c, apierrors.ErrGeneric)
	}

	err = tracker.TrackActivity[dao.Form, dao.WorkspaceActivity](s.tracker, activities.EntityCreateActivity, nil, nil, *form, user)
	if err != nil {
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
	user := c.(FormContext).User
	workspace := c.(FormContext).Workspace
	form := c.(FormContext).Form
	oldForm := StructToJSONMap(form)

	var req reqForm
	if err := c.Bind(&req); err != nil {
		return EErrorDefined(c, apierrors.ErrFormBadRequest)
	}

	if err := c.Validate(req); err != nil {
		return EErrorDefined(c, apierrors.ErrFormRequestValidate)
	}

	requestMap := StructToJSONMap(req)
	var reqEndDate, currentEndDate string
	if req.EndDate != nil {
		if v, err := notifications.FormatDate(req.EndDate.String(), "2006-01-02", nil); err != nil {
			return EErrorDefined(c, apierrors.ErrGeneric)
		} else {
			reqEndDate = v
		}
	} else {
		requestMap["end_date"] = nil
	}

	if form.EndDate != nil {
		if v, err := notifications.FormatDate(form.EndDate.String(), "2006-01-02", nil); err != nil {
			return EErrorDefined(c, apierrors.ErrGeneric)
		} else {
			currentEndDate = v
		}
	}

	if _, ok := requestMap["fields"]; ok {
		requestMap["fields"] = req.Fields
	}

	newForm, err := req.toDao(&form, requestMap)
	if err != nil {
		return EErrorDefined(c, apierrors.ErrFormBadConvertRequest.WithFormattedMessage(err.Error()))
	}

	if newForm.EndDate != nil && !newForm.EndDate.Time.After(time.Now().Truncate(24*time.Hour).UTC().Add(-time.Millisecond)) {
		return EErrorDefined(c, apierrors.ErrFormEndDate)
	}

	newForm.UpdatedById = uuid.NullUUID{UUID: user.ID, Valid: true}
	newForm.Workspace = &workspace

	if err := checkFormFields(&form.Fields); err != nil {
		return EErrorDefined(c, apierrors.ErrFormCheckFields.WithFormattedMessage(err.Error()))
	}

	updateFields := []string{"updated_by", "workspace_detail"}
	for k := range requestMap {
		updateFields = append(updateFields, k)
	}
	if err := s.db.Select(updateFields).Updates(&newForm).Error; err != nil {
		return EErrorDefined(c, apierrors.ErrGeneric)
	}
	data := StructToJSONMap(form)
	data["end_date"] = reqEndDate
	data["end_date_activity_val"] = reqEndDate
	oldForm["end_date"] = currentEndDate
	oldForm["end_date_activity_val"] = currentEndDate

	err = tracker.TrackActivity[dao.Form, dao.FormActivity](s.tracker, activities.EntityUpdatedActivity, data, oldForm, form, user)
	if err != nil {
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
	form := c.(FormContext).Form
	user := c.(FormContext).User
	data := map[string]interface{}{"old_title": form.Title}
	err := tracker.TrackActivity[dao.Form, dao.WorkspaceActivity](s.tracker, activities.EntityDeleteActivity, data, nil, form, user)
	if err != nil {
		errStack.GetError(c, err)
	}

	if err := s.db.Omit(clause.Associations).Delete(&form).Error; err != nil {
		return EErrorDefined(c, apierrors.ErrGeneric)
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
	form := c.(AnswerFormContext).Form
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
	form := c.(FormContext).Form
	workspace := c.(FormContext).Workspace

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

	query := s.db.
		Joins("Form").
		Joins("Responder").
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
	form := c.(FormContext).Form
	workspace := c.(FormContext).Workspace
	rawAnswerSeq := strings.TrimSuffix(c.Param("answerSeq"), "/")
	answerSeq, err := strconv.ParseInt(rawAnswerSeq, 10, 64)
	if err != nil {
		return EErrorDefined(c, apierrors.ErrFormAnswerNotFound)
	}
	var answer dao.FormAnswer

	if err := s.db.
		Preload("Responder").
		Preload("Attachment.Asset").
		Preload("Form").
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
// @Param answer body []reqAnswer true "Данные ответа"
// @Success 201 {object} respAnswers "Отправленный ответ"
// @Failure 400 {object} apierrors.DefinedError "Ошибка валидации ответа"
// @Failure 403 {object} apierrors.DefinedError "Требуется аутентификация"
// @Failure 404 {object} apierrors.DefinedError "Форма не найдена"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/forms/{formSlug}/answer/ [post]
func (s *Services) createAnswerAuth(c echo.Context) error {
	form := c.(AnswerFormContext).Form
	user := c.(AnswerFormContext).User

	var userAnswer types2.FormFieldsSlice

	if err := c.Bind(&userAnswer); err != nil {
		return EErrorDefined(c, apierrors.ErrFormBadRequest)
	}

	if !form.Active {
		return EErrorDefined(c, apierrors.ErrFormAnswerEnd)
	}

	if len(form.Fields) != len(userAnswer) {
		return EErrorDefined(c, apierrors.ErrLenAnswers)
	}

	resultAnswers, err := formAnswer(userAnswer, &form)
	if err != nil {
		return EErrorDefined(c, apierrors.ErrFormCheckAnswers)
	}
	if len(resultAnswers) == 0 {
		return EErrorDefined(c, apierrors.ErrFormEmptyAnswers)
	}

	var uuid string
	for _, field := range resultAnswers {
		if field.Type == "attachment" {
			uuid = fmt.Sprint(field.Val)
		}
	}

	var answer dao.FormAnswer
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		var seqId int
		// Calculate sequence id
		var lastId sql.NullInt64
		row := tx.Model(&dao.FormAnswer{}).
			Select("max(seq_id)").
			Unscoped().
			Where("form_id = ?", form.ID).
			Row()
		if err := row.Scan(&lastId); err != nil {
			return err
		}
		// Just use the last ID specified (which should be the greatest) and add one to it
		if lastId.Valid {
			seqId = int(lastId.Int64)
			seqId++
		} else {
			seqId = 1
		}

		answer.ID = dao.GenUUID()
		answer.Fields = resultAnswers
		answer.Form = &form
		answer.Workspace = form.Workspace
		answer.Responder = user
		answer.SeqId = seqId
		answer.FormDate = form.UpdatedAt

		if len(uuid) > 0 {
			var formAttachment dao.FormAttachment
			if err := tx.Where("workspace_id = ?", form.WorkspaceId).
				Where("form_id = ?", form.ID).
				Where("id = ?", uuid).
				First(&formAttachment).Error; err != nil {
				if err == gorm.ErrRecordNotFound {
					return apierrors.ErrFormAttachmentNotFound
				}

				return err
			}
			answer.AttachmentId = &formAttachment.Id
		}

		if err := tx.Model(&dao.FormAnswer{}).Create(&answer).Error; err != nil {
			return err
		}
		//data := StructToJSONMap(answer)

		//if user == nil {
		//	actor = dao.User{}
		//} else {
		//	actor = *user
		//}

		// TODO добавить активность по ответам?
		//return s.tracker.TrackActivity(tracker.FORM_ANSWER_SEND_ACTIVITY, data, nil, form.ID.String(), tracker.ENTITY_TYPE_FORM, nil, *user)
		return nil

	}); err != nil {
		return EError(c, err)
	}

	if form.TargetProjectId.Valid {
		go func(form *dao.Form, answer *dao.FormAnswer, user *dao.User) {
			if err := s.createAnswerIssue(form, answer, user); err != nil {
				slog.Error("Create answer issue", "formId", form.ID, "err", err)
			}
		}(&form, &answer, user)
	}

	result := respAnswers{
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
// @Param answer body []reqAnswer true "Данные ответа"
// @Success 201 {object} respAnswers "Отправленный ответ"
// @Failure 400 {object} apierrors.DefinedError "Ошибка валидации ответа"
// @Failure 403 {object} apierrors.DefinedError "Требуется аутентификация"
// @Failure 404 {object} apierrors.DefinedError "Форма не найдена"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/forms/{formSlug}/answer/ [post]
func (s *Services) createAnswerNoAuth(c echo.Context) error {
	return s.createAnswerAuth(c)
}

const answerIssueTmpl = `{{if .User}}<p>Пользователь: {{.User.GetName}}, {{.User.Email}}</p>{{else}}<p>Анонимный пользователь</p>{{end}}<ol>{{- range .Answers -}}<li><p><span style="font-size: 14px"><strong>{{- .Label -}}</strong></span><br><span style="font-size: 14px">{{- getValString .Type .Val -}}</span></p></li>{{- end -}}</ol>`

func (s *Services) createAnswerIssue(form *dao.Form, answer *dao.FormAnswer, user *dao.User) error {
	t, err := template.New("AnswerIssue").Funcs(template.FuncMap{
		"getValString": func(t string, val interface{}) template.HTML {
			switch t {
			case "checkbox":
				if v := val.(bool); v {
					return template.HTML("Да")
				} else {
					return template.HTML("Нет")
				}
			case "date":
				return template.HTML(time.UnixMilli(int64(val.(float64))).Format("02.01.2006"))
			case "multiselect":
				if values, ok := val.([]interface{}); ok {
					var stringValues []string
					for _, v := range values {
						stringValues = append(stringValues, fmt.Sprint(v))
					}
					return template.HTML(strings.Join(stringValues, "<br>"))
				}
				return template.HTML(fmt.Sprint(val))
			}
			return template.HTML(fmt.Sprint(val))
		},
	}).Parse(answerIssueTmpl)
	if err != nil {
		return err
	}

	buf := new(bytes.Buffer)
	if err := t.Execute(buf, struct {
		User    *dao.User
		Answers types2.FormFieldsSlice
	}{
		User:    user,
		Answers: answer.Fields,
	}); err != nil {
		return err
	}

	var defaultAssignees []uuid.UUID
	if err := s.db.Select("member_id").
		Model(&dao.ProjectMember{}).
		Where("project_id = ? and is_default_assignee = true", form.TargetProjectId.String).
		Find(&defaultAssignees).Error; err != nil {
		return err
	}

	systemUser := dao.GetSystemUser(s.db)

	issue := &dao.Issue{
		ID:              dao.GenUUID(),
		Name:            fmt.Sprintf("Ответ №%d формы \"%s\"", answer.SeqId, form.Title),
		CreatedById:     systemUser.ID,
		ProjectId:       uuid.Must(uuid.FromString(form.TargetProjectId.String)),
		WorkspaceId:     form.WorkspaceId,
		DescriptionHtml: buf.String(),
		//DescriptionStripped: issue.DescriptionStripped,
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := dao.CreateIssue(tx, issue); err != nil {
			return err
		}
		systemUserID := uuid.NullUUID{UUID: systemUser.ID, Valid: true}
		var newAssignees []dao.IssueAssignee
		for _, watcher := range defaultAssignees {
			newAssignees = append(newAssignees, dao.IssueAssignee{
				Id:          dao.GenUUID(),
				AssigneeId:  watcher,
				IssueId:     issue.ID,
				ProjectId:   issue.ProjectId,
				WorkspaceId: issue.WorkspaceId,
				CreatedById: systemUserID,
				UpdatedById: systemUserID,
			})
		}
		return tx.CreateInBatches(&newAssignees, 10).Error
	})
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
// @Success 201 {object} dto.Attachment "Созданное вложение"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/forms/{formSlug}/form-attachments/ [post]
func (s *Services) createFormAttachments(c echo.Context) error {
	user := *c.(AnswerFormContext).User
	form := c.(AnswerFormContext).Form

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

	if err := s.storage.SaveReader(
		assetSrc,
		asset.Size,
		assetId,
		asset.Header.Get("Content-Type"),
		&filestorage.Metadata{
			WorkspaceId: form.WorkspaceId.String(),
			FormId:      form.ID.String(),
		},
	); err != nil {
		return EError(c, err)
	}

	userID := uuid.NullUUID{UUID: user.ID, Valid: true}
	formAttachment := dao.FormAttachment{
		Id:          dao.GenID(),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		CreatedById: userID,
		UpdatedById: userID,
		AssetId:     assetId,
		FormId:      form.ID.String(),
		WorkspaceId: form.WorkspaceId.String(),
	}

	fa := dao.FileAsset{
		Id:          assetId,
		CreatedById: userID,
		WorkspaceId: uuid.NullUUID{UUID: form.WorkspaceId, Valid: true},
		Name:        fileName,
		ContentType: asset.Header.Get("Content-Type"),
		FileSize:    int(asset.Size),
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := s.db.Create(&fa).Error; err != nil {
			return err
		}

		if err := s.db.Create(&formAttachment).Error; err != nil {
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
	workspaceId := c.(AnswerFormContext).Form.WorkspaceId
	formId := c.(AnswerFormContext).Form.ID.String()
	isAdmin := c.(AnswerFormContext).IsAdminWorkspace
	userId := c.(AnswerFormContext).User.ID

	attachmentId := c.Param("attachmentId")

	var attachment dao.FormAttachment
	if err := s.db.
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

	if err := s.db.Omit(clause.Associations).
		Delete(&attachment).Error; err != nil {
		if errors.Is(err, gorm.ErrForeignKeyViolated) {
			return EErrorDefined(c, apierrors.ErrAttachmentInUse)
		}
		return EError(c, err)
	}

	return c.NoContent(http.StatusOK)
}

func formAnswer(answers types2.FormFieldsSlice, form *dao.Form) (types2.FormFieldsSlice, error) {

	validator := FormValidator()
	var resultAnswer types2.FormFieldsSlice

	for i, field := range form.Fields {
		validFunc := validator[field.Type]
		checkVal := validFunc(answers[i].Val, field.Required, field.Validate)
		if !checkVal {
			return nil, fmt.Errorf("field missing or wrong type")
		}

		resultAnswer = append(resultAnswer,
			types2.FormFields{
				Type:  field.Type,
				Label: field.Label,
				Val:   answers[i].Val,
			})
	}
	return resultAnswer, nil
}

func checkFormFields(fields *types2.FormFieldsSlice) error {
	validator := FormValidator()
	for i, field := range *fields {
		if _, ok := validator[field.Type]; !ok {
			return fmt.Errorf("unknown field type")
		}
		(*fields)[i].Val = nil

		if (*fields)[i].Validate == nil {
			(*fields)[i].Validate = &types2.ValidationRule{}
		}

		(*fields)[i].Validate.ValidationType = strings.TrimSpace((*fields)[i].Validate.ValidationType)
		if len((*fields)[i].Validate.ValidationType) != 0 {
			typeArr := strings.Split((*fields)[i].Validate.ValidationType, " ")
			var lenOpt int
			for _, t := range typeArr {
				if v, ok := formTypeValidator[t]; ok {
					var fieldSupport bool
					for _, s := range v.FieldTypeSupport {
						if field.Type != s {
							continue
						} else {
							if v.CountOpt == 0 {
								fieldSupport = true
								break
							}
							if len((*fields)[i].Validate.Opt) >= lenOpt+v.CountOpt {
								for _, opt := range (*fields)[i].Validate.Opt[lenOpt : lenOpt+v.CountOpt-1] {
									switch v.TypeOpt {
									case types.Float64:
										if _, okType := opt.(float64); !okType {
											return fmt.Errorf("wrong opt type")
										}
									default:
										return fmt.Errorf("opt type not supported")
									}
								}
							} else {
								return fmt.Errorf("error count args for validation opt")

							}
							lenOpt += v.CountOpt
							fieldSupport = true
						}
					}

					if !fieldSupport {
						return fmt.Errorf("form field not suported this validation type")

					}
				} else {
					return fmt.Errorf("unknown validation type")
				}
			}
		}

		switch field.Type {
		case "numeric":
			(*fields)[i].Validate.ValueType = "numeric"
		case "checkbox":
			(*fields)[i].Validate.ValueType = "bool"
		case "input":
			(*fields)[i].Validate.ValueType = "string"
		case "textarea":
			(*fields)[i].Validate.ValueType = "string"
		case "color":
			(*fields)[i].Validate.ValueType = "string"
		case "date":
			(*fields)[i].Validate.ValidationType = "only_integer min_max"
			(*fields)[i].Validate.Opt = []interface{}{math.MinInt64, math.MaxInt64}
			(*fields)[i].Validate.ValueType = "numeric"
		case "attachment":
			(*fields)[i].Validate.ValueType = "uuid"
		case "select":
			(*fields)[i].Validate.ValueType = "select"
		case "multiselect":
			(*fields)[i].Validate.ValueType = "multiselect"
		}
	}
	return nil
}

// ***** REQUEST *****

type reqForm struct {
	Title           string                 `json:"title,omitempty" validate:"required"`
	Description     types2.RedactorHTML    `json:"description,omitempty"`
	AuthRequire     bool                   `json:"auth_require,omitempty"`
	EndDate         *types2.TargetDate     `json:"end_date,omitempty" extensions:"x-nullable"`
	TargetProjectId *string                `json:"target_project_id" extensions:"x-nullable"`
	Fields          types2.FormFieldsSlice `json:"fields,omitempty"`
}

func (rf *reqForm) toDao(form *dao.Form, updFields map[string]interface{}) (*dao.Form, error) {
	allowedForm := []string{"title", "description", "auth_require", "end_date", "fields", "target_project_id"}

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
			form.TargetProjectId = sql.NullString{Valid: true, String: *rf.TargetProjectId}
		}
		form.Fields = rf.Fields
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
						form.TargetProjectId = sql.NullString{Valid: false}
						continue
					}
					if id, ok := value.(string); ok {
						form.TargetProjectId = sql.NullString{Valid: true, String: id}
					} else {
						return nil, fmt.Errorf("target_project_id")
					}
				}
			}
		}
	}

	return form, nil
}

type reqAnswer struct {
	Val interface{} `json:"value,omitempty"`
}

//**RESPONSE**

type respAnswers struct {
	Form   dto.FormLight          `json:"form"`
	Fields types2.FormFieldsSlice `json:"fields"`
}
