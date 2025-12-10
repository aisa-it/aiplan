// Пакет aiplan предоставляет функциональность для управления пользователями, тарифами и шаблонами в системе планирования. Он включает в себя создание, обновление, удаление и получение информации о пользователях, тарифах и шаблонах, а также управление импортом данных.  Также предоставляет функциональность для управления правами доступа и отправки уведомлений пользователям.  Пакет предназначен для использования в административной панели системы планирования.
//
// Основные возможности:
//   - Управление пользователями: создание, обновление, удаление, получение информации о пользователях.
//   - Управление тарифами: создание, обновление, удаление, получение информации о тарифах.
//   - Управление шаблонами: сброс шаблонов почты к значениям по умолчанию.
//   - Управление импортом: получение списка активных импортов данных.
//   - Управление правами доступа: назначение ролей пользователям.
//   - Отправка уведомлений: отправка уведомлений пользователям по электронной почте.
package aiplan

import (
	"bytes"
	"compress/gzip"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/apierrors"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dto"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/gofrs/uuid"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	userUpdateFields = []string{
		"username",
		"password",
		"email",
		"telegram_id",
		"first_name",
		"last_name",
		"is_superuser",
		"is_active",
		"blocked_until",
		"user_timezone",
		"role",
		"is_bot",
		"updated_by_id",
	}
)

type UserCreateRequest struct {
	Email       string `json:"email"`
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name"`
	Password    string `json:"password"`
	WorkspaceId string `json:"workspace_id"` // optional
	Role        int    `json:"role"`         // optional
}

type WorkspaceContext struct {
	AuthContext
	Workspace       dao.Workspace
	WorkspaceMember dao.WorkspaceMember
}

type ReleaseNoteContext struct {
	AuthContext
	ReleaseNote dao.ReleaseNote
}

func (s *Services) StaffPermissionsMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		user := *c.(AuthContext).User
		if !user.IsSuperuser {
			return EErrorDefined(c, apierrors.ErrNotEnoughRights)
		}

		return next(c)
	}
}

func (s *Services) ReleaseNotesMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		noteId := c.Param("noteId")
		if noteId == "" {
			return next(c)
		}

		var note dao.ReleaseNote
		if err := s.db.Where("id = ?", noteId).Or("tag_name = ?", noteId).First(&note).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return EErrorDefined(c, apierrors.ErrReleaseNoteNotFound)
			}
			return EError(c, err)
		}

		return next(ReleaseNoteContext{c.(AuthContext), note})
	}
}

func (s *Services) UserExistsMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		userId := c.Param("userId")
		if userId == "" {
			return next(c)
		}

		exists, err := dao.UserExists(s.db, userId)
		if err != nil {
			return EError(c, err)
		}
		if !exists {
			return EErrorDefined(c, apierrors.ErrUserNotFound)
		}

		return next(c)
	}
}

func (s *Services) AddAdminServices(g *echo.Group) {
	staffPermissionGroup := g.Group("admin/", s.StaffPermissionsMiddleware)

	workspacesGroup := staffPermissionGroup.Group("workspaces/")
	projectsGroup := staffPermissionGroup.Group("projects/:workspaceId/")
	usersGroup := staffPermissionGroup.Group("users/", s.UserExistsMiddleware)
	releaseNoteGroup := staffPermissionGroup.Group("release-notes/", s.ReleaseNotesMiddleware)
	feedbacksGroup := staffPermissionGroup.Group("feedbacks/")
	templatesGroup := staffPermissionGroup.Group("templates/")
	importsGroup := staffPermissionGroup.Group("imports/")

	staffPermissionGroup.GET("activities/", s.geRootActivityList)
	workspacesGroup.GET("", s.getAllWorkspaceList)

	projectsGroup.GET("", s.getWorkspaceProjectList)

	usersGroup.GET("", s.getAllUserList)
	usersGroup.GET("export/", s.exportUsersList)
	usersGroup.POST("", s.createUser)

	usersGroup.GET(":userId/", s.getUserById)
	usersGroup.PATCH(":userId/", s.updateUser)
	usersGroup.DELETE(":userId/", s.deleteUser)
	usersGroup.GET(":userId/feedback/", s.getUserFeedback)

	usersGroup.GET(":userId/workspaces/", s.geWorkspaceListByUser)
	usersGroup.GET(":userId/workspaces/:workspaceId/", s.getProjectListByUser)

	usersGroup.POST(":userId/workspaces/:workspaceId/member/", s.updateWorkspaceMemberAdmin)
	usersGroup.DELETE(":userId/workspaces/:workspaceId/member/", s.deleteWorkspaceMemberAdmin)

	usersGroup.POST(":userId/workspaces/:workspaceId/projects/:projectId/member/", s.updateProjectMemberAdmin)
	usersGroup.DELETE(":userId/workspaces/:workspaceId/projects/:projectId/member/", s.deleteProjectMemberAdmin)
	usersGroup.POST("message/", s.createMessageForMember)

	releaseNoteGroup.GET("", s.getProductUpdateList)
	releaseNoteGroup.POST("", s.createReleaseNote)

	releaseNoteGroup.GET(":noteId/", s.getReleaseNote)
	releaseNoteGroup.PATCH(":noteId/", s.updateReleaseNote)
	releaseNoteGroup.DELETE(":noteId/", s.deleteReleaseNote)

	feedbacksGroup.GET("", s.getAllFeedbackList)
	feedbacksGroup.GET("export/", s.exportFeedbackList)

	templatesGroup.POST("reset/", s.reloadTemplates)

	importsGroup.GET("", s.getRunningImportList)
}

// getAllWorkspaceList godoc
// @id getAllWorkspaceList
// @Summary Пространство: получение всех рабочих пространств
// @Description Возвращает список всех рабочих пространств с поддержкой пагинации, сортировки и поиска
// @Tags AdminPanel
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param offset query int false "Смещение для пагинации" default(0)
// @Param limit query int false "Количество результатов на странице" default(100)
// @Param order_by query string false "Поле для сортировки"
// @Param desc query bool false "Направление сортировки: true - по убыванию, false - по возрастанию" default(false)
// @Param search_query query string false "Строка для поиска"
// @Success 200 {object} dao.PaginationResponse{result=[]dto.WorkspaceWithCount} "Список рабочих пространств"
// @Failure 403 {object} apierrors.DefinedError "Ошибка: доступ запрещен"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 500 {object} apierrors.DefinedError "Внутренняя ошибка сервера"
// @Router /api/auth/admin/workspaces [get]
func (s *Services) getAllWorkspaceList(c echo.Context) error {
	offset := 0
	limit := 100
	orderBy := ""
	desc := false
	searchQuery := ""

	if err := echo.QueryParamsBinder(c).
		Int("offset", &offset).
		Int("limit", &limit).
		String("order_by", &orderBy).
		Bool("desc", &desc).
		String("search_query", &searchQuery).
		BindError(); err != nil {
		return EError(c, err)
	}

	if limit > 100 {
		limit = 100
	}

	query := s.db.Preload(clause.Associations).
		Select("*, (?) as total_members, (?) as total_projects, (ts_rank(name_tokens, plainto_tsquery('russian', lower(?)))) as search_rank, ts_headline('russian', name, plainto_tsquery('russian', lower(?))) as name_highlighted",
			s.db.Select("count(*)").Where("workspace_id = workspaces.id").Model(&dao.WorkspaceMember{}),
			s.db.Select("count(*)").Where("workspace_id = workspaces.id").Model(&dao.Project{}),
			searchQuery,
			searchQuery,
		)

	if orderBy != "" {
		query = query.Order(clause.OrderByColumn{Column: clause.Column{Name: orderBy}, Desc: desc})
	}

	if searchQuery != "" {
		escapedSearchQuery := PrepareSearchRequest(searchQuery)
		query = query.Where("lower(name) like ? or name_tokens @@ plainto_tsquery('russian', lower(?))", escapedSearchQuery, strings.ToLower(searchQuery)).Order("search_rank desc")
	}

	var workspaces []dao.WorkspaceWithCount
	resp, err := dao.PaginationRequest(
		offset,
		limit,
		query,
		&workspaces,
	)
	if err != nil {
		return EError(c, err)
	}
	resp.Result = utils.SliceToSlice(resp.Result.(*[]dao.WorkspaceWithCount), func(wwc *dao.WorkspaceWithCount) dto.WorkspaceWithCount { return *wwc.ToDTO() })

	return c.JSON(http.StatusOK, resp)
}

// getWorkspaceProjectList godoc
// @id getWorkspaceProjectList
// @Summary Проекты: получение проектов рабочего пространства
// @Description Возвращает список проектов внутри указанного рабочего пространства с поддержкой пагинации, сортировки и поиска
// @Tags AdminPanel
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceId path string true "ID рабочего пространства"
// @Param offset query int false "Смещение для пагинации" default(0)
// @Param limit query int false "Количество результатов на странице" default(100)
// @Param order_by query string false "Поле для сортировки"
// @Param desc query bool false "Направление сортировки: true - по убыванию, false - по возрастанию" default(false)
// @Param search_query query string false "Строка для поиска"
// @Success 200 {object} dao.PaginationResponse{result=[]dto.Project} "Список проектов рабочего пространства"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 403 {object} apierrors.DefinedError "Ошибка: доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Рабочее пространство не найдено"
// @Failure 500 {object} apierrors.DefinedError "Внутренняя ошибка сервера"
// @Router /api/auth/admin/projects/{workspaceId} [get]
func (s *Services) getWorkspaceProjectList(c echo.Context) error {
	workspaceId := c.Param("workspaceId")
	offset := 0
	limit := 100
	orderBy := ""
	desc := false
	searchQuery := ""

	if err := echo.QueryParamsBinder(c).
		Int("offset", &offset).
		Int("limit", &limit).
		String("order_by", &orderBy).
		Bool("desc", &desc).
		String("search_query", &searchQuery).
		BindError(); err != nil {
		return EError(c, err)
	}

	if limit > 100 {
		limit = 100
	}
	query := dao.PreloadProjectMembersWithFilters(s.db.
		Preload("Workspace").
		Preload("ProjectLead").
		Where("workspace_id = ?", workspaceId).
		Select("*, (?) as total_members, (ts_rank(name_tokens, plainto_tsquery('russian', lower(?)))) as search_rank, ts_headline('russian', name, plainto_tsquery('russian', lower(?))) as name_highlighted",
			s.db.Select("count(*)").Where("project_id = projects.id").Model(&dao.ProjectMember{}),
			searchQuery,
			searchQuery,
		))

	if orderBy != "" {
		query = query.Order(clause.OrderByColumn{Column: clause.Column{Name: orderBy}, Desc: desc})
	}

	if searchQuery != "" {
		escapedSearchQuery := PrepareSearchRequest(searchQuery)
		query = query.Where("lower(name) LIKE ? OR name_tokens @@ plainto_tsquery('russian', lower(?))", escapedSearchQuery, searchQuery).Order("search_rank desc")
	}

	var projects []dao.ProjectWithCount
	resp, err := dao.PaginationRequest(
		offset,
		limit,
		query,
		&projects,
	)
	if err != nil {
		return EError(c, err)
	}

	resp.Result = utils.SliceToSlice(resp.Result.(*[]dao.ProjectWithCount), func(pwc *dao.ProjectWithCount) dto.Project { return *pwc.ToDTO() })

	return c.JSON(http.StatusOK, resp)
}

// ############# Global users management ###################

// getAllUserList godoc
// @id getAllUserList
// @Summary Пользователи: получение всех пользователей
// @Description Возвращает список всех пользователей с поддержкой пагинации, сортировки и поиска
// @Tags AdminPanel
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param offset query int false "Смещение для пагинации" default(0)
// @Param limit query int false "Количество результатов на странице" default(100)
// @Param order_by query string false "Поле для сортировки"
// @Param desc query bool false "Направление сортировки: true - по убыванию, false - по возрастанию" default(false)
// @Param search_query query string false "Строка для поиска"
// @Success 200 {object} dao.PaginationResponse{result=[]dto.UserLight} "Список пользователей"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 403 {object} apierrors.DefinedError "Ошибка: доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Внутренняя ошибка сервера"
// @Router /api/auth/admin/users [get]
func (s *Services) getAllUserList(c echo.Context) error {
	offset := 0
	limit := 100
	orderBy := ""
	desc := false
	searchQuery := ""

	if err := echo.QueryParamsBinder(c).
		Int("offset", &offset).
		Int("limit", &limit).
		String("order_by", &orderBy).
		Bool("desc", &desc).
		String("search_query", &searchQuery).
		BindError(); err != nil {
		return EError(c, err)
	}

	if limit > 100 {
		limit = 100
	}

	query := s.db

	if orderBy != "" {
		query = query.Order(clause.OrderByColumn{Column: clause.Column{Name: orderBy}, Desc: desc})
	} else {
		query = query.Order("id")
	}

	if searchQuery != "" {
		escapedSearchQuery := PrepareSearchRequest(searchQuery)
		query = query.
			Where("lower(email) LIKE ? OR lower(first_name) LIKE ? OR lower(last_name) LIKE ?", escapedSearchQuery, escapedSearchQuery, escapedSearchQuery)
	}

	var users []dao.User
	resp, err := dao.PaginationRequest(
		offset,
		limit,
		query,
		&users,
	)
	if err != nil {
		return EError(c, err)
	}

	resp.Result = utils.SliceToSlice(resp.Result.(*[]dao.User), func(u *dao.User) dto.UserLight { return *u.ToLightDTO() })

	return c.JSON(http.StatusOK, resp)
}

func (s *Services) exportUsersList(c echo.Context) error {
	var bufio bytes.Buffer
	gz := gzip.NewWriter(&bufio)
	w := csv.NewWriter(gz)
	w.Write([]string{"ID", "FirstName", "LastName", "Username", "Email", "CreationDate", "LastActivity", "IsSuperUser", "IsActive", "IsBot"})

	var users []dao.User
	s.db.FindInBatches(&users, 100, func(tx *gorm.DB, batch int) error {
		for _, user := range users {
			if err := w.Write([]string{
				user.ID.String(),
				user.FirstName,
				user.LastName,
				getNilString(user.Username),
				user.Email,
				user.CreatedAt.Format(time.RFC3339),
				user.LastActive.Format(time.RFC3339),
				getBoolCSV(user.IsSuperuser),
				getBoolCSV(user.IsActive),
				getBoolCSV(user.IsBot),
			}); err != nil {
				return err
			}
		}
		return nil
	})

	w.Flush()
	gz.Close()

	c.Response().Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=aiplan_users_%s.csv.gz", time.Now().Format("02-01-2006")))
	return c.Stream(http.StatusOK, http.DetectContentType(bufio.Bytes()), &bufio)
}

func getNilString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func getBoolCSV(b bool) string {
	if b {
		return "Y"
	}
	return "N"
}

// createUser godoc
// @id createUser
// @Summary Пользователи: создание пользователя
// @Description Создает нового пользователя и добавляет его в рабочее пространство при необходимости
// @Tags AdminPanel
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body UserCreateRequest true "Данные для создания пользователя"
// @Success 200 {object} dto.UserLight "Созданный пользователь"
// @Failure 400 {object} apierrors.DefinedError "Некорректные данные пользователя"
// @Failure 403 {object} apierrors.DefinedError "Ошибка: доступ запрещен"
// @Failure 409 {object} apierrors.DefinedError "Пользователь с таким email уже существует"
// @Failure 500 {object} apierrors.DefinedError "Внутренняя ошибка сервера"
// @Router /api/auth/admin/users [post]
func (s *Services) createUser(c echo.Context) error {
	user := *c.(AuthContext).User

	var req UserCreateRequest
	if err := c.Bind(&req); err != nil {
		return EError(c, err)
	}

	if req.Email == "" || req.Password == "" {
		return EErrorDefined(c, apierrors.ErrLoginCredentialsRequired)
	}

	if !ValidateEmail(req.Email) {
		return EErrorDefined(c, apierrors.ErrInvalidEmail)
	}

	if req.WorkspaceId != "" && req.Role == 0 {
		return EErrorDefined(c, apierrors.ErrWorkspaceRoleRequired)
	}

	createdByID := uuid.NullUUID{UUID: user.ID, Valid: true}
	u := dao.User{
		ID:          dao.GenUUID(),
		Email:       req.Email,
		FirstName:   req.FirstName,
		LastName:    req.LastName,
		Password:    dao.GenPasswordHash(req.Password),
		CreatedByID: createdByID,
		Theme:       types.DefaultTheme,
		IsActive:    true,
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		var workspace dao.Workspace
		if req.WorkspaceId != "" {
			if err := s.db.Where("id = ?", req.WorkspaceId).First(&workspace).Error; err != nil {
				return err
			}

			u.LastWorkspaceId = uuid.NullUUID{UUID: workspace.ID, Valid: true}
		}
		if err := s.db.Create(&u).Error; err != nil {
			return err
		}

		if req.WorkspaceId != "" {
			return s.db.Create(&dao.WorkspaceMember{
				ID:          dao.GenID(),
				WorkspaceId: workspace.ID,
				MemberId:    u.ID,
				Role:        req.Role,
				CreatedById: createdByID,
			}).Error
		}
		return nil
	}); err != nil {
		return EError(c, err)
	}
	return c.JSON(http.StatusOK, u.ToLightDTO())
}

// getUserById godoc
// @id getUserById
// @Summary Пользователи: получение пользователя по ID
// @Description Возвращает информацию о пользователе по его ID
// @Tags AdminPanel
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param userId path string true "ID пользователя"
// @Success 200 {object} dto.UserLight "Информация о пользователе"
// @Failure 403 {object} apierrors.DefinedError "Ошибка: доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Пользователь не найден"
// @Failure 500 {object} apierrors.DefinedError "Внутренняя ошибка сервера"
// @Router /api/auth/admin/users/{userId} [get]
func (s *Services) getUserById(c echo.Context) error {
	userId := c.Param("userId")

	var user dao.User
	if err := s.db.Where("id = ?", userId).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return EErrorDefined(c, apierrors.ErrUserNotFound)
		}
		return EError(c, err)
	}
	return c.JSON(http.StatusOK, user.ToLightDTO())
}

// updateUser godoc
// @id updateUser
// @Summary Пользователи: изменение пользователя
// @Description Обновляет информацию о пользователе
// @Tags AdminPanel
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param userId path string true "ID пользователя"
// @Param data body map[string]interface{} true "Данные для обновления пользователя"
// @Success 200 {object} dto.UserLight "Обновленный пользователь"
// @Failure 400 {object} apierrors.DefinedError "Некорректные данные для обновления"
// @Failure 403 {object} apierrors.DefinedError "Ошибка: доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Пользователь не найден"
// @Failure 500 {object} apierrors.DefinedError "Внутренняя ошибка сервера"
// @Router /api/auth/admin/users/{userId} [patch]
func (s *Services) updateUser(c echo.Context) error {
	userId := c.Param("userId")

	var admin *dao.User
	if context, ok := c.(AuthContext); ok {
		admin = context.User
	} else {
		admin = c.(WorkspaceContext).User
	}

	var user dao.User
	if err := s.db.Where("id = ?", userId).First(&user).Error; err != nil {
		return EError(c, err)
	}

	if user.IsIntegration {
		return EErrorDefined(c, apierrors.ErrCannotEditIntegration)
	}

	var data map[string]interface{}
	if err := json.NewDecoder(c.Request().Body).Decode(&data); err != nil {
		return EError(c, err)
	}

	if email, ok := data["email"].(string); ok && !ValidateEmail(email) {
		return EErrorDefined(c, apierrors.ErrInvalidEmail)
	}

	if password, ok := data["password"].(string); ok && !strings.HasPrefix(password, "pbkdf2_sha256$") {
		data["password"] = dao.GenPasswordHash(password)
	}

	data["updated_by_id"] = admin.ID

	delete(data, "blocked_until")
	if a, ok := data["is_active"]; ok {
		if aa, ok2 := a.(bool); ok2 && aa {
			data["blocked_until"] = nil
		}
	}

	if err := s.db.Model(&user).Select(userUpdateFields).Updates(data).Error; err != nil {
		if err == gorm.ErrDuplicatedKey {
			return EErrorDefined(c, apierrors.ErrDuplicateEmail)
		}
		return EError(c, err)
	}
	return c.JSON(http.StatusOK, user.ToLightDTO())
}

// deleteUser godoc
// @id deleteUser
// @Summary Пользователи: удаление пользователя
// @Description Удаляет пользователя по его ID
// @Tags AdminPanel
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param userId path string true "ID пользователя"
// @Success 200 "Пользователь успешно удален"
// @Failure 403 {object} apierrors.DefinedError "Ошибка: доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Внутренняя ошибка сервера"
// @Router /api/auth/admin/users/{userId} [delete]
func (s *Services) deleteUser(c echo.Context) error {
	userIdStr := c.Param("userId")
	userId, err := uuid.FromString(userIdStr)
	if err != nil {
		return EError(c, err)
	}

	if err := s.business.DeleteUser(userId); err != nil {
		return EError(c, err)
	}

	return c.NoContent(http.StatusOK)
}

// getUserFeedback godoc
// @id getUserFeedback
// @Summary Пользователи: получение отзыва пользователя
// @Description Возвращает отзыв пользователя по его ID
// @Tags AdminPanel
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param userId path string true "ID пользователя"
// @Success 200 {object} dto.UserFeedback "Отзыв пользователя"
// @Failure 204 "Отзыв не найден"
// @Failure 403 {object} apierrors.DefinedError "Ошибка: доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Внутренняя ошибка сервера"
// @Router /api/auth/admin/users/{userId}/feedback [get]
func (s *Services) getUserFeedback(c echo.Context) error {
	userId := c.Param("userId")

	var feedback dao.UserFeedback
	if err := s.db.Where("user_id = ?", userId).Preload(clause.Associations).First(&feedback).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.NoContent(http.StatusNoContent)
		}
		return EError(c, err)
	}
	return c.JSON(http.StatusOK, feedback.ToDTO())
}

// geWorkspaceListByUser godoc
// @id geWorkspaceListByUser
// @Summary Пользователи: получение пространств пользователя
// @Description Возвращает список рабочих пространств, к которым принадлежит пользователь, с поддержкой пагинации, сортировки и поиска
// @Tags AdminPanel
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param userId path string true "ID пользователя"
// @Param offset query int false "Смещение для пагинации" default(0)
// @Param limit query int false "Количество результатов на странице" default(100)
// @Param all query bool false "Все пространства, или где состоит пользователь" default(false)
// @Param order_by query string false "Поле для сортировки"
// @Param desc query bool false "Направление сортировки: true - по убыванию, false - по возрастанию" default(false)
// @Param search_query query string false "Строка для поиска"
// @Success 200 {object} dao.PaginationResponse{result=[]dto.Workspace} "Список рабочих пространств"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 403 {object} apierrors.DefinedError "Ошибка: доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Внутренняя ошибка сервера"
// @Router /api/auth/admin/users/{userId}/workspaces [get]
func (s *Services) geWorkspaceListByUser(c echo.Context) error {
	userId := c.Param("userId")

	offset := 0
	limit := 100
	orderBy := ""
	desc := false
	searchQuery := ""
	all := false

	if err := echo.QueryParamsBinder(c).
		Int("offset", &offset).
		Int("limit", &limit).
		String("order_by", &orderBy).
		Bool("desc", &desc).
		String("search_query", &searchQuery).
		Bool("all", &all).
		BindError(); err != nil {
		return EError(c, err)
	}

	if limit > 100 {
		limit = 100
	}

	query := s.db.Model(&dao.Workspace{})

	switch orderBy {
	case "role":
		query = query.Select("workspaces.*, COALESCE((SELECT role FROM workspace_members WHERE workspace_members.workspace_id = workspaces.id AND workspace_members.member_id = ?), 0) AS role", userId)
		if desc {
			orderBy += " DESC"
		}
		orderBy += ", lower(workspaces.name)"
	default:
		orderBy = "name"
		if desc {
			orderBy += " DESC"
		}
	}

	var workspaces []dao.Workspace

	query = query.Set("userID", userId).
		Order(orderBy)

	if searchQuery != "" {
		escapedSearchQuery := PrepareSearchRequest(searchQuery)
		query = query.Where("lower(name) LIKE ? OR lower(slug) LIKE ? OR name_tokens @@ plainto_tsquery('russian', lower(?))",
			escapedSearchQuery, escapedSearchQuery, searchQuery)
	}

	if !all {
		query = query.
			Where("workspaces.id in (?)", s.db.Model(&dao.WorkspaceMember{}).
				Select("workspace_id").
				Where("member_id = ?", userId))
	}

	resp, err := dao.PaginationRequest(
		offset,
		limit,
		query,
		&workspaces,
	)
	if err != nil {
		return EError(c, err)
	}

	resp.Result = utils.SliceToSlice(resp.Result.(*[]dao.Workspace), func(w *dao.Workspace) dto.Workspace { return *w.ToDTO() })

	return c.JSON(http.StatusOK, resp)
}

// getProjectListByUser godoc
// @id getProjectListByUser
// @Summary Пользователи: получение проектов пользователя в рабочем пространстве
// @Description Возвращает список проектов пользователя в указанном рабочем пространстве с поддержкой пагинации, сортировки и поиска
// @Tags AdminPanel
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param userId path string true "ID пользователя"
// @Param workspaceId path string true "ID рабочего пространства"
// @Param offset query int false "Смещение для пагинации" default(0)
// @Param limit query int false "Количество результатов на странице" default(100)
// @Param all query bool false "Все проекты, или где состоит пользователь" default(false)
// @Param order_by query string false "Поле для сортировки"
// @Param desc query bool false "Направление сортировки: true - по убыванию, false - по возрастанию" default(false)
// @Param search_query query string false "Строка для поиска"
// @Success 200 {object} dao.PaginationResponse{result=[]dto.ProjectLight} "Список проектов пользователя"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 403 {object} apierrors.DefinedError "Ошибка: доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Внутренняя ошибка сервера"
// @Router /api/auth/admin/users/{userId}/workspaces/{workspaceId} [get]
func (s *Services) getProjectListByUser(c echo.Context) error {
	userId := c.Param("userId")
	workspaceId := c.Param("workspaceId")

	offset := 0
	limit := 100
	orderBy := ""
	desc := false
	searchQuery := ""
	all := false

	if err := echo.QueryParamsBinder(c).
		Int("offset", &offset).
		Int("limit", &limit).
		String("order_by", &orderBy).
		Bool("desc", &desc).
		String("search_query", &searchQuery).
		Bool("all", &all).
		BindError(); err != nil {
		return EError(c, err)
	}

	if limit > 100 {
		limit = 100
	}

	var projects []dao.Project
	query := s.db.Model(&dao.Project{}).
		Set("userId", userId).
		Where("workspace_id = ?", workspaceId).
		Order("name")

	if searchQuery != "" {
		escapedSearchQuery := PrepareSearchRequest(searchQuery)
		query = query.Where("lower(name) LIKE ? OR name_tokens @@ plainto_tsquery('russian', lower(?))",
			escapedSearchQuery, searchQuery)
	}
	if !all {
		query = query.
			Where("id in (?)", s.db.Model(&dao.ProjectMember{}).
				Select("project_id").
				Where("member_id = ?", userId))
	}

	resp, err := dao.PaginationRequest(
		offset,
		limit,
		query,
		&projects,
	)
	if err != nil {
		return EError(c, err)
	}

	resp.Result = utils.SliceToSlice(resp.Result.(*[]dao.Project), func(p *dao.Project) dto.ProjectLight { return *p.ToLightDTO() })

	return c.JSON(http.StatusOK, resp)
}

// createMessageForMember godoc
// @id createMessageForMember
// @Summary Пользователи: Отправка сообщения
// @Description Позволяет отправить сообщение всем или выбранным пользователям. Поддерживается отправка отложенных сообщений.
// @Tags AdminPanel
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body requestMessage true "Информация о сообщении"
// @Success 200 "Сообщения успешно отправлены"
// @Failure 400 {object} apierrors.DefinedError "Некорректные данные запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/admin/users/message/ [post]
func (s *Services) createMessageForMember(c echo.Context) error {
	var req requestMessage
	if err := c.Bind(&req); err != nil {
		return EError(c, err)
	}
	if req.SendAt.IsZero() {
		req.SendAt = time.Now()
	}

	var members []dao.User
	var notificationSentAt []dao.DeferredNotifications

	query := s.db

	if len(req.Members) > 0 {
		query = query.Where("id IN (?)", req.Members)
	}
	if err := query.Find(&members).Error; err != nil {
		return EErrorDefined(c, apierrors.ErrGeneric)
	}
	if len(members) > 0 {
		for _, member := range members {
			payload := map[string]interface{}{
				"id":    dao.GenID(),
				"title": req.Title,
				"msg":   req.Msg,
			}
			payloadBytes, err := json.Marshal(payload)
			if err != nil {
				return EErrorDefined(c, apierrors.ErrGeneric)
			}
			tmpNotify := dao.DeferredNotifications{
				ID: dao.GenUUID(),

				UserID: member.ID,
				User:   &member,

				NotificationType:    "service_message",
				DeliveryMethod:      "telegram",
				AttemptCount:        0,
				LastAttemptAt:       time.Time{},
				TimeSend:            &req.SendAt,
				NotificationPayload: payloadBytes,
			}

			notificationSentAt = append(notificationSentAt, tmpNotify)
			tmpNotify.ID = dao.GenUUID()
			tmpNotify.DeliveryMethod = "email"
			notificationSentAt = append(notificationSentAt, tmpNotify)
			tmpNotify.ID = dao.GenUUID()
			tmpNotify.DeliveryMethod = "app"
			notificationSentAt = append(notificationSentAt, tmpNotify)
		}
	}

	if len(notificationSentAt) > 0 {
		if err := s.db.Omit(clause.Associations).CreateInBatches(&notificationSentAt, 10).Error; err != nil {
			return err
		}
	}

	return c.NoContent(http.StatusOK)
}

// updateWorkspaceMemberAdmin обновляет роль участника рабочего пространства.
// @Summary Пользователи: Обновить роль участника пространства
// @Description Метод позволяет обновить роль участника рабочего пространства. Если участник отсутствует, он будет создан.
// @Tags AdminPanel
// @Accept json
// @Produce json
// @Param workspaceId path string true "ID рабочего пространства"
// @Param userId path string true "ID пользователя"
// @Param body body roleUpdRequest true "Роль участника"
// @Success 204 "Роль успешно обновлена"
// @Failure 400 {object} apierrors.DefinedError "Некорректные данные или отсутствует роль"
// @Failure 403 {object} apierrors.DefinedError "Ошибка: доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Рабочее пространство или пользователь не найдены"
// @Failure 500 {object} apierrors.DefinedError "Внутренняя ошибка сервера"
// @Router /api/auth/admin/users/{userId}/workspaces/{workspaceId}/member/ [post]
func (s *Services) updateWorkspaceMemberAdmin(c echo.Context) error {
	userId := c.Param("userId")
	workspaceId := c.Param("workspaceId")
	superUser := *c.(AuthContext).User

	var role roleUpdRequest

	if err := c.Bind(&role); err != nil {
		return EErrorDefined(c, apierrors.ErrWorkspaceRoleRequired)
	}

	err := c.Validate(role)
	if err != nil {
		return EErrorDefined(c, apierrors.ErrWorkspaceRoleRequired)
	}

	var user dao.User
	var workspace dao.Workspace
	var member dao.WorkspaceMember

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", userId).First(&user).Error; err != nil {
			return apierrors.ErrGeneric
		}

		if err := tx.Where("id = ?", workspaceId).Find(&workspace).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return apierrors.ErrWorkspaceNotFound
			}
			return apierrors.ErrGeneric
		}

		superUserID := uuid.NullUUID{UUID: superUser.ID, Valid: true}

		if err := tx.Where("member_id = ?", userId).
			Where("workspace_id = ?", workspaceId).
			First(&member).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				member = dao.WorkspaceMember{
					ID:          dao.GenID(),
					WorkspaceId: workspace.ID,
					MemberId:    user.ID,
					CreatedById: superUserID,
					Role:        role.Role,
				}
				if err := tx.Model(&dao.WorkspaceMember{}).Create(&member).Error; err != nil {
					return apierrors.ErrGeneric
				}
				return nil
			} else {
				return apierrors.ErrGeneric
			}
		}

		oldMemberRole := member.Role

		member.Role = role.Role
		member.UpdatedById = superUserID

		if err := tx.Model(&dao.WorkspaceMember{}).
			Where("id = ?", member.ID).
			Save(&member).Error; err != nil {
			return apierrors.ErrGeneric
		}

		var projects []dao.Project
		if err := tx.Where("workspace_id = ?", workspace.ID).Find(&projects).Error; err != nil {
			return err
		}

		if role.Role == types.AdminRole {
			for _, project := range projects {
				if err := tx.Clauses(clause.OnConflict{
					Columns:   []clause.Column{{Name: "project_id"}, {Name: "member_id"}},
					DoUpdates: clause.Assignments(map[string]interface{}{"role": types.AdminRole, "updated_at": time.Now(), "updated_by_id": superUserID}),
				}).Create(&dao.ProjectMember{
					ID:                              dao.GenID(),
					CreatedAt:                       time.Now(),
					CreatedById:                     superUserID,
					WorkspaceId:                     workspace.ID,
					ProjectId:                       project.ID,
					Role:                            types.AdminRole,
					MemberId:                        user.ID,
					ViewProps:                       types.DefaultViewProps,
					NotificationAuthorSettingsEmail: types.DefaultProjectMemberNS,
					NotificationAuthorSettingsApp:   types.DefaultProjectMemberNS,
					NotificationAuthorSettingsTG:    types.DefaultProjectMemberNS,
					NotificationSettingsEmail:       types.DefaultProjectMemberNS,
					NotificationSettingsApp:         types.DefaultProjectMemberNS,
					NotificationSettingsTG:          types.DefaultProjectMemberNS,
				}).Error; err != nil {
					return err
				}
			}
		}

		if role.Role != types.AdminRole && oldMemberRole == types.AdminRole {
			if err := tx.
				Where("workspace_id = ?", workspace.ID).
				Where("member_id = ?", user.ID).
				Delete(&dao.ProjectMember{}).Error; err != nil {
				return err
			}
		}

		return nil
	}); err != nil {
		return EError(c, err)
	}

	return c.NoContent(http.StatusOK)
}

// deleteWorkspaceMemberAdmin удаляет участника рабочего пространства, включая его связи с проектами.
// @Summary Пользователи: Удалить участника пространства
// @Description Удаляет участника рабочего пространства, а также все его связи с проектами внутри данного пространства.
// @Tags AdminPanel
// @Accept json
// @Produce json
// @Param workspaceId path string true "ID рабочего пространства"
// @Param userId path string true "ID пользователя"
// @Success 204 "Участник успешно удален"
// @Failure 403 {object} apierrors.DefinedError "Ошибка: доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Участник рабочего пространства не найден"
// @Failure 500 {object} apierrors.DefinedError "Внутренняя ошибка сервера"
// @Router /api/auth/admin/users/{userId}/workspaces/{workspaceId}/member/ [delete]
func (s *Services) deleteWorkspaceMemberAdmin(c echo.Context) error {
	userId := c.Param("userId")
	workspaceId := c.Param("workspaceId")

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		var newOwner dao.WorkspaceMember
		if err := s.db.
			Model(&dao.WorkspaceMember{}).
			Joins("Member").
			Where("workspace_id = ?", workspaceId).
			Where("is_bot = false").         // only humans
			Where("is_active = true").       // only active users
			Where("member_id != ?", userId). // not requested member
			First(&newOwner).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				// TODO: Remove workspace if no owners available
				return apierrors.ErrDeleteLastWorkspaceMember
			}
		}

		var requestedMember dao.WorkspaceMember
		if err := tx.
			Preload("Workspace").
			Joins("Member").
			Where("workspace_id = ?", workspaceId).
			Where("member_id = ?", userId).
			First(&requestedMember).Error; err != nil {
			return err
		}
		if requestedMember.Member.IsSuperuser && requestedMember.MemberId != c.(AuthContext).User.ID {
			return apierrors.ErrDeleteSuperUser
		}

		return dao.DeleteWorkspaceMember(&newOwner, &requestedMember, tx)
	}); err != nil {
		return EError(c, err)
	}
	return c.NoContent(http.StatusOK)
}

// updateProjectMemberAdmin обновляет роль участника проекта в рабочем пространстве.
// @Summary Пользователи: Обновить роль участника проекта
// @Description Метод обновляет роль участника проекта в рамках рабочего пространства. Если участник не найден в проекте, он будет создан. Проверяется, что участник является членом рабочего пространства.
// @Tags AdminPanel
// @Accept json
// @Produce json
// @Param workspaceId path string true "ID рабочего пространства"
// @Param userId path string true "ID пользователя"
// @Param projectId path string true "ID проекта"
// @Param body body roleUpdRequest true "Роль участника проекта"
// @Success 200 "Роль успешно обновлена"
// @Failure 400 {object} apierrors.DefinedError "Некорректные данные или отсутствует роль"
// @Failure 403 {object} apierrors.DefinedError "Ошибка: доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Рабочее пространство, проект или участник не найдены"
// @Failure 500 {object} apierrors.DefinedError "Внутренняя ошибка сервера"
// @Router /api/auth/admin/users/{userId}/workspaces/{workspaceId}/projects/{projectId}/member/ [post]
func (s *Services) updateProjectMemberAdmin(c echo.Context) error {
	userId := c.Param("userId")
	workspaceId := c.Param("workspaceId")
	projectId := c.Param("projectId")
	superUser := *c.(AuthContext).User

	var role roleUpdRequest

	if err := c.Bind(&role); err != nil {
		return EErrorDefined(c, apierrors.ErrWorkspaceRoleRequired)
	}

	err := c.Validate(role)
	if err != nil {
		return EErrorDefined(c, apierrors.ErrWorkspaceRoleRequired)
	}

	var user dao.User
	var workspace dao.Workspace
	var project dao.Project
	var member dao.ProjectMember

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", userId).First(&user).Error; err != nil {
			return apierrors.ErrGeneric
		}

		if err := tx.Where("id = ?", workspaceId).Find(&workspace).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return apierrors.ErrWorkspaceNotFound
			}
			return apierrors.ErrGeneric
		}

		var existsWorkspaceMember bool

		if err := tx.Model(&dao.WorkspaceMember{}).
			Select("EXISTS(?)",
				tx.Model(&dao.WorkspaceMember{}).
					Select("1").
					Where("workspace_id = ?", workspaceId).
					Where("member_id = ?", userId),
			).
			Find(&existsWorkspaceMember).Error; err != nil {
			return apierrors.ErrGeneric
		}

		if !existsWorkspaceMember {
			return apierrors.ErrWorkspaceMemberNotFound
		}

		if err := tx.Where("id = ?", projectId).First(&project).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return apierrors.ErrProjectNotFound
			}
			return apierrors.ErrGeneric
		}

		superUserID := uuid.NullUUID{UUID: superUser.ID, Valid: true}

		if err := tx.Where("member_id = ?", userId).
			Where("project_id = ?", projectId).
			First(&member).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				member = dao.ProjectMember{
					ID:          dao.GenID(),
					ProjectId:   uuid.Must(uuid.FromString(projectId)),
					MemberId:    user.ID,
					WorkspaceId: uuid.Must(uuid.FromString(workspaceId)),
					CreatedById: superUserID,
					CreatedAt:   time.Now(),
					ViewProps:   types.DefaultViewProps,
					Role:        role.Role,
				}
				if err := tx.Model(&dao.ProjectMember{}).Create(&member).Error; err != nil {
					return apierrors.ErrGeneric
				}
				return nil
			} else {
				return apierrors.ErrGeneric
			}
		}

		member.Role = role.Role
		member.UpdatedById = superUserID
		if err := tx.Model(&dao.ProjectMember{}).
			Where("id = ?", member.ID).
			Save(&member).Error; err != nil {
			return apierrors.ErrGeneric
		}

		return nil
	}); err != nil {
		return EError(c, err)
	}

	return c.NoContent(http.StatusOK)
}

// deleteProjectMemberAdmin удаляет участника проекта из рабочего пространства.
// @Summary Пользователи: Удалить участника проекта
// @Description Метод удаляет участника проекта из рабочего пространства. Если участник не найден в проекте, возвращается ошибка.
// @Tags AdminPanel
// @Accept json
// @Produce json
// @Param workspaceId path string true "ID рабочего пространства"
// @Param userId path string true "ID пользователя"
// @Param projectId path string true "ID проекта"
// @Success 200 "Участник успешно удален из проекта"
// @Failure 400 {object} apierrors.DefinedError "Некорректные данные"
// @Failure 403 {object} apierrors.DefinedError "Ошибка: доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Рабочее пространство, проект или участник не найдены"
// @Failure 500 {object} apierrors.DefinedError "Внутренняя ошибка сервера"
// @Router /api/auth/admin/users/{userId}/workspaces/{workspaceId}/projects/{projectId}/member/ [delete]
func (s *Services) deleteProjectMemberAdmin(c echo.Context) error {
	userId := c.Param("userId")
	workspaceId := c.Param("workspaceId")
	projectId := c.Param("projectId")

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		result := tx.
			Where("workspace_id = ?", workspaceId).
			Where("project_id = ?", projectId).
			Where("member_id = ?", userId).
			Delete(&dao.ProjectMember{})

		if result.Error != nil {
			return EErrorDefined(c, apierrors.ErrGeneric)
		}

		if result.RowsAffected == 0 {
			return EErrorDefined(c, apierrors.ErrWorkspaceMemberNotFound)
		}

		return nil
	}); err != nil {
		return EError(c, err)
	}

	return c.NoContent(http.StatusOK)
}

// ############# Release notes management ###################

// createReleaseNote godoc
// @id createReleaseNote
// @Summary Релизы: создание примечания к релизу
// @Description Создает новое примечание к релизу
// @Tags AdminPanel
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param data body dto.ReleaseNoteLight true "Данные примечании к релизу. ID, TagName, PublishedAt и AuthorId проставляются автоматически."
// @Success 201 "Примечание к релизу успешно создано"
// @Failure 400 {object} apierrors.DefinedError "Некорректные данные запроса"
// @Failure 403 {object} apierrors.DefinedError "Ошибка: доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Внутренняя ошибка сервера"
// @Router /api/auth/admin/release-notes [post]
func (s *Services) createReleaseNote(c echo.Context) error {
	var note dao.ReleaseNote
	if err := c.Bind(&note); err != nil {
		return EError(c, err)
	}
	note.AuthorId = c.(AuthContext).User.ID
	note.TagName = appVersion
	note.PublishedAt = time.Now()
	if len(note.Body.Body) == 0 {
		return EErrorDefined(c, apierrors.ErrReleaseNoteEmptyBody)
	}
	if err := s.db.Create(&note).Error; err != nil {
		if err == gorm.ErrDuplicatedKey {
			return EErrorDefined(c, apierrors.ErrReleaseNoteExists)
		}
		return EError(c, err)
	}
	return c.NoContent(http.StatusCreated)
}

// getReleaseNote godoc
// @id getReleaseNote
// @Summary Релизы: получение примечания к релизу
// @Description Возвращает информацию о примечании к релизу по ID
// @Tags ReleaseNotes
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param noteId path string true "ID или версия релиза примечания к релизу"
// @Success 200 {object} dto.ReleaseNoteLight "Информация о примечании к релизу"
// @Failure 403 {object} apierrors.DefinedError "Ошибка: доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Примечание к релизу не найдено"
// @Failure 500 {object} apierrors.DefinedError "Внутренняя ошибка сервера"
// @Router /api/auth/admin/release-notes/{noteId} [get]
func (s *Services) getReleaseNote(c echo.Context) error {
	rn := c.(ReleaseNoteContext).ReleaseNote
	return c.JSON(http.StatusOK, rn.ToLightDTO())
}

// updateReleaseNote godoc
// @id updateReleaseNote
// @Summary Релизы: редактирование примечания к релизу
// @Description Обновляет информацию о примечании к релизу
// @Tags AdminPanel
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param noteId path string true "ID примечания к релизу"
// @Param data body dto.ReleaseNoteLight true "Данные для обновления примечания к релизу"
// @Success 200 {object} dto.ReleaseNoteLight "Обновленное примечание к релизу"
// @Failure 400 {object} apierrors.DefinedError "Некорректные данные запроса"
// @Failure 403 {object} apierrors.DefinedError "Ошибка: доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Примечание к релизу не найдено"
// @Failure 500 {object} apierrors.DefinedError "Внутренняя ошибка сервера"
// @Router /api/auth/admin/release-notes/{noteId} [patch]
func (s *Services) updateReleaseNote(c echo.Context) error {
	var data dao.ReleaseNote
	if err := c.Bind(&data); err != nil {
		return EError(c, err)
	}

	data.ID = c.(ReleaseNoteContext).ReleaseNote.ID
	data.TagName = c.(ReleaseNoteContext).ReleaseNote.TagName
	data.AuthorId = c.(ReleaseNoteContext).User.ID
	if len(data.Body.Body) == 0 {
		return EErrorDefined(c, apierrors.ErrReleaseNoteEmptyBody)
	}
	if err := s.db.Save(&data).Error; err != nil {
		return EError(c, err)
	}

	return c.JSON(http.StatusOK, data.ToLightDTO())
}

// deleteReleaseNote godoc
// @id deleteReleaseNote
// @Summary Релизы: удаление примечания к релизу
// @Description Удаляет примечание к релизу по ID
// @Tags AdminPanel
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param noteId path string true "ID примечания к релизу"
// @Success 200 "Примечание к релизу успешно удалено"
// @Failure 403 {object} apierrors.DefinedError "Ошибка: доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Примечание к релизу не найдено"
// @Failure 500 {object} apierrors.DefinedError "Внутренняя ошибка сервера"
// @Router /api/auth/admin/release-notes/{noteId} [delete]
func (s *Services) deleteReleaseNote(c echo.Context) error {
	note := c.(ReleaseNoteContext).ReleaseNote
	if err := s.db.Delete(&note).Error; err != nil {
		return EError(c, err)
	}
	return c.NoContent(http.StatusOK)
}

// ############# Users feedbacks ###################

// getAllFeedbackList godoc
// @id getAllFeedbackList
// @Summary Feedback: получение всех отзывов пользователей
// @Description Возвращает список всех отзывов пользователей с поддержкой пагинации, сортировки и фильтрации
// @Tags AdminPanel
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param offset query int false "Смещение для пагинации" default(0)
// @Param limit query int false "Количество результатов на странице" default(100)
// @Param order_by query string false "Поле для сортировки"
// @Param desc query bool false "Направление сортировки: true - по убыванию, false - по возрастанию" default(false)
// @Success 200 {object} dao.PaginationResponse{result=[]dto.UserFeedback} "Список отзывов пользователей"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 403 {object} apierrors.DefinedError "Ошибка: доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Внутренняя ошибка сервера"
// @Router /api/auth/admin/feedbacks [get]
func (s *Services) getAllFeedbackList(c echo.Context) error {
	offset := 0
	limit := 100
	orderBy := ""
	desc := false

	if err := echo.QueryParamsBinder(c).
		Int("offset", &offset).
		Int("limit", &limit).
		String("order_by", &orderBy).
		Bool("desc", &desc).
		BindError(); err != nil {
		return EError(c, err)
	}

	if limit > 100 {
		limit = 100
	}

	query := s.db.Preload(clause.Associations)

	if orderBy != "" {
		query = query.Order(clause.OrderByColumn{Column: clause.Column{Name: orderBy}, Desc: desc})
	}

	var feedbacks []dao.UserFeedback
	resp, err := dao.PaginationRequest(
		offset,
		limit,
		query,
		&feedbacks,
	)
	if err != nil {
		return EError(c, err)
	}

	resp.Result = utils.SliceToSlice(resp.Result.(*[]dao.UserFeedback), func(uf *dao.UserFeedback) dto.UserFeedback { return *uf.ToDTO() })

	return c.JSON(http.StatusOK, resp)
}

// exportFeedbackList godoc
// @id exportFeedbackList
// @Summary Feedback: экспорт отзывов пользователей
// @Description Экспортирует отзывы пользователей в сжатом CSV файле
// @Tags AdminPanel
// @Security ApiKeyAuth
// @Accept json
// @Produce application/gzip
// @Success 200 "Отчёт успешно экспортирован"
// @Failure 403 {object} apierrors.DefinedError "Ошибка: доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Внутренняя ошибка сервера"
// @Router /api/auth/admin/feedbacks/export [get]
func (s *Services) exportFeedbackList(c echo.Context) error {
	var count int64
	if err := s.db.Model(&dao.UserFeedback{}).Count(&count).Error; err != nil {
		return EError(c, err)
	}

	var bufio bytes.Buffer
	gz := gzip.NewWriter(&bufio)
	w := csv.NewWriter(gz)
	w.Write([]string{"Email", "CreatedAt", "UpdatedAt", "Stars", "Feedback"})

	offset := 0
	for {
		var feedbacks []dao.UserFeedback
		_, err := dao.PaginationRequest(
			offset,
			100,
			s.db.Preload(clause.Associations).Order("created_at"),
			&feedbacks,
		)
		if err != nil {
			return EError(c, err)
		}
		if len(feedbacks) == 0 {
			break
		}
		offset += 100

		for _, feedback := range feedbacks {
			if err := w.Write([]string{
				feedback.User.Email,
				feedback.CreatedAt.Format(time.RFC3339),
				feedback.UpdatedAt.Format(time.RFC3339),
				fmt.Sprint(feedback.Stars),
				feedback.Feedback,
			}); err != nil {
				return EError(c, err)
			}
		}
	}
	w.Flush()
	gz.Close()

	c.Response().Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=aiplan_feedbacks_%s.csv.gz", time.Now().Format("02-01-2006")))
	return c.Stream(http.StatusOK, http.DetectContentType(bufio.Bytes()), &bufio)
}

// reloadTemplates godoc
// @id reloadTemplates
// @Summary templates: сброс
// @Description сброс шаблонов почты к установкам по умолчанию
// @Tags AdminPanel
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Success 200 "Сброс успешно завершен"
// @Failure 403 {object} apierrors.DefinedError "Ошибка: доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Ошибка"
// @Router /api/auth/admin/templates/reset/ [post]
func (s *Services) reloadTemplates(c echo.Context) error {
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("1 = 1").Delete(&dao.Template{}).Error; err != nil {
			return err
		}

		s.emailService.CreateNewTemplates(tx)

		return nil
	}); err != nil {
		return EErrorDefined(c, apierrors.ErrGeneric)
	}
	return c.NoContent(http.StatusOK)
}

// getRunningImportList godoc
// @id getRunningImportList
// @Summary templates: список текущих импортов
// @Description получение текущих импортов в активном статусе
// @Tags AdminPanel
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Success 200 {object} []issues_import.ImportStatus "Статусы импортов"
// @Failure 403 {object} apierrors.DefinedError "Ошибка: доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Ошибка"
// @Router /api/auth/admin/imports/ [get]
func (s *Services) getRunningImportList(c echo.Context) error {
	res, err := s.importService.GetActiveImports()
	if err != nil {
		return EError(c, err)
	}

	return c.JSON(http.StatusOK, res)
}

type roleUpdRequest struct {
	Role int `json:"role" validate:"required"`
}

// geRootActivityList godoc
// @id geRootActivityList
// @Summary Активности: получение активностей верхнего уровня
// @Description Возвращает список активностей верхнего уровня с поддержкой пагинации
// @Tags Workspace
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param day query string false "День выборки активностей" default("")
// @Param offset query int false "Смещение для пагинации" default(-1)
// @Param limit query int false "Количество результатов на странице" default(100)
// @Success 200 {object} dao.PaginationResponse{result=[]dto.EntityActivityFull} "Список активностей"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 403 {object} apierrors.DefinedError "Ошибка: доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Внутренняя ошибка сервера"
// @Router /api/auth/admin/activities/ [get]
func (s *Services) geRootActivityList(c echo.Context) error {

	var day DayRequest
	offset := -1
	limit := 100

	if err := echo.QueryParamsBinder(c).
		TextUnmarshaler("day", &day).
		Int("offset", &offset).
		Int("limit", &limit).BindError(); err != nil {
		return EError(c, err)
	}

	var root dao.RootActivity
	root.UnionCustomFields = "'root' AS entity_type"

	unionTable := dao.BuildUnionSubquery(s.db, "union_activities", dao.FullActivity{}, root)
	query := unionTable.
		Joins("Project").
		Joins("Workspace").
		Joins("Actor").
		Joins("Issue").
		Joins("Doc").
		Joins("Form").
		Order("union_activities.created_at desc")

	if !time.Time(day).IsZero() {
		query = query.Where("union_activities.created_at >= ?", time.Time(day)).Where("union_activities.created_at < ?", time.Time(day).Add(time.Hour*24))
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

	resp.Result = utils.SliceToSlice(resp.Result.(*[]dao.FullActivity), func(ea *dao.FullActivity) dto.EntityActivityFull { return *ea.ToDTO() })

	return c.JSON(http.StatusOK, resp)
}
