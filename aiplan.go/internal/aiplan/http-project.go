// Пакет предоставляет функциональность для управления тегами задач в системе планирования.  Включает создание, обновление, удаление и получение списков тегов, а также их связывание с проектами и пользователями.
//
// Основные возможности:
//   - Создание, обновление и удаление тегов задач.
//   - Получение списка тегов задач для проекта.
//   - Связывание тегов задач с проектами и пользователями.
//   - Поддержка пагинации при получении списков тегов.
//   - Валидация данных тегов при создании и обновлении.
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
	"sheff.online/aiplan/internal/aiplan/notifications"
	"sheff.online/aiplan/internal/aiplan/types"
	"sheff.online/aiplan/internal/aiplan/utils"

	"github.com/gofrs/uuid"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	tracker "sheff.online/aiplan/internal/aiplan/activity-tracker"
	"sheff.online/aiplan/internal/aiplan/dao"
	filestorage "sheff.online/aiplan/internal/aiplan/file-storage"
)

type ProjectContext struct {
	WorkspaceContext
	Project       dao.Project
	ProjectMember dao.ProjectMember
}

func (s *Services) ProjectMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		projectId := c.Param("projectId")
		workspace := c.(WorkspaceContext).Workspace
		user := c.(WorkspaceContext).User

		if etag := c.Request().Header.Get("If-None-Match"); etag != "" {
			var exist bool
			if err := s.db.Model(&dao.Project{}).Select("count(*) > 0").Where("encode(hash, 'hex') = ?", etag).Find(&exist).Error; err != nil {
				return EError(c, err)
			}

			if exist {
				return c.NoContent(http.StatusNotModified)
			}
		}

		// Joins faster than Preload(clause.Associations)
		var project dao.Project
		if err := s.db.
			Joins("Workspace").
			Joins("ProjectLead").
			Where("projects.workspace_id = ?", workspace.ID).
			Where("projects.id = ? OR projects.identifier = ?", projectId, projectId). // Search by id or identifier
			Set("userId", user.ID).
			Preload("DefaultAssigneesDetails", "is_default_assignee = ?", true).
			Preload("DefaultWatchersDetails", "is_default_watcher = ?", true).
			First(&project).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return EErrorDefined(c, apierrors.ErrProjectNotFound)
			}
			return EError(c, err)
		}

		if project.CurrentUserMembership == nil {
			return EErrorDefined(c, apierrors.ErrProjectNotFound)
		}

		return next(ProjectContext{c.(WorkspaceContext), project, *project.CurrentUserMembership})
	}
}

func (s *Services) AddProjectServices(g *echo.Group) {
	workspaceGroup := g.Group("workspaces/:workspaceSlug", s.WorkspaceMiddleware)
	workspaceGroup.Use(s.LastVisitedWorkspaceMiddleware)

	projectGroup := workspaceGroup.Group("/projects/:projectId", s.ProjectMiddleware)
	projectGroup.Use(s.ProjectPermissionMiddleware)

	workspaceGroup.Use(s.WorkspacePermissionMiddleware)

	projectAdminGroup := projectGroup.Group("", s.ProjectAdminPermissionMiddleware)

	// ../front/services/project.service.ts
	workspaceGroup.GET("/projects/", s.getProjectList)
	workspaceGroup.POST("/projects/", s.createProject)

	projectGroup.GET("/", s.getProject)
	projectGroup.PATCH("/", s.updateProject)
	projectGroup.DELETE("/", s.deleteProject)

	projectGroup.GET("/activities/", s.getProjectActivityList)

	workspaceGroup.GET("/project-identifiers/", s.checkProjectIdentifierAvailability)

	projectGroup.GET("/members/", s.getProjectMemberList)
	projectGroup.GET("/members/:memberId/", s.getProjectMember)
	projectGroup.PATCH("/members/:memberId/", s.updateProjectMember)
	projectGroup.DELETE("/members/:memberId/", s.deleteProjectMember)
	projectGroup.POST("/members/add/", s.addMemberToProject)

	projectGroup.POST("/me/notifications/", s.updateMyNotifications)

	workspaceGroup.POST("/projects/join/", s.joinProjects)

	projectGroup.POST("/project-views/", s.updateProjectView)

	projectGroup.GET("/project-members/me/", s.getProjectMemberMe)

	workspaceGroup.GET("/user-favorite-projects/", s.getFavoriteProjects)
	workspaceGroup.POST("/user-favorite-projects/", s.addProjectToFavorites)
	workspaceGroup.DELETE("/user-favorite-projects/:projectId/", s.removeProjectFromFavorites)

	projectGroup.GET("/project-estimates/", s.getProjectEstimatePointsList)

	projectGroup.GET("/estimates/", s.getProjectEstimatesList)
	projectGroup.POST("/estimates/", s.createProjectEstimate)

	projectGroup.GET("/estimates/:estimateId/", s.getProjectEstimate)
	projectGroup.PATCH("/estimates/:estimateId/", s.updateProjectEstimate)
	projectGroup.DELETE("/estimates/:estimateId/", s.deleteProjectEstimate)

	projectGroup.POST("/issues/", s.createIssue)
	projectGroup.POST("/issues/search/", s.getIssueList)

	// Labels
	projectGroup.GET("/issue-labels/", s.getIssueLabelList)
	projectGroup.POST("/issue-labels/", s.createIssueLabel)
	projectGroup.GET("/issue-labels/:labelId/", s.getIssueLabel)
	projectGroup.PATCH("/issue-labels/:labelId/", s.updateIssueLabel)
	projectGroup.DELETE("/issue-labels/:labelId/", s.deleteIssueLabel)

	projectGroup.DELETE("/bulk-delete-issues/", s.deleteIssuesBulk)

	// States
	projectGroup.GET("/states/", s.getStateList)
	projectGroup.POST("/states/", s.createState)
	projectGroup.GET("/states/:stateId/", s.getState)
	projectGroup.PATCH("/states/:stateId/", s.updateState)
	projectGroup.DELETE("/states/:stateId/", s.deleteState)

	projectGroup.POST("/rules-log/", s.getRulesLog)

	// Issue Templates
	projectGroup.GET("/templates/", s.getProjectIssueTemplates)
	projectAdminGroup.POST("/templates/", s.createIssueTemplate)
	projectGroup.GET("/templates/:templateId/", s.getIssueTemplate)
	projectAdminGroup.PATCH("/templates/:templateId/", s.updateIssueTemplate)
	projectAdminGroup.DELETE("/templates/:templateId/", s.deleteIssueTemplate)

	/* Not in use
	projectGroup.GET("/issue-properties/", s.issuePropertiesList)
	projectGroup.POST("/issue-properties/", s.issuePropertyCreateOrUpdate)
	projectGroup.GET("/issue-properties/:propertyId/", s.issuePropertiesList)
	projectGroup.PATCH("/issue-properties/:propertyId/", s.issuePropertyCreateOrUpdate)
	projectGroup.DELETE("/issue-properties/:propertyId/", s.issuePropertyDelete)
	*/
}

// getProjectList godoc
// @id getProjectList
// @Summary Проекты: получение списка проектов
// @Description Возвращает список всех проектов в рабочем пространстве, к которым у пользователя есть доступ.
// Список можно отфильтровать по названию проекта.
// @Tags Projects
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param search_query query string false "Поисковый запрос для фильтрации проектов по названию"
// @Success 200 {array} dto.ProjectLight "Список проектов с информацией о количестве участников и статусе избранного"
// @Failure 400 {object} apierrors.DefinedError "Ошибка запроса"
// @Router /api/auth/workspaces/{workspaceSlug}/projects [get]
func (s *Services) getProjectList(c echo.Context) error {
	user := c.(WorkspaceContext).User
	workspace := c.(WorkspaceContext).Workspace
	workspaceMember := c.(WorkspaceContext).WorkspaceMember

	searchQuery := ""
	if err := echo.QueryParamsBinder(c).
		String("search_query", &searchQuery).
		BindError(); err != nil {
		return EError(c, err)
	}

	var projects []dao.ProjectWithCount
	query := s.db.
		Preload("DefaultAssigneesDetails", "is_default_assignee = ?", true).
		Preload("DefaultWatchersDetails", "is_default_watcher = ?", true).
		Preload("Workspace").
		Preload("Workspace.Owner").
		Preload("ProjectLead").
		Select("*,(?) as total_members, (?) as is_favorite",
			s.db.Model(&dao.ProjectMember{}).Select("count(*)").Where("project_members.project_id = projects.id"),
			s.db.Model(&dao.ProjectFavorites{}).Select("count(*) > 0").Where("project_favorites.project_id = projects.id").Where("user_id = ?", user.ID),
		).
		Set("userId", user.ID). // Check if project favorite for this user and get memberships
		Where("workspace_id = ?", workspace.ID).
		Order("is_favorite desc, lower(name)")

	if searchQuery != "" {
		escapedSearchQuery := PrepareSearchRequest(searchQuery)
		query = query.Where(
			"lower(name) LIKE ? OR name_tokens @@ plainto_tsquery('russian', lower(?))",
			escapedSearchQuery, searchQuery)
	}

	if workspaceMember.Role != types.AdminRole && !user.IsSuperuser {
		query = query.Where("id in (?) or public = true", s.db.Model(&dao.ProjectMember{}).Select("project_id").Where("member_id = ?", user.ID))
	}

	if err := query.Find(&projects).Error; err != nil {
		return EError(c, err)
	}
	return c.JSON(
		http.StatusOK,
		utils.SliceToSlice(&projects, func(p *dao.ProjectWithCount) dto.ProjectLight { return *p.ToLightDTO() }))
}

var allowedFields []string = []string{"name", "description", "description_text", "description_html", "public", "identifier", "default_assignees", "default_watchers", "project_lead_id", "emoji", "cover_image", "rules_script"}

// updateProject godoc
// @id updateProject
// @Summary Проекты: изменение проекта
// @Description Обновляет информацию о проекте, включая название, ответственного и списки наблюдателей и исполнителей.
// @Tags Projects
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param project body dto.Project true "Данные проекта для обновления"
// @Success 200 {object} dto.Project "Информация о проекте после обновления"
// @Failure 400 {object} apierrors.DefinedError "Ошибка запроса или валидации данных"
// @Failure 403 {object} apierrors.DefinedError "Недостаточно прав для выполнения операции"
// @Failure 404 {object} apierrors.DefinedError "Администратор проекта не найден"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId} [patch]
func (s *Services) updateProject(c echo.Context) error {
	user := c.(ProjectContext).User
	project := c.(ProjectContext).Project

	// Pre-update activity tracking
	oldProjectMap := StructToJSONMap(project)

	oldLead := project.ProjectLeadId
	id := project.ID
	if err := c.Bind(&project); err != nil {
		return EError(c, err)
	}
	var newLead dao.ProjectMember

	project.ID = id
	project.UpdatedById = &user.ID
	project.Name = strings.TrimSpace(project.Name)

	err := c.Validate(project)
	if err != nil {
		return EError(c, err)
	}

	changeProjectLead := oldLead != project.ProjectLeadId

	// Check new owner id exists and admin
	if changeProjectLead {
		if err := s.db.
			Joins("Member").
			Where("project_id = ?", project.ID).
			Where("member_id = ?", project.ProjectLeadId).
			Where("project_members.role = 15").First(&newLead).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return EErrorDefined(c, apierrors.ErrProjectAdminNotFound)
			}
			return EError(c, err)
		}
		oldProjectMap["project_lead_activity_val"] = project.ProjectLead.Email
	}

	if !user.IsSuperuser && user.ID != oldLead && changeProjectLead {
		return EErrorDefined(c, apierrors.ErrChangeProjectLeadForbidden)
	}

	if project.RulesScript != nil && *project.RulesScript != oldProjectMap["rules_script"] {
		*project.RulesScript = strings.ReplaceAll(*project.RulesScript, "\u00A0", " ")
	}
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&dao.ProjectMember{}).
			Where("project_id = ?", project.ID).
			Updates(map[string]interface{}{
				"is_default_assignee": gorm.Expr("(member_id in (?))", project.DefaultAssignees),
				"is_default_watcher":  gorm.Expr("(member_id in (?))", project.DefaultWatchers),
			}).Error; err != nil {
			return err
		}

		if err := tx.Select(allowedFields).Updates(&project).Error; err != nil {
			return err
		}

		if err := tx.Preload("DefaultAssigneesDetails", "is_default_assignee = ?", true).
			Preload("DefaultWatchersDetails", "is_default_watcher = ?", true).
			Preload("DefaultAssigneesDetails.Member").
			Preload("DefaultWatchersDetails.Member").
			Where("id = ?", project.ID).
			First(&project).Error; err != nil {
			return err
		}

		return nil
	}); err != nil {
		return EError(c, err)
	}
	// Post-update activity tracking
	newProjectMap := StructToJSONMap(project)
	if changeProjectLead {
		newProjectMap["project_lead_activity_val"] = newLead.Member.Email
	}

	//s.tracker.TrackActivity(tracker.PROJECT_UPDATED_ACTIVITY, newProjectMap, oldProjectMap, project.ID, tracker.ENTITY_TYPE_PROJECT, &project, *user)

	err = tracker.TrackActivity[dao.Project, dao.ProjectActivity](s.tracker, tracker.ENTITY_UPDATED_ACTIVITY, newProjectMap, oldProjectMap, project, user)
	if err != nil {
		errStack.GetError(c, err)
	}

	return c.JSON(http.StatusOK, project.ToDTO())
}

// getProject godoc
// @id getProject
// @Summary Проекты: получение проекта
// @Description Возвращает информацию о проекте по его идентификатору.
// @Tags Projects
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "Идентификатор проекта"
// @Success 200 {object} dto.Project "Информация о проекте"
// @Failure 404 {object} apierrors.DefinedError "Проект не найден"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId} [get]
func (s *Services) getProject(c echo.Context) error {
	project := c.(ProjectContext).Project
	c.Response().Header().Add("ETag", hex.EncodeToString(project.Hash))
	return c.JSON(http.StatusOK, project.ToDTO())
}

// deleteProject godoc
// @id deleteProject
// @Summary Проекты: удаление проекта
// @Description Удаляет проект, если у пользователя есть соответствующие права.
// @Tags Projects
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Success 200 "Проект успешно удален"
// @Failure 403 {object} apierrors.DefinedError "Нет прав на удаление проекта"
// @Failure 404 {object} apierrors.DefinedError "Проект не найден"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId} [delete]
func (s *Services) deleteProject(c echo.Context) error {
	user := c.(ProjectContext).User
	project := c.(ProjectContext).Project
	projectMember := c.(ProjectContext).ProjectMember
	workspace := c.(ProjectContext).Workspace
	workspaceMember := c.(ProjectContext).WorkspaceMember

	s.business.ProjectCtx(c, user, &project, &projectMember, &workspace, &workspaceMember)
	defer s.business.ProjectCtxClean()

	if err := s.business.DeleteProject(); err != nil {
		return EError(c, err)
	}

	//currentInst := StructToJSONMap(project)
	//if !user.IsSuperuser && user.ID != project.ProjectLeadId && workspaceMember.Role != types.AdminRole {
	//	return EErrorDefined(c, apierrors.ErrDeleteProjectForbidden)
	//}
	//
	////if err := s.tracker.TrackActivity(tracker.PROJECT_DELETED_ACTIVITY, nil, currentInst, project.ID, tracker.ENTITY_TYPE_PROJECT, &project, *user); err != nil {
	////	return EError(c, err)
	////}
	//
	//err := tracker.TrackActivity[dao.Project, dao.WorkspaceActivity](s.tracker, tracker.ENTITY_DELETE_ACTIVITY, nil, nil, project, user)
	//if err != nil {
	//	errStack.GetError(c, err)
	//}
	//
	//// Soft-delete project
	//if err := s.db.Session(&gorm.Session{SkipHooks: true}).Omit(clause.Associations).Delete(&project).Error; err != nil {
	//	return EError(c, err)
	//}
	//
	//// Start hard deleting in foreground
	//go func(project dao.Project) {
	//	if err := s.db.Unscoped().Delete(&project).Error; err != nil {
	//		slog.Error("Hard delete project", "projectId", project.ID, "err", err)
	//	}
	//}(project)

	return c.NoContent(http.StatusOK)
}

// createProject godoc
// @id createProject
// @Summary Проекты: создание проекта
// @Description Создает новый проект в рабочем пространстве. Необходимы права на создание проектов.
// @Tags Projects
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param request body CreateProjectRequest true "Информация о проекте"
// @Success 200 {object} dto.Project "Созданный проект"
// @Failure 400 {object} apierrors.DefinedError "Ошибка запроса"
// @Failure 409 {object} apierrors.DefinedError "Конфликт идентификатора проекта"
// @Router /api/auth/workspaces/{workspaceSlug}/projects [post]
func (s *Services) createProject(c echo.Context) error {
	user := c.(WorkspaceContext).User
	workspace := c.(WorkspaceContext).Workspace

	// Tariffication
	if user.Tariffication != nil {
		var count int64
		if err := s.db.Model(&dao.Project{}).
			Where("workspace_id = ?", workspace.ID).
			Where("id in (?) or created_by_id = ?", s.db.Select("project_id").Where("member_id = ?", user.ID).Model(&dao.ProjectMember{}), user.ID).
			Count(&count).Error; err != nil {
			return EError(c, err)
		}

		if count > int64(user.Tariffication.ProjectsLimit) {
			return EErrorDefined(c, apierrors.ErrProjectLimitExceed)
		}
	}

	project := dao.Project{
		ID:          dao.GenID(),
		WorkspaceId: workspace.ID,
		CreatedById: user.ID,
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		var req CreateProjectRequest
		if err := c.Bind(&req); err != nil {
			return err
		}
		project.Name = strings.TrimSpace(req.Name)

		err := c.Validate(req)
		if err != nil {
			return err
		}

		req.Bind(&project)

		project.Identifier = strings.ToUpper(strings.Trim(project.Identifier, " "))
		if project.Identifier == "" {
			return errors.New("project identifier empty")
		}
		if project.ProjectLeadId == "" {
			project.ProjectLeadId = user.ID
		}

		// Create project
		if err := tx.Create(&project).Error; err != nil {
			return err
		}

		return tx.Create(&[]dao.State{
			{
				ID: dao.GenID(), ProjectId: project.ID, WorkspaceId: workspace.ID, CreatedById: &user.ID,
				Name:     "Новая",
				Color:    "#26b5ce",
				Sequence: 15000,
				Group:    "backlog",
				Default:  true,
			},
			{
				ID: dao.GenID(), ProjectId: project.ID, WorkspaceId: workspace.ID, CreatedById: &user.ID,
				Name:     "Открыта",
				Color:    "#f2c94c",
				Sequence: 25000,
				Group:    "unstarted",
			},
			{
				ID: dao.GenID(), ProjectId: project.ID, WorkspaceId: workspace.ID, CreatedById: &user.ID,
				Name:     "В работе",
				Color:    "#5e6ad2",
				Sequence: 35000,
				Group:    "started",
			},
			{
				ID: dao.GenID(), ProjectId: project.ID, WorkspaceId: workspace.ID, CreatedById: &user.ID,
				Name:     "Выполнена",
				Color:    "#4cb782",
				Sequence: 45000,
				Group:    "completed",
			},
			{
				ID: dao.GenID(), ProjectId: project.ID, WorkspaceId: workspace.ID, CreatedById: &user.ID,
				Name:     "Отменена",
				Color:    "#eb5757",
				Sequence: 55000,
				Group:    "cancelled",
			},
		}).Error
	}); err != nil {
		if err == gorm.ErrDuplicatedKey {
			return EErrorDefined(c, apierrors.ErrProjectIdentifierConflict)
		}
		return EError(c, err)
	}

	//newProjectMap := map[string]interface{}{
	//	"name": project.Name,
	//}
	//
	//if err := s.tracker.TrackActivity(tracker.PROJECT_CREATED_ACTIVITY, newProjectMap, nil, project.ID, tracker.ENTITY_TYPE_PROJECT, &project, *user); err != nil {
	//	return EError(c, err)
	//}

	err := tracker.TrackActivity[dao.Project, dao.WorkspaceActivity](s.tracker, tracker.ENTITY_CREATE_ACTIVITY, nil, nil, project, user)
	if err != nil {
		errStack.GetError(c, err)
	}

	return c.JSON(http.StatusOK, project.ToDTO())
}

// ############# Activities methods ###################

// getProjectActivityList godoc
// @id getProjectActivityList
// @Summary Проекты: получение активностей рабочего проекта
// @Description Возвращает список активностей для указанного проекта с возможностью пагинации.
// @Tags Projects
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param offset query int false "Смещение для пагинации" default(0)
// @Param limit query int false "Количество записей на странице" default(100)
// @Success 200 {object} dao.PaginationResponse{result=[]dto.EntityActivityFull} "Список активностей проекта"
// @Failure 400 {object} apierrors.DefinedError "Ошибка запроса"
// @Failure 404 {object} apierrors.DefinedError "Проект не найден"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/activities [get]
func (s *Services) getProjectActivityList(c echo.Context) error {
	projectId := c.(ProjectContext).Project.ID
	workspaceId := c.(ProjectContext).Workspace.ID

	offset := -1
	limit := 100

	if err := echo.QueryParamsBinder(c).
		Int("offset", &offset).
		Int("limit", &limit).BindError(); err != nil {
		return EError(c, err)
	}

	var issue dao.IssueActivity
	issue.UnionCustomFields = "'issue' AS entity_type"
	var project dao.ProjectActivity
	project.UnionCustomFields = "'project' AS entity_type"

	unionTable := dao.BuildUnionSubquery(s.db, "union_activities", dao.FullActivity{}, issue, project)

	query := unionTable.
		Joins("Project").
		Joins("Workspace").
		Joins("Actor").
		Joins("Issue").
		Order("union_activities.created_at desc").
		Where("union_activities.workspace_id = ?", workspaceId).
		Where("union_activities.project_id = ?", projectId)

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

// ############# Identifier methods ###################

// checkProjectIdentifierAvailability godoc
// Deprecated
// @ Deprecated
// @id checkProjectIdentifierAvailability
// @Summary Проекты: проверка идентификатора проекта на уникальность
// @Description Возвращает информацию о том, существует ли идентификатор проекта в указанном рабочем пространстве.
// @Tags Projects
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param name query string true "Идентификатор проекта"
// @Success 200 {object} CheckProjectIdentifierAvailabilityResponse "Статус доступности идентификатора"
// @Failure 400 {object} apierrors.DefinedError "Идентификатор проекта обязателен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/project-identifiers [get]
func (s *Services) checkProjectIdentifierAvailability(c echo.Context) error {
	name := strings.ToUpper(strings.TrimSpace(c.QueryParam("name")))
	workspace := c.(WorkspaceContext).Workspace

	if name == "" {
		return EErrorDefined(c, apierrors.ErrProjectIdentifierRequired)
	}

	var identifiers []string
	if err := s.db.Select("identifier").
		Model(&dao.Project{}).
		Where("identifier = ?", name).
		Where("workspace_id = ?", workspace.ID).
		Find(&identifiers).Error; err != nil {
		return EError(c, err)
	}

	response := CheckProjectIdentifierAvailabilityResponse{
		Exists:      len(identifiers),
		Identifiers: identifiers,
	}

	return c.JSON(http.StatusOK, response)
}

// ############# Project Members methods ###################

// getProjectMemberList godoc
// @id getProjectMemberList
// @Summary Проекты (участники): получение списка участников проекта
// @Description Возвращает список участников проекта с возможностью фильтрации по имени или электронной почте.
// @Tags Projects
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param offset query int false "Смещение для пагинации" default(-1)
// @Param limit query int false "Количество участников на странице" default(100)
// @Param search_query query string false "Поисковый запрос для фильтрации участников по имени или электронной почте" default("")
// @Param order_by query string false "Поле для сортировки" default("last_name")
// @Param desc query bool false "Направление сортировки: true - по убыванию, false - по возрастанию " default(true)
// @Param find_by query []string false "Список полей для поиска" default(["username", "email", "last_name", "first_name"])
// @Success 200 {object} dao.PaginationResponse{result=[]dto.ProjectMemberLight,my_entity=dto.ProjectMemberLight} "Список пользователей проекта"
// @Failure 400 {object} apierrors.DefinedError "Ошибка запроса"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/members [get]
func (s *Services) getProjectMemberList(c echo.Context) error {
	project := c.(ProjectContext).Project
	projectMember := c.(ProjectContext).ProjectMember

	projectMember.Member = c.(ProjectContext).User

	offset := -1
	limit := 100
	searchQuery := ""
	orderBy := ""
	desc := true
	var findBy []string
	fields := []string{"username", "email", "last_name", "first_name"}

	if err := echo.QueryParamsBinder(c).
		Int("offset", &offset).
		Int("limit", &limit).
		String("search_query", &searchQuery).
		String("order_by", &orderBy).
		Strings("find_by", &findBy).
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

	if searchQuery != "" {
		searchQuery = PrepareSearchRequest(searchQuery)
	}

	findMap := make(map[string]struct{}, len(findBy))

	for _, str := range findBy {
		findMap[str] = struct{}{}
	}

	customFind := len(findBy) > 0
	findString := ""
	var searchQueryList []interface{}
	respSort := false
	for _, field := range fields {
		if !respSort && field == orderBy {
			respSort = true
		}
		if _, ok := findMap[field]; ok || !customFind {
			searchQueryList = append(searchQueryList, searchQuery)
			if len(findString) == 0 {
				findString = "lower(" + field + ") LIKE ?"
				continue
			}
			findString = findString + " OR lower(" + field + ") LIKE ?"
		}
	}

	if desc {
		orderBy = fmt.Sprintf("%s %s", orderBy, "desc")
	} else {
		orderBy = fmt.Sprintf("%s %s", orderBy, "asc")
	}

	query := s.db.
		Where("project_id = ?", project.ID).
		Joins("Member").
		Preload(clause.Associations).
		Preload("Workspace.Owner").
		Order(orderBy)

	if searchQuery != "" {
		query = query.Where(findString, searchQueryList...)
	}

	var members []dao.ProjectMember
	count, err := dao.PaginationRequest(
		offset,
		limit,
		query,
		&members,
	)
	if err != nil {
		return EError(c, err)
	}

	count.Result = utils.SliceToSlice(count.Result.(*[]dao.ProjectMember), func(pm *dao.ProjectMember) dto.ProjectMemberLight { return *pm.ToLightDTO() })

	count.MyEntity = projectMember.ToLightDTO()

	return c.JSON(http.StatusOK, count)
}

// getProjectMember godoc
// @id getProjectMember
// @Summary Проекты (участники): получение информации об участнике проекта
// @Description Возвращает информацию о конкретном участнике проекта по его идентификатору.
// @Tags Projects
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param memberId path string true "ID участника проекта"
// @Success 200 {object} dto.ProjectMemberLight "Информация о члене проекта"
// @Failure 404 {object} apierrors.DefinedError "Участник проекта не найден"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/members/{memberId} [get]
func (s *Services) getProjectMember(c echo.Context) error {
	project := c.(ProjectContext).Project
	memberId := c.Param("memberId")

	var member dao.ProjectMember
	if err := s.db.
		Where("project_id = ?", project.ID).
		Where("project_members.id = ?", memberId).
		Joins("Workspace").
		Joins("Member").
		Preload(clause.Associations).
		Preload("Workspace.Owner").
		Find(&member).Error; err != nil {
		return EError(c, err)
	}
	return c.JSON(http.StatusOK, member.ToLightDTO())
}

// updateProjectMember godoc
// @id updateProjectMember
// @Summary Проекты (участники): обновление роли участника проекта
// @Description Обновляет роль участника проекта по его идентификатору. Проверяет, что обновление не нарушает ограничения по ролям и статусу участника.
// @Tags Projects
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param memberId path string true "ID участника проекта"
// @Param role body map[string]int true "Обновленная роль участника проекта"
// @Success 200 {object} dto.ProjectMemberLight "Информация о обновленном участнике проекта"
// @Failure 400 {object} apierrors.DefinedError "Ошибка запроса"
// @Failure 403 {object} apierrors.DefinedError "Нет прав на изменение роли"
// @Failure 404 {object} apierrors.DefinedError "Участник проекта не найден"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/members/{memberId} [patch]
func (s *Services) updateProjectMember(c echo.Context) error {
	user := *c.(ProjectContext).User
	project := c.(ProjectContext).Project
	memberId := c.Param("memberId")

	projectMember := c.(ProjectContext).ProjectMember

	var requestedProjectMember dao.ProjectMember
	if err := s.db.Where("project_members.id = ?", memberId).
		Where("project_id = ?", project.ID).
		Preload("Workspace").
		Preload("Workspace.Owner").
		Preload("Member").
		Preload("Project").
		First(&requestedProjectMember).Error; err != nil {
		return EError(c, err)
	}

	var isWorkspaceAdmin bool
	if err := s.db.
		Select("count(*) > 0").
		Where("role = ?", types.AdminRole).
		Where("workspace_id = ?", project.WorkspaceId).
		Where("member_id = ?", requestedProjectMember.MemberId).
		Model(&dao.WorkspaceMember{}).
		Find(&isWorkspaceAdmin).Error; err != nil {
		return EError(c, err)
	}
	if isWorkspaceAdmin {
		return EErrorDefined(c, apierrors.ErrCannotUpdateWorkspaceAdmin)
	}

	var data map[string]int
	if err := json.NewDecoder(c.Request().Body).Decode(&data); err != nil {
		return EError(c, err)
	}

	if requestedProjectMember.MemberId == project.ProjectLeadId {
		return EErrorDefined(c, apierrors.ErrChangeLeadRoleForbidden)
	}

	if projectMember.Role < requestedProjectMember.Role {
		return EErrorDefined(c, apierrors.ErrCannotUpdateHigherRole)
	}

	oldMemberMap := StructToJSONMap(requestedProjectMember)
	requestedProjectMember.Role = data["role"]
	requestedProjectMember.UpdatedById = &user.ID

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if requestedProjectMember.Role == types.GuestRole {
			if requestedProjectMember.IsDefaultAssignee {
				requestedProjectMember.IsDefaultAssignee = false
				var defaultAssignees []string
				for _, assignee := range project.DefaultAssignees {
					if assignee != requestedProjectMember.ID {
						defaultAssignees = append(defaultAssignees, assignee)
					}
				}
				project.DefaultAssignees = defaultAssignees

				if err := tx.Model(&dao.Project{}).
					Where("id = ?", project.ID).
					Select("default_assignees").
					Updates(&project).Error; err != nil {
					return err
				}
			}
			var issues []dao.Issue

			if err := tx.Model(&dao.Issue{}).
				Where("project_id = ?", project.ID).
				Find(&issues).Error; err != nil {
				return err
			}

		}

		if err := tx.Model(&dao.ProjectMember{}).
			Where("id = ?", requestedProjectMember.ID).
			Select("role", "updated_by_id", "is_default_assignee", "project_id", "member_id").
			Updates(&requestedProjectMember).Error; err != nil {
			return err
		}

		return nil
	}); err != nil {
		return EError(c, err)
	}

	newMemberMap := StructToJSONMap(requestedProjectMember)

	// Трек активности после обновления роли участника
	//if err := s.tracker.TrackActivity(tracker.PROJECT_MEMBER_UPDATED_ACTIVITY, newMemberMap, oldMemberMap, project.ID, tracker.ENTITY_TYPE_PROJECT, &project, user); err != nil {
	//	return EError(c, err)
	//}

	err := tracker.TrackActivity[dao.Project, dao.ProjectActivity](s.tracker, tracker.ENTITY_UPDATED_ACTIVITY, newMemberMap, oldMemberMap, project, &user)
	if err != nil {
		errStack.GetError(c, err)
	}

	return c.JSON(http.StatusOK, requestedProjectMember.ToLightDTO())
}

// deleteProjectMember godoc
// @id deleteProjectMember
// @Summary Проекты (участники): удаление участника из проекта
// @Description Удаляет участника проекта по его идентификатору. Проверяет права пользователя и ограничения на удаление.
// @Tags Projects
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param memberId path string true "ID участника проекта"
// @Success 204 {object} nil "Успешное удаление участника проекта"
// @Failure 403 {object} apierrors.DefinedError "Нет прав на удаление участника"
// @Failure 404 {object} apierrors.DefinedError "Участник проекта не найден"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/members/{memberId} [delete]
func (s *Services) deleteProjectMember(c echo.Context) error {
	user := c.(ProjectContext).User
	project := c.(ProjectContext).Project
	projectMember := c.(ProjectContext).ProjectMember
	workspace := c.(ProjectContext).Workspace
	workspaceMember := c.(ProjectContext).WorkspaceMember
	memberId := c.Param("memberId")

	if projectMember.Member == nil {
		projectMember.Member = user
	}

	var requestedMember dao.ProjectMember
	if err := s.db.
		Joins("Member").
		Joins("Project").
		Where("project_members.id = ?", memberId).
		Where("project_id = ?", project.ID).
		First(&requestedMember).Error; err != nil {
		return EErrorDefined(c, apierrors.ErrProjectMemberNotFound)
	}

	s.business.ProjectCtx(c, user, &project, &projectMember, &workspace, &workspaceMember)
	defer s.business.ProjectCtxClean()

	if err := s.business.DeleteProjectMember(&projectMember, &requestedMember); err != nil {
		return EError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// addMemberToProject godoc
// @id addMemberToProject
// @Summary Проекты (участники): добавление участника в проект
// @Description Добавляет нового участника в проект. Проверяет наличие пользователя в рабочем пространстве и уникальность его участия в проекте.
// @Tags Projects
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param memberId body dto.ProjectMember true "Данные участника проекта"
// @Success 201 {object} dto.ProjectMemberLight "Успешное добавление участника"
// @Failure 400 {object} apierrors.DefinedError "Роль и ID участника обязательны"
// @Failure 404 {object} apierrors.DefinedError "Пользователь не найден или не является участником рабочего пространства"
// @Failure 409 {object} apierrors.DefinedError "Пользователь уже является участником проекта"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/members/add [post]
func (s *Services) addMemberToProject(c echo.Context) error {
	user := *c.(ProjectContext).User
	project := c.(ProjectContext).Project
	workspace := c.(ProjectContext).Workspace

	var projectMember dao.ProjectMember
	if err := c.Bind(&projectMember); err != nil {
		return EError(c, err)
	}

	if projectMember.Role == 0 || projectMember.MemberId == "" {
		return EErrorDefined(c, apierrors.ErrRoleAndMemberIDRequired)
	}

	var member dao.User
	if err := s.db.Where("id = ?", projectMember.MemberId).First(&member).Error; err != nil {
		return EError(c, err)
	}

	// Check if the user is a member in the workspace
	var workspaceMember dao.WorkspaceMember
	if err := s.db.
		Where("workspace_id = ?", project.WorkspaceId).
		Where("member_id = ?", projectMember.MemberId).
		First(&workspaceMember).Error; err == gorm.ErrRecordNotFound {
		return EErrorDefined(c, apierrors.ErrUserNotInWorkspace)
	} else if err != nil {
		return EError(c, err)
	}

	// Check if the user is already member of project
	var exists bool
	if err := s.db.Table("project_members").
		Where("member_id = ?", projectMember.MemberId).
		Where("project_id = ?", project.ID).
		Select("count(*) > 0").
		Find(&exists).Error; err != nil {
		return EError(c, err)
	}
	if exists {
		return EErrorDefined(c, apierrors.ErrUserAlreadyInProject)
	}

	projectMember.ID = dao.GenID()
	projectMember.ProjectId = project.ID
	projectMember.CreatedById = &user.ID
	projectMember.WorkspaceId = workspaceMember.WorkspaceId
	projectMember.ViewProps = dao.DefaultViewProps

	if err := s.db.Create(&projectMember).Error; err != nil {
		return EError(c, err)
	}
	projectMember.CreatedBy = &user
	projectMember.Project = &project
	projectMember.Member = &member
	projectMember.Workspace = &workspace

	//s.notificationsService.Tg.AddedToProjectNotify(projectMember)
	go s.emailService.ProjectInvitation(projectMember) // TODO добавить в пул воркеров на отправку

	// Трекинг активности при добавлении пользователя в проект
	newMemberMap := StructToJSONMap(projectMember)

	newMemberMap["updateScopeId"] = projectMember.MemberId
	newMemberMap["member_activity_val"] = projectMember.Role

	//if err := s.tracker.TrackActivity(tracker.PROJECT_MEMBER_ADDED_ACTIVITY, newMemberMap, nil, project.ID, tracker.ENTITY_TYPE_PROJECT, &project, user); err != nil {
	//	return EError(c, err)
	//}
	err := tracker.TrackActivity[dao.ProjectMember, dao.ProjectActivity](s.tracker, tracker.ENTITY_ADD_ACTIVITY, newMemberMap, nil, projectMember, &user)
	if err != nil {
		errStack.GetError(c, err)
	}

	return c.JSON(http.StatusCreated, projectMember.ToLightDTO())
}

// updateMyNotifications godoc
// @id updateMyNotifications
// @Summary Проекты (участники): обновление настроек уведомлений текущего участника проекта
// @Description Обновляет настройки уведомлений для текущего участника проекта.
// @Tags Projects
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param notificationSettings body projectNotificationRequest true "Настройки уведомлений"
// @Success 204 "Настройки успешно обновлены"
// @Failure 400 {object} apierrors.DefinedError "Ошибка при обновлении настроек уведомлений"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/me/notifications/ [post]
func (s *Services) updateMyNotifications(c echo.Context) error {
	projectMember := c.(ProjectContext).ProjectMember
	var req projectNotificationRequest
	fields, err := BindData(c, "", &req)
	if err != nil {
		return EErrorDefined(c, apierrors.ErrGeneric)
	}

	for _, field := range fields {
		switch field {
		case "notification_settings_app":
			projectMember.NotificationSettingsApp = req.NotificationSettingsApp
		case "notification_author_settings_app":
			projectMember.NotificationAuthorSettingsApp = req.NotificationAuthorSettingsApp
		case "notification_settings_tg":
			projectMember.NotificationSettingsTG = req.NotificationSettingsTG
		case "notification_author_settings_tg":
			projectMember.NotificationAuthorSettingsTG = req.NotificationAuthorSettingsTG
		case "notification_settings_email":
			projectMember.NotificationSettingsEmail = req.NotificationSettingsEmail
		case "notification_author_settings_email":
			projectMember.NotificationAuthorSettingsEmail = req.NotificationAuthorSettingsEmail
		}
	}

	if err := s.db.Select(fields).Updates(&projectMember).Error; err != nil {
		return EError(c, err)
	}
	return c.NoContent(http.StatusOK)
}

// ############# Join projects methods ###################

// joinProjects godoc
// @id joinProjects
// @Summary Проекты: подключение пользователя к списку проектов
// @Description Позволяет пользователю присоединиться к нескольким проектам в рабочем пространстве.
// @Tags Projects
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projects body JoinProjectsRequest true "ID проектов для подключения"
// @Success 201 {object} JoinProjectsSuccessResponse "Сообщение об успешном подключении к проектам"
// @Failure 400 {object} apierrors.DefinedError "Ошибка при подключении к проектам"
// @Failure 403 {object} apierrors.DefinedError "Попытка подключиться к закрытому проекту"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/join/ [post]
func (s *Services) joinProjects(c echo.Context) error {
	user := c.(WorkspaceContext).User
	workspace := c.(WorkspaceContext).Workspace
	workspaceMember := c.(WorkspaceContext).WorkspaceMember

	var req JoinProjectsRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return EError(c, err)
	}

	projectIDs := req.ProjectIDs

	role := types.MemberRole
	if workspaceMember.Role == types.GuestRole {
		role = types.GuestRole
	}

	var memberships []dao.ProjectMember
	for _, projectId := range projectIDs {
		var project dao.Project
		if err := s.db.Where("workspace_id = ?", workspace.ID).
			Where("id = ?", projectId).
			First(&project).Error; err != nil {
			return EError(c, err)
		}

		if !project.Public {
			return EErrorDefined(c, apierrors.ErrProjectIsPrivate.WithFormattedMessage(projectId))
		}

		memberships = append(memberships, dao.ProjectMember{
			ID:          dao.GenID(),
			ProjectId:   projectId,
			MemberId:    user.ID,
			Role:        role,
			WorkspaceId: workspaceMember.WorkspaceId,
			CreatedById: &user.ID,
			CreatedAt:   time.Now(),
			ViewProps:   dao.DefaultViewProps,
		})
	}

	if err := s.db.Create(&memberships).Error; err != nil {
		return EError(c, err)
	}

	return c.JSON(http.StatusCreated, JoinProjectsSuccessResponse{Message: "Projects joined successfully"})
}

// ############# Views methods ###################

// updateProjectView godoc
// @id updateProjectView
// @Summary Проекты: установка фильтров для просмотра проектов
// @Description Позволяет пользователю установить настройки просмотра для конкретного проекта.
// @Tags Projects
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param view_props body types.ViewProps true "Настройки просмотра проекта"
// @Success 204 {string} string "Настройки просмотра успешно обновлены"
// @Failure 400 {object} apierrors.DefinedError "Ошибка при установке настроек просмотра"
// @Failure 404 {object} apierrors.DefinedError "Проект не найден"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/project-views/ [post]
func (s *Services) updateProjectView(c echo.Context) error {
	user := *c.(ProjectContext).User
	project := c.(ProjectContext).Project

	var viewProps types.ViewProps

	if err := c.Bind(&viewProps); err != nil {
		return EError(c, err)
	}

	if err := c.Validate(viewProps); err != nil {
		return EErrorDefined(c, apierrors.ErrInvalidProjectViewProps.WithFormattedMessage(err))
	}

	if err := s.db.Model(&dao.ProjectMember{}).
		Where("project_id = ?", project.ID).
		Where("member_id = ?", user.ID).
		Update("view_props", viewProps).Error; err != nil {
		return EError(c, err)
	}

	return c.NoContent(http.StatusOK)
}

// ############# Me methods ###################

// getProjectMemberMe godoc
// @id getProjectMemberMe
// @Summary Проекты: получение информации о членстве текущего пользователя в проекте
// @Description Возвращает информацию о текущем пользователе как члене проекта.
// @Tags Projects
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Success 200 {object} dto.ProjectMember "Информация о членстве пользователя в проекте"
// @Failure 404 {object} apierrors.DefinedError "Членство в проекте не найдено"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/project-members/me [get]
func (s *Services) getProjectMemberMe(c echo.Context) error {
	projectMember := c.(ProjectContext).ProjectMember
	return c.JSON(http.StatusOK, projectMember.ToDTO())
}

// ############# User favorite projects methods ###################

// getFavoriteProjects godoc
// @id getFavoriteProjects
// @Summary Проекты: получение списка избранных проектов пользователя
// @Description Возвращает список избранных проектов текущего пользователя в рабочем пространстве.
// @Tags Projects
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Success 200 {array} dto.ProjectFavorites "Список избранных проектов"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/user-favorite-projects/ [get]
func (s *Services) getFavoriteProjects(c echo.Context) error {
	user := *c.(WorkspaceContext).User
	workspace := c.(WorkspaceContext).Workspace

	var favorites []dao.ProjectFavorites
	if err := s.db.Where("user_id = ?", user.ID).
		Where("workspace_id = ?", workspace.ID).
		Preload("Workspace").
		Preload("Workspace.Owner").
		Preload("Project").
		Preload("Project.ProjectLead").
		Preload("Project.DefaultAssigneesDetails").
		Preload("Project.DefaultWatchersDetails").
		Set("userId", user.ID).
		Find(&favorites).Error; err != nil {
		return EError(c, err)
	}

	return c.JSON(
		http.StatusOK,
		utils.SliceToSlice(&favorites, func(p *dao.ProjectFavorites) dto.ProjectFavorites { return *p.ToDTO() }),
	)
}

// addProjectToFavorites godoc
// @id addProjectToFavorites
// @Summary Проекты: добавление проекта в список избранных проектов пользователя
// @Description Добавляет указанный проект в список избранных проектов текущего пользователя в рабочем пространстве.
// @Tags Projects
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param project body AddProjectToFavoritesRequest true "ID проекта для добавления в избранное"
// @Success 201 {object} dto.ProjectFavorites "Добавленный проект в избранных"
// @Success 200 "Проект уже в избранных"
// @Failure 400 {object} apierrors.DefinedError "Ошибка запроса"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/user-favorite-projects/ [post]
func (s *Services) addProjectToFavorites(c echo.Context) error {
	user := *c.(WorkspaceContext).User
	workspace := c.(WorkspaceContext).Workspace

	var req AddProjectToFavoritesRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return EError(c, err)
	}

	projectID := req.ProjectID

	project, err := dao.GetProject(s.db, workspace.Slug, user.ID, projectID)
	if err != nil {
		return EError(c, err)
	}

	projectFavorite := dao.ProjectFavorites{
		Id:          dao.GenID(),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		CreatedById: &user.ID,
		ProjectId:   project.ID,
		UserId:      user.ID,
		WorkspaceId: project.Workspace.ID,
	}
	if err := s.db.Create(&projectFavorite).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return c.NoContent(http.StatusOK)
		}
		return EError(c, err)
	}
	return c.JSON(http.StatusCreated, projectFavorite.ToDTO())
}

// removeProjectFromFavorites godoc
// @id removeProjectFromFavorites
// @Summary Проекты: удаление проекта из списка избранных проектов пользователя
// @Description Удаляет указанный проект из списка избранных проектов текущего пользователя в рабочем пространстве.
// @Tags Projects
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта для удаления из избранного"
// @Success 204 "Проект успешно удален из избранных"
// @Failure 404 {object} apierrors.DefinedError "Проект не найден"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/user-favorite-projects/{projectId} [delete]
func (s *Services) removeProjectFromFavorites(c echo.Context) error {
	user := *c.(WorkspaceContext).User
	workspace := c.(WorkspaceContext).Workspace
	projectId := c.Param("projectId")

	project, err := dao.GetProject(s.db, workspace.Slug, user.ID, projectId)
	if err != nil {
		return EError(c, err)
	}

	if err := s.db.Where("project_id = ?", project.ID).
		Where("user_id = ?", user.ID).
		Where("workspace_id = ?", project.Workspace.ID).
		Delete(&dao.ProjectFavorites{}).Error; err != nil {
		return EError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// ############# Identifier methods ###################

// getProjectEstimatePointsList godoc
// Deprecated
// @ Deprecated
func (s *Services) getProjectEstimatePointsList(c echo.Context) error {
	// @id getProjectEstimatePointsList
	// @Summary Проекты: получение значений оценок для проекта
	// @Description Возвращает список оценок для указанного проекта
	// @Tags Projects
	// @Security ApiKeyAuth
	// @Param workspaceSlug path string true "Slug рабочего пространства"
	// @Param projectId path string true "ID проекта"
	// @Success 200 {array} dto.EstimatePoint "Список значений оценок"
	// @Failure 404 {object} apierrors.DefinedError "Проект не найден"
	// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
	// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/project-estimates [get]
	project := c.(ProjectContext).Project

	estimatePoints := make([]dao.EstimatePoint, 0)
	if project.EstimateId != nil {
		if err := s.db.
			Where("project_id = ?", project.ID).
			Where("estimate_id = ?", project.EstimateId).
			Find(&estimatePoints).Error; err != nil {
			return EError(c, err)
		}
	}
	return c.JSON(
		http.StatusOK,
		utils.SliceToSlice(&estimatePoints, func(ep *dao.EstimatePoint) dto.EstimatePoint { return *ep.ToDTO() }),
	)
}

// getProjectEstimatesList godoc
// Deprecated
// @ Deprecated
func (s *Services) getProjectEstimatesList(c echo.Context) error {
	// @id getProjectEstimatesList
	// @Summary Проекты: получение списка оценок проекта
	// @Description Возвращает все оценки, связанные с проектом
	// @Tags Projects
	// @Security ApiKeyAuth
	// @Param workspaceSlug path string true "Slug рабочего пространства"
	// @Param projectId path string true "ID проекта"
	// @Success 200 {array} dto.Estimate "Список оценок проекта"
	// @Failure 404 {object} apierrors.DefinedError "Проект не найден"
	// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
	// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/estimates [get]
	project := c.(ProjectContext).Project

	var estimates []dao.Estimate
	if err := s.db.
		Preload(clause.Associations).
		Where("project_id = ?", project.ID).
		Find(&estimates).Error; err != nil {
		return EError(c, err)
	}

	return c.JSON(
		http.StatusOK,
		utils.SliceToSlice(&estimates, func(e *dao.Estimate) dto.Estimate { return *e.ToDTO() }),
	)
}

// createProjectEstimate godoc
// Deprecated
// @ Deprecated
func (s *Services) createProjectEstimate(c echo.Context) error {
	// @id createProjectEstimate
	// @Summary Проекты: создание новой оценки для проекта
	// @Description Создает оценку и связанные значения для указанного проекта
	// @Tags Projects
	// @Security ApiKeyAuth
	// @Accept json
	// @Produce json
	// @Param workspaceSlug path string true "Slug рабочего пространства"
	// @Param projectId path string true "ID проекта"
	// @Param data body EstimatePayload true "Данные оценки"
	// @Success 201 {object} EstimatePayloadResponse "Созданная оценка с точками"
	// @Failure 400 {object} apierrors.DefinedError "Неверные данные или недостаточно точек оценки"
	// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
	// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/estimates [post]
	user := *c.(ProjectContext).User
	project := c.(ProjectContext).Project

	var data EstimatePayload
	if err := c.Bind(&data); err != nil {
		return EError(c, err)
	}

	if len(data.EstimatePoints) < 1 || len(data.EstimatePoints) > 8 {
		return EErrorDefined(c, apierrors.ErrEstimatePointsRequired)
	}

	data.Estimate.Id = dao.GenID()
	data.Estimate.CreatedAt = time.Now()
	data.Estimate.CreatedById = &user.ID
	data.Estimate.UpdatedAt = time.Now()
	data.Estimate.UpdatedById = &user.ID
	data.Estimate.WorkspaceId = project.WorkspaceId
	data.Estimate.ProjectId = project.ID

	if err := s.db.Create(&data.Estimate).Error; err != nil {
		return EError(c, err)
	}

	for i := 0; i < len(data.EstimatePoints); i++ {
		data.EstimatePoints[i].Id = dao.GenID()
		data.EstimatePoints[i].CreatedAt = time.Now()
		data.EstimatePoints[i].CreatedById = &user.ID
		data.EstimatePoints[i].UpdatedAt = time.Now()
		data.EstimatePoints[i].UpdatedById = &user.ID
		data.EstimatePoints[i].WorkspaceId = project.WorkspaceId
		data.EstimatePoints[i].ProjectId = project.ID
		data.EstimatePoints[i].EstimateId = data.Estimate.Id
	}

	if err := s.db.CreateInBatches(&data.EstimatePoints, 10).Error; err != nil {
		return EError(c, err)
	}
	resp := EstimatePayloadResponse{
		Estimate:       data.Estimate.ToDTO(),
		EstimatePoints: utils.SliceToSlice(&data.EstimatePoints, func(ep *dao.EstimatePoint) dto.EstimatePoint { return *ep.ToDTO() }),
	}
	return c.JSON(http.StatusCreated, resp)
}

// getProjectEstimate godoc
// Deprecated
// @ Deprecated
func (s *Services) getProjectEstimate(c echo.Context) error {
	// @id getProjectEstimate
	// @Summary Проекты: получение информации об оценке
	// @Description Возвращает данные по оценке проекта по ее ID
	// @Tags Projects
	// @Security ApiKeyAuth
	// @Param workspaceSlug path string true "Slug рабочего пространства"
	// @Param projectId path string true "ID проекта"
	// @Param estimateId path string true "ID оценки"
	// @Success 200 {object} dto.Estimate "Информация об оценке"
	// @Failure 404 {object} apierrors.DefinedError "Оценка не найдена"
	// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
	// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/estimates/{estimateId} [get]
	project := c.(ProjectContext).Project
	estimateId := c.Param("estimateId")

	var estimate dao.Estimate
	if err := s.db.
		Preload(clause.Associations).
		Where("project_id = ?", project.ID).
		Where("estimates.id = ?", estimateId).
		Find(&estimate).Error; err != nil {
		return EError(c, err)
	}

	return c.JSON(http.StatusOK, estimate.ToDTO())
}

// updateProjectEstimate godoc
// Deprecated
// @ Deprecated
func (s *Services) updateProjectEstimate(c echo.Context) error {
	// @id updateProjectEstimate
	// @Summary Проекты: обновление существующей оценки проекта
	// @Description Обновляет оценку проекта и значения, связанные с ней
	// @Tags Projects
	// @Security ApiKeyAuth
	// @Accept json
	// @Produce json
	// @Param workspaceSlug path string true "Slug рабочего пространства"
	// @Param projectId path string true "ID проекта"
	// @Param estimateId path string true "ID оценки"
	// @Param data body EstimatePayload true "Данные для обновления оценки"
	// @Success 200 {object} EstimatePayloadResponse "Обновленная оценка и точки"
	// @Failure 400 {object} apierrors.DefinedError "Неверные данные оценки"
	// @Failure 404 {object} apierrors.DefinedError "Оценка не найдена"
	// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
	// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/estimates/{estimateId} [patch]
	project := c.(ProjectContext).Project
	estimateId := c.Param("estimateId")

	var estimate dao.Estimate
	if err := s.db.
		Preload("Points").
		Where("project_id = ?", project.ID).
		Where("estimates.id = ?", estimateId).
		Find(&estimate).Error; err != nil {
		return EError(c, err)
	}

	var data EstimatePayload

	data.Estimate = estimate
	data.EstimatePoints = estimate.Points

	if err := c.Bind(&data); err != nil {
		return EError(c, err)
	}

	data.Estimate.Workspace = nil
	if err := s.db.Save(&data.Estimate).Error; err != nil {
		return EError(c, err)
	}

	if err := s.db.Save(&data.EstimatePoints).Error; err != nil {
		return EError(c, err)
	}

	resp := EstimatePayloadResponse{
		Estimate:       data.Estimate.ToDTO(),
		EstimatePoints: utils.SliceToSlice(&data.EstimatePoints, func(ep *dao.EstimatePoint) dto.EstimatePoint { return *ep.ToDTO() }),
	}

	return c.JSON(http.StatusOK, resp)
}

// deleteProjectEstimate godoc
// Deprecated
// @ Deprecated
func (s *Services) deleteProjectEstimate(c echo.Context) error {
	// @id deleteProjectEstimate
	// @Summary Проекты: удаление оценки проекта
	// @Description Удаляет оценку и все ее значения для указанного проекта
	// @Tags Projects
	// @Security ApiKeyAuth
	// @Param workspaceSlug path string true "Slug рабочего пространства"
	// @Param projectId path string true "ID проекта"
	// @Param estimateId path string true "ID оценки"
	// @Success 204 "Оценка успешно удалена"
	// @Failure 404 {object} apierrors.DefinedError "Оценка не найдена"
	// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
	// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/estimates/{estimateId} [delete]
	project := c.(ProjectContext).Project
	estimateId := c.Param("estimateId")

	if err := s.db.
		Where("project_id = ?", project.ID).
		Where("estimate_id = ?", estimateId).Delete(&dao.EstimatePoint{}).Error; err != nil {
		return EError(c, err)
	}

	if err := s.db.
		Where("project_id = ?", project.ID).
		Where("estimates.id = ?", estimateId).Delete(&dao.Estimate{}).Error; err != nil {
		return EError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

type IssueCreateRequest struct {
	dto.Issue

	BlockersList  []string `json:"blockers_list"`
	AssigneesList []string `json:"assignees_list"`
	WatchersList  []string `json:"watchers_list"`
	LabelsList    []string `json:"labels_list"`
	BlocksList    []string `json:"blocks_list"`
}

// createIssue godoc
// @id createIssue
// @Summary Задачи: создание новой задачи
// @Description Создает новую задачу в проекте с указанными параметрами и настройками
// @Tags Issues
// @Security ApiKeyAuth
// @Accept multipart/form-data
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param issue body IssueCreateRequest true "Данные задачи"
// @Success 201 {object} NewIssueID "ID созданной задачи"
// @Failure 400 {object} apierrors.DefinedError "Неверные данные задачи"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/issues/ [post]
func (s *Services) createIssue(c echo.Context) error {
	user := *c.(ProjectContext).User
	workspace := c.(ProjectContext).Workspace
	project := c.(ProjectContext).Project

	// Tariffication
	if user.Tariffication != nil {
		var count int64
		if err := s.db.Model(&dao.Issue{}).
			Where("project_id = ?", project.ID).
			Where("created_by_id = ?", user.ID).
			Count(&count).Error; err != nil {
			return EError(c, err)
		}

		if count > int64(user.Tariffication.IssuesLimit) {
			return EErrorDefined(c, apierrors.ErrIssueLimitExceed)
		}
	}

	var issue IssueCreateRequest
	form, _ := c.MultipartForm()

	// If comment without attachments
	if form == nil {
		if err := c.Bind(&issue); err != nil {
			return EError(c, err)
		}
	} else {
		// else get comment data from "comment" value
		if c.FormValue("issue") == "" {
			//TODO: defined error
			return EError(c, nil)
		}
		if err := json.Unmarshal([]byte(c.FormValue("issue")), &issue); err != nil {
			return EError(c, err)
		}

		// Tariffication
		if len(form.File["files"]) > 0 && user.Tariffication != nil && !user.Tariffication.AttachmentsAllow {
			return EError(c, apierrors.ErrAssetsNotAllowed)
		}
	}
	if len(strings.TrimSpace(issue.Name)) == 0 {
		return EErrorDefined(c, apierrors.ErrIssueNameEmpty)
	}

	var parentId uuid.NullUUID
	if issue.ParentId != nil {
		if u, err := uuid.FromString(*issue.ParentId); err == nil {
			parentId.UUID = u
			parentId.Valid = true
		}
	}

	issueNew := dao.Issue{
		ID:                  dao.GenUUID(),
		Name:                issue.Name,
		Priority:            issue.Priority,
		StartDate:           issue.StartDate,
		TargetDate:          issue.TargetDate,
		CompletedAt:         issue.CompletedAt,
		SequenceId:          issue.SequenceId,
		CreatedById:         user.ID,
		ParentId:            parentId,
		ProjectId:           project.ID,
		StateId:             issue.StateId,
		UpdatedById:         &user.ID,
		WorkspaceId:         workspace.ID,
		DescriptionHtml:     issue.DescriptionHtml,
		DescriptionStripped: issue.DescriptionStripped,
		DescriptionType:     issue.DescriptionType,
		Draft:               issue.Draft,
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := dao.CreateIssue(tx, &issueNew); err != nil {
			return err
		}
		if issueNew.TargetDate != nil && (len(issue.AssigneesList) != 0 || len(issue.WatchersList) != 0) {
			dateStr, err := notifications.FormatDate(issueNew.TargetDate.Time.String(), "2006-01-02", nil)
			if err != nil {
				return err
			}
			userIds := issue.WatchersList
			userIds = append(userIds, issue.AssigneesList...)

			if err := notifications.CreateDeadlineNotification(tx, &issueNew, &dateStr, &userIds); err != nil {
				return err
			}
		}
		if form != nil {
			// Save inline attachments
			for _, f := range form.File["files"] {
				fileAsset := dao.FileAsset{
					Id:          dao.GenUUID(),
					CreatedAt:   time.Now(),
					CreatedById: &user.ID,
					Name:        f.Filename,
					FileSize:    int(f.Size),
					WorkspaceId: &issueNew.WorkspaceId,
					IssueId:     uuid.NullUUID{Valid: true, UUID: issueNew.ID},
				}

				if err := s.uploadAssetForm(tx, f, &fileAsset,
					filestorage.Metadata{
						WorkspaceId: issueNew.WorkspaceId,
						ProjectId:   issueNew.ProjectId,
						IssueId:     issueNew.ID.String(),
					}); err != nil {
					return err
				}

				issueNew.InlineAttachments = append(issueNew.InlineAttachments, fileAsset)
			}
		}

		// Fill params
		issueId := issueNew.ID.String()

		// Add blockers
		if len(issue.BlockersList) > 0 {
			var newBlockers []dao.IssueBlocker
			for _, blocker := range issue.BlockersList {
				blockerUUID, err := uuid.FromString(fmt.Sprint(blocker))
				if err != nil {
					return err
				}
				newBlockers = append(newBlockers, dao.IssueBlocker{
					Id:          dao.GenID(),
					BlockedById: blockerUUID,
					BlockId:     issueNew.ID,
					ProjectId:   project.ID,
					WorkspaceId: issueNew.WorkspaceId,
					CreatedById: &user.ID,
					UpdatedById: &user.ID,
				})
			}
			if err := tx.CreateInBatches(&newBlockers, 10).Error; err != nil {
				return err
			}
		}

		// Add assignees
		if len(issue.AssigneesList) > 0 {
			issue.AssigneesList = utils.SetToSlice(utils.SliceToSet(issue.AssigneesList))
			var newAssignees []dao.IssueAssignee
			for _, assignee := range issue.AssigneesList {
				newAssignees = append(newAssignees, dao.IssueAssignee{
					Id:          dao.GenID(),
					AssigneeId:  fmt.Sprint(assignee),
					IssueId:     issueId,
					ProjectId:   project.ID,
					WorkspaceId: issueNew.WorkspaceId,
					CreatedById: &user.ID,
					UpdatedById: &user.ID,
				})
			}
			if err := tx.CreateInBatches(&newAssignees, 10).Error; err != nil {
				return err
			}
		}

		// Add watchers
		if len(issue.WatchersList) > 0 {
			issue.WatchersList = utils.SetToSlice(utils.SliceToSet(issue.WatchersList))
			var newWatchers []dao.IssueWatcher
			for _, watcher := range issue.WatchersList {
				newWatchers = append(newWatchers, dao.IssueWatcher{
					Id:          dao.GenID(),
					WatcherId:   fmt.Sprint(watcher),
					IssueId:     issueId,
					ProjectId:   project.ID,
					WorkspaceId: issueNew.WorkspaceId,
					CreatedById: &user.ID,
					UpdatedById: &user.ID,
				})
			}
			if err := tx.CreateInBatches(&newWatchers, 10).Error; err != nil {
				return err
			}
		}

		// Add labels
		if len(issue.LabelsList) > 0 {
			var newLabels []dao.IssueLabel
			for _, label := range issue.LabelsList {
				newLabels = append(newLabels, dao.IssueLabel{
					Id:          dao.GenID(),
					LabelId:     fmt.Sprint(label),
					IssueId:     issueId,
					ProjectId:   project.ID,
					WorkspaceId: issueNew.WorkspaceId,
					CreatedById: &user.ID,
					UpdatedById: &user.ID,
				})
			}
			if err := tx.CreateInBatches(&newLabels, 10).Error; err != nil {
				return err
			}
		}

		// Add blocked
		if len(issue.BlocksList) > 0 {
			var newBlocked []dao.IssueBlocker
			for _, block := range issue.BlocksList {
				blockUUID, err := uuid.FromString(fmt.Sprint(block))
				if err != nil {
					return err
				}
				newBlocked = append(newBlocked, dao.IssueBlocker{
					Id:          dao.GenID(),
					BlockId:     blockUUID,
					BlockedById: issueNew.ID,
					ProjectId:   project.ID,
					WorkspaceId: issueNew.WorkspaceId,
					CreatedById: &user.ID,
					UpdatedById: &user.ID,
				})
			}
			if err := tx.CreateInBatches(&newBlocked, 10).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return EError(c, err)
	}

	issueNew.Project = &project
	err := tracker.TrackActivity[dao.Issue, dao.ProjectActivity](s.tracker, tracker.ENTITY_CREATE_ACTIVITY, nil, nil, issueNew, &user)
	if err != nil {
		errStack.GetError(c, err)
	}
	if issueNew.ParentId.Valid {

		data := make(map[string]interface{})
		oldData := make(map[string]interface{})

		oldData["parent"] = nil
		data["parent"] = issueNew.ParentId.UUID.String()

		err := tracker.TrackActivity[dao.Issue, dao.IssueActivity](s.tracker, tracker.ENTITY_UPDATED_ACTIVITY, data, oldData, issueNew, &user)
		if err != nil {
			errStack.GetError(c, err)
		}

	}

	return c.JSON(http.StatusCreated, NewIssueID{Id: issueNew.ID.String()})
}

// ############# Labels methods ###################

// getIssueLabelList godoc
// @id getIssueLabelList
// @Summary Проекты (теги): получение списка тегов
// @Description Возвращает список всех тегов, связанных с проектом, с возможностью фильтрации по названию
// @Tags Projects
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param search_query query string false "Поисковый запрос для фильтрации тегов по названию"
// @Success 200 {array} dto.LabelLight "Список тегов"
// @Failure 404 {object} apierrors.DefinedError "Проект не найден"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/issue-labels [get]
func (s *Services) getIssueLabelList(c echo.Context) error {
	project := c.(ProjectContext).Project

	searchQuery := ""

	if err := echo.QueryParamsBinder(c).
		String("search_query", &searchQuery).
		BindError(); err != nil {
		return EError(c, err)
	}

	query := s.db.
		Where("project_id = ?", project.ID).
		Preload("Parent").
		Order("name")

	if searchQuery != "" {
		escapedSearchQuery := PrepareSearchRequest(searchQuery)
		query = query.Where("lower(name) like ? or name_tokens @@ plainto_tsquery('russian', lower(?))", escapedSearchQuery, searchQuery)
	}

	var labels []dao.Label
	if err := query.Find(&labels).Error; err != nil {
		return EError(c, err)
	}
	return c.JSON(
		http.StatusOK,
		utils.SliceToSlice(&labels, func(l *dao.Label) dto.LabelLight { return *l.ToLightDTO() }),
	)
}

// createIssueLabel godoc
// @id createIssueLabel
// @Summary Проекты (теги): создание тега
// @Description Создает новый тег для проекта
// @Tags Projects
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param data body dto.LabelLight true "Данные тега"
// @Success 201 {object} dto.LabelLight"Созданный тег"
// @Failure 400 {object} apierrors.DefinedError "Некорректные данные тега"
// @Failure 409 {object} apierrors.DefinedError "Тег с таким именем уже существует"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/issue-labels [post]
func (s *Services) createIssueLabel(c echo.Context) error {
	user := *c.(ProjectContext).User
	project := c.(ProjectContext).Project

	var label dao.Label
	if err := json.NewDecoder(c.Request().Body).Decode(&label); err != nil {
		return EError(c, err)
	}

	label.ID = dao.GenID()
	label.CreatedAt = time.Now()
	label.UpdatedAt = time.Now()
	label.CreatedById = &user.ID
	label.UpdatedById = &user.ID
	label.ProjectId = project.ID
	label.WorkspaceId = project.WorkspaceId

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&label).Error; err != nil {
			if err == gorm.ErrDuplicatedKey {
				return err
			}
			return err
		}

		//if err := s.tracker.TrackActivity(tracker.PROJECT_LABEL_CREATED_ACTIVITY, StructToJSONMap(label), nil, project.ID, tracker.ENTITY_TYPE_PROJECT, &project, user); err != nil {
		//	return err
		//}

		return nil
	}); err != nil {
		if err == gorm.ErrDuplicatedKey {
			return EErrorDefined(c, apierrors.ErrTagAlreadyExists)
		}
		return EError(c, err)
	}

	err := tracker.TrackActivity[dao.Label, dao.ProjectActivity](s.tracker, tracker.ENTITY_CREATE_ACTIVITY, nil, nil, label, &user)
	if err != nil {
		errStack.GetError(c, err)
	}

	return c.JSON(http.StatusCreated, label.ToLightDTO())
}

// getIssueLabel godoc
// @id getIssueLabel
// @Summary Проекты (теги): получение информации о теге
// @Description Возвращает данные тега по его ID
// @Tags Projects
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param labelId path string true "ID тега"
// @Success 200 {object} dto.LabelLight "Информация о теге"
// @Failure 404 {object} apierrors.DefinedError "Тег не найден"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/issue-labels/{labelId} [get]
func (s *Services) getIssueLabel(c echo.Context) error {
	project := c.(ProjectContext).Project
	labelId := c.Param("labelId")

	var label dao.Label
	if err := s.db.
		Where("project_id = ?", project.ID).
		Where("id = ?", labelId).
		Preload("Parent").Order("name").Find(&label).Error; err != nil {
		return EError(c, err)
	}

	return c.JSON(http.StatusOK, label.ToLightDTO())
}

// updateIssueLabel godoc
// @id updateIssueLabel
// @Summary Проекты (теги): обновление тега
// @Description Обновляет выбранные данные тега по его ID
// @Tags Projects
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param labelId path string true "ID тега"
// @Param data body dto.LabelLight true "Данные для обновления тега"
// @Success 200 {object} dto.LabelLight "Обновленный тег"
// @Failure 400 {object} apierrors.DefinedError "Некорректные данные тега"
// @Failure 404 {object} apierrors.DefinedError "Тег не найден"
// @Failure 409 {object} apierrors.DefinedError "Тег с таким именем уже существует"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/issue-labels/{labelId} [patch]
func (s *Services) updateIssueLabel(c echo.Context) error {
	user := *c.(ProjectContext).User
	project := c.(ProjectContext).Project
	labelId := c.Param("labelId")

	var label dao.Label
	if err := s.db.Where("id = ?", labelId).
		Where("project_id = ?", project.ID).
		Find(&label).Error; err != nil {
		return EError(c, err)
	}

	// Pre-update activity tracking
	oldLabelMap := StructToJSONMap(label)
	oldLabelMap["updateScope"] = "label"
	oldLabelMap["updateScopeId"] = labelId

	if err := c.Bind(&label); err != nil {
		return EError(c, err)
	}

	label.UpdatedById = &user.ID
	label.UpdatedAt = time.Now()

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		// Обновляем только выбранные поля
		if err := tx.Model(&label).
			Select([]string{"name", "description", "parent_id", "color"}).
			Updates(&label).Error; err != nil {
			if err == gorm.ErrDuplicatedKey {
				return apierrors.ErrTagAlreadyExists
			}
			return err

		}

		// Post-update activity tracking
		newLabelMap := StructToJSONMap(label)
		newLabelMap["updateScope"] = "label"
		newLabelMap["updateScopeId"] = labelId

		//if err := s.tracker.TrackActivity(tracker.PROJECT_LABEL_UPDATED_ACTIVITY, newLabelMap, oldLabelMap, project.ID, tracker.ENTITY_TYPE_PROJECT, &project, user); err != nil {
		//trackErr := tracker.newTrackError()
		//trackErr.AddErr(err)

		//	errStack.GetError(c, errStack.TrackErrorStack(err))
		//	return err
		//}
		err := tracker.TrackActivity[dao.Project, dao.ProjectActivity](s.tracker, tracker.ENTITY_UPDATED_ACTIVITY, newLabelMap, oldLabelMap, project, &user)
		if err != nil {
			errStack.GetError(c, err)
			//return err
		}

		return nil
	}); err != nil {
		return EError(c, err)
	}

	return c.JSON(http.StatusOK, label.ToLightDTO())
}

// deleteIssueLabel godoc
// @id deleteIssueLabel
// @Summary Проекты (теги): удаление тега
// @Description Удаляет тег по его ID из проекта
// @Tags Projects
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param labelId path string true "ID тега"
// @Success 204 "Тег успешно удален"
// @Failure 404 {object} apierrors.DefinedError "Тег не найден"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/issue-labels/{labelId} [delete]
func (s *Services) deleteIssueLabel(c echo.Context) error {
	user := *c.(ProjectContext).User
	project := c.(ProjectContext).Project
	labelId := c.Param("labelId")

	var label dao.Label
	if err := s.db.Where("id = ?", labelId).
		Where("project_id = ?", project.ID).
		Find(&label).Error; err != nil {
		return EError(c, err)
	}

	//TODO check it
	err := tracker.TrackActivity[dao.Label, dao.ProjectActivity](s.tracker, tracker.ENTITY_DELETE_ACTIVITY, nil, nil, label, &user)
	if err != nil {
		errStack.GetError(c, err)
	}

	if err := s.db.Delete(&label).Error; err != nil {
		return EError(c, err)
	}

	// Трек активности после успешного удаления тега
	//if err := s.tracker.TrackActivity(tracker.PROJECT_LABEL_DELETED_ACTIVITY, nil, StructToJSONMap(label), project.ID, tracker.ENTITY_TYPE_PROJECT, &project, user); err != nil {
	//	return EError(c, err)
	//}

	return c.NoContent(http.StatusOK)
}

// ############# Issues bulk methods ###################

// deleteIssuesBulk godoc
// @id deleteIssuesBulk
// @Summary Задачи: массовое удаление задач
// @Description Удаляет несколько задач в проекте по их ID
// @Tags Issues
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param issuesIds body []string true "Список ID задач для удаления"
// @Success 200 {object} map[string]string "Сообщение о количестве удаленных задач"
// @Failure 400 {object} apierrors.DefinedError "Ошибка: ID задач не указаны"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/bulk-delete-issues [delete]
func (s *Services) deleteIssuesBulk(c echo.Context) error {
	project := c.(ProjectContext).Project

	var issuesIds []string
	if err := json.NewDecoder(c.Request().Body).Decode(&issuesIds); err != nil {
		return EError(c, err)
	}

	if len(issuesIds) < 1 {
		return EErrorDefined(c, apierrors.ErrIssueIDsRequired)
	}

	var issues []dao.Issue
	if err := s.db.
		Where("project_id = ?", project.ID).
		Where("id in ?", issuesIds).
		Find(&issues).Error; err != nil {
		return EError(c, err)
	}

	totalIssues := len(issues)

	if err := s.db.Delete(&issues).Error; err != nil {
		return EError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": fmt.Sprintf("%d issues were deleted", totalIssues),
	})
}

// ############# States methods ###################

// getStateList godoc
// @id getStateList
// @Summary Проекты (статусы): получение статусов проекта
// @Description Возвращает список всех статусов проекта с возможностью фильтрации по названию
// @Tags Projects
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param search_query query string false "Поисковый запрос для фильтрации статусов по названию"
// @Success 200 {object} map[string][]dto.StateLight "Группированный список статусов"
// @Failure 404 {object} apierrors.DefinedError "Проект не найден"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/states [get]
func (s *Services) getStateList(c echo.Context) error {
	project := c.(ProjectContext).Project

	if etag := c.Request().Header.Get("If-None-Match"); etag != "" {
		etagHash, err := hex.DecodeString(etag)
		if err != nil {
			return EError(c, err)
		}

		var state dao.State
		if err := s.db.Model(&dao.State{}).Select("digest(string_agg(hash, '' order by sequence), 'sha256') as hash").Where("project_id = ?", project.ID).Find(&state).Error; err != nil {
			return EError(c, err)
		}

		if bytes.Equal(etagHash, state.Hash) {
			return c.NoContent(http.StatusNotModified)
		}
	}

	searchQuery := ""

	if err := echo.QueryParamsBinder(c).
		String("search_query", &searchQuery).
		BindError(); err != nil {
		return EError(c, err)
	}
	query := s.db.
		Preload(clause.Associations).
		Order("sequence").
		Where("project_id = ?", project.ID)

	if searchQuery != "" {
		escapedSearchQuery := PrepareSearchRequest(searchQuery)
		query = query.Where("lower(name) like ? or name_tokens @@ plainto_tsquery('russian', lower(?))", escapedSearchQuery, strings.ToLower(searchQuery))
	}

	var states []dao.State
	if err := query.Find(&states).Error; err != nil {
		return EError(c, err)
	}

	result := make(map[string][]dto.StateLight)
	hash := sha256.New()
	for _, state := range states {
		arr, ok := result[state.Group]
		if !ok {
			arr = make([]dto.StateLight, 0)
		}
		arr = append(arr, *state.ToLightDTO())
		result[state.Group] = arr
		hash.Write(state.Hash)
	}
	c.Response().Header().Add("ETag", hex.EncodeToString(hash.Sum(nil)))
	return c.JSON(http.StatusOK, result)
}

// createState godoc
// @id createState
// @Summary Проекты (статусы): создание статуса
// @Description Создает новый статус для проекта
// @Tags Projects
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param data body dto.StateLight true "Данные статуса"
// @Success 201 {object} dto.StateLight "Созданный статус"
// @Failure 400 {object} apierrors.DefinedError "Некорректные данные для статуса"
// @Failure 409 {object} apierrors.DefinedError "Статус с таким именем уже существует"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/states [post]
func (s *Services) createState(c echo.Context) error {
	user := *c.(ProjectContext).User
	project := c.(ProjectContext).Project

	var state dao.State
	if err := json.NewDecoder(c.Request().Body).Decode(&state); err != nil {
		return EError(c, err)
	}

	state.ID = dao.GenID()
	state.ProjectId = project.ID
	state.WorkspaceId = project.WorkspaceId
	state.CreatedById = &user.ID
	state.UpdatedById = &user.ID
	state.CreatedAt = time.Now()
	state.UpdatedAt = time.Now()

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := updateStatesGroup(tx, &state, "create"); err != nil {
			return err
		}

		// Трек активности после успешного создания статуса
		//if err := s.tracker.TrackActivity(tracker.PROJECT_STATE_CREATED_ACTIVITY, StructToJSONMap(state), nil, project.ID, tracker.ENTITY_TYPE_PROJECT, &project, user); err != nil {
		//	return EError(c, err)
		//}

		return nil
	}); err != nil {
		return EError(c, err)
	}

	err := tracker.TrackActivity[dao.State, dao.ProjectActivity](s.tracker, tracker.ENTITY_CREATE_ACTIVITY, nil, nil, state, &user)
	if err != nil {
		errStack.GetError(c, err)
	}

	return c.JSON(http.StatusCreated, state.ToLightDTO())
}

// getState godoc
// @id getState
// @Summary Проекты (статусы): получение информации о статусе
// @Description Возвращает данные статуса по его ID
// @Tags Projects
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param stateId path string true "ID статуса"
// @Success 200 {object} dto.StateLight "Информация о статусе"
// @Failure 404 {object} apierrors.DefinedError "Статус не найден"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/states/{stateId} [get]
func (s *Services) getState(c echo.Context) error {
	project := c.(ProjectContext).Project
	stateId := c.Param("stateId")

	var state dao.State
	if err := s.db.
		Preload(clause.Associations).
		Where("project_id = ?", project.ID).
		Where("id = ?", stateId).
		First(&state).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return EErrorDefined(c, apierrors.ErrProjectStateNotFound)
		}
		return EError(c, err)
	}
	return c.JSON(http.StatusOK, state.ToLightDTO())
}

type UpdateStateRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	Group       *string `json:"group,omitempty"`
	Color       *string `json:"color,omitempty"`
	Default     *bool   `json:"default,omitempty"`
	SeqId       *int    `json:"group_seq_id,omitempty"`

	UpdatedAt   time.Time `json:"-"`
	UpdatedById string    `json:"-"`
	Sequence    uint64    `json:"sequence,omitempty"`
}

func (s *UpdateStateRequest) Update(fields []string, state *dao.State) {
	for _, field := range fields {
		switch field {
		case "name":
			state.Name = *s.Name
		case "description":
			state.Description = *s.Description
		case "color":
			state.Color = *s.Color
		case "updated_by":
			state.UpdatedById = &s.UpdatedById
		case "group":
			state.Group = *s.Group
		case "default":
			state.Default = *s.Default
		case "group_seq_id":
			state.SeqId = s.SeqId
		}
	}
}

// updateState godoc
// @id updateState
// @Summary Проекты (статусы): обновление статуса
// @Description Обновляет данные существующего статуса по его ID
// @Tags Projects
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param stateId path string true "ID статуса"
// @Param data body UpdateStateRequest true "Данные для обновления статуса"
// @Success 200 {object} dto.StateLight "Обновленный статус"
// @Failure 400 {object} apierrors.DefinedError "Некорректные данные для статуса"
// @Failure 404 {object} apierrors.DefinedError "Статус не найден"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/states/{stateId} [patch]
func (s *Services) updateState(c echo.Context) error {
	user := *c.(ProjectContext).User
	project := c.(ProjectContext).Project
	stateId := c.Param("stateId")

	var req UpdateStateRequest
	fields, err := BindData(c, "", &req)
	if err != nil {
		return err
	}

	var state dao.State
	if err := s.db.
		Preload(clause.Associations).
		Where("project_id = ?", project.ID).
		Where("id = ?", stateId).
		Find(&state).Error; err != nil {
		return EError(c, err)
	}

	// Pre-update activity tracking
	oldStateMap := StructToJSONMap(state)
	oldStateMap["updateScope"] = "status"
	oldStateMap["updateScopeId"] = stateId

	var currentDefaultState dao.State
	if err := s.db.
		Preload(clause.Associations).
		Where("project_id = ?", project.ID).
		Where(gorm.Expr(`"default" = ?`, true)).
		First(&currentDefaultState).Error; err == nil {
		if currentDefaultState.Name != "" {
			if req.Default != nil && *req.Default == true {
				oldStateMap["default_activity_val"] = currentDefaultState.Name
				oldStateMap["updateScopeId"] = currentDefaultState.ID
			}
			//oldStateMap["default_state"] = currentDefaultState.Name
		}
	}

	req.UpdatedById = user.ID
	fields = append(fields, "updated_by")

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if req.Default != nil && *req.Default {
			// Change other states to false default
			if err := tx.Model(&dao.State{}).
				Where("project_id = ?", project.ID).
				UpdateColumn("default", false).Error; err != nil {
				return err
			}
		}

		req.Update(fields, &state)

		if state.Sequence >= getDefaultStateSeq(state.Group) && state.Sequence < getDefaultStateSeq(state.Group)+10000 {
			if state.SeqId == nil {
				if err := tx.Select(fields).Updates(&state).Error; err != nil {
					return err
				}
				return nil
			}
			if err := updateStatesGroup(tx, &state, "update"); err != nil {
				return err
			}
		} else {
			tmpState := state
			tmpState.Group = oldStateMap["group"].(string)
			if err := updateStatesGroup(tx, &tmpState, "delete"); err != nil {
				return err
			}
			if err := updateStatesGroup(tx, &state, "create"); err != nil {
				return err
			}

		}
		return nil
	}); err != nil {
		return EError(c, err)
	}
	// Post-update activity tracking
	newStateMap := StructToJSONMap(state)

	newStateMap["updateScope"] = "status"
	newStateMap["updateScopeId"] = stateId
	if req.Default != nil && *req.Default == true {
		newStateMap["default_activity_val"] = state.Name
	}
	//nm["default_activity_val"] = state.Name
	//fmt.Println("++++++", oldStateMap["default"], newStateMap["default"])

	//if err := s.tracker.TrackActivity(tracker.PROJECT_STATE_UPDATED_ACTIVITY, newStateMap, oldStateMap, project.ID, tracker.ENTITY_TYPE_PROJECT, &project, user); err != nil {
	//	return EError(c, err)
	//}

	err = tracker.TrackActivity[dao.Project, dao.ProjectActivity](s.tracker, tracker.ENTITY_UPDATED_ACTIVITY, newStateMap, oldStateMap, project, &user)
	if err != nil {
		errStack.GetError(c, err)
	}

	return c.JSON(http.StatusOK, state.ToLightDTO())
}

// deleteState godoc
// @id deleteState
// @Summary Проекты (статусы): удаление статуса
// @Description Удаляет статус по его ID, если он не является статусом по умолчанию и не используется задачами
// @Tags Projects
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param stateId path string true "ID статуса"
// @Success 204 "Статус успешно удален"
// @Failure 400 {object} apierrors.DefinedError "Статус не может быть удален"
// @Failure 404 {object} apierrors.DefinedError "Статус не найден"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/states/{stateId} [delete]
func (s *Services) deleteState(c echo.Context) error {
	user := *c.(ProjectContext).User
	project := c.(ProjectContext).Project
	stateId := c.Param("stateId")

	var state dao.State
	if err := s.db.
		Preload(clause.Associations).
		Where("project_id = ?", project.ID).
		Where("id = ?", stateId).
		Find(&state).Error; err != nil {
		return EError(c, err)
	}

	if state.Default {
		return EErrorDefined(c, apierrors.ErrDefaultStateCannotBeDeleted)
	}

	var issueExists bool
	if err := s.db.Model(&dao.Issue{}).
		Select("count(id) > 0").
		Where("state_id = ?", state.ID).
		Find(&issueExists).Error; err != nil {
		return EError(c, err)
	}
	if issueExists {
		return EErrorDefined(c, apierrors.ErrStateNotEmptyCannotDelete)
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := updateStatesGroup(tx, &state, "delete"); err != nil {
			return err
		}

		err := tracker.TrackActivity[dao.State, dao.ProjectActivity](s.tracker, tracker.ENTITY_DELETE_ACTIVITY, nil, nil, state, &user)
		if err != nil {
			errStack.GetError(c, err)
		}

		if err := tx.Omit(clause.Associations).Delete(&state).Error; err != nil {
			return err
		}

		//if err := s.tracker.TrackActivity(tracker.PROJECT_STATE_DELETED_ACTIVITY, nil, StructToJSONMap(state), project.ID, tracker.ENTITY_TYPE_PROJECT, &project, user); err != nil {
		//	return err
		//}

		return nil
	}); err != nil {
		return EError(c, err)
	}

	return c.NoContent(http.StatusOK)
}

func updateStatesGroup(tx *gorm.DB, state *dao.State, action string) error {
	if _, ok := utils.ValidIssueStatusGroup[state.Group]; !ok {
		return apierrors.ErrProjectGroupNotFound
	}

	var groupState []dao.State

	if err := tx.Where("project_id = ?", state.ProjectId).
		Where("\"group\" = ?", state.Group).
		Order("sequence").
		Find(&groupState).Error; err != nil {
		return err
	}

	var stateSeqId int

	var remainingStates, newGroupState []dao.State
	switch action {
	case "update":
		stateSeqId = *state.SeqId
		if stateSeqId < 1 || stateSeqId > len(groupState) {
			return apierrors.ErrProjectStateInvalidSeqId
		}
		remainingStates = make([]dao.State, 0, len(groupState)-1)
		newGroupState = make([]dao.State, 0, len(groupState))
	case "delete":
		remainingStates = make([]dao.State, 0, len(groupState)-1)
		newGroupState = make([]dao.State, 0, len(groupState)-1)
	case "create":
		remainingStates = make([]dao.State, 0, len(groupState))
		newGroupState = make([]dao.State, 0, len(groupState)+1)
	}

	var updatedState dao.State
	for _, st := range groupState {
		if st.ID == state.ID {
			updatedState = *state
		} else {
			remainingStates = append(remainingStates, st)
		}
	}
	switch action {
	case "update":
		newGroupState = append(newGroupState, remainingStates[:stateSeqId-1]...)
		newGroupState = append(newGroupState, updatedState)
		newGroupState = append(newGroupState, remainingStates[stateSeqId-1:]...)
	case "delete":
		newGroupState = append(newGroupState, remainingStates...)
	case "create":
		updatedState = *state
		newGroupState = append(newGroupState, remainingStates...)
		newGroupState = append(newGroupState, updatedState)
	}

	if len(newGroupState) == 0 {
		//return ErrProjectGroupStateEmpty
		return nil
	}

	groupRange := uint64(10000)
	startSeq := getDefaultStateSeq(state.Group)
	step := groupRange / uint64(len(newGroupState))

	currentSeq := startSeq
	for i := range newGroupState {
		newGroupState[i].Sequence = currentSeq
		currentSeq += step
	}

	if err := tx.Model(&dao.State{}).Omit(clause.Associations).
		Save(&newGroupState).Error; err != nil {
		return err
	}

	return nil
}

func getDefaultStateSeq(group string) uint64 {
	switch group {
	case "backlog":
		return 15000
	case "unstarted":
		return 25000
	case "started":
		return 35000
	case "completed":
		return 45000
	case "cancelled":
		return 55000
	}
	return 5000
}

// ############# Rules log ################

// getRulesLog godoc
// @id getRulesLog
// @Summary Проекты (логи): получение логов правил
// @Description Получает логи правил проекта с возможностью фильтрации по типу и пагинации
// @Tags Projects
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param offset query int false "Смещение для пагинации" default(-1)
// @Param limit query int false "Лимит записей" default(30)
// @Param data body GetRulesLogfilterRequest true "Фильтр запроса"
// @Success 200 {object} dao.PaginationResponse{result=[]dto.RulesLog} "Список логов правил с пагинацией"
// @Failure 204 "Нет контента (пользователь не имеет доступа)"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/rules-log [post]
func (s *Services) getRulesLog(c echo.Context) error {
	project := c.(ProjectContext).Project
	projectMember := c.(ProjectContext).ProjectMember

	var param GetRulesLogfilterRequest
	offset := -1
	limit := 30

	if err := echo.QueryParamsBinder(c).
		Int("offset", &offset).
		Int("limit", &limit).BindError(); err != nil {
		return EError(c, err)
	}

	if err := c.Bind(&param); err != nil {
		return EError(c, err)
	}

	err := c.Validate(param)
	if err != nil {
		return EError(c, err)
	}

	var logs []dao.RulesLog
	var query *gorm.DB
	if projectMember.Role == types.AdminRole {
		query = s.db.
			Preload("User").
			Preload("Issue").
			Preload("Project").
			Preload("Workspace").
			Where("project_id = ?", project.ID).Order("time")
		if len(param.Select) != 0 {
			query = query.Where("type IN (?)", param.Select)
		}
	} else {
		return c.NoContent(http.StatusOK)
	}

	resp, err := dao.PaginationRequest(offset, limit, query, &logs)
	if err != nil {
		if err != gorm.ErrRecordNotFound {
			return EError(c, err)
		}
	}
	resp.Result = utils.SliceToSlice(resp.Result.(*[]dao.RulesLog), func(rl *dao.RulesLog) dto.RulesLog { return *rl.ToDTO() })

	return c.JSON(http.StatusOK, resp)
}

// JoinProjectsRequest описывает структуру запроса для подключения к проектам.
type JoinProjectsRequest struct {
	ProjectIDs []string `json:"project_ids" example:"[\"project1\", \"project2\"]"`
}

// JoinProjectsSuccessResponse описывает структуру успешного ответа.
type JoinProjectsSuccessResponse struct {
	Message string `json:"message" example:"Projects joined successfully"`
}

// CheckProjectIdentifierAvailabilityResponse описывает структуру успешного ответа.
type CheckProjectIdentifierAvailabilityResponse struct {
	Exists      int      `json:"exists" example:"1"`
	Identifiers []string `json:"identifiers" example:"[\"PROJECT1\", \"PROJECT2\"]"`
}

// AddProjectToFavoritesRequest описывает структуру запроса для метода addProjectToFavorites.
type AddProjectToFavoritesRequest struct {
	ProjectID string `json:"project" example:"project123" validate:"required"`
}

// EstimatePayload описывает структуру данных для создания и обновления оценки.
type EstimatePayload struct {
	Estimate       dao.Estimate        `json:"estimate"`
	EstimatePoints []dao.EstimatePoint `json:"estimate_points"`
}

type EstimatePayloadResponse struct {
	Estimate       *dto.Estimate       `json:"estimate,omitempty"`
	EstimatePoints []dto.EstimatePoint `json:"estimate_points"`
}

// GetRulesLogfilterRequest описывает структуру запроса для метода GetRulesLog.
type GetRulesLogfilterRequest struct {
	Select []string `json:"select" validate:"omitempty,dive,oneof=error print success fail"`
}

type projectNotificationRequest struct {
	NotificationSettingsTG          types.ProjectMemberNS `json:"notification_settings_tg"`
	NotificationAuthorSettingsTG    types.ProjectMemberNS `json:"notification_author_settings_tg"`
	NotificationSettingsEmail       types.ProjectMemberNS `json:"notification_settings_email"`
	NotificationAuthorSettingsEmail types.ProjectMemberNS `json:"notification_author_settings_email"`
	NotificationSettingsApp         types.ProjectMemberNS `json:"notification_settings_app"`
	NotificationAuthorSettingsApp   types.ProjectMemberNS `json:"notification_author_settings_app"`
}

// ############# Issue Templates methods ###################

// getProjectIssueTemplates godoc
// @id getProjectIssueTemplates
// @Summary Проекты (шаблоны задач): получение списка шаблонов задач
// @Description Возвращает список шаблонов задач с пагинацией.
// @Tags Projects
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param offset query int false "Смещение для пагинации" default(-1)
// @Param limit query int false "Количество участников на странице" default(100)
// @Success 200 {object} dao.PaginationResponse{result=[]dto.IssueTemplate} "Список шаблонов задач"
// @Failure 400 {object} apierrors.DefinedError "Ошибка запроса"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/templates [get]
func (s *Services) getProjectIssueTemplates(c echo.Context) error {
	project := c.(ProjectContext).Project

	offset := 0
	limit := 100

	if err := echo.QueryParamsBinder(c).
		Int("offset", &offset).
		Int("limit", &limit).
		BindError(); err != nil {
		return EError(c, err)
	}

	query := s.db.Where("workspace_id = ?", project.WorkspaceId).Where("project_id = ?", project.ID).Order("created_at desc")

	var templates []dao.IssueTemplate
	resp, err := dao.PaginationRequest(offset, limit, query, &templates)
	if err != nil {
		return EError(c, err)
	}

	resp.Result = utils.SliceToSlice(resp.Result.(*[]dao.IssueTemplate), func(t *dao.IssueTemplate) dto.IssueTemplate {
		return *t.ToDTO()
	})

	return c.JSON(http.StatusOK, resp)
}

// createIssueTemplate godoc
// @id createIssueTemplate
// @Summary Проекты (шаблоны задач): создание шаблона
// @Description Создает новый шаблон задач для проекта
// @Tags Projects
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param data body dto.IssueTemplate true "Данные шаблона"
// @Success 201 "Шаблон создан"
// @Failure 400 {object} apierrors.DefinedError "Некорректные данные шаблона"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/templates [post]
func (s *Services) createIssueTemplate(c echo.Context) error {
	project := c.(ProjectContext).Project
	user := c.(ProjectContext).User

	var req dto.IssueTemplate
	if err := c.Bind(&req); err != nil {
		return EError(c, err)
	}

	it := dao.IssueTemplate{
		Id:          dao.GenUUID(),
		CreatedById: uuid.Must(uuid.FromString(user.ID)),
		UpdatedById: uuid.Must(uuid.FromString(user.ID)),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		WorkspaceId: uuid.Must(uuid.FromString(project.WorkspaceId)),
		ProjectId:   uuid.Must(uuid.FromString(project.ID)),
		Name:        req.Name,
		Template:    req.Template,
	}

	if err := s.db.Create(&it).Error; err != nil {
		if err == gorm.ErrDuplicatedKey {
			return EErrorDefined(c, apierrors.ErrIssueTemplateDuplicatedName)
		}
		return EError(c, err)
	}

	err := tracker.TrackActivity[dao.IssueTemplate, dao.ProjectActivity](s.tracker, tracker.ENTITY_CREATE_ACTIVITY, nil, nil, it, user)
	if err != nil {
		errStack.GetError(c, err)
	}

	return c.NoContent(http.StatusCreated)
}

// getIssueTemplate godoc
// @id getIssueTemplate
// @Summary Проекты (шаблоны задач): получение информации о шаблоне
// @Description Возвращает данные шаблона задач по его ID
// @Tags Projects
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param templateId path string true "ID шаблона"
// @Success 200 {object} dto.IssueTemplate "Информация о шаблоне"
// @Failure 404 {object} apierrors.DefinedError "Шаблон не найден"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/templates/{templateId} [get]
func (s *Services) getIssueTemplate(c echo.Context) error {
	project := c.(ProjectContext).Project
	templateId := c.Param("templateId")

	var issueTemplate dto.IssueTemplate
	if err := s.db.Model(&dao.IssueTemplate{}).
		Where("workspace_id = ?", project.WorkspaceId).Where("project_id = ?", project.ID).Where("id = ?", templateId).
		First(&issueTemplate).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return EErrorDefined(c, apierrors.ErrIssueTemplateNotFound)
		}
		return EError(c, err)
	}

	return c.JSON(http.StatusOK, issueTemplate)
}

// updateIssueTemplate godoc
// @id updateIssueTemplate
// @Summary Проекты (шаблоны задач): обновление шаблона
// @Description Обновляет шаблон задач для проекта
// @Tags Projects
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param templateId path string true "ID шаблона"
// @Param data body dto.IssueTemplate true "Данные шаблона"
// @Success 200 "Шаблон обновлен"
// @Failure 400 {object} apierrors.DefinedError "Некорректные данные шаблона"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/templates/{templateId} [patch]
func (s *Services) updateIssueTemplate(c echo.Context) error {
	project := c.(ProjectContext).Project
	user := c.(ProjectContext).User
	templateId := c.Param("templateId")

	var template dao.IssueTemplate
	if err := s.db.Where("id = ?", templateId).
		Where("project_id = ?", project.ID).
		Find(&template).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return EErrorDefined(c, apierrors.ErrIssueTemplateNotFound)
		}
		return EError(c, err)
	}

	oldTemplateMap := StructToJSONMap(template)
	oldTemplateMap["updateScope"] = "template"
	oldTemplateMap["updateScopeId"] = templateId

	var req dto.IssueTemplate
	if err := c.Bind(&req); err != nil {
		return EError(c, err)
	}

	var fields []string
	if req.Name != template.Name {
		fields = append(fields, "name")
		template.Name = req.Name
	}
	if req.Template != template.Template {
		fields = append(fields, "template")
		template.Template = req.Template
	}

	if len(fields) > 0 {
		fields = append(fields, "updated_by_id")
		template.UpdatedById = uuid.Must(uuid.FromString(user.ID))
		if err := s.db.
			Select(fields).
			Updates(&template).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				return EErrorDefined(c, apierrors.ErrIssueTemplateDuplicatedName)
			}
			return EError(c, err)
		}
		newTemplateMap := StructToJSONMap(template)
		newTemplateMap["updateScope"] = "template"
		newTemplateMap["updateScopeId"] = templateId

		err := tracker.TrackActivity[dao.Project, dao.ProjectActivity](s.tracker, tracker.ENTITY_UPDATED_ACTIVITY, newTemplateMap, oldTemplateMap, project, user)
		if err != nil {
			errStack.GetError(c, err)
			//return err
		}
	}

	return c.NoContent(http.StatusOK)
}

// deleteIssueTemplate godoc
// @id deleteIssueTemplate
// @Summary Проекты (шаблоны задач): удаление шаблона
// @Description Удаляет шаблон задач для проекта
// @Tags Projects
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param templateId path string true "ID шаблона"
// @Success 200 "Шаблон удален"
// @Failure 400 {object} apierrors.DefinedError "Некорректные данные шаблона"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/templates/{templateId} [delete]
func (s *Services) deleteIssueTemplate(c echo.Context) error {
	project := c.(ProjectContext).Project
	user := c.(ProjectContext).User
	templateId := c.Param("templateId")

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		var template dao.IssueTemplate
		if err := tx.
			Where("workspace_id = ?", project.WorkspaceId).
			Where("project_id = ?", project.ID).
			Where("id = ?", templateId).
			First(&template).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return EErrorDefined(c, apierrors.ErrIssueTemplateNotFound)
			}
			return EError(c, err)
		}
		err := tracker.TrackActivity[dao.IssueTemplate, dao.ProjectActivity](s.tracker, tracker.ENTITY_DELETE_ACTIVITY, nil, nil, template, user)
		if err != nil {
			errStack.GetError(c, err)
			return err
		}

		if err := s.db.
			Delete(&template).Error; err != nil {
			return EError(c, err)
		}

		return nil
	}); err != nil {
		return EError(c, err)
	}
	return c.NoContent(http.StatusOK)
}

/*
// ############# Issue properties methods ###################

// Получение параметров задач
func (s *Services) issuePropertiesList(c echo.Context) error {
	user := *c.(AuthContext).User
	slug := c.Param("workspaceSlug")
	projectId := c.Param("projectId")

	var issueProperty dao.IssueProperty
	if err := s.db.Joins("Workspace").
		Where("slug = ?", slug).
		Where("project_id = ?", projectId).
		Where("user_id = ?", user.ID).Find(&issueProperty).Error; err != nil {
		return EError(c, err)
	}
	if issueProperty.Id == "" {
		res := make([]string, 0)
		return c.JSON(http.StatusOK, res)
	}

	return c.JSON(http.StatusOK, issueProperty)
}

// Создание параметров задач
func (s *Services) issuePropertyCreateOrUpdate(c echo.Context) error {
	user := *c.(AuthContext).User
	slug := c.Param("workspaceSlug")
	projectId := c.Param("projectId")

	var project dao.Project
	if err := s.db.Where("projects.id = ?", projectId).
		Where("slug = ?", slug).
		Joins("Workspace").Find(&project).Error; err != nil {
		return EError(c, err)
	}

	status := http.StatusOK

	var issueProperty dao.IssueProperty
	if err := s.db.Joins("Workspace").
		Where("slug = ?", slug).
		Where("project_id = ?", projectId).
		Where("user_id = ?", user.ID).Find(&issueProperty).Error; err != nil {
		return EError(c, err)
	}
	if issueProperty.Id == "" {
		issueProperty = dao.IssueProperty{
			Id:          dao.GenID(),
			CreatedById: &user.ID,
			CreatedAt:   time.Now(),
			UpdatedById: &user.ID,
			UpdatedAt:   time.Now(),
			ProjectId:   projectId,
			WorkspaceId: project.WorkspaceId,
			UserId:      user.ID,
		}
		status = http.StatusCreated
	}

	if err := c.Bind(&issueProperty); err != nil {
		return EError(c, err)
	}

	if err := s.db.Omit(clause.Associations).Save(&issueProperty).Error; err != nil {
		return EError(c, err)
	}
	return c.JSON(status, issueProperty)
}

// Удаление параметров задачи
func (s *Services) issuePropertyDelete(c echo.Context) error {
	projectId := c.Param("projectId")
	propertyId := c.Param("propertyId")

	if err := s.db.Where("project_id = ?", projectId).Where("id = ?", propertyId).Delete(&dao.IssueProperty{}).Error; err != nil {
		return EError(c, err)
	}
	return c.NoContent(http.StatusOK)
}
*/
