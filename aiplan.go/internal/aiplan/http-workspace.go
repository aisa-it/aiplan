// Пакет aiplan предоставляет функциональность для управления рабочими пространствами, включая создание, редактирование, добавление участников, интеграции и историю изменений.  Он включает в себя API для работы с рабочими пространствами, а также логику для управления пользователями и их правами доступа.
package aiplan

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"sheff.online/aiplan/internal/aiplan/apierrors"
	errStack "sheff.online/aiplan/internal/aiplan/stack-error"

	"sheff.online/aiplan/internal/aiplan/dto"
	"sheff.online/aiplan/internal/aiplan/utils"

	filestorage "sheff.online/aiplan/internal/aiplan/file-storage"
	"sheff.online/aiplan/internal/aiplan/types"

	"github.com/gofrs/uuid"
	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	"github.com/sethvargo/go-password/password"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	tracker "sheff.online/aiplan/internal/aiplan/activity-tracker"
	"sheff.online/aiplan/internal/aiplan/dao"
)

func (s *Services) LastVisitedWorkspaceMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		workspaceContext, ok := c.(WorkspaceContext)
		if !ok {
			return next(c)
		}

		workspace := workspaceContext.Workspace
		user := workspaceContext.User

		if user.LastWorkspaceId != &workspace.ID {
			user.LastWorkspace = &workspace
			if err := s.db.Model(&user).Update("last_workspace_id", workspace.ID).Error; err != nil {
				return EError(c, err)
			}
		}

		return next(c)
	}
}

func (s *Services) WorkspaceMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		user := c.(AuthContext).User
		slugOrId := c.Param("workspaceSlug")

		if etag := c.Request().Header.Get("If-None-Match"); etag != "" {
			var exist bool
			if err := s.db.Model(&dao.Workspace{}).Select("count(*) > 0").Where("encode(hash, 'hex') = ?", etag).Find(&exist).Error; err != nil {
				return EError(c, err)
			}

			if exist {
				return c.NoContent(http.StatusNotModified)
			}
		}

		var workspace dao.Workspace
		if err := s.db.
			Joins("Owner").
			Joins("LogoAsset").
			Set("userID", user.ID).
			Where("workspaces.id = ? or slug = ?", slugOrId, slugOrId).
			First(&workspace).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return EErrorDefined(c, apierrors.ErrWorkspaceNotFound)
			}
			return EError(c, err)
		}

		var workspaceMember dao.WorkspaceMember
		if workspace.CurrentUserMembership != nil {
			workspaceMember = *workspace.CurrentUserMembership
		} else {
			return EErrorDefined(c, apierrors.ErrWorkspaceNotFound)
		}
		workspaceMember.Workspace = &workspace

		return next(WorkspaceContext{c.(AuthContext), workspace, workspaceMember})
	}
}

// AddWorkspaceServices - добавление сервисов рабочих пространств
func (s *Services) AddWorkspaceServices(g *echo.Group) {
	workspaceGroup := g.Group("workspaces/:workspaceSlug", s.WorkspaceMiddleware)
	workspaceGroup.Use(s.LastVisitedWorkspaceMiddleware)
	workspaceGroup.Use(s.WorkspacePermissionMiddleware)

	// ../front/services/workspace.service.ts
	g.GET("users/me/workspaces/", s.getUserWorkspaceList)

	// Favorites
	g.GET("users/user-favorite-workspaces/", s.getFavoriteWorkspaceList)
	g.POST("users/user-favorite-workspaces/", s.addWorkspaceToFavorites)
	g.DELETE("users/user-favorite-workspaces/:workspaceID/", s.removeWorkspaceFromFavorites)

	g.POST("workspaces/", s.createWorkspace)

	workspaceGroup.GET("/", s.getWorkspace)
	workspaceGroup.PATCH("/", s.updateWorkspace)
	workspaceGroup.POST("/logo/", s.updateWorkspaceLogo)
	workspaceGroup.DELETE("/logo/", s.deleteWorkspaceLogo)
	workspaceGroup.DELETE("/", s.deleteWorkspace)

	workspaceGroup.POST("/invite/", s.addToWorkspace)

	workspaceGroup.GET("/activities/", s.getWorkspaceActivityList)

	workspaceGroup.GET("/members/", s.getWorkspaceMemberList)
	workspaceGroup.PATCH("/members/:memberId/", s.updateWorkspaceMember)
	workspaceGroup.PATCH("/members/:memberId/set-email/", s.updateUserEmail)
	workspaceGroup.DELETE("/members/:memberId/", s.deleteWorkspaceMember)

	workspaceGroup.GET("/members/activities/", s.getWorkspaceMembersActivityList)

	workspaceGroup.POST("/members/message/", s.createMessageForWorkspaceMember)

	workspaceGroup.GET("/token/", s.getWorkspaceToken)
	workspaceGroup.POST("/token/reset/", s.resetWorkspaceToken)

	g.GET("users/last-visited-workspace/", s.getLastVisitedWorkspace)

	workspaceGroup.GET("/workspace-members/me/", s.getWorkspaceMemberMe)

	workspaceGroup.GET("/states/", s.getWorkspaceStateList)

	workspaceGroup.GET("/jitsi-token/", s.getWorkspaceJitsiToken)

	workspaceGroup.GET("/integrations/", s.getIntegrationList)
	workspaceGroup.POST("/integrations/add/:name/", s.addIntegrationToWorkspace)
	workspaceGroup.DELETE("/integrations/:name/", s.deleteIntegrationFromWorkspace)
}

// getWorkspaceMemberMe godoc
// @id getWorkspaceMemberMe
// @Summary Пространство: получение информации о текущем участнике рабочего пространства
// @Description Возвращает данные участника для текущего пользователя в рабочем пространстве.
// @Tags Workspace
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Success 200 {object} dto.WorkspaceMember "Успешный ответ с данными текущего участника рабочего пространства"
// @Failure 403 {object} apierrors.DefinedError "Ошибка: доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Ошибка: рабочее пространство не найдено"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/workspace-members/me/ [get]
func (s *Services) getWorkspaceMemberMe(c echo.Context) error {
	wm := c.(WorkspaceContext).WorkspaceMember
	return c.JSON(http.StatusOK, wm.ToDTO())
}

// ############# Workspace methods ###################

// getWorkspace godoc
// @id getWorkspace
// @Summary Пространство: получение информации о рабочем пространстве
// @Description Возвращает информацию о рабочем пространстве по его ID
// @Tags Workspace
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Success 200 {object} dto.Workspace "Информация о рабочем пространстве"
// @Failure 404 {object} apierrors.DefinedError "Ошибка: рабочее пространство не найдено"
// @Failure 403 {object} apierrors.DefinedError "Ошибка: доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug} [get]
func (s *Services) getWorkspace(c echo.Context) error {
	workspace := c.(WorkspaceContext).Workspace
	c.Response().Header().Add("ETag", hex.EncodeToString(workspace.Hash))
	return c.JSON(http.StatusOK, workspace.ToDTO())
}

// updateWorkspace godoc
// @id updateWorkspace
// @Summary Пространство: обновление данных рабочего пространства
// @Description Обновляет информацию о рабочем пространстве
// @Tags Workspace
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param workspace body dto.Workspace false "Объект рабочего пространства с обновленными данными"
// @Success 200 {object} dto.Workspace "Информация о обновленном рабочем пространстве"
// @Failure 400 {object} apierrors.DefinedError "Ошибка запроса"
// @Failure 403 {object} apierrors.DefinedError "Ошибка: доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Ошибка: рабочее пространство или администратор не найдены"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug} [patch]
func (s *Services) updateWorkspace(c echo.Context) error {
	user := c.(WorkspaceContext).User
	workspace := c.(WorkspaceContext).Workspace

	// Pre-update activity tracking
	oldWorkspaceMap := StructToJSONMap(workspace)

	oldOwnerId := workspace.OwnerId
	id := workspace.ID
	if err := c.Bind(&workspace); err != nil {
		return EError(c, err)
	}
	workspace.ID = id
	workspace.UpdatedById = &user.ID
	workspace.Name = strings.TrimSpace(workspace.Name)
	var newMemberOwnerId, newMemberOwnerEmail string
	err := c.Validate(workspace)
	if err != nil {
		return EError(c, err)
	}

	changeOwner := oldOwnerId != workspace.OwnerId
	// Check new owner id exists and admin
	if changeOwner {
		var member dao.WorkspaceMember
		if err := s.db.
			Joins("Member").
			Where("workspace_id = ?", workspace.ID).
			Where("member_id = ?", workspace.OwnerId).
			Where("workspace_members.role = ?", 15).Find(&member).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return EErrorDefined(c, apierrors.ErrWorkspaceAdminNotFound)
			}
			return EError(c, err)
		}
		newMemberOwnerId = member.MemberId
		newMemberOwnerEmail = member.Member.Email
	}

	if !user.IsSuperuser && user.ID != oldOwnerId && oldOwnerId != workspace.OwnerId {
		return EErrorDefined(c, apierrors.ErrPermissionChangeWorkspaceOwner)
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Select([]string{"name", "description", "company_size", "owner_id"}).Updates(&workspace).Error; err != nil {
			return err
		}

		// Post-update activity tracking
		newWorkspaceMap := StructToJSONMap(workspace)
		if changeOwner {
			newWorkspaceMap["owner_id_activity_val"] = newMemberOwnerEmail
			newWorkspaceMap["owner_id_updateScopeId"] = newMemberOwnerId
			newWorkspaceMap["owner_id_field_log"] = "owner"
			oldWorkspaceMap["owner_id_activity_val"] = user.Email
			oldWorkspaceMap["owner_id_updateScopeId"] = user.ID
			oldWorkspaceMap["owner_id_field_log"] = "owner"
		}

		err = tracker.TrackActivity[dao.Workspace, dao.WorkspaceActivity](s.tracker, tracker.ENTITY_UPDATED_ACTIVITY, newWorkspaceMap, oldWorkspaceMap, workspace, user)
		if err != nil {
			errStack.GetError(c, err)
		}
		return nil
		//return s.tracker.TrackActivity(tracker.WORKSPACE_UPDATED_ACTIVITY, newWorkspaceMap, oldWorkspaceMap, workspace.ID, tracker.ENTITY_TYPE_WORKSPACE, nil, *user)
	}); err != nil {
		return EError(c, err)
	}

	return c.JSON(http.StatusOK, workspace.ToDTO())
}

// updateWorkspaceLogo godoc
// @id updateWorkspaceLogo
// @Summary Пространство (логотип): обновление пространства
// @Description Загружает новый логотип для указанного рабочего пространства и обновляет запись в базе данных.
// @Tags Workspace
// @Accept multipart/form-data
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param file formData file true "Файл логотипа"
// @Success 200 {object} dto.Workspace "Обновленное рабочее пространство"
// @Failure 400 {object} apierrors.DefinedError "Ошибка: неверный формат файла"
// @Failure 403 {object} apierrors.DefinedError "Ошибка: недостаточно прав для обновления логотипа"
// @Failure 404 {object} apierrors.DefinedError "Ошибка: рабочее пространство не найдено"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/logo [post]
func (s *Services) updateWorkspaceLogo(c echo.Context) error {
	user := c.(WorkspaceContext).User
	workspace := c.(WorkspaceContext).Workspace

	if user.Tariffication != nil && !user.Tariffication.AttachmentsAllow {
		return EError(c, apierrors.ErrAssetsNotAllowed)
	}

	file, err := c.FormFile("file")
	if err != nil {
		return EError(c, err)
	}

	fileAsset := dao.FileAsset{
		Id:          dao.GenUUID(),
		CreatedById: &user.ID,
		WorkspaceId: &workspace.ID,
	}

	oldLogoId := workspace.LogoId

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		var oldLogo dao.FileAsset
		if workspace.LogoAsset != nil {
			if err := tx.Where("id = ?", workspace.LogoId).First(&oldLogo).Error; err != nil {
				if err != gorm.ErrRecordNotFound {
					return err
				}
			}
		}

		if err := s.uploadAssetForm(tx, file, &fileAsset, filestorage.Metadata{
			WorkspaceId: workspace.ID,
		}); err != nil {
			return err
		}

		workspace.LogoId = uuid.NullUUID{UUID: fileAsset.Id, Valid: true}
		workspace.LogoAsset = &fileAsset
		if err := tx.Select("logo_id").Updates(&workspace).Error; err != nil {
			return err
		}

		if !oldLogo.Id.IsNil() {
			if err := tx.Delete(&oldLogo).Error; err != nil {
				return err
			}
		}

		//Трекинг активности
		oldMap := map[string]interface{}{
			"logo": oldLogoId.UUID.String(),
		}
		newMap := map[string]interface{}{
			"logo": fileAsset.Id.String(),
		}

		err = tracker.TrackActivity[dao.Workspace, dao.WorkspaceActivity](s.tracker, tracker.ENTITY_UPDATED_ACTIVITY, newMap, oldMap, workspace, user)
		if err != nil {
			errStack.GetError(c, err)
		}

		//if err := s.tracker.TrackActivity(tracker.WORKSPACE_LOGO_UPDATED_ACTIVITY, newMemberMap, oldMemberMap, workspace.ID, tracker.ENTITY_TYPE_WORKSPACE, nil, *user); err != nil {
		//	return err
		//}

		return nil
	}); err != nil {
		return EError(c, err)
	}

	return c.JSON(http.StatusOK, workspace.ToDTO())
}

// deleteWorkspaceLogo godoc
// @id deleteWorkspaceLogo
// @Summary Пространство (логотип): удаление логотипа пространства
// @Description Удаляет логотип указанного рабочего пространства и обновляет запись в базе данных.
// @Tags Workspace
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Success 200 {object} dto.Workspace "Обновленное рабочее пространство"
// @Failure 403 {object} apierrors.DefinedError "Ошибка: недостаточно прав для удаления логотипа"
// @Failure 404 {object} apierrors.DefinedError "Ошибка: рабочее пространство не найдено"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/logo [delete]
func (s *Services) deleteWorkspaceLogo(c echo.Context) error {
	user := c.(WorkspaceContext).User
	workspace := c.(WorkspaceContext).Workspace
	oldLogoId := workspace.LogoId.UUID.String()

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		workspace.UpdatedById = &user.ID
		workspace.LogoId = uuid.NullUUID{}
		if err := tx.Select("logo_id").Updates(&workspace).Error; err != nil {
			return err
		}

		if workspace.LogoAsset != nil {
			if err := tx.Delete(&workspace.LogoAsset).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return EError(c, err)
	}

	//Трекинг активности
	oldMap := map[string]interface{}{
		"logo": oldLogoId,
	}
	newMap := map[string]interface{}{
		"logo": uuid.NullUUID{}.UUID.String(),
	}

	err := tracker.TrackActivity[dao.Workspace, dao.WorkspaceActivity](s.tracker, tracker.ENTITY_UPDATED_ACTIVITY, newMap, oldMap, workspace, user)
	if err != nil {
		errStack.GetError(c, err)
	}

	return c.JSON(http.StatusOK, workspace.ToDTO())
}

// deleteWorkspace godoc
// @id deleteWorkspace
// @Summary Пространство: удаление пространства
// @Description Удаляет указанное рабочее пространство. Доступно только для суперпользователей и владельца рабочего пространства.
// @Tags Workspace
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Success 200 "Рабочее пространство успешно удалено"
// @Failure 403 {object} apierrors.DefinedError "Ошибка: недостаточно прав для удаления рабочего пространства"
// @Failure 404 {object} apierrors.DefinedError "Ошибка: рабочее пространство не найдено"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug} [delete]
func (s *Services) deleteWorkspace(c echo.Context) error {
	user := c.(WorkspaceContext).User
	workspace := c.(WorkspaceContext).Workspace

	if !user.IsSuperuser && user.ID != workspace.OwnerId {
		return EErrorDefined(c, apierrors.ErrDeleteWorkspaceForbidden)
	}

	err := tracker.TrackActivity[dao.Workspace, dao.RootActivity](s.tracker, tracker.ENTITY_DELETE_ACTIVITY, nil, nil, workspace, user)
	if err != nil {
		errStack.GetError(c, err)
		return err
	}
	// Cancel jira imports
	if err := s.importService.CancelWorkspaceImports(workspace.ID); err != nil {
		return EError(c, err)
	}

	// Soft-delete projects
	if err := s.db.Session(&gorm.Session{SkipHooks: true}).Omit(clause.Associations).Where("workspace_id = ?", workspace.ID).Delete(&dao.Project{}).Error; err != nil {
		return EError(c, err)
	}

	// Soft-delete workspace
	if err := s.db.Session(&gorm.Session{SkipHooks: true}).Omit(clause.Associations).Delete(&workspace).Error; err != nil {
		return EError(c, err)
	}

	// Workspaces will be hard deleted by cron
	// Start hard deleting in foreground
	/*go func(workspace dao.Workspace) {
		if err := s.db.Unscoped().Delete(&workspace).Error; err != nil {
			slog.Error("Hard delete workspace", "workspaceId", workspace.ID, "err", err)
		}
	}(workspace)*/

	return c.NoContent(http.StatusOK)
}

// ############# Activities methods ###################

// getWorkspaceActivityList godoc
// @id getWorkspaceActivityList
// @Summary Пространство: получение активностей рабочего пространства
// @Description Возвращает список активностей рабочего пространства с поддержкой пагинации
// @Tags Workspace
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param day query string false "День выборки активностей" default("")
// @Param offset query int false "Смещение для пагинации" default(-1)
// @Param limit query int false "Количество результатов на странице" default(100)
// @Success 200 {object} dao.PaginationResponse{result=[]dto.EntityActivityFull} "Список активностей рабочего пространства"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 404 {object} apierrors.DefinedError "Рабочее пространство не найдено"
// @Failure 500 {object} apierrors.DefinedError "Внутренняя ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/activities [get]
func (s *Services) getWorkspaceActivityList(c echo.Context) error {
	workspaceId := c.(WorkspaceContext).Workspace.ID

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
	var workspace dao.WorkspaceActivity
	workspace.UnionCustomFields = "'workspace' AS entity_type"
	var form dao.FormActivity
	form.UnionCustomFields = "'form' AS entity_type"
	var doc dao.DocActivity
	doc.UnionCustomFields = "'doc' AS entity_type"

	unionTable := dao.BuildUnionSubquery(s.db, "union_activities", dao.FullActivity{}, issue, project, workspace, form, doc)
	query := unionTable.
		Joins("Project").
		Joins("Workspace").
		Joins("Actor").
		Joins("Issue").
		Joins("Doc").
		Joins("Form").
		Order("union_activities.created_at desc").
		Where("union_activities.workspace_id = ?", workspaceId)

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

// ############# Workspace members methods ###################

// getWorkspaceMemberList godoc
// @id getWorkspaceMemberList
// @Summary Пространство (участники): получение списка участников пространства
// @Description Возвращает список участников указанного рабочего пространства. Включает поиск по email или имени.
// @Tags Workspace
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param offset query int false "Смещение для пагинации" default(-1)
// @Param limit query int false "Ограничение количества записей на странице" default(100)
// @Param search_query query string false "Поисковый запрос для фильтрации участников по email или имени" default("")
// @Param order_by query string false "Поле для сортировки: 'last_name' (по умолчанию), 'email', 'role'" default("last_name")
// @Param desc query bool false "Направление сортировки: true - по убыванию, false - по возрастанию" default(true)
// @Success 200 {object} dao.PaginationResponse{result=[]dto.WorkspaceMember} "Список участников с учетом пагинации"
// @Failure 400 {object} apierrors.DefinedError "Ошибка валидации данных запроса"
// @Failure 403 {object} apierrors.DefinedError "Ошибка: доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Ошибка: рабочее пространство не найдено"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/members [get]
func (s *Services) getWorkspaceMemberList(c echo.Context) error {
	workspaceMember := c.(WorkspaceContext).WorkspaceMember

	offset := -1
	limit := 100
	searchQuery := ""
	orderBy := ""
	desc := true

	if err := echo.QueryParamsBinder(c).
		Int("offset", &offset).
		Int("limit", &limit).
		String("search_query", &searchQuery).
		String("order_by", &orderBy).
		Bool("desc", &desc).
		BindError(); err != nil {
		return EError(c, err)
	}

	switch orderBy {
	case "email":
		orderBy = "lower(email)"
	case "role":
		break
	default:
		orderBy = "lower(last_name)"
	}

	if desc {
		orderBy = fmt.Sprintf("%s %s", orderBy, "desc")
	} else {
		orderBy = fmt.Sprintf("%s %s", orderBy, "asc")
	}

	query := s.db.Preload("Workspace").
		Preload("Workspace.Owner").
		Joins("Member").
		Preload("Member").
		Where("workspace_id in (?)", workspaceMember.WorkspaceId).
		Order(orderBy)

	if searchQuery != "" {
		escapedSearchQuery := PrepareSearchRequest(searchQuery)
		query = query.Where("lower(email) like ? or lower(last_name) like ? or lower(first_name) like ?", escapedSearchQuery, escapedSearchQuery, escapedSearchQuery)
	}

	var members []dao.WorkspaceMember
	res, err := dao.PaginationRequest(
		offset,
		limit,
		query,
		&members,
	)
	if err != nil {
		return EError(c, err)
	}

	res.Result = utils.SliceToSlice(res.Result.(*[]dao.WorkspaceMember), func(wm *dao.WorkspaceMember) dto.WorkspaceMember { return *wm.ToDTO() })

	return c.JSON(http.StatusOK, res)
}

// updateWorkspaceMember godoc
// @id updateWorkspaceMember
// @Summary Пространство (участники): обновление роли участника пространства
// @Description Изменяет роль участника в рабочем пространстве. Администраторы могут назначать и изменять роли участников.
// @Tags Workspace
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param memberId path string true "ID участника для обновления роли"
// @Param role body requestRoleMember true "Новая роль участника"
// @Success 200 {object} dto.WorkspaceMember "Роль участника успешно обновлена"
// @Failure 400 {object} apierrors.DefinedError "Ошибка валидации данных запроса"
// @Failure 403 {object} apierrors.DefinedError "Ошибка: недостаточно прав для обновления роли"
// @Failure 404 {object} apierrors.DefinedError "Ошибка: участник или рабочее пространство не найдены"
// @Failure 409 {object} apierrors.DefinedError "Ошибка: запрещено обновлять роль владельца рабочего пространства"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/members/{memberId} [patch]
func (s *Services) updateWorkspaceMember(c echo.Context) error {
	user := *c.(WorkspaceContext).User
	workspace := c.(WorkspaceContext).Workspace
	workspaceMember := c.(WorkspaceContext).WorkspaceMember
	requestedMemberId := c.Param("memberId")

	var req requestRoleMember

	if err := c.Bind(&req); err != nil {
		return EError(c, err)
	}

	var requestedMember dao.WorkspaceMember
	if err := s.db.
		Preload("Member").
		Where("id = ?", requestedMemberId).
		Where("workspace_id = ?", workspace.ID).
		First(&requestedMember).Error; err != nil {
		return EError(c, err)
	}

	if requestedMember.MemberId == workspace.OwnerId {
		return EErrorDefined(c, apierrors.ErrUpdateOwnerForbidden)
	}

	if user.ID == requestedMember.MemberId {
		return EErrorDefined(c, apierrors.ErrUpdateOwnUserForbidden)
	}

	if workspaceMember.Role < requestedMember.Role {
		return EErrorDefined(c, apierrors.ErrUpdateHigherRoleUserForbidden)
	}
	oldMemberMap := StructToJSONMap(requestedMember)
	var newMemberMap map[string]interface{}

	var oldMemberRole int
	if req.Role != nil {
		oldMemberRole = *req.Role

		if err := s.db.Transaction(func(tx *gorm.DB) error {
			requestedMember.UpdatedById = &user.ID
			requestedMember.UpdatedAt = time.Now()
			requestedMember.Role = *req.Role
			if err := tx.Save(&requestedMember).Error; err != nil {
				return err
			}

			var projects []dao.Project
			if err := tx.Where("workspace_id = ?", workspace.ID).Find(&projects).Error; err != nil {
				return err
			}

			// -> AdminRole = add admin memberships to all projects
			if *req.Role == types.AdminRole {
				for _, project := range projects {
					if err := tx.Clauses(clause.OnConflict{
						Columns:   []clause.Column{{Name: "project_id"}, {Name: "member_id"}},
						DoUpdates: clause.Assignments(map[string]interface{}{"role": types.AdminRole, "updated_at": time.Now(), "updated_by_id": user.ID}),
					}).Create(&dao.ProjectMember{
						ID:          dao.GenID(),
						CreatedAt:   time.Now(),
						CreatedById: &user.ID,
						WorkspaceId: workspace.ID,
						ProjectId:   project.ID,
						Role:        types.AdminRole,
						MemberId:    requestedMember.MemberId,
					}).Error; err != nil {
						return err
					}
				}
			}

			// AdminRole -> not AdminRole = remove all memberships
			if *req.Role != types.AdminRole && oldMemberRole == types.AdminRole {
				if err := tx.
					Where("workspace_id = ?", workspace.ID).
					Where("member_id = ?", requestedMember.MemberId).
					Delete(&dao.ProjectMember{}).Error; err != nil {
					return err
				}
			}

			newMemberMap = StructToJSONMap(requestedMember)
			newMemberMap["updateScopeId"] = requestedMember.MemberId

			// Трек активности после обновления роли участника
			//if err := s.tracker.TrackActivity(tracker.WORKSPACE_MEMBER_UPDATED_ACTIVITY, newMemberMap, oldMemberMap, workspace.ID, tracker.ENTITY_TYPE_WORKSPACE, nil, user); err != nil {
			//	return EError(c, err)
			//}

			return nil
		}); err != nil {
			return EError(c, err)
		}
	}

	err := tracker.TrackActivity[dao.WorkspaceMember, dao.WorkspaceActivity](s.tracker, tracker.ENTITY_UPDATED_ACTIVITY, newMemberMap, oldMemberMap, requestedMember, &user)
	if err != nil {
		errStack.GetError(c, err)
	}

	return c.JSON(http.StatusOK, requestedMember.ToDTO())
}

// updateUserEmail godoc
// @id updateUserEmail
// @Summary Пространство (участники): назначение email для участника пространства
// @Description Устанавливает email для участника рабочего пространства. Операция доступна только администраторам рабочего пространства.
// @Tags Workspace
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param memberId path string true "ID участника для установки email"
// @Param email body requestEmailMember false "Новый email участника"
// @Success 200 "Email успешно установлен"
// @Failure 400 {object} apierrors.DefinedError "Ошибка валидации данных запроса"
// @Failure 403 {object} apierrors.DefinedError "Ошибка: недостаточно прав для установки email"
// @Failure 404 {object} apierrors.DefinedError "Ошибка: участник или рабочее пространство не найдены"
// @Failure 409 {object} apierrors.DefinedError "Ошибка: email уже назначен данному участнику"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/members/{memberId}/set-email/ [patch]
func (s *Services) updateUserEmail(c echo.Context) error {
	workspace := c.(WorkspaceContext).Workspace
	workspaceMember := c.(WorkspaceContext).WorkspaceMember
	requestedMemberId := c.Param("memberId")

	if workspaceMember.Role != types.AdminRole {
		return EErrorDefined(c, apierrors.ErrNotEnoughRights)
	}

	var requestedMember dao.WorkspaceMember
	if err := s.db.
		Where("member_id = ?", requestedMemberId).
		Where("workspace_id = ?", workspace.ID).
		Preload("Member").
		First(&requestedMember).Error; err != nil {
		return EError(c, err)
	}

	if requestedMember.Member.Email != "" {
		return EErrorDefined(c, apierrors.ErrMemberAlreadyHasEmail)
	}

	var req requestEmailMember
	if err := c.Bind(&req); err != nil {
		return EError(c, err)
	}

	if !ValidateEmail(req.Email) {
		return EErrorDefined(c, apierrors.ErrInvalidEmail)
	}

	if err := s.db.Model(&dao.User{}).
		Where("id = ?", requestedMember.MemberId).
		UpdateColumn("email", req.Email).Error; err != nil {
		return EError(c, err)
	}
	return c.NoContent(http.StatusOK)
}

// deleteWorkspaceMember godoc
// @id deleteWorkspaceMember
// @Summary Пространство (участники): удаление участника пространства
// @Description Удаляет указанного участника из рабочего пространства
// @Tags Workspace
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param memberId path string true "ID участника для удаления"
// @Success 204 "Участник успешно удален из рабочего пространства"
// @Failure 400 {object} apierrors.DefinedError "Ошибка: недопустимое действие или запрос"
// @Failure 403 {object} apierrors.DefinedError "Ошибка: доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Ошибка: участник или рабочее пространство не найдены"
// @Failure 409 {object} apierrors.DefinedError "Ошибка: невозможно удалить участника с более высокой ролью"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/members/{memberId} [delete]
func (s *Services) deleteWorkspaceMember(c echo.Context) error {
	user := c.(WorkspaceContext).User
	workspace := c.(WorkspaceContext).Workspace
	workspaceMember := c.(WorkspaceContext).WorkspaceMember
	requestedMemberId := c.Param("memberId")

	if workspaceMember.Member == nil {
		workspaceMember.Member = user
	}

	var requestedMember dao.WorkspaceMember
	if err := s.db.Preload("Member").
		Where("id = ?", requestedMemberId).
		Where("workspace_id = ?", workspace.ID).
		First(&requestedMember).Error; err != nil {
		return EError(c, err)
	}
	requestedMember.Workspace = &workspace

	// One cannot remove role higher than his own role
	if workspaceMember.Role < requestedMember.Role && !user.IsSuperuser {
		return EErrorDefined(c, apierrors.ErrCannotRemoveHigherRoleUser)
	} else if requestedMember.Member.IsSuperuser && workspaceMember.ID != requestedMemberId {
		return EErrorDefined(c, apierrors.ErrDeleteSuperUser)
	}
	if workspace.OwnerId == requestedMember.MemberId {
		if !user.IsSuperuser {
			return EErrorDefined(c, apierrors.ErrCannotDeleteWorkspaceAdmin)
		}
	}

	if user.ID == requestedMember.Member.ID && !user.IsSuperuser {
		return EErrorDefined(c, apierrors.ErrCannotRemoveSelfFromWorkspace)
	}

	// Delete workspace if this is last member(last user leaves workspace)
	var possibleOwners []dao.WorkspaceMember
	if err := s.db.
		Model(&dao.WorkspaceMember{}).
		Joins("Member").
		Where("workspace_id = ?", workspace.ID).
		Where("is_bot = false").                           // only humans
		Where("is_active = true").                         // only active users
		Where("is_onboarded = true").                      // only onboarded users
		Where("member_id != ?", requestedMember.MemberId). // not requested member
		Order("last_active DESC").
		Find(&possibleOwners).Error; err != nil {
		return EError(c, err)
	}

	// If last member, delete workspace. Member will be owner or/and superuser if this is last member
	if len(possibleOwners) < 1 {
		return s.deleteWorkspace(c)
	}

	if user.ID != requestedMember.Member.ID {
		// If not current user - set current user as new owner
		s.business.WorkspaceCtx(c, workspaceMember.Member, &workspace, &workspaceMember)
		defer s.business.WorkspaceCtxClean()

		if err := s.business.DeleteWorkspaceMember(&workspaceMember, &requestedMember); err != nil {
			return EError(c, err)
		}
	} else {
		newOwner := possibleOwners[0]
		newOwner.Workspace = &workspace
		s.business.WorkspaceCtx(c, newOwner.Member, &workspace, &newOwner)
		defer s.business.WorkspaceCtxClean()

		if err := s.business.DeleteWorkspaceMember(&newOwner, &requestedMember); err != nil {
			return EError(c, err)
		}
	}

	//oldMemberMap := StructToJSONMap(requestedMember)

	//if err := s.tracker.TrackActivity(tracker.WORKSPACE_MEMBER_REMOVED_ACTIVITY, nil, oldMemberMap, workspace.ID, tracker.ENTITY_TYPE_WORKSPACE, nil, *c.(WorkspaceContext).User); err != nil {
	//	return err
	//}

	return c.NoContent(http.StatusNoContent)

}

// getWorkspaceMembersActivityList godoc
// @id getWorkspaceMembersActivityList
// @Summary Пространство: активность участников
// @Description активность участников пространства
// @Tags Workspace
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param from query string true "Начальная дата периода в формате YYYY-MM-DD"
// @Param to query string true "Конечная дата периода в формате YYYY-MM-DD"
// @Success 200 {object}  map[string]types.ActivityTable "таблица активностей"
// @Failure 400 {object} apierrors.DefinedError "Некорректные данные запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/members/activities/ [get]
func (s *Services) getWorkspaceMembersActivityList(c echo.Context) error {
	workspace := c.(WorkspaceContext).Workspace
	workspaceMember := c.(WorkspaceContext).WorkspaceMember

	if workspaceMember.Role != types.AdminRole {
		return EErrorDefined(c, apierrors.ErrWorkspaceAdminRoleRequired)
	}

	var from, to DayRequest
	if err := echo.QueryParamsBinder(c).
		TextUnmarshaler("from", &from).
		TextUnmarshaler("to", &to).
		BindError(); err != nil {
		return EError(c, err)
	}

	unionTable := dao.BuildUnionSubquery(s.db, "fa", dao.FullActivity{}, dao.FormActivity{}, dao.IssueActivity{}, dao.DocActivity{}, dao.WorkspaceActivity{}, dao.ProjectActivity{})

	query := unionTable.Where("fa.workspace_id = ?", workspace.ID)
	activities, err := GetActivitiesTable(query, from, to)
	if err != nil {
		return EError(c, err)
	}

	return c.JSON(http.StatusOK, activities)
}

// createMessageForWorkspaceMember godoc
// @id createMessageForWorkspaceMember
// @Summary Пространство: Отправка сообщений участникам
// @Description Позволяет отправить сообщение всем участникам рабочего пространства или выбранным участникам. Поддерживается отправка отложенных сообщений.
// @Tags Workspace
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param data body requestMessage true "Информация о сообщении"
// @Success 200 "Сообщения успешно отправлены"
// @Failure 400 {object} apierrors.DefinedError "Некорректные данные запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/members/message/ [post]
func (s *Services) createMessageForWorkspaceMember(c echo.Context) error {
	workspace := c.(WorkspaceContext).Workspace
	user := c.(WorkspaceContext).User
	var req requestMessage
	if err := c.Bind(&req); err != nil {
		return EError(c, err)
	}
	if req.SendAt.IsZero() {
		req.SendAt = time.Now()
	}

	var members []dao.WorkspaceMember
	var notificationSentAt []dao.DeferredNotifications

	query := s.db.Preload("Member").Where("workspace_id = ?", workspace.ID)

	if len(req.Members) > 0 {
		query = query.Where("id IN (?)", req.Members)
	}
	if err := query.Find(&members).Error; err != nil {
		return EErrorDefined(c, apierrors.ErrGeneric)
	}
	if len(members) > 0 {
		for _, member := range members {
			payload := map[string]interface{}{
				"id":        dao.GenID(),
				"title":     req.Title,
				"msg":       req.Msg,
				"author_id": user.ID,
			}
			payloadBytes, err := json.Marshal(payload)
			if err != nil {
				return EErrorDefined(c, apierrors.ErrGeneric)
			}
			tmpNotify := dao.DeferredNotifications{
				ID: dao.GenID(),

				UserID: member.MemberId,
				User:   member.Member,

				WorkspaceID:         &workspace.ID,
				Workspace:           &workspace,
				NotificationType:    "message",
				DeliveryMethod:      "telegram",
				AttemptCount:        0,
				LastAttemptAt:       time.Time{},
				TimeSend:            &req.SendAt,
				NotificationPayload: payloadBytes,
			}

			notificationSentAt = append(notificationSentAt, tmpNotify)
			tmpNotify.ID = dao.GenID()
			tmpNotify.DeliveryMethod = "email"
			notificationSentAt = append(notificationSentAt, tmpNotify)
			tmpNotify.ID = dao.GenID()
			tmpNotify.DeliveryMethod = "app"
			notificationSentAt = append(notificationSentAt, tmpNotify)
		}
	}

	if len(notificationSentAt) > 0 {
		if err := s.db.Omit(clause.Associations).Create(&notificationSentAt).Error; err != nil {
			return EErrorDefined(c, apierrors.ErrGeneric)
		}
	}
	return c.NoContent(http.StatusOK)
}

// addToWorkspace godoc
// @id addToWorkspace
// @Summary Пространство: приглашение новых участников пространства
// @Description Приглашает новых пользователей или существующих в системе в указанное рабочее пространство, Приглашённые получают роль, определённую отправителем.
// @Tags Workspace
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param invite body requestMembersInvite true "Список email и ролей для приглашения пользователей в рабочее пространство"
// @Success 200 {object} map[string]interface{} "Сообщение об успешной отправке приглашений"
// @Failure 400 {object} apierrors.DefinedError "Ошибка валидации данных запроса, например, некорректный email"
// @Failure 403 {object} apierrors.DefinedError "Ошибка: доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Ошибка: рабочее пространство не найдено"
// @Failure 409 {object} apierrors.DefinedError "Ошибка: пользователь уже является участником рабочего пространства"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/invite [post]
func (s *Services) addToWorkspace(c echo.Context) error {
	issuer := *c.(WorkspaceContext).User
	workspace := c.(WorkspaceContext).Workspace

	var req requestMembersInvite

	if err := c.Bind(&req); err != nil {
		return EError(c, err)
	}

	type memberTracker struct {
		pm   dao.ProjectMember
		data map[string]interface{}
	}

	var createMemberLog []memberTracker

	for _, invite := range req.Emails {
		invite.Email = strings.TrimSpace(invite.Email)
		if !ValidateEmail(invite.Email) {
			return EErrorDefined(c, apierrors.ErrInvalidEmail.WithFormattedMessage(invite.Email))
		}

		if !IsValidRole(invite.Role) {
			return EErrorDefined(c, apierrors.ErrUnsupportedRole.WithFormattedMessage(invite.Role))
		}

		var user dao.User
		var workspaceMember dao.WorkspaceMember
		if err := s.db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("email = ?", invite.Email).First(&user).Error; err != nil {
				if err == gorm.ErrRecordNotFound {
					// Create new user
					pass := dao.GenPassword()
					user = dao.User{
						ID:              dao.GenID(),
						Email:           invite.Email,
						Password:        dao.GenPasswordHash(pass),
						CreatedByID:     &issuer.ID,
						Theme:           types.DefaultTheme,
						IsActive:        true,
						LastWorkspaceId: &workspace.ID,
					}

					if err := tx.Create(&user).Error; err != nil {
						return err
					}

					if err := s.emailService.NewUserPasswordNotify(user, pass); err != nil {
						return err
					}
				} else {
					return err
				}
			}
			var existingMember dao.WorkspaceMember
			if err := tx.Where("member_id = ? AND workspace_id = ?", user.ID, workspace.ID).First(&existingMember).Error; err == nil {
				return apierrors.ErrInviteMemberExist
			}

			workspaceMember = dao.WorkspaceMember{
				ID:          dao.GenID(),
				WorkspaceId: workspace.ID,
				MemberId:    user.ID,
				Role:        invite.Role,
				CreatedById: &issuer.ID,
				Member:      &user,
				Workspace:   &workspace,
				CreatedBy:   &issuer,
			}
			if err := tx.Omit(clause.Associations).Create(&workspaceMember).Error; err != nil {
				if err == gorm.ErrDuplicatedKey {
					return nil
				}
				return err
			}

			if workspaceMember.Role == types.AdminRole {
				var projects []dao.Project
				if err := tx.Where("workspace_id = ?", workspace.ID).Find(&projects).Error; err != nil {
					return err
				}

				for _, project := range projects {
					projectMember := dao.ProjectMember{
						ID:          dao.GenID(),
						CreatedAt:   time.Now(),
						CreatedById: &user.ID,
						WorkspaceId: workspace.ID,
						ProjectId:   project.ID,
						Role:        types.AdminRole,
						MemberId:    workspaceMember.MemberId,
						Member:      &user,
					}
					if err := tx.Clauses(clause.OnConflict{
						Columns:   []clause.Column{{Name: "project_id"}, {Name: "member_id"}},
						DoUpdates: clause.Assignments(map[string]interface{}{"role": types.AdminRole, "updated_at": time.Now(), "updated_by_id": user.ID}),
					}).Create(&projectMember).Error; err != nil {
						return err
					}

					newMemberMap := StructToJSONMap(projectMember)

					newMemberMap["updateScopeId"] = projectMember.MemberId
					newMemberMap["member_activity_val"] = projectMember.Role

					createMemberLog = append(createMemberLog, memberTracker{projectMember, newMemberMap})
				}
			}

			s.notificationsService.Tg.WorkspaceInvitation(workspaceMember)
			go s.emailService.WorkspaceInvitation(workspaceMember) // TODO в пул воркеров на отправку

			return nil
		}); err != nil {
			return EError(c, err)
		}

		for _, m := range createMemberLog {
			err := tracker.TrackActivity[dao.ProjectMember, dao.ProjectActivity](s.tracker, tracker.ENTITY_ADD_ACTIVITY, m.data, nil, m.pm, &issuer)
			if err != nil {
				errStack.GetError(c, err)
			}
		}

		//newMemberMap := StructToJSONMap(workspaceMember)
		//if err := s.tracker.TrackActivity(tracker.WORKSPACE_MEMBER_ADDED_ACTIVITY, newMemberMap, nil, workspace.ID, tracker.ENTITY_TYPE_WORKSPACE, nil, issuer); err != nil {
		//	return EError(c, err)
		//}

		data := map[string]interface{}{
			"updateScopeId":       workspaceMember.MemberId,
			"member_activity_val": workspaceMember.Role,
		}

		err := tracker.TrackActivity[dao.WorkspaceMember, dao.WorkspaceActivity](s.tracker, tracker.ENTITY_ADD_ACTIVITY, data, nil, workspaceMember, &issuer)
		if err != nil {
			errStack.GetError(c, err)
		}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Emails sent successfully",
	})
}

// getUserWorkspaceList godoc
// @id getUserWorkspaceList
// @Summary Пространство: получение рабочих пространств пользователя
// @Description Возвращает список рабочих пространств, в которых состоит текущий пользователь, с возможностью поиска по имени.
// @Tags Workspace
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param search_query query string false "Поисковый запрос для фильтрации рабочих пространств по имени"
// @Success 200 {array} dto.WorkspaceWithCount "Список рабочих пространств с количеством участников и проектов"
// @Failure 400 {object} apierrors.DefinedError "Ошибка валидации запроса"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/users/me/workspaces/ [get]
func (s *Services) getUserWorkspaceList(c echo.Context) error {
	user := *c.(AuthContext).User
	searchQuery := ""

	if err := echo.QueryParamsBinder(c).
		String("search_query", &searchQuery).
		BindError(); err != nil {
		return EError(c, err)
	}

	var workspaces []dao.WorkspaceWithCount
	query := s.db.Model(&dao.Workspace{}).
		Select("*,(?) as total_members,(?) as total_projects,(?) as is_favorite",
			s.db.Model(&dao.WorkspaceMember{}).Select("count(*)").Where("workspace_id = workspaces.id"),
			s.db.Model(&dao.Project{}).Select("count(*)").Where("workspace_id = workspaces.id"),
			s.db.Model(&dao.WorkspaceFavorites{}).Select("count(*) > 0").Where("workspace_favorites.workspace_id = workspaces.id").Where("user_id = ?", user.ID),
		).
		Preload("Owner").
		Set("userID", user.ID).
		Order("is_favorite desc, lower(name)")

	if searchQuery != "" {
		escapedSearchQuery := PrepareSearchRequest(searchQuery)
		query = query.Where("lower(name) LIKE ? OR name_tokens @@ plainto_tsquery('russian', lower(?))",
			escapedSearchQuery, searchQuery)
	}

	if err := query.
		Where("workspaces.id in (?)", s.db.Model(&dao.WorkspaceMember{}).
			Select("workspace_id").
			Where("member_id = ?", user.ID)).
		Find(&workspaces).Error; err != nil {
		return EError(c, err)
	}
	return c.JSON(http.StatusOK, utils.SliceToSlice(&workspaces, func(w *dao.WorkspaceWithCount) dto.WorkspaceWithCount { return *w.ToDTO() }))
}

// getProductUpdateList godoc
// @id getProductUpdateList
// @Summary Релизы: получение списка обновлений
// @Description Возвращает список обновлений
// @Tags ReleaseNotes
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {array} dto.ReleaseNoteLight "Список обновлений"
// @Failure 500 {object} apierrors.DefinedError "Ошибка при получении обновлений"
// @Router /api/auth/release-notes/ [get]
func (s *Services) getProductUpdateList(c echo.Context) error {
	var notes []dao.ReleaseNote
	if err := s.db.Preload("Author").Order("published_at DESC").Find(&notes).Error; err != nil {
		return EError(c, err)
	}

	return c.JSON(http.StatusOK, utils.SliceToSlice(&notes, func(n *dao.ReleaseNote) dto.ReleaseNoteLight { return *n.ToLightDTO() }))
}

// createWorkspace godoc
// @id createWorkspace
// @Summary Пространство: создание нового пространства
// @Description Создает новое рабочее пространство с заданными параметрами.
// @Tags Workspace
// @Produce json
// @Security ApiKeyAuth
// @Param request body CreateWorkspaceRequest true "Информация о новом рабочем пространстве"
// @Success 201 {object} dto.Workspace "Созданное рабочее пространство"
// @Failure 400 {object} apierrors.DefinedError "Ошибка: неверные параметры запроса"
// @Failure 409 {object} apierrors.DefinedError "Ошибка: конфликт с существующим рабочим пространством"
// @Router /api/auth/workspaces/ [post]
func (s *Services) createWorkspace(c echo.Context) error {
	user := *c.(AuthContext).User

	// Tariffication
	if user.Tariffication != nil {
		var count int64
		if err := s.db.Model(&dao.Workspace{}).
			Where("created_by_id = ? or owner_id = ?", user.ID, user.ID).
			Or("id in (?)", s.db.Select("workspace_id").Where("member_id = ?", user.ID).Model(&dao.WorkspaceMember{})).
			Count(&count).Error; err != nil {
			return EError(c, err)
		}

		if count > int64(user.Tariffication.WorkspacesLimit) {
			return EErrorDefined(c, apierrors.ErrWorkspaceLimitExceed)
		}
	}

	var workspace dao.Workspace
	var req CreateWorkspaceRequest
	if err := c.Bind(&req); err != nil {
		return EError(c, err)
	}
	if req.Name == "" {
		return EErrorDefined(c, apierrors.ErrWorkspaceNameRequired)
	}
	if !CheckWorkspaceSlug(req.Slug) {
		return EErrorDefined(c, apierrors.ErrForbiddenSlug)
	}

	req.Name = strings.TrimSpace(req.Name)

	err := c.Validate(req)
	if err != nil {
		return EError(c, err)
	}

	req.Bind(&workspace)
	workspace.ID = dao.GenID()
	workspace.OwnerId = user.ID
	workspace.CreatedById = user.ID
	workspace.IntegrationToken = password.MustGenerate(64, 30, 0, false, true)

	if err := s.db.Create(&workspace).Error; err != nil {
		if err == gorm.ErrDuplicatedKey {
			return EErrorDefined(c, apierrors.ErrWorkspaceSlugConflict)
		}
		return EError(c, err)
	}

	workspaceMember := dao.WorkspaceMember{
		ID:          dao.GenID(),
		WorkspaceId: workspace.ID,
		MemberId:    user.ID,
		CreatedById: &user.ID,
		Role:        15,
	}
	if err := s.db.Create(&workspaceMember).Error; err != nil {
		return EError(c, err)
	}

	err = tracker.TrackActivity[dao.Workspace, dao.RootActivity](s.tracker, tracker.ENTITY_CREATE_ACTIVITY, nil, nil, workspace, &user)
	if err != nil {
		errStack.GetError(c, err)
	}

	return c.JSON(http.StatusCreated, workspace.ToDTO())
}

// ############# Last Visited Workspace methods ###################

// getLastVisitedWorkspace godoc
// @id getLastVisitedWorkspace
// @Summary Пространство: получение последнего посещенного рабочего пространства
// @Description Возвращает информацию о последнем посещенном рабочем пространстве пользователя. Если ID последнего рабочего пространства отсутствует, возвращает пустые данные.
// @Tags Workspace
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} responseLastWorkspace "Детали последнего рабочего пространства и проекта"
// @Failure 500 {object} apierrors.DefinedError "Внутренняя ошибка сервера"
// @Router /api/auth/users/last-visited-workspace [get]
func (s *Services) getLastVisitedWorkspace(c echo.Context) error {
	user := *c.(AuthContext).User

	if user.LastWorkspaceId == nil {
		return c.JSON(http.StatusOK, responseLastWorkspace{
			WorkspaceDetails: make([]interface{}, 0),
			ProjectDetails:   struct{}{},
		})
	}

	var workspace dao.Workspace
	if err := s.db.Where("id = ?", user.LastWorkspaceId).Find(&workspace).Error; err != nil {
		return EError(c, err)
	}

	var projectMember []dao.ProjectMember
	if err := s.db.Preload("Workspace").
		Preload("Workspace.Owner").
		Preload("Project").
		Preload("Member").
		Where("workspace_id = ?", workspace.ID).
		Where("member_id = ?", user.ID).
		Find(&projectMember).Error; err != nil {
		return EError(c, err)
	}

	return c.JSON(http.StatusOK, responseLastWorkspace{
		WorkspaceDetails: workspace.ToLightDTO(),
		ProjectDetails:   utils.SliceToSlice(&projectMember, func(pm *dao.ProjectMember) dto.ProjectMember { return *pm.ToDTO() }),
	})
}

// ############# Workspace Views methods ###################

// Создание просмотра рабочего пространства
func (s *Services) createWorkspaceView(c echo.Context) error {
	user := *c.(AuthContext).User
	slug := c.Param("workspaceSlug")

	var data map[string]json.RawMessage
	if err := json.NewDecoder(c.Request().Body).Decode(&data); err != nil {
		return EError(c, err)
	}

	var workspaceMember dao.WorkspaceMember
	if err := s.db.Where("member_id = ?", user.ID).
		Joins("Workspace").
		Where("slug = ?", slug).
		Find(&workspaceMember).Error; err != nil {
		return EError(c, err)
	}

	if err := s.db.Save(&workspaceMember).Error; err != nil {
		return EError(c, err)
	}

	return c.NoContent(http.StatusOK)
}

// getWorkspaceToken godoc
// @id getWorkspaceToken
// @Summary Пространство (токен): получение токена для пространства
// @Description Возвращает токен интеграции для указанного рабочего пространства, если пользователь имеет необходимые права доступа.
// @Tags Workspace
// @Produce plain
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Success 200 {string} string "Токен интеграции рабочего пространства"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Рабочее пространство не найдено"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/token [get]
func (s *Services) getWorkspaceToken(c echo.Context) error {
	user := c.(WorkspaceContext).User
	workspace := c.(WorkspaceContext).Workspace
	workspaceMember := c.(WorkspaceContext).WorkspaceMember

	if !user.IsSuperuser && workspaceMember.Role != types.AdminRole && workspace.OwnerId != workspaceMember.MemberId {
		return c.NoContent(http.StatusForbidden)
	}
	return c.String(http.StatusOK, workspace.IntegrationToken)
}

// resetWorkspaceToken godoc
// @id resetWorkspaceToken
// @Summary Пространство (токен): сброс токена для пространства
// @Description Генерирует новый токен интеграции для указанного рабочего пространства.
// @Tags Workspace
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Success 201 {string} string "Токен интеграции успешно сброшен"
// @Failure 400 {object} apierrors.DefinedError "Ошибка в запросе"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Рабочее пространство не найдено"
// @Failure 500 {object} apierrors.DefinedError "Внутренняя ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/token/reset/ [post]
func (s *Services) resetWorkspaceToken(c echo.Context) error {
	user := c.(WorkspaceContext).User
	workspace := c.(WorkspaceContext).Workspace
	//TODO check it
	//oldToken := map[string]interface{}{
	//	"integration_token": workspace.IntegrationToken,
	//}

	newToken := map[string]interface{}{
		"integration_token": password.MustGenerate(64, 30, 0, false, true),
	}

	if err := s.db.Model(&workspace).UpdateColumn("integration_token", newToken["integration_token"]).Error; err != nil {
		return EError(c, err)
	}

	//Трекинг активности
	oldMap := map[string]interface{}{
		"integration_token": "",
	}
	newMap := map[string]interface{}{
		"integration_token": "******",
	}

	err := tracker.TrackActivity[dao.Workspace, dao.WorkspaceActivity](s.tracker, tracker.ENTITY_UPDATED_ACTIVITY, newMap, oldMap, workspace, user)
	if err != nil {
		errStack.GetError(c, err)
	}

	//if err := s.tracker.TrackActivity(tracker.WORKSPACE_TOKEN_UPDATED_ACTIVITY, newToken, oldToken, workspace.ID, tracker.ENTITY_TYPE_WORKSPACE, nil, *user); err != nil {
	//	return EError(c, err)
	//}

	return c.NoContent(http.StatusCreated)
}

// getWorkspaceStateList godoc
// @id getWorkspaceStateList
// @Summary Пространство: получение состояния рабочего пространства
// @Description Возвращает список состояний, сгруппированных по проектам, для указанного рабочего пространства.
// @Tags Workspace
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Success 200 {object} map[string][]dto.StateLight "Список состояний, сгруппированных по проектам"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Рабочее пространство не найдено"
// @Failure 500 {object} apierrors.DefinedError "Внутренняя ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/states/ [get]
func (s *Services) getWorkspaceStateList(c echo.Context) error {
	workspace := c.(WorkspaceContext).Workspace

	if etag := c.Request().Header.Get("If-None-Match"); etag != "" {
		etagHash, err := hex.DecodeString(etag)
		if err != nil {
			return EError(c, err)
		}

		var state dao.State
		if err := s.db.Model(&dao.State{}).Select("digest(string_agg(hash, '' order by sequence), 'sha256') as hash").Where("workspace_id = ?", workspace.ID).Find(&state).Error; err != nil {
			return EError(c, err)
		}

		if bytes.Equal(etagHash, state.Hash) {
			return c.NoContent(http.StatusNotModified)
		}
	}

	var states []dao.State
	if err := s.db.
		Preload(clause.Associations).
		Order("sequence").
		Where("workspace_id = ?", workspace.ID).
		Find(&states).Error; err != nil {
		return EError(c, err)
	}

	result := make(map[string][]dto.StateLight)
	hash := sha256.New()
	for _, state := range states {
		arr, ok := result[state.ProjectId]
		if !ok {
			arr = make([]dto.StateLight, 0)
		}
		arr = append(arr, *state.ToLightDTO())
		result[state.ProjectId] = arr
		hash.Write(state.Hash)
	}
	c.Response().Header().Add("ETag", hex.EncodeToString(hash.Sum(nil)))
	return c.JSON(http.StatusOK, result)
}

// getWorkspaceJitsiToken godoc
// @id getWorkspaceJitsiToken
// @Summary Пространство: получение токена для Jitsi-комнаты рабочего пространства
// @Description Генерирует и возвращает JWT-токен для доступа пользователя в комнату Jitsi, соответствующую рабочему пространству.
// @Tags Workspace
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Success 200 {object} map[string]string "JWT-токен для комнаты Jitsi"
// @Failure 403 {object} apierrors.DefinedError "Ошибка: доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Ошибка: рабочее пространство не найдено"
// @Failure 500 {object} apierrors.DefinedError "Внутренняя ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/jitsi-token [get]
func (s *Services) getWorkspaceJitsiToken(c echo.Context) error {
	user := c.(WorkspaceContext).User
	workspace := c.(WorkspaceContext).Workspace

	claims := jwt.MapClaims{
		"exp":  jwt.NewNumericDate(time.Now().Add(time.Hour * 24)),
		"iss":  "aiplan",
		"room": workspace.Slug,
		"context": map[string]interface{}{
			"user": map[string]interface{}{
				"avatar":    user.Avatar,
				"name":      user.GetName(),
				"email":     user.Email,
				"id":        user.ID,
				"moderator": false,
			},
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	ret, err := token.SignedString([]byte(cfg.JitsiSecretKey))
	if err != nil {
		return EError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]string{
		"jitsi_token": ret,
	})
}

// ############# User favorite workspaces methods ###################

// getFavoriteWorkspaceList godoc
// @id getFavoriteWorkspaceList
// @Summary Пространство (избранное): получение избранных пространств
// @Description Возвращает список избранных рабочих пространств с информацией о владельце.
// @Tags Workspace
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {array} dto.WorkspaceFavorites "Список избранных рабочих пространств"
// @Failure 500 {object} apierrors.DefinedError "Ошибка при получении избранных рабочих пространств"
// @Router /api/auth/users/user-favorite-workspaces/ [get]
func (s *Services) getFavoriteWorkspaceList(c echo.Context) error {
	user := *c.(AuthContext).User

	var favorites []dao.WorkspaceFavorites
	if err := s.db.Where("user_id = ?", user.ID).
		Preload("Workspace").
		Preload("Workspace.Owner").
		Set("userId", user.ID).
		Find(&favorites).Error; err != nil {
		return EError(c, err)
	}
	for i := range favorites {
		favorites[i].Workspace.IsFavorite = true
	}

	return c.JSON(http.StatusOK, utils.SliceToSlice(&favorites, func(wf *dao.WorkspaceFavorites) dto.WorkspaceFavorites { return *wf.ToDao() }))
}

// addWorkspaceToFavorites godoc
// @id addWorkspaceToFavorites
// @Summary Пространство (избранное): добавление пространства в избранное
// @Description Добавляет рабочее пространство в избранное для текущего пользователя.
// @Tags Workspace
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspace body requestAddFavorite false "ID рабочего пространства"
// @Success 200 {string} string "No Content"
// @Success 201 {object} dto.WorkspaceFavorites "Созданное избранное рабочее пространство"
// @Failure 400 {object} apierrors.DefinedError "Ошибка в запросе"
// @Failure 404 {object} apierrors.DefinedError "Рабочее пространство не найдено"
// @Failure 409 {object} apierrors.DefinedError "Рабочее пространство уже в избранном"
// @Router /api/auth/users/user-favorite-workspaces/ [post]
func (s *Services) addWorkspaceToFavorites(c echo.Context) error {
	user := *c.(AuthContext).User

	var req requestAddFavorite
	if err := c.Bind(&req); err != nil {
		return EError(c, err)
	}

	workspace, err := dao.GetWorkspaceByID(s.db, req.Workspace, user.ID)
	if err != nil {
		return EError(c, err)
	}

	workspaceFavorite := dao.WorkspaceFavorites{
		ID:          dao.GenID(),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		CreatedById: &user.ID,
		WorkspaceId: workspace.ID,
		Workspace:   &workspace,
		UserId:      user.ID,
	}
	if err := s.db.Create(&workspaceFavorite).Error; err != nil {
		if err == gorm.ErrDuplicatedKey {
			return c.NoContent(http.StatusOK)
		}
		return EError(c, err)
	}
	return c.JSON(http.StatusCreated, workspaceFavorite.ToDao())
}

// removeWorkspaceFromFavorites godoc
// @id removeWorkspaceFromFavorites
// @Summary Пространство (избранное): удаление пространства из избранного
// @Description Удаляет рабочее пространство из избранного для текущего пользователя по его ID.
// @Tags Workspace
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceID path string true "ID рабочего пространства"
// @Success 204 {string} string "No Content"
// @Failure 400 {object} apierrors.DefinedError "Ошибка в запросе"
// @Failure 404 {object} apierrors.DefinedError "Рабочее пространство не найдено"
// @Failure 500 {object} apierrors.DefinedError "Внутренняя ошибка сервера"
// @Router /api/auth/users/user-favorite-workspaces/{workspaceID} [delete]
func (s *Services) removeWorkspaceFromFavorites(c echo.Context) error {
	user := *c.(AuthContext).User
	workspaceID := c.Param("workspaceID")
	workspace, err := dao.GetWorkspaceByID(s.db, workspaceID, user.ID)

	if err != nil {
		return EError(c, err)
	}

	if err := s.db.Where("workspace_id = ?", workspace.ID).
		Where("user_id = ?", user.ID).
		Delete(&dao.WorkspaceFavorites{}).Error; err != nil {
		return EError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// getIntegrationList godoc
// @id getIntegrationList
// @Summary Пространство (интеграции): получение интеграций
// @Description получение интеграций
// @Tags Workspace
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Success 200 {array} integrations.Integration "интеграции"
// @Failure 400 {object} apierrors.DefinedError "Ошибка в запросе"
// @Failure 403 {object} apierrors.DefinedError "Ошибка: доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Ошибка: рабочее пространство не найдено"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/integrations/ [get]
func (s *Services) getIntegrationList(c echo.Context) error {
	workspace := c.(WorkspaceContext).Workspace

	return c.JSON(http.StatusOK, s.integrationsService.GetIntegrations(workspace.ID))
}

// addIntegrationToWorkspace godoc
// @id addIntegrationToWorkspace
// @Summary Пространство (интеграции): добавление интеграции
// @Description добавление интеграции в пространство
// @Tags Workspace
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param name path string true "имя интеграции"
// @Success 201  "ok"
// @Failure 400 {object} apierrors.DefinedError "Ошибка в запросе"
// @Failure 403 {object} apierrors.DefinedError "Ошибка: доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Ошибка: рабочее пространство не найдено"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/integrations/add/{name}/ [post]
func (s *Services) addIntegrationToWorkspace(c echo.Context) error {
	workspace := c.(WorkspaceContext).Workspace
	user := c.(WorkspaceContext).User
	name := c.Param("name")

	if c.(WorkspaceContext).WorkspaceMember.Role != types.AdminRole {
		return EErrorDefined(c, apierrors.ErrNotEnoughRights)
	}

	integration := s.integrationsService.GetIntegrationUser(name)
	if integration == nil {
		return EErrorDefined(c, apierrors.ErrIntegrationNotFound)
	}

	workspaceMember := dao.WorkspaceMember{
		ID:          dao.GenID(),
		WorkspaceId: workspace.ID,
		MemberId:    integration.ID,
		Role:        types.MemberRole,
		CreatedById: &user.ID,
		Member:      integration,
	}
	if err := s.db.Save(&workspaceMember).Error; err != nil {
		return EError(c, err)
	}

	data := map[string]interface{}{
		"member_key":               "integration",
		"integration_activity_val": name,
	}

	err := tracker.TrackActivity[dao.WorkspaceMember, dao.WorkspaceActivity](s.tracker, tracker.ENTITY_ADD_ACTIVITY, data, nil, workspaceMember, user)
	if err != nil {
		errStack.GetError(c, err)
	}

	return c.NoContent(http.StatusCreated)
}

// deleteIntegrationFromWorkspace godoc
// @id deleteIntegrationFromWorkspace
// @Summary Пространство (интеграции): удаление интеграции
// @Description удаление интеграции из пространства
// @Tags Workspace
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param name path string true "имя интеграции"
// @Success 200  "ok"
// @Failure 400 {object} apierrors.DefinedError "Ошибка в запросе"
// @Failure 403 {object} apierrors.DefinedError "Ошибка: доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Ошибка: рабочее пространство не найдено"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/integrations/{name}/ [post]
func (s *Services) deleteIntegrationFromWorkspace(c echo.Context) error {
	workspace := c.(WorkspaceContext).Workspace
	name := c.Param("name")

	if c.(WorkspaceContext).WorkspaceMember.Role != types.AdminRole {
		return EErrorDefined(c, apierrors.ErrNotEnoughRights)
	}

	integration := s.integrationsService.GetIntegrationUser(name)
	if integration == nil {
		return EErrorDefined(c, apierrors.ErrIntegrationNotFound)
	}

	data := map[string]interface{}{
		"member_key":               "integration",
		"integration_activity_val": name,
	}

	var wm dao.WorkspaceMember
	if err := s.db.Joins("Member").Where("workspace_id = ? and member_id = ?", workspace.ID, integration.ID).First(&wm).Error; err != nil {
		return EError(c, err)
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		err := tracker.TrackActivity[dao.WorkspaceMember, dao.WorkspaceActivity](s.tracker, tracker.ENTITY_REMOVE_ACTIVITY, data, nil, wm, c.(WorkspaceContext).User)
		if err != nil {
			errStack.GetError(c, err)
			return err
		}

		return s.db.Session(&gorm.Session{SkipHooks: true}).Where("workspace_id = ? and member_id = ?", workspace.ID, integration.ID).Delete(&dao.WorkspaceMember{}).Error
	}); err != nil {
		return EError(c, err)
	}

	return c.NoContent(http.StatusOK)
}

// ******* RESPONSE *******

type responseLastWorkspace struct {
	WorkspaceDetails interface{} `json:"workspace_details"`
	ProjectDetails   interface{} `json:"project_details"`
}

//***** REQUEST ******

type requestRoleMember struct {
	Role *int `json:"role"`
}

type requestEmailMember struct {
	Email string `json:"email"`
}

type requestMembersInvite struct {
	Emails []struct {
		Email string `json:"email"`
		Role  int    `json:"role"`
	} `json:"emails"`
}

type requestAddFavorite struct {
	Workspace string `json:"workspace"`
}

type requestMessage struct {
	Title   string    `json:"title"`
	Msg     string    `json:"msg"`
	SendAt  time.Time `json:"send_at,omitempty"`
	Members []string  `json:"members,omitempty"`
}
