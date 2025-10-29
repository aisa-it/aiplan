// Пакет aiplan предоставляет функциональность для управления планированием, задачами и отслеживанием прогресса в проектах. Он включает в себя функции для создания, редактирования и просмотра задач, а также для управления пользователями и их правами доступа. Пакет также предоставляет возможности для интеграции с другими сервисами, такими как Telegram и email.
//
// Основные возможности:
//   - Управление задачами: создание, редактирование, удаление, назначение задач пользователям.
//   - Управление проектами: создание, редактирование, удаление проектов, добавление пользователей в проекты.
//   - Управление пользователями: создание, редактирование, удаление пользователей, назначение ролей и прав доступа.
//   - Интеграция с Telegram: отправка уведомлений о задачах и событиях в Telegram.
//   - Интеграция с email: отправка уведомлений о задачах и событиях по email.
//   - Отслеживание прогресса: отслеживание прогресса выполнения задач и проектов.
//   - Отчетность: формирование отчетов о выполненных задачах и проектах.
package aiplan

import (
	"encoding/base64"
	"errors"
	"fmt"
	uuid5 "github.com/gofrs/uuid/v5"
	"log"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/apierrors"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dto"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"

	memErr "github.com/aisa-it/aiplan-mem/apierror"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/gofrs/uuid"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/sethvargo/go-password/password"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type UserUpdateRequest struct {
	Username      *string    `json:"username" validate:"omitempty,username" extensions:"x-nullable"`
	TelegramId    *int64     `json:"telegram_id" extensions:"x-nullable"`
	FirstName     *string    `json:"first_name" validate:"omitempty,fullName" extensions:"x-nullable"`
	LastName      *string    `json:"last_name" validate:"omitempty,fullName" extensions:"x-nullable"`
	StatusEmoji   *string    `json:"status_emoji" validate:"omitempty,statusEmoji" extensions:"x-nullable"`
	Status        *string    `json:"status" extensions:"x-nullable"`
	StatusEndDate *time.Time `json:"status_end_date" extensions:"x-nullable"`

	UserTimezone *string             `json:"user_timezone" extensions:"x-nullable"`
	Settings     *types.UserSettings `json:"settings,omitempty" extensions:"x-nullable"`
	Theme        *types.Theme        `json:"theme" extensions:"x-nullable"`
	ViewProps    *types.ViewProps    `json:"view_props" extensions:"x-nullable"`
}

func (s *Services) AddUserServices(g *echo.Group) {
	g.GET("users/:userId/", s.getUser)

	g.GET("users/me/", s.getCurrentUser)
	g.PATCH("users/me/", s.updateCurrentUser)
	g.Group("", middleware.BodyLimit("20M")).POST("users/me/avatar/", s.updateCurrentUserAvatar)
	g.DELETE("users/me/avatar/", s.deleteCurrentUserAvatar)

	g.POST("users/me/onboard/", s.updateUserOnBoard)
	g.POST("users/me/view-props/", s.updateUserViewProps)

	g.POST("users/me/change-email/", s.changeMyEmail)
	g.POST("users/me/verification-email/", s.verifyMyEmail)

	g.GET("users/me/activities/", s.getMyActivityList)
	g.GET("users/:userId/activities/", s.getUserActivityList)
	g.GET("users/me/activities/table/", s.getMyActivitiesTable)
	g.GET("users/:userId/activities/table/", s.getUserActivitiesTable)

	g.POST("change-my-password/", s.updateMyPassword)
	g.POST("reset-user-password/:uidb64/", s.resetUserPassword)
	g.GET("notification-bot-link/", s.getTGBotLink)

	g.GET("users/me/all/projects/", s.getCurrentUserAllProjectList)

	g.GET("users/me/token/", s.getMyAuthToken)
	g.POST("users/me/token/reset/", s.resetMyAuthToken)

	g.POST("sign-out/", s.signOut)
	g.POST("sign-out-everywhere/", s.signOutEverywhere)

	g.GET("users/me/feedback/", s.getMyFeedback)
	g.POST("users/me/feedback/", s.createMyFeedback)
	g.DELETE("users/me/feedback/", s.deleteMyFeedback)

	g.GET("users/me/notifications/", s.getMyNotificationList)
	g.DELETE("users/me/notifications/", s.deleteMyNotifications)
	g.POST("users/me/notifications/", s.updateToReadMyNotifications)

	// Search Filters
	g.GET("filters/", s.getSearchFilterList)
	g.POST("filters/", s.createSearchFilter)

	g.POST("filters/members/", s.getFilterMemberList)
	g.POST("filters/states/", s.getFilterStateList)
	g.POST("filters/labels/", s.getFilterLabelList)

	filterGroup := g.Group("filters/:filterId", s.SearchFiltersMiddleware)
	filterGroup.GET("/", s.getSearchFilter)
	filterGroup.PATCH("/", s.updateSearchFilter)
	filterGroup.DELETE("/", s.deleteSearchFilter)

	g.GET("users/me/filters/", s.getMySearchFilterList)

	myFilterGroup := g.Group("users/me/filters/:filterId", s.SearchFiltersMiddleware)
	myFilterGroup.POST("/", s.addSearchFilterToMe)
	myFilterGroup.DELETE("/", s.deleteSearchFilterFromMe)

	releaseNoteGroup := g.Group("release-notes/", s.ReleaseNotesMiddleware)
	releaseNoteGroup.GET("", s.getProductUpdateList)
	releaseNoteGroup.GET(":noteId/", s.getRecentReleaseNoteList)

	g.GET("jitsi-url/", func(c echo.Context) error {
		u := "meet.jit.si" // fallback to jitsi instance
		if cfg.JitsiURL != nil {
			u = cfg.JitsiURL.Host
		}
		return c.JSON(http.StatusOK, map[string]string{"url": u})
	})
}

func (s *Services) AddUserWithoutAuthServices(g *echo.Group) {
	g.POST("forgot-password/", s.forgotPassword)
	g.POST("reset-password/:uidb64/:token/", s.resetPassword)

	g.POST("sign-up/", s.signUp)
}

// getUser godoc
// @id getUser
// @Summary Пользователи: получение пользователя по ID
// @Description Возвращает информацию о пользователе по его ID
// @Tags Users
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param userId path string true "ID пользователя"
// @Success 200 {object} dto.UserLight "Информация о пользователе"
// @Failure 403 {object} apierrors.DefinedError "Ошибка: доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Пользователь не найден"
// @Failure 500 {object} apierrors.DefinedError "Внутренняя ошибка сервера"
// @Router /api/auth/users/{userId} [get]
func (s *Services) getUser(c echo.Context) error {
	userId := c.Param("userId")

	var user dao.User
	if err := s.db.Where("id = ? or email = ?", userId, userId).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return EErrorDefined(c, apierrors.ErrUserNotFound)
		}
		return EError(c, err)
	}
	return c.JSON(http.StatusOK, user.ToLightDTO())
}

// getCurrentUser godoc
// @id getCurrentUser
// @Summary Пользователи: получение данных о текущем пользователе
// @Description Возвращает информацию о текущем пользователе
// @Tags Users
// @Security ApiKeyAuth
// @Produce json
// @Success 200 {object} dto.User "Информация о текущем пользователе"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/users/me/ [get]
func (s *Services) getCurrentUser(c echo.Context) error {
	user := *c.(AuthContext).User
	var count int
	if err := s.db.Select("count(*)").
		Where("viewed = false").
		Where("user_id = ?", user.ID).
		Where("deleted_at IS NULL").
		Model(&dao.UserNotifications{}).
		Find(&count).Error; err != nil {
		return EErrorDefined(c, apierrors.ErrGeneric)
	}

	userMe := user.ToDTO()
	userMe.NotificationCount = count

	return c.JSON(http.StatusOK, userMe)
}

// updateCurrentUser godoc
// @id updateCurrentUser
// @Summary Пользователи: обновление данных о текущем пользователе
// @Description Обновляет информацию о текущем пользователе
// @Tags Users
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body UserUpdateRequest true "Данные для обновления пользователя"
// @Success 200 {object} dto.User "Обновленные данные пользователя"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 409 {object} apierrors.DefinedError "Конфликт имени пользователя"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/users/me/ [patch]
func (s *Services) updateCurrentUser(c echo.Context) error {
	user := c.(AuthContext).User // c.MustGet("User").(dao.User)

	var req UserUpdateRequest
	if err := c.Bind(&req); err != nil {
		return EError(c, err)
	}

	if user.IsOnboarded {
		err := c.Validate(req)
		if err != nil {
			return EError(c, err)
		}
	}
	if req.UserTimezone != nil {
		_, err := time.LoadLocation(*req.UserTimezone)
		if err != nil {
			return EErrorDefined(c, apierrors.ErrBadTimezone)
		}
	}

	updateMap := make(map[string]interface{})
	for k, v := range StructToJSONMap(req) {
		if v != nil {
			updateMap[k] = v
		}
	}

	if v, ok := updateMap["settings"]; ok {
		settings := v.(types.UserSettings)
		if settings.DeadlineNotification != user.Settings.DeadlineNotification {
			diff := user.Settings.DeadlineNotification - settings.DeadlineNotification

			err := s.db.
				Model(&dao.DeferredNotifications{}).
				Where("user_id = ?", user.ID).
				Where("sent_at IS NULL").
				Where("attempt_count < ?", 3).
				Where("notification_type = ?", "deadline_notification").
				Update("time_send", gorm.Expr("time_send + ?", diff)).Error

			if err != nil {
				return EError(c, err)
			}
		}
	}

	if req.Status != nil && req.StatusEmoji != nil && req.StatusEndDate == nil {
		user.StatusEndDate.Valid = false
		user.StatusEndDate.Time = time.Time{}
		updateMap["status_end_date"] = nil
	}

	if err := s.db.Model(&user).Updates(updateMap).Error; err != nil {
		if err == gorm.ErrDuplicatedKey {
			return EErrorDefined(c, apierrors.ErrUsernameConflict)
		}
		return EError(c, err)
	}

	return c.JSON(http.StatusOK, user.ToDTO())
}

// updateCurrentUserAvatar godoc
// @id updateCurrentUserAvatar
// @Summary Пользователи: обновление аватара текущего пользователя
// @Description Обновляет аватар текущего пользователя
// @Tags Users
// @Security ApiKeyAuth
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "Файл аватара"
// @Success 200 {object} dto.User "Обновленные данные пользователя"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/users/me/avatar/ [post]
func (s *Services) updateCurrentUserAvatar(c echo.Context) error {
	user := *c.(AuthContext).User

	file, err := c.FormFile("file")
	if err != nil {
		return EError(c, err)
	}

	fileAsset := dao.FileAsset{
		Id:          dao.GenUUID(),
		CreatedById: &user.ID,
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := s.uploadAvatarForm(tx, file, &fileAsset); err != nil {
			return err
		}

		user.AvatarId = uuid.NullUUID{UUID: fileAsset.Id, Valid: true}
		user.AvatarAsset = &fileAsset
		return tx.Omit(clause.Associations).Save(&user).Error
	}); err != nil {
		return EError(c, err)
	}
	return c.JSON(http.StatusOK, user.ToDTO())
}

// deleteCurrentUserAvatar godoc
// @id deleteCurrentUserAvatar
// @Summary Пользователи: удаление аватара текущего пользователя
// @Description Удаляет аватар текущего пользователя
// @Tags Users
// @Security ApiKeyAuth
// @Produce json
// @Success 200 {object} dto.User "Обновленные данные пользователя"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/users/me/avatar/ [delete]
func (s *Services) deleteCurrentUserAvatar(c echo.Context) error {
	user := *c.(AuthContext).User

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		user.AvatarId = uuid.NullUUID{}
		user.Avatar = ""
		if err := tx.Omit(clause.Associations).Save(&user).Error; err != nil {
			return err
		}

		if user.AvatarAsset != nil {
			if err := tx.Delete(&user.AvatarAsset).Error; err != nil {
				return err
			}
		}
		user.AvatarAsset = nil
		return nil
	}); err != nil {
		return EError(c, err)
	}
	return c.JSON(http.StatusOK, user.ToDTO())
}

// updateUserOnBoard godoc
// @id updateUserOnBoard
// @Summary Пользователи: онбординг пользователя
// @Description Обновляет статус онбординга текущего пользователя и сохраняет его данные
// @Tags Users
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body dto.User true "Данные пользователя для онбординга"
// @Success 200 {object} dto.User "Обновленные данные пользователя"
// @Success 304 "Данные уже обновлены"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 409 {object} apierrors.DefinedError "Конфликт имени пользователя"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/users/me/onboard/ [post]
func (s *Services) updateUserOnBoard(c echo.Context) error {
	user := *c.(AuthContext).User

	if user.IsOnboarded {
		return c.NoContent(http.StatusNotModified)
	}

	id := user.ID
	if err := c.Bind(&user); err != nil {
		return EError(c, err)
	}
	user.ID = id
	user.IsOnboarded = true
	user.Theme = types.DefaultTheme
	if user.FirstName == "" || user.LastName == "" {
		return EErrorDefined(c, apierrors.ErrNameRequired)
	}

	err := c.Validate(user)
	if err != nil {
		return EError(c, err)
	}

	if err := s.db.Model(&user).Select("first_name", "last_name", "username", "role", "telegram_id", "is_onboarded").Updates(&user).Error; err != nil {
		if err == gorm.ErrDuplicatedKey {
			return EErrorDefined(c, apierrors.ErrUsernameConflict)
		} else {
			return EError(c, err)
		}
	}
	return c.JSON(http.StatusOK, user.ToDTO())
}

// updateUserViewProps godoc
// @id updateUserViewProps
// @Summary Пользователи: обновление пользовательских настроек представления
// @Description Обновляет свойства представления текущего пользователя
// @Tags Users
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body types.ViewProps true "Свойства представления пользователя"
// @Success 204 "Настройки успешно обновлены"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/users/me/view-props/ [post]
func (s *Services) updateUserViewProps(c echo.Context) error {
	user := *c.(AuthContext).User

	var props types.ViewProps
	if err := c.Bind(&props); err != nil {
		return EError(c, err)
	}

	if err := s.db.Model(&user).Update("view_props", props).Error; err != nil {
		return EError(c, err)
	}
	return c.NoContent(http.StatusOK)
}

// getUserActivityList godoc
// @id getUserActivityList
// @Summary Пользователи: получение последних действий указанного пользователя
// @Description Возвращает список последних 100 действий текущего пользователя из смежных пространств с текущим пользователем, если он не суперюзер
// @Tags Users
// @Security ApiKeyAuth
// @Produce json
// @Param userId path string true "ID пользователь" default("")
// @Param day query string false "День выборки активностей" default("")
// @Param offset query int false "Смещение для пагинации" default(-1)
// @Param limit query int false "Лимит результатов" default(100)
// @Success 200 {object} dao.PaginationResponse{result=[]dto.EntityActivityFull} "Список действий пользователя"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/users/:userId/activities/ [get]
func (s *Services) getUserActivityList(c echo.Context) error {
	currentUser := *c.(AuthContext).User
	userId := c.Param("userId")

	var day DayRequest
	offset := -1
	limit := 100

	if err := echo.QueryParamsBinder(c).
		TextUnmarshaler("day", &day).
		Int("offset", &offset).
		Int("limit", &limit).BindError(); err != nil {
		return EError(c, err)
	}

	var issue dao.IssueActivity
	issue.UnionCustomFields = "'issue' AS entity_type"
	var project dao.ProjectActivity
	project.UnionCustomFields = "'project' AS entity_type"
	unionTable := dao.BuildUnionSubquery(s.db, "fa", dao.FullActivity{}, issue, project)

	query := unionTable.
		Joins("Workspace").
		Joins("Actor").
		Joins("Issue").
		Joins("Project").
		Joins("Form").
		Joins("Doc").
		Order("fa.created_at desc").
		Where("fa.actor_id = ?", userId).
		Where("field NOT IN (?)", []string{"start_date", "end_date"}).
		Where("fa.entity_type = 'issue' OR (fa.entity_type = 'project' AND fa.field = 'issue')").
		Set("userId", currentUser.ID)

	if !currentUser.IsSuperuser {
		query.Where("fa.workspace_id in (?)", s.db.Select("workspace_id").Where("member_id = ?", currentUser.ID).Model(&dao.WorkspaceMember{}))
	}

	if !time.Time(day).IsZero() {
		query = query.Where("fa.created_at >= ?", time.Time(day)).Where("fa.created_at < ?", time.Time(day).Add(time.Hour*24))
	}

	var activities []dao.FullActivity
	resp, err := dao.PaginationRequest(
		offset,
		limit,
		query,
		&activities,
	)
	if err != nil {
		return EError(c, err)
	}

	resp.Result = utils.SliceToSlice(resp.Result.(*[]dao.FullActivity), func(pa *dao.FullActivity) dto.EntityActivityFull { return *pa.ToDTO() })

	return c.JSON(http.StatusOK, resp)
}

// getMyActivityList godoc
// @id getMyActivityList
// @Summary Пользователи: получение последних действий пользователя
// @Description Возвращает список последних 100 действий текущего пользователя
// @Tags Users
// @Security ApiKeyAuth
// @Produce json
// @Param day query string false "День выборки активностей" default("")
// @Param offset query int false "Смещение для пагинации" default(-1)
// @Param limit query int false "Лимит результатов" default(100)
// @Param workspace query []string false "Workspace IDs"
// @Param project query []string false "Project IDs"
// @Success 200 {object} dao.PaginationResponse{result=[]dto.EntityActivityFull} "Список действий пользователя"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/users/me/activities/ [get]
func (s *Services) getMyActivityList(c echo.Context) error {
	user := *c.(AuthContext).User

	var workspaceIds, projectIds []string
	var day DayRequest
	offset := -1
	limit := 100

	if err := echo.QueryParamsBinder(c).
		TextUnmarshaler("day", &day).
		Int("offset", &offset).
		Int("limit", &limit).
		Strings("workspace", &workspaceIds).
		Strings("project", &projectIds).
		BindError(); err != nil {
		return EError(c, err)
	}

	var issue dao.IssueActivity
	issue.UnionCustomFields = "'issue' AS entity_type"
	var form dao.FormActivity
	form.UnionCustomFields = "'form' AS entity_type"
	var project dao.ProjectActivity
	project.UnionCustomFields = "'project' AS entity_type"
	var workspace dao.WorkspaceActivity
	workspace.UnionCustomFields = "'workspace' AS entity_type"
	var root dao.RootActivity
	root.UnionCustomFields = "'root' AS entity_type"
	var doc dao.DocActivity
	doc.UnionCustomFields = "'doc' AS entity_type"
	var sprint dao.SprintActivity
	sprint.UnionCustomFields = "'sprint' AS entity_type"

	unionTable := dao.BuildUnionSubquery(s.db, "fa", dao.FullActivity{}, issue, project, workspace, form, root, doc, sprint)
	query := unionTable.
		Joins("Actor").
		Joins("Project").
		Joins("Workspace").
		Joins("Issue").
		Joins("Doc").
		Joins("Form").
		Joins("Sprint").
		Order("fa.created_at desc").
		Where("fa.actor_id = ?", user.ID).
		//Where("field NOT IN (?)", []string{"start_date", "end_date"}). //TODO create & move to ActivitySkipper
		Set("userId", user.ID)

	if !time.Time(day).IsZero() {
		query = query.Where("fa.created_at >= ?", time.Time(day)).Where("fa.created_at < ?", time.Time(day).Add(time.Hour*24))
	}

	var activities []dao.FullActivity
	if len(workspaceIds) > 0 {
		query = query.Where("fa.workspace_id IN (?)", workspaceIds)
	}

	if len(projectIds) > 0 {
		query = query.Where("fa.project_id IN (?)", projectIds)
	}

	resp, err := dao.PaginationRequest(
		offset,
		limit,
		query,
		&activities,
	)
	if err != nil {
		return EError(c, err)
	}

	resp.Result = utils.SliceToSlice(resp.Result.(*[]dao.FullActivity), func(ea *dao.FullActivity) dto.EntityActivityFull { return *ea.ToDTO() })

	return c.JSON(http.StatusOK, resp)
}

// getMyActivitiesTable godoc
// @id getMyActivitiesTable
// @Summary Пользователи: получение таблицы активностей пользователя
// @Description Возвращает таблицу активностей пользователя за указанный период.
// @Tags Users
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param from query string true "Начальная дата периода в формате YYYY-MM-DD"
// @Param to query string true "Конечная дата периода в формате YYYY-MM-DD"
// @Param workspace query []string false "Workspace IDs"
// @Param project query []string false "Project IDs"
// @Success 200 {object} types.ActivityTable "Таблица активностей пользователя"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима аутентификация"
// @Failure 500 {object} apierrors.DefinedError "Внутренняя ошибка сервера"
// @Router /api/auth/users/me/activities/table/ [get]
func (s *Services) getMyActivitiesTable(c echo.Context) error {
	user := c.(AuthContext).User

	var workspaceIds, projectIds []string

	var from, to DayRequest

	if err := echo.QueryParamsBinder(c).
		TextUnmarshaler("from", &from).
		TextUnmarshaler("to", &to).
		Strings("workspace", &workspaceIds).
		Strings("project", &projectIds).
		BindError(); err != nil {
		return EError(c, err)
	}

	var issue dao.IssueActivity
	issue.UnionCustomFields = "'issue' AS entity_type"
	var form dao.FormActivity
	form.UnionCustomFields = "'form' AS entity_type"
	var project dao.ProjectActivity
	project.UnionCustomFields = "'project' AS entity_type"
	var workspace dao.WorkspaceActivity
	workspace.UnionCustomFields = "'workspace' AS entity_type"
	var root dao.RootActivity
	root.UnionCustomFields = "'root' AS entity_type"
	var doc dao.DocActivity
	doc.UnionCustomFields = "'doc' AS entity_type"
	var sprint dao.SprintActivity
	sprint.UnionCustomFields = "'sprint' AS entity_type"

	unionTable := dao.BuildUnionSubquery(s.db, "fa", dao.FullActivity{}, issue, project, workspace, form, root, doc, sprint)
	query := unionTable.
		Where("fa.actor_id = ?", user.ID)
	//	Where("field NOT IN (?)", []string{"start_date", "end_date"}) //TODO create & move to ActivitySkipper

	if len(workspaceIds) > 0 {
		query = query.Where("fa.workspace_id IN (?)", workspaceIds)
	}

	if len(projectIds) > 0 {
		query = query.Where("fa.project_id IN (?)", projectIds)
	}

	tables, err := GetActivitiesTable(query, from, to)
	if err != nil {
		return EError(c, err)
	}

	resp := tables[user.ID]
	if resp == nil {
		return c.JSON(http.StatusOK, struct{}{})
	}

	return c.JSON(http.StatusOK, resp)
}

// getUserActivitiesTable godoc
// @id getUserActivitiesTable
// @Summary Пользователи: получение таблицы активностей указанного пользователя
// @Description Возвращает таблицу активностей пользователя за указанный период.
// @Tags Users
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param userId path string true "ID пользователя"
// @Param from query string true "Начальная дата периода в формате YYYY-MM-DD"
// @Param to query string true "Конечная дата периода в формате YYYY-MM-DD"
// @Success 200 {object} types.ActivityTable "Таблица активностей пользователя"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима аутентификация"
// @Failure 500 {object} apierrors.DefinedError "Внутренняя ошибка сервера"
// @Router /api/auth/users/{userId}/activities/table/ [get]
func (s *Services) getUserActivitiesTable(c echo.Context) error {
	currentUser := c.(AuthContext).User
	userId := c.Param("userId")

	var from, to DayRequest

	if err := echo.QueryParamsBinder(c).
		TextUnmarshaler("from", &from).
		TextUnmarshaler("to", &to).
		BindError(); err != nil {
		return EError(c, err)
	}

	// If email provided
	if _, err := uuid.FromString(userId); err != nil {
		if err := s.db.Select("id").Where("email = ?", userId).Model(&dao.User{}).Find(&userId).Error; err != nil {
			return EError(c, err)
		}
		if userId == "" {
			return EErrorDefined(c, apierrors.ErrUserNotFound)
		}
	}

	var issue dao.IssueActivity
	issue.UnionCustomFields = "'issue' AS entity_type"
	var project dao.ProjectActivity
	project.UnionCustomFields = "'project' AS entity_type"
	unionTable := dao.BuildUnionSubquery(s.db, "fa", dao.FullActivity{}, issue, project)

	query := unionTable.
		Where("fa.actor_id = ?", userId).
		Where("field NOT IN (?)", []string{"start_date", "end_date"}).
		Where("fa.entity_type = 'issue' OR (fa.entity_type = 'project' AND fa.field = 'issue')")

	//Where("entity_type NOT IN (?)", []string{tracker.ENTITY_TYPE_PROJECT, tracker.ENTITY_TYPE_WORKSPACE})
	//query := s.db.
	//	Where("actor_id = ?", userId).
	//	Where("entity_type NOT IN (?)", []string{tracker.ENTITY_TYPE_PROJECT, tracker.ENTITY_TYPE_WORKSPACE}).
	//	Where("field NOT IN (?)", []string{"start_date", "end_date"}) //TODO create & move to ActivitySkipper

	if !currentUser.IsSuperuser {
		query = query.Where("fa.workspace_id in (?)", s.db.Select("workspace_id").Where("member_id = ?", currentUser.ID).Model(&dao.WorkspaceMember{}))
	}

	tables, err := GetActivitiesTable(query, from, to)
	if err != nil {
		return EError(c, err)
	}

	resp := tables[userId]
	if resp == nil {
		return c.JSON(http.StatusOK, struct{}{})
	}

	return c.JSON(http.StatusOK, resp)
}

// forgotPassword godoc
// @id forgotPassword
// @Summary Пользователи (управление доступом): запрос на восстановление пароля
// @Description Отправляет запрос на восстановление пароля для указанного email. Возвращает статус 200 даже если пользователь не найден или неактивен для повышения безопасности.
// @Tags Users
// @Accept json
// @Produce json
// @Param data body EmailCaptchaRequest true "Данные для восстановления пароля"
// @Success 200 "Запрос на восстановление пароля успешно отправлен"
// @Failure 400 {object} apierrors.DefinedError "Некорректные данные запроса или ошибка капчи"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/forgot-password/ [post]
func (s *Services) forgotPassword(c echo.Context) error {
	var data EmailCaptchaRequest
	if err := c.Bind(&data); err != nil {
		return EError(c, err)
	}

	if !CaptchaService.Validate(data.CaptchaPayload) {
		return EError(c, nil)
	}

	var user dao.User
	if err := s.db.Where("email = ?", data.Email).First(&user).Error; err != nil {
		if err != gorm.ErrRecordNotFound {
			return EError(c, err)
		}
		return c.NoContent(http.StatusOK)
	}

	if !user.IsActive {
		return c.NoContent(http.StatusOK)
	}

	user.Token = dao.GenID()
	if err := s.db.Model(&user).Select("Token").Updates(&user).Error; err != nil {
		return EError(c, err)
	}

	if err := s.emailService.UserPasswordForgotNotify(user, user.Token); err != nil {
		return EError(c, err)
	}

	return c.NoContent(http.StatusOK)
}

// updateMyPassword godoc
// @id updateMyPassword
// @Summary Пользователи (управление доступом): смена пароля текущего пользователя
// @Description Позволяет текущему пользователю изменить свой пароль. В случае успеха завершает все активные сеансы пользователя.
// @Tags Users
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body PasswordRequest true "Новые данные пароля"
// @Success 200 {object} PasswordResponse "Пароль успешно изменен"
// @Failure 400 {object} apierrors.DefinedError "Пароли не совпадают или некорректные данные"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/change-my-password/ [post]
func (s *Services) updateMyPassword(c echo.Context) error {
	user := *c.(AuthContext).User
	accessToken := c.(AuthContext).AccessToken

	var data PasswordRequest
	if err := c.Bind(&data); err != nil {
		return EError(c, err)
	}

	if data.NewPassword != data.ConfirmPassword {
		return EErrorDefined(c, apierrors.ErrPasswordsNotEqual)
	}

	user.Password = dao.GenPasswordHash(data.NewPassword)
	tm := time.Now()
	user.LastLogoutTime = &tm
	user.LastLogoutIp = c.RealIP()
	user.Token = ""
	if err := s.db.Model(&user).Select("LastLogoutTime", "LastLogoutIp", "Password", "Token").Updates(&user).Error; err != nil {
		log.Println(err)
	}

	//Blacklist token
	if accessToken.JWT != nil {
		if err := s.memDB.BlacklistToken(accessToken.JWT.Signature); err != nil {
			return EError(c, err)
		}
	} else {
		if !user.IsActive {
			return EErrorDefined(c, apierrors.ErrLoginTriesExceed)
		}
	}

	//Reset all user sessions
	if err := dao.ResetUserSessions(s.db, &user); err != nil {
		return EError(c, err)
	}
	s.notificationsService.Ws.CloseUserSessions(user.ID)

	//Email notification
	if err := s.emailService.ChangePasswordNotify(user); err != nil {
		return EError(c, err)
	}

	response := PasswordResponse{
		Status:  http.StatusOK,
		Message: "Password updated successfully",
	}

	return c.JSON(http.StatusOK, response)
}

// changeMyEmail godoc
// @id changeMyEmail
// @Summary Пользователи (управление доступом): смена email текущего пользователя
// @Description Позволяет текущему пользователю изменить свой Email. В случае успеха отправляет код верификации на новую почту.
// @Tags Users
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body EmailRequest true "Новые данные email"
// @Success 200 "Проверочный код отправлен"
// @Failure 400 {object} apierrors.DefinedError "Ошибка"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 429 {object} apierrors.DefinedError "Слишком частые запросы"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/users/me/change-email/ [post]
func (s *Services) changeMyEmail(c echo.Context) error {
	user := *c.(AuthContext).User

	var data EmailRequest
	if err := c.Bind(&data); err != nil {
		return EError(c, err)
	}

	newEmail := strings.TrimSpace(strings.ToLower(data.NewEmail))

	if newEmail == user.Email {
		return EErrorDefined(c, apierrors.ErrEmailIsExist)
	}

	if !ValidateEmail(newEmail) {
		return EErrorDefined(c, apierrors.ErrInvalidEmail.WithFormattedMessage(newEmail))
	}

	res, err := s.memDB.SaveEmailCode(uuid5.UUID(uuid.FromStringOrNil(user.ID)), newEmail)
	if err != nil {
		if errors.Is(err, memErr.ErrLimitEmailCodeReached) {
			return EErrorDefined(c, apierrors.ErrEmailChangeLimit)
		}
		return EError(c, err)
	}

	err = s.emailService.UserChangeEmailNotify(user, newEmail, res.Code)
	if err != nil {
		return EError(c, err)
	}

	return c.NoContent(http.StatusOK)
}

// verifyMyEmail godoc
// @id verifyMyEmail
// @Summary Пользователи (управление доступом): Верификация Email
// @Description Позволяет текущему пользователю изменить свой Email. Сравнивает код верификации отправленый на новый Email.
// @Tags Users
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body EmailVerifyRequest true "Новые данные email"
// @Success 200 "Email пользователя изменен"
// @Failure 400 {object} apierrors.DefinedError "Oшибка"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/users/me/verification-email/ [post]
func (s *Services) verifyMyEmail(c echo.Context) error {
	user := *c.(AuthContext).User

	var data EmailVerifyRequest
	if err := c.Bind(&data); err != nil {
		return EError(c, err)
	}

	newEmail := strings.TrimSpace(strings.ToLower(data.NewEmail))

	if newEmail == user.Email {
		return EErrorDefined(c, apierrors.ErrEmailIsExist)
	}

	if !ValidateEmail(newEmail) {
		return EErrorDefined(c, apierrors.ErrInvalidEmail.WithFormattedMessage(newEmail))
	}

	var existUser dao.User
	if err := s.db.Where("email = ?", newEmail).Find(&existUser).Error; err != nil {
		return EError(c, err)
	}

	code, err := s.memDB.VerifyEmailCode(uuid5.UUID(uuid.FromStringOrNil(user.ID)), newEmail, data.Code)
	if err != nil {
		if errors.Is(err, memErr.ErrVerification) {
			return EErrorDefined(c, apierrors.ErrEmailVerify)
		}
		return EError(c, err)
	}

	if !code {
		return EErrorDefined(c, apierrors.ErrEmailVerify)
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if existUser.ID != "" {
			if err := s.business.ReplaceUser(tx, user.ID, existUser.ID); err != nil {
				return err
			}

			//Reset all old user sessions
			if err := dao.ResetUserSessions(tx, &user); err != nil {
				return err
			}
			s.notificationsService.Ws.CloseUserSessions(user.ID)

			if !c.(AuthContext).TokenAuth {
				if err := s.memDB.BlacklistToken(c.(AuthContext).AccessToken.JWT.Signature); err != nil {
					return err
				}

				if err := s.memDB.BlacklistToken(c.(AuthContext).RefreshToken.JWT.Signature); err != nil {
					return err
				}

				clearAuthCookies(c)
			}

			if err := tx.Where("id = ?", user.ID).Delete(&dao.User{}).Error; err != nil {
				return err
			}

			return nil
		}
		fmt.Println("shit")
		user.Email = newEmail
		return tx.Select("email").Updates(&user).Error
	}); err != nil {
		return EError(c, err)
	}

	return c.NoContent(http.StatusOK)
}

// resetPassword godoc
// @id resetPassword
// @Summary Пользователи (управление доступом): сброс пароля по ссылке из почты
// @Description Позволяет пользователю сбросить пароль после перехода по ссылке из письма. Сбрасывает все активные сеансы пользователя после успешного обновления пароля.
// @Tags Users
// @Accept json
// @Produce json
// @Param uidb64 path string true "ID пользователя в формате base64"
// @Param token path string true "Токен для сброса пароля"
// @Param data body PasswordRequest true "Новые данные пароля"
// @Success 200 {object} PasswordResponse "Пароль успешно сброшен"
// @Failure 400 {object} apierrors.DefinedError "Пароли не совпадают или некорректные данные"
// @Failure 401 {object} apierrors.DefinedError "Невалидный токен сброса пароля"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/reset-password/{uidb64}/{token}/ [post]
func (s *Services) resetPassword(c echo.Context) error {
	uidb64 := c.Param("uidb64")
	token := c.Param("token")

	id, err := base64.StdEncoding.DecodeString(uidb64)
	if err != nil {
		return EError(c, err)
	}

	var user dao.User
	if err := s.db.Where("id = ?", string(id)).Where("token = ?", token).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return EErrorDefined(c, apierrors.ErrInvalidResetToken)
		}
		return EError(c, err)
	}

	var data PasswordRequest
	if err := c.Bind(&data); err != nil {
		return EError(c, err)
	}

	if data.NewPassword != data.ConfirmPassword {
		return EErrorDefined(c, apierrors.ErrPasswordsNotEqual)
	}

	user.Password = dao.GenPasswordHash(data.NewPassword)
	tm := time.Now()
	user.LastLogoutTime = &tm
	user.LastLogoutIp = c.RealIP()
	user.Token = ""
	if err := s.db.Model(&user).Select("LastLogoutTime", "LastLogoutIp", "Password", "Token").Updates(&user).Error; err != nil {
		return EError(c, err)
	}

	//Reset all user sessions
	if err := dao.ResetUserSessions(s.db, &user); err != nil {
		return EError(c, err)
	}
	s.notificationsService.Ws.CloseUserSessions(user.ID)

	//Email notification
	if err := s.emailService.ChangePasswordNotify(user); err != nil {
		return EError(c, err)
	}

	response := PasswordResponse{
		Status:  http.StatusOK,
		Message: "Password updated successfully",
	}

	return c.JSON(http.StatusOK, response)
}

// signOut godoc
// @id signOut
// @Summary Пользователи (управление доступом): выход из текущей сессии
// @Description Завершает текущую сессию пользователя, обновляя время и IP последнего выхода. Если refresh-токен отсутствует, возвращает ошибку.
// @Tags Users
// @Security ApiKeyAuth
// @Success 200 "Успешный выход из текущей сессии"
// @Failure 400 {object} apierrors.DefinedError "Требуется refresh-токен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/sign-out/ [post]
func (s *Services) signOut(c echo.Context) error {
	if c.(AuthContext).TokenAuth {
		return c.NoContent(http.StatusOK)
	}

	if refreshToken := c.(AuthContext).RefreshToken; refreshToken == nil {
		return EErrorDefined(c, apierrors.ErrRefreshTokenRequired)
	} else {
		u := *c.(AuthContext).User
		tm := time.Now()
		u.LastLogoutTime = &tm
		u.LastLogoutIp = c.RealIP()
		if err := s.db.Model(&u).Select("LastLogoutTime", "LastLogoutIp").Updates(&u).Error; err != nil {
			return EError(c, err)
		}

		if err := s.memDB.BlacklistToken(c.(AuthContext).AccessToken.JWT.Signature); err != nil {
			return EError(c, err)
		}

		if err := s.memDB.BlacklistToken(refreshToken.JWT.Signature); err != nil {
			return EError(c, err)
		}

		clearAuthCookies(c)

		return c.NoContent(http.StatusOK)
	}
}

// signOutEverywhere godoc
// @id signOutEverywhere
// @Summary Пользователи (управление доступом): выход из всех сессий
// @Description Завершает все активные сессии пользователя, обновляя время и IP последнего выхода. Если refresh-токен отсутствует, возвращает ошибку.
// @Tags Users
// @Security ApiKeyAuth
// @Success 200 "Успешный выход из всех сессий"
// @Failure 400 {object} apierrors.DefinedError "Требуется refresh-токен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/sign-out-everywhere/ [post]
func (s *Services) signOutEverywhere(c echo.Context) error {
	if refreshToken := c.(AuthContext).RefreshToken; refreshToken == nil {
		return EErrorDefined(c, apierrors.ErrRefreshTokenRequired)
	} else {
		u := *c.(AuthContext).User
		tm := time.Now()
		u.LastLogoutTime = &tm
		u.LastLogoutIp = c.RealIP()
		if err := s.db.Model(&u).Select("LastLogoutTime", "LastLogoutIp").Updates(&u).Error; err != nil {
			return EError(c, err)
		}

		//Reset all user sessions
		if err := dao.ResetUserSessions(s.db, &u); err != nil {
			return EError(c, err)
		}
		s.notificationsService.Ws.CloseUserSessions(u.ID)

		if c.(AuthContext).TokenAuth {
			return c.NoContent(http.StatusOK)
		}

		if err := s.memDB.BlacklistToken(c.(AuthContext).AccessToken.JWT.Signature); err != nil {
			return EError(c, err)
		}

		if err := s.memDB.BlacklistToken(refreshToken.JWT.Signature); err != nil {
			return EError(c, err)
		}

		clearAuthCookies(c)

		return c.NoContent(http.StatusOK)
	}
}

// resetUserPassword godoc
// @id resetUserPassword
// @Summary Пользователи (управление доступом): смена пароля другого пользователя (только для суперпользователя)
// @Description Позволяет суперпользователю изменить пароль другого пользователя, идентифицированного через `uidb64`.
// @Tags Users
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param uidb64 path string true "ID пользователя в формате base64"
// @Param data body PasswordRequest true "Новые данные пароля"
// @Success 200 {object} PasswordResponse "Пароль успешно изменен"
// @Failure 400 {object} apierrors.DefinedError "Пароли не совпадают или некорректные данные"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/reset-user-password/{uidb64}/ [post]
func (s *Services) resetUserPassword(c echo.Context) error {
	admin := *c.(AuthContext).User
	uidb64 := c.Param("uidb64")

	if !admin.IsSuperuser {
		return EErrorDefined(c, apierrors.ErrChangePasswordForbidden)
	}

	id, err := base64.StdEncoding.DecodeString(uidb64)
	if err != nil {
		return EError(c, err)
	}

	var user dao.User
	if err := s.db.Where("id = ?", string(id)).Find(&user).Error; err != nil {
		return EError(c, err)
	}

	var data PasswordRequest
	if err := c.Bind(&data); err != nil {
		return EError(c, err)
	}

	if data.NewPassword != data.ConfirmPassword {
		return EErrorDefined(c, apierrors.ErrPasswordsNotEqual)
	}

	user.Password = dao.GenPasswordHash(data.NewPassword)
	tm := time.Now()
	user.LastLogoutTime = &tm
	user.LastLogoutIp = c.RealIP()
	if err := s.db.Model(&user).Select("LastLogoutTime", "LastLogoutIp", "Password").Updates(&user).Error; err != nil {
		log.Println(err)
	}

	// Reset all user sessions
	if err := dao.ResetUserSessions(s.db, &user); err != nil {
		return EError(c, err)
	}
	s.notificationsService.Ws.CloseUserSessions(user.ID)

	response := PasswordResponse{
		Status:  http.StatusOK,
		Message: "Password updated successfully",
	}

	return c.JSON(http.StatusOK, response)
}

// getTGBotLink godoc
// @id getTGBotLink
// @Summary Интеграции: получение ссылки на Telegram бота
// @Description Возвращает ссылку для подключения к Telegram боту уведомлений, если он доступен
// @Tags Integrations
// @Produce json
// @Success 200 {object} map[string]string "Ссылка на Telegram бота"
// @Success 200 {object} map[string]bool "Поле 'disabled': true, если бот недоступен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/notification-bot-link/ [get]
func (s *Services) getTGBotLink(c echo.Context) error {
	if s.notificationsService.Tg.GetBotLink() != "" {
		return c.JSON(http.StatusOK, map[string]string{
			"url": s.notificationsService.Tg.GetBotLink(),
		})
	}
	return c.JSON(http.StatusOK, map[string]bool{
		"disabled": true,
	})
}

// getCurrentUserAllProjectList godoc
// @id getCurrentUserAllProjectList
// @Summary Пользователи: получение всех проектов текущего пользователя
// @Description Возвращает список всех проектов, к которым принадлежит текущий пользователь, с возможностью поиска по имени
// @Tags Users
// @Security ApiKeyAuth
// @Produce json
// @Param search_query query string false "Строка поиска по имени проекта"
// @Success 200 {array} dto.ProjectLight "Список проектов пользователя"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/users/me/all/projects/ [get]
func (s *Services) getCurrentUserAllProjectList(c echo.Context) error {
	user := *c.(AuthContext).User

	searchQuery := ""
	if err := echo.QueryParamsBinder(c).
		String("search_query", &searchQuery).
		BindError(); err != nil {
		return EError(c, err)
	}
	var projects []dao.Project
	query := dao.PreloadProjectMembersWithFilters(
		s.db.
			Preload(clause.Associations).
			Preload("Workspace.Owner").
			Order("name"))

	if searchQuery != "" {
		escapedSearchQuery := PrepareSearchRequest(searchQuery)
		query = query.Where("lower(name) LIKE ? OR name_tokens @@ plainto_tsquery('russian', lower(?))", escapedSearchQuery, searchQuery)
	}

	query = query.Where("id in (?)",
		s.db.Model(&dao.ProjectMember{}).
			Select("project_id").
			Where("member_id = ?", user.ID))

	if err := query.Find(&projects).Error; err != nil {
		return EError(c, err)
	}

	projectsDTO := make([]dto.ProjectLight, 0)
	for _, project := range projects {
		projectsDTO = append(projectsDTO, *project.ToLightDTO())
	}
	return c.JSON(http.StatusOK, projectsDTO)
}

// getMyAuthToken godoc
// @id getMyAuthToken
// @Summary Пользователи (управление доступом): получение текущего токена авторизации пользователя
// @Description Возвращает текущий токен авторизации пользователя, если он существует
// @Tags Users
// @Security ApiKeyAuth
// @Produce plain
// @Success 200 {string} string "Текущий токен авторизации"
// @Failure 204 "Токен не установлен"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/users/me/token/ [get]
func (s *Services) getMyAuthToken(c echo.Context) error {
	user := *c.(AuthContext).User
	if user.AuthToken == nil {
		return c.NoContent(http.StatusNoContent)
	}
	return c.String(http.StatusOK, *user.AuthToken)
}

// resetMyAuthToken godoc
// @id resetMyAuthToken
// @Summary Пользователи (управление доступом): сброс токена авторизации пользователя
// @Description Создает новый токен авторизации для текущего пользователя и возвращает успешный ответ
// @Tags Users
// @Security ApiKeyAuth
// @Produce json
// @Success 201 "Новый токен создан"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/users/me/token/reset/ [post]
func (s *Services) resetMyAuthToken(c echo.Context) error {
	user := *c.(AuthContext).User

	if err := s.db.Model(&user).UpdateColumn("auth_token", password.MustGenerate(64, 30, 0, false, true)).Error; err != nil {
		return EError(c, err)
	}
	return c.NoContent(http.StatusCreated)
}

// signUp godoc
// @id signUp
// @Summary Пользователи (управление доступом): регистрация нового пользователя
// @Description Регистрирует нового пользователя по email, проверяет капчу и отправляет временный пароль по email
// @Tags Users
// @Accept json
// @Produce json
// @Param data body EmailCaptchaRequest true "Данные для регистрации пользователя"
// @Success 200 "Пользователь успешно зарегистрирован"
// @Failure 400 {object} apierrors.DefinedError "Некорректные данные"
// @Failure 403 {object} apierrors.DefinedError "Регистрация отключена"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/sign-up/ [post]
func (s *Services) signUp(c echo.Context) error {
	if !cfg.SignUpEnable {
		return EErrorDefined(c, apierrors.ErrSignupDisabled)
	}

	var req EmailCaptchaRequest
	if err := c.Bind(&req); err != nil {
		return EError(c, err)
	}

	req.Email = strings.ToLower(req.Email)

	if !ValidateEmail(req.Email) {
		return EErrorDefined(c, apierrors.ErrInvalidEmail.WithFormattedMessage(req.Email))
	}

	if !cfg.CaptchaDisabled && !CaptchaService.Validate(req.CaptchaPayload) {
		return EErrorDefined(c, apierrors.ErrCaptchaFail)
	}

	var exist bool
	if err := s.db.Model(&dao.User{}).
		Select("EXISTS(?)",
			s.db.Model(&dao.User{}).
				Select("1").
				Where("email = ?", req.Email),
		).
		Find(&exist).Error; err != nil {
		return EError(c, err)
	}
	if exist {
		// For security reasons wait 5 seconds
		time.Sleep(time.Second * 5)
		return EErrorDefined(c, apierrors.ErrUserAlreadyExist)
	}

	pass := dao.GenPassword()
	user := dao.User{
		ID:       dao.GenID(),
		Email:    req.Email,
		Password: dao.GenPasswordHash(pass),
		Theme:    types.DefaultTheme,
		IsActive: true,
	}

	if err := s.db.Create(&user).Error; err != nil {
		return EError(c, err)
	}

	for i := range 5 {
		err := s.emailService.NewUserPasswordNotify(user, pass)
		if err == nil {
			return c.NoContent(http.StatusOK)
		}
		slog.Error("Send user sign up email notification", "email", user.Email, "try", i+1, "err", err)
		time.Sleep(time.Second * 5)
	}
	// If failed to deliver mail delete user and return error
	if err := s.db.Unscoped().Delete(&user).Error; err != nil {
		slog.Error("Delete failed user", "err", err)
	}
	return EErrorDefined(c, apierrors.ErrNewUserMailFailed)
}

// ############# User feedback ###################

// getMyFeedback godoc
// @id getMyFeedback
// @Summary Пользователи: получение отзыва текущего пользователя
// @Description Возвращает отзыв, оставленный текущим пользователем
// @Tags Users
// @Security ApiKeyAuth
// @Produce json
// @Success 200 {object} dto.UserFeedback "Отзыв пользователя"
// @Success 204 "Отзыв не найден"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/users/me/feedback/ [get]
func (s *Services) getMyFeedback(c echo.Context) error {
	user := c.(AuthContext).User

	var feedback dao.UserFeedback
	if err := s.db.Where("user_id = ?", user.ID).Preload(clause.Associations).First(&feedback).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.NoContent(http.StatusNoContent)
		}
		return EError(c, err)
	}
	return c.JSON(http.StatusOK, feedback.ToDTO())
}

// createMyFeedback godoc
// @id createMyFeedback
// @Summary Пользователи: отправка отзыва от текущего пользователя
// @Description Сохраняет или обновляет отзыв, предоставленный текущим пользователем
// @Tags Users
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body PostFeedbackRequest true "Данные отзыва"
// @Success 200 "Отзыв успешно обновлен"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/users/me/feedback/ [post]
func (s *Services) createMyFeedback(c echo.Context) error {
	user := c.(AuthContext).User

	var feedback PostFeedbackRequest
	if err := c.Bind(&feedback); err != nil {
		return EError(c, err)
	}

	if feedback.Stars > 5 || feedback.Stars < 0 {
		feedback.Stars = 5
	}

	if err := s.db.Save(&dao.UserFeedback{
		UserID:    user.ID,
		UpdatedAt: time.Now(),
		Stars:     feedback.Stars,
		Feedback:  feedback.Feedback,
	}).Error; err != nil {
		return EError(c, err)
	}
	return c.NoContent(http.StatusOK)
}

// deleteMyFeedback godoc
// @id deleteMyFeedback
// @Summary Пользователи: удаление отзыва текущего пользователя
// @Description Удаляет отзыв, предоставленную текущим пользователем
// @Tags Users
// @Security ApiKeyAuth
// @Produce json
// @Success 200 "Отзыв успешно удален"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/users/me/feedback/ [delete]
func (s *Services) deleteMyFeedback(c echo.Context) error {
	user := c.(AuthContext).User

	if err := s.db.Where("user_id = ?", user.ID).Delete(&dao.UserFeedback{}).Error; err != nil {
		return EError(c, err)
	}
	return c.NoContent(http.StatusOK)
}

// getMyNotificationList godoc
// @id getMyNotificationList
// @Summary Пользователи: Получение уведомлений
// @Description Позволяет пользователю получить список своих уведомлений с поддержкой пагинации.
// @Tags Notifications
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param offset query int false "Смещение для пагинации" default(0)
// @Param limit query int false "Количество записей на странице" default(100)
// @Success 200 {object} dao.PaginationResponse{result=[]notifications.NotificationResponse} "Список уведомлений"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/users/me/notifications [get]
func (s *Services) getMyNotificationList(c echo.Context) error {
	user := c.(AuthContext).User
	offset := -1
	limit := 100

	if err := echo.QueryParamsBinder(c).
		Int("offset", &offset).
		Int("limit", &limit).BindError(); err != nil {
		return EError(c, err)
	}

	// TODO refactoring & add other Activities
	var issue dao.IssueActivity
	issue.UnionCustomFields = "'issue' AS entity_type"
	var project dao.ProjectActivity
	project.UnionCustomFields = "'project' AS entity_type"
	var doc dao.DocActivity
	doc.UnionCustomFields = "'doc' AS entity_type"
	var form dao.FormActivity
	form.UnionCustomFields = "'form' AS entity_type"
	var workspace dao.WorkspaceActivity
	workspace.UnionCustomFields = "'workspace' AS entity_type"

	unionTable := dao.BuildUnionSubquery(s.db, "ua", dao.FullActivity{}, issue, project, doc, form, workspace)

	var userNotifications []dao.UserNotifications

	getActivityId := func(u *dao.UserNotifications) *string {
		if u.IssueActivityId != nil {
			return u.IssueActivityId
		}
		if u.ProjectActivityId != nil {
			return u.ProjectActivityId
		}
		if u.DocActivityId != nil {
			return u.DocActivityId
		}
		if u.FormActivityId != nil {
			return u.FormActivityId
		}
		if u.WorkspaceActivityId != nil {
			return u.WorkspaceActivityId
		}
		return nil
	}

	query := s.db.
		//Preload("EntityActivity").
		//Preload("EntityActivity.Actor").
		//Preload("EntityActivity.Issue").
		//Preload("EntityActivity.Project").
		//Preload("EntityActivity.Workspace").
		Joins("IssueActivity").
		Joins("ProjectActivity").
		Joins("DocActivity").

		//Joins("IssueActivity").
		//Preload("IssueActivity.Actor").
		//Preload("IssueActivity.Issue").
		//Preload("IssueActivity.Project").
		//Preload("IssueActivity.Workspace").
		//Joins("ProjectActivity").
		//Preload("ProjectActivity.Actor").
		//Preload("ProjectActivity.Project").
		//Preload("ProjectActivity.Workspace").
		//Joins("WorkspaceActivity").
		//Preload("WorkspaceActivity.Actor").
		//Preload("WorkspaceActivity.Workspace").
		//Joins("DocActivity").
		//Preload("DocActivity.Actor").
		//Preload("DocActivity.Workspace").
		//Preload("DocActivity.Doc").
		//Joins("FormActivity").
		//Preload("FormActivity.Actor").
		//Preload("FormActivity.Workspace").
		//Preload("FormActivity.Form").
		//Joins("RootActivity").
		//Preload("RootActivity.Actor").
		Joins("Comment").
		Preload("Comment.Actor").
		Preload("Comment.Issue").
		Preload("Comment.Project").
		Preload("Comment.Workspace").
		Joins("Workspace").
		Joins("Author").
		Joins("Issue").
		Preload("Issue.Project").
		Where("user_id = ?", user.ID).Order("created_at desc")

	resp, err := dao.PaginationRequest(
		offset,
		limit,
		query,
		&userNotifications,
	)
	if err != nil {
		return EErrorDefined(c, apierrors.ErrGeneric)
	}

	elements := utils.All(*resp.Result.(*[]dao.UserNotifications))
	elements = utils.Filter(
		elements,
		func(t dao.UserNotifications) bool {
			if id := getActivityId(&t); id != nil {
				return true
			}
			return false
		})

	res := utils.Collect(elements)

	qqq := utils.SliceToSlice(&res, func(t *dao.UserNotifications) string {
		if id := getActivityId(t); id != nil {
			return *id
		}
		return ""
	})

	var fa []dao.FullActivity
	if err := unionTable.
		Joins("Project").
		Joins("Workspace").
		Joins("Actor").
		Joins("Issue").
		Joins("Doc").
		Where("ua.id IN (?)", qqq).
		Find(&fa).Error; err != nil {
		if err != gorm.ErrRecordNotFound {
			return EErrorDefined(c, apierrors.ErrGeneric)
		}
	}

	idMap := utils.SliceToMap(&fa, func(t *dao.FullActivity) string {
		return t.Id
	})

	for i := 0; i < len(*resp.Result.(*[]dao.UserNotifications)); i++ {
		var id *string
		if results, ok := resp.Result.(*[]dao.UserNotifications); ok {
			id = getActivityId(&(*results)[i])
			if id == nil {
				continue
			}

			if v, ok := idMap[*id]; ok {
				(*resp.Result.(*[]dao.UserNotifications))[i].FullActivity = &v
			}
		}
	}

	resp.Result = userNotifyToSimple(s.db, resp.Result)
	return c.JSON(http.StatusOK, resp)
}

func userNotifyToSimple(tx *gorm.DB, from interface{}) *[]notifications.NotificationResponse {
	temp := from.(*[]dao.UserNotifications)
	res := make([]notifications.NotificationResponse, 0, len(*temp))
	for _, notify := range *temp {
		tmp := notifications.NotificationResponse{
			Id:        notify.ID,
			Type:      notify.Type,
			Viewed:    &notify.Viewed,
			CreatedAt: notify.CreatedAt.UTC(),
		}

		switch tmp.Type {
		case "service_message":
			tmp.Data = notifications.NotificationResponseMessage{
				Title: notify.Title,
				Msg:   notify.Msg,
			}
		case "message":
			var project *dto.ProjectLight

			if notify.Issue != nil {
				project = notify.Issue.Project.ToLightDTO()
			}
			tmp.Detail = notifications.NotificationDetailResponse{
				User:      notify.Author.ToLightDTO(),
				Issue:     notify.Issue.ToLightDTO(),
				Project:   project,
				Workspace: notify.Workspace.ToLightDTO(),
			}
			tmp.Data = notifications.NotificationResponseMessage{
				Title: notify.Title,
				Msg:   notify.Msg,
			}
		case "comment":
			if notify.Comment != nil {
				tmp.Detail = notifications.NotificationDetailResponse{
					User:      notify.Comment.Actor.ToLightDTO(),
					Issue:     notify.Comment.Issue.ToLightDTO(),
					Project:   notify.Comment.Project.ToLightDTO(),
					Workspace: notify.Comment.Workspace.ToLightDTO(),
				}
			}
			tmp.Data = notify.Comment.ToLightDTO()
		case "activity":
			if notify.FullActivity != nil {
				tmp.Detail = notifications.NotificationDetailResponse{
					User:      notify.FullActivity.Actor.ToLightDTO(),
					Issue:     notify.FullActivity.Issue.ToLightDTO(),
					Project:   notify.FullActivity.Project.ToLightDTO(),
					Workspace: notify.FullActivity.Workspace.ToLightDTO(),
					Doc:       notify.FullActivity.Doc.ToLightDTO(),
				}
				entityActivity := notify.FullActivity.ToLightDTO()
				//entityActivity.NewEntity = dao.GetActionEntity(*notify.FullActivity, "New")
				//entityActivity.OldEntity = dao.GetActionEntity(*notify.FullActivity, "Old")
				tmp.Data = entityActivity
			}
			if notify.EntityActivity != nil {
				tmp.Detail = notifications.NotificationDetailResponse{
					User:      notify.EntityActivity.Actor.ToLightDTO(),
					Issue:     notify.EntityActivity.Issue.ToLightDTO(),
					Project:   notify.EntityActivity.Project.ToLightDTO(),
					Workspace: notify.EntityActivity.Workspace.ToLightDTO(),
					//Doc:       notify.EntityActivity.Doc.ToLightDTO(),
				}
				//tmp.Data = notify.IssueActivity.ToLightDTO()
			}

			if notify.EntityActivity != nil {
				//var user dto.UserLight
				//if notify.TargetUser != nil {
				//	user = *notify.TargetUser.ToLightDTO()
				//}
				entityActivity := notify.EntityActivity.ToLightDTO()
				entityActivity.NewEntity = dao.GetActionEntity(*notify.EntityActivity, "New")
				entityActivity.OldEntity = dao.GetActionEntity(*notify.EntityActivity, "Old")
				tmp.Data = entityActivity
			}

		}
		res = append(res, tmp)

	}

	return &res
}

// deleteMyNotifications godoc
// @id deleteMyNotifications
// @Summary Пользователи: Удаление всех уведомлений
// @Description Позволяет пользователю удалить все свои уведомления из базы данных.
// @Tags Notifications
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Success 200 "Уведомления успешно удалены"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/users/me/notifications [delete]
func (s *Services) deleteMyNotifications(c echo.Context) error {
	user := c.(AuthContext).User

	if err := s.db.Where("user_id = ?", user.ID).Delete(&dao.UserNotifications{}).Error; err != nil {
		return EErrorDefined(c, apierrors.ErrGeneric)
	}
	return c.NoContent(http.StatusOK)
}

// updateToReadMyNotifications godoc
// @id updateToReadMyNotifications
// @Summary Пользователи: Пометить уведомления как прочитанные
// @Description Позволяет пользователю пометить определенные уведомления как прочитанные и возвращает количество непрочитанных уведомлений.
// @Tags Notifications
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body NotificationViewRequest true "Список идентификаторов уведомлений для пометки как прочитанные"
// @Success 200 {object} NotificationIdResponse "Количество непрочитанных уведомлений"
// @Failure 400 {object} apierrors.DefinedError "Некорректные идентификаторы уведомлений"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/users/me/notifications [post]
func (s *Services) updateToReadMyNotifications(c echo.Context) error {
	user := c.(AuthContext).User
	var count int

	var notification NotificationViewRequest
	if err := c.Bind(&notification); err != nil {
		return EErrorDefined(c, apierrors.ErrBadNotifyIds)
	}

	if notification.ViewedAll == true {
		if err := s.db.Model(&dao.UserNotifications{}).
			Where("user_id = ?", user.ID).
			Where("viewed = false").
			Where("deleted_at is NULL").
			Update("viewed", true).
			Error; err != nil {
			return EErrorDefined(c, apierrors.ErrGeneric)
		}
		count = 0
	} else {
		if err := s.db.Model(&dao.UserNotifications{}).
			Where("user_id = ?", user.ID).
			Where("id IN ?", notification.Ids).
			Where("deleted_at is NULL").
			Update("viewed", true).
			Error; err != nil {
			return EErrorDefined(c, apierrors.ErrGeneric)
		}

		if err := s.db.Select("count(*)").
			Where("viewed = false").
			Where("user_id = ?", user.ID).
			Where("deleted_at IS NULL").
			Model(&dao.UserNotifications{}).
			Find(&count).Error; err != nil {
			return EErrorDefined(c, apierrors.ErrGeneric)
		}
	}

	return c.JSON(http.StatusOK, NotificationIdResponse{count})
}

// ############# Search filters ###################

// getSearchFilterList godoc
// @id getSearchFilterList
// @Summary Фильтры: получение списка доступных фильтров поиска
// @Description Возвращает список доступных фильтров поиска с возможностью пагинации и поиска по имени
// @Tags Search Filters
// @Security ApiKeyAuth
// @Produce json
// @Param offset query int false "Смещение для пагинации" default(-1)
// @Param limit query int false "Лимит результатов" default(100)
// @Param search_query query string false "Строка поиска по имени фильтра"
// @Success 200 {object} dao.PaginationResponse{result=[]dto.SearchFilterLight} "Список доступных фильтров поиска"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/filters/ [get]
func (s *Services) getSearchFilterList(c echo.Context) error {
	user := c.(AuthContext).User

	offset := -1
	limit := 100
	searchQuery := ""

	if err := echo.QueryParamsBinder(c).
		Int("offset", &offset).
		Int("limit", &limit).
		String("search_query", &searchQuery).
		BindError(); err != nil {
		return EError(c, err)
	}

	query := s.db.Preload(clause.Associations)
	if !user.IsSuperuser {
		query = query.Where("public = true")
	}

	if searchQuery != "" {
		escapedSearchQuery := PrepareSearchRequest(searchQuery)
		query = query.Where("lower(name) like ? or name_tokens @@ plainto_tsquery('russian', lower(?))", escapedSearchQuery, searchQuery)
	}

	var filters []dao.SearchFilter
	resp, err := dao.PaginationRequest(
		offset,
		limit,
		query,
		&filters,
	)
	if err != nil {
		return EError(c, err)
	}

	result := resp.Result.(*[]dao.SearchFilter)
	filtersDTO := make([]dto.SearchFilterLight, 0)
	for _, filter := range *result {
		filtersDTO = append(filtersDTO, *filter.ToLightDTO())
	}

	resp.Result = &filtersDTO

	return c.JSON(http.StatusOK, resp)
}

// createSearchFilter godoc
// @id createSearchFilter
// @Summary Фильтры: создание нового фильтра поиска
// @Description Создает новый фильтр поиска для текущего пользователя
// @Tags Search Filters
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body dto.SearchFilterLight true "Данные нового фильтра поиска"
// @Success 201 {object} dto.SearchFilterFull "Созданный фильтр поиска"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/filters/ [post]
func (s *Services) createSearchFilter(c echo.Context) error {
	user := c.(AuthContext).User

	filter, _, err := bindSearchFilter(c, nil)
	if err != nil {
		return EError(c, err)
	}

	if err := s.db.Model(user).Association("SearchFilters").Append(filter); err != nil {
		return EError(c, err)
	}
	return c.JSON(http.StatusCreated, filter.ToFullDTO())
}

// getSearchFilter godoc
// @id getSearchFilter
// @Summary Фильтры: получение фильтра поиска
// @Description Возвращает данные фильтра поиска по его ID
// @Tags Search Filters
// @Security ApiKeyAuth
// @Produce json
// @Param filterId path string true "ID фильтра поиска"
// @Success 200 {object} dto.SearchFilterFull "Фильтр поиска"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Недостаточно прав для доступа к фильтру"
// @Failure 404 {object} apierrors.DefinedError "Фильтр не найден"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/filters/{filterId}/ [get]
func (s *Services) getSearchFilter(c echo.Context) error {
	filter := c.(SearchFilterContext).Filter
	return c.JSON(http.StatusOK, filter.ToFullDTO())
}

// updateSearchFilter godoc
// @id updateSearchFilter
// @Summary Фильтры: обновление фильтра поиска
// @Description Обновляет фильтр поиска по его ID для текущего пользователя
// @Tags Search Filters
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param filterId path string true "ID фильтра поиска"
// @Param data body dto.SearchFilterLight true "Данные для обновления фильтра"
// @Success 204 "Фильтр успешно обновлен"
// @Failure 400 {object} apierrors.DefinedError "Некорректные данные для обновления"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Недостаточно прав для обновления фильтра"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/filters/{filterId}/ [patch]
func (s *Services) updateSearchFilter(c echo.Context) error {
	filter := c.(SearchFilterContext).Filter
	user := c.(SearchFilterContext).User

	if filter.AuthorID != user.ID {
		return EErrorDefined(c, apierrors.ErrNotOwnFilter)
	}

	newFilter, fields, err := bindSearchFilter(c, &filter)
	if err != nil {
		return EError(c, err)
	}

	if err := s.db.Select(fields).Updates(newFilter).Error; err != nil {
		return EError(c, err)
	}
	return c.NoContent(http.StatusOK)
}

// deleteSearchFilter godoc
// @id deleteSearchFilter
// @Summary Фильтры: удаление фильтра поиска
// @Description Удаляет фильтр поиска по его ID для текущего пользователя или суперпользователя
// @Tags Search Filters
// @Security ApiKeyAuth
// @Param filterId path string true "ID фильтра поиска"
// @Success 204 "Фильтр успешно удален"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Недостаточно прав для удаления фильтра"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/filters/{filterId}/ [delete]
func (s *Services) deleteSearchFilter(c echo.Context) error {
	filter := c.(SearchFilterContext).Filter
	user := c.(SearchFilterContext).User

	if filter.AuthorID != user.ID && !user.IsSuperuser {
		return EErrorDefined(c, apierrors.ErrNotOwnFilter)
	}

	if err := s.db.Select(clause.Associations).Delete(&filter).Error; err != nil {
		return EError(c, err)
	}

	return c.NoContent(http.StatusOK)
}

// getMySearchFilterList godoc
// @id getMySearchFilterList
// @Summary Фильтры: получение фильтров поиска текущего пользователя
// @Description Возвращает список фильтров поиска, созданных текущим пользователем
// @Tags Search Filters
// @Security ApiKeyAuth
// @Produce json
// @Success 200 {array} dto.SearchFilterFull "Список фильтров поиска пользователя"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/users/me/filters/ [get]
func (s *Services) getMySearchFilterList(c echo.Context) error {
	user := c.(AuthContext).User

	var filters []dao.SearchFilter
	if err := s.db.Model(&user).Association("SearchFilters").Find(&filters); err != nil {
		return EError(c, err)
	}

	filtersDTO := make([]dto.SearchFilterLight, 0)
	for _, filter := range filters {
		filtersDTO = append(filtersDTO, *filter.ToLightDTO())
	}

	return c.JSON(http.StatusOK, filtersDTO)
}

// addSearchFilterToMe godoc
// @id addSearchFilterToMe
// @Summary Фильтры: добавление фильтра поиска в список текущего пользователя
// @Description Добавляет указанный фильтр поиска в список доступных фильтров текущего пользователя
// @Tags Search Filters
// @Security ApiKeyAuth
// @Param filterId path string true "ID фильтра поиска"
// @Success 200 "Фильтр добавлен в список пользователя"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Невозможно добавить приватный фильтр, недостаточно прав"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/users/me/filters/{filterId}/ [post]
func (s *Services) addSearchFilterToMe(c echo.Context) error {
	filter := c.(SearchFilterContext).Filter
	user := c.(SearchFilterContext).User

	if !filter.Public && !user.IsSuperuser {
		return EErrorDefined(c, apierrors.ErrCannotAddNonPublicFilter)
	}

	if err := s.db.Model(&user).Association("SearchFilters").Append(&filter); err != nil {
		return EError(c, err)
	}
	return c.NoContent(http.StatusOK)
}

// deleteSearchFilterFromMe godoc
// @id deleteSearchFilterFromMe
// @Summary Фильтры: удаление фильтра поиска из списка текущего пользователя
// @Description Удаляет указанный фильтр поиска из списка доступных фильтров текущего пользователя
// @Tags Search Filters
// @Security ApiKeyAuth
// @Param filterId path string true "ID фильтра поиска"
// @Success 200 "Фильтр успешно удален из списка пользователя"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Невозможно удалить собственный фильтр"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/users/me/filters/{filterId}/ [delete]
func (s *Services) deleteSearchFilterFromMe(c echo.Context) error {
	filter := c.(SearchFilterContext).Filter
	user := c.(SearchFilterContext).User

	if filter.AuthorID == user.ID {
		return EErrorDefined(c, apierrors.ErrCannotRemoveOwnFilter)
	}

	if err := s.db.Model(&user).Association("SearchFilters").Delete(&filter); err != nil {
		return EError(c, err)
	}
	return c.NoContent(http.StatusOK)
}

// getFilterMemberList godoc
// @id getFilterMemberList
// @Summary Проекты: получение списка участников выбранных проектов
// @Description Возвращает список участников (пользователей) для указанных проектов с возможностью поиска по имени пользователя.
// @Tags Projects
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param offset query int false "Смещение для пагинации" default(0)
// @Param limit query int false "Лимит записей" default(100)
// @Param data body FilterParams true "Параметры фильтрации участников"
// @Success 200 {object} dao.PaginationResponse{result=[]dto.UserLight} "Список участников проектов"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/filters/members/ [post]
func (s *Services) getFilterMemberList(c echo.Context) error {
	user, param, offset, limit, err := ExtractFilterRequest(c)
	if err != nil {
		return EError(c, err)
	}

	userIds := dao.GetUserNeighbors(s.db, user.ID, param.WorkspaceIDs, param.ProjectIDs)

	var users []dao.User

	query := s.db.Preload(clause.Associations).
		Where("is_integration = ? AND is_bot = ?", false, false).
		Where("id IN (?)", userIds)

	if param.SearchQuery != "" {
		searchQuery := "%" + strings.ToLower(param.SearchQuery) + "%"
		query = query.
			Where("lower(username) LIKE ? OR lower(first_name) LIKE ? OR lower(last_name) LIKE ?", searchQuery, searchQuery, searchQuery)
	}

	query = query.
		Order("CASE WHEN last_name = '' THEN 1 ELSE 0 END").
		Order("lower(last_name)").
		Order("lower(first_name)").
		Order("lower(email)")

	resp, err := dao.PaginationRequest(offset, limit, query, &users)
	if err != nil {
		if err != gorm.ErrRecordNotFound {
			return EError(c, err)
		}
	}

	result := resp.Result.(*[]dao.User)
	usersDTO := make([]dto.UserLight, 0)
	for _, user := range *result {
		usersDTO = append(usersDTO, *user.ToLightDTO())
	}

	resp.Result = &usersDTO

	return c.JSON(http.StatusOK, resp)
}

// getFilterStateList godoc
// @id getFilterStateList
// @Summary Проекты: получение списка статусов выбранных проектов
// @Description Возвращает список статусов для указанных проектов с возможностью поиска по названию статуса или проекта
// @Tags Projects
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param offset query int false "Смещение для пагинации" default(0)
// @Param limit query int false "Лимит записей" default(100)
// @Param data body FilterParams true "Параметры фильтрации статусов"
// @Success 200 {object} dao.PaginationResponse{result=[]dto.StateLight} "Список статусов проектов"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/filters/states/ [post]
func (s *Services) getFilterStateList(c echo.Context) error {
	user, param, offset, limit, err := ExtractFilterRequest(c)
	if err != nil {
		return EError(c, err)
	}

	projectQuery := dao.PrepareFilterProjectsQuery(s.db, user.ID, param.WorkspaceIDs, param.ProjectIDs)

	var states []dao.State

	stateQuery := s.db.Preload("Project").
		Joins("JOIN projects ON projects.id = states.project_id").
		Where("project_id IN (?)", projectQuery)

	if param.SearchQuery != "" {
		searchQuery := strings.ToLower(param.SearchQuery)
		stateQuery = stateQuery.Where("lower(states.name) LIKE ? OR lower(projects.name) LIKE ?",
			"%"+searchQuery+"%", "%"+searchQuery+"%")
	}

	resp, err := dao.PaginationRequest(offset, limit, stateQuery, &states)
	if err != nil {
		if err != gorm.ErrRecordNotFound {
			return EError(c, err)
		}
	}

	result := resp.Result.(*[]dao.State)
	statesDTO := make([]dto.StateLight, 0)
	for _, state := range *result {
		statesDTO = append(statesDTO, *state.ToLightDTO())
	}

	resp.Result = &statesDTO

	return c.JSON(http.StatusOK, resp)
}

// getFilterLabelList godoc
// @id getFilterLabelList
// @Summary Проекты: получение списка тегов (меток) выбранных проектов
// @Description Возвращает список тегов для указанных проектов с возможностью поиска по названию тега или проекта
// @Tags Projects
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param offset query int false "Смещение для пагинации" default(0)
// @Param limit query int false "Лимит записей" default(100)
// @Param data body FilterParams true "Параметры фильтрации тегов"
// @Success 200 {object} dao.PaginationResponse{result=[]dto.LabelLight} "Список тегов проектов"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/filters/labels/ [post]
func (s *Services) getFilterLabelList(c echo.Context) error {
	user, param, offset, limit, err := ExtractFilterRequest(c)
	if err != nil {
		return EError(c, err)
	}

	projectQuery := dao.PrepareFilterProjectsQuery(s.db, user.ID, param.WorkspaceIDs, param.ProjectIDs)

	var labels []dao.Label

	labelQuery := s.db.Preload("Project").
		Joins("JOIN projects ON projects.id = labels.project_id").
		Where("project_id IN (?)", projectQuery)

	if param.SearchQuery != "" {
		searchQuery := strings.ToLower(param.SearchQuery)
		labelQuery = labelQuery.Where("lower(labels.name) LIKE ? OR lower(projects.name) LIKE ?",
			"%"+searchQuery+"%", "%"+searchQuery+"%")
	}

	resp, err := dao.PaginationRequest(offset, limit, labelQuery, &labels)
	if err != nil {
		if err != gorm.ErrRecordNotFound {
			return EError(c, err)
		}
	}

	result := resp.Result.(*[]dao.Label)
	labelsDTO := make([]dto.LabelLight, 0)
	for _, label := range *result {
		labelsDTO = append(labelsDTO, *label.ToLightDTO())
	}

	resp.Result = &labelsDTO
	return c.JSON(http.StatusOK, resp)
}

// getRecentReleaseNoteList godoc
// @id getRecentReleaseNoteList
// @Summary Релизы: получение примечаний к релизу начиная с указанной версии
// @Description Возвращает информацию о примечаниях к релизу
// @Tags ReleaseNotes
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param noteId path string true "ID или версия релиза примечания к релизу"
// @Success 200 {array} dto.ReleaseNoteLight "Информация о примечании к релизу"
// @Failure 403 {object} apierrors.DefinedError "Ошибка: доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Внутренняя ошибка сервера"
// @Router /api/auth/release-notes/{noteId} [get]
func (s *Services) getRecentReleaseNoteList(c echo.Context) error {
	noteContext := c.(ReleaseNoteContext)

	var notes []dao.ReleaseNote
	if err := s.db.Where("tag_name >= ?", noteContext.ReleaseNote.TagName).Find(&notes).Error; err != nil {
		return EError(c, err)
	}
	notesDTO := make([]dto.ReleaseNoteLight, 0)
	for _, note := range notes {
		notesDTO = append(notesDTO, *note.ToLightDTO())
	}
	return c.JSON(http.StatusOK, notesDTO)
}

// EmailCaptchaRequest представляет структуру данных для запроса на восстановление пароля
type EmailCaptchaRequest struct {
	Email          string `json:"email" validate:"required,email"`
	CaptchaPayload string `json:"captcha_payload" validate:"required"`
}

// PostFeedbackRequest представляет структуру данных для запроса отправки отзыва
type PostFeedbackRequest struct {
	Stars    int    `json:"stars"`
	Feedback string `json:"feedback"`
}

// PasswordRequest представляет структуру данных для запроса регистрации пользователя
type PasswordRequest struct {
	NewPassword     string `json:"new_password" validate:"required,min=8"`
	ConfirmPassword string `json:"confirm_password" validate:"required,eqfield=NewPassword"`
}

// EmailRequest представляет структуру данных для смены почты пользователя
type EmailRequest struct {
	NewEmail string `json:"new_email" validate:"required"`
}

// EmailVerifyRequest представляет структуру данных для валидации почты пользователя
type EmailVerifyRequest struct {
	NewEmail string `json:"new_email" validate:"required"`
	Code     string `json:"code" validate:"required"`
}

// PasswordResponse представляет структуру данных для успешного ответа регистрации пользователя
type PasswordResponse struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
}

type NotificationViewRequest struct {
	Ids       []string `json:"ids,omitempty"`
	ViewedAll bool     `json:"viewed_all,omitempty"`
}

type NotificationIdResponse struct {
	Count int `json:"count"`
}

func bindSearchFilter(c echo.Context, filter *dao.SearchFilter) (*dao.SearchFilter, []string, error) {
	var req dto.SearchFilterLight
	fields, err := BindData(c, "search_filter", &req)
	if err != nil {
		return nil, nil, apierrors.ErrFilterBadRequest
	}

	if err := c.Validate(&req); err != nil {
		return nil, nil, err
	}

	if filter == nil {
		var user *dao.User
		if authCtx, ok := c.(AuthContext); ok {
			user = authCtx.User
		} else {
			user = c.(SearchFilterContext).User
		}
		return &dao.SearchFilter{
			ID: dao.GenUUID(),
			//CreatedAt:   time.Now(),
			//UpdatedAt:   time.Now(),
			AuthorID:    user.ID,
			Name:        req.Name,
			Description: req.Description,
			Public:      req.Public,
			Filter:      req.Filter,
			Author:      user,
		}, fields, nil
	} else {
		var resFields []string
		for _, field := range fields {
			switch field {
			case "name":
				CompareAndAddFields(&filter.Name, &req.Name, field, &resFields)
			case "description":
				CompareAndAddFields(&filter.Description, &req.Description, field, &resFields)
			case "public":
				CompareAndAddFields(&filter.Public, &req.Public, field, &resFields)
			case "filter":
				CompareAndAddFields(&filter.Filter, &req.Filter, field, &resFields)
			}
		}
		return filter, resFields, nil
	}
}
