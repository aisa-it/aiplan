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

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/apierrors"
	errStack "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/stack-error"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/limiter"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dto"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"

	tracker "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/activity-tracker"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	filestorage "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/file-storage"
	"github.com/gofrs/uuid"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

		/*
			if etag := c.Request().Header.Get("If-None-Match"); etag != "" {
			   			var exist bool
			   			if err := s.db.Model(&dao.Project{}).
			   				Select("EXISTS(?)",
			   					s.db.Model(&dao.Project{}).
			   						Select("1").
			   						Where("encode(hash, 'hex') = ?", etag),
			   				).
			   				Find(&exist).Error; err != nil {
			   				return EError(c, err)
			   			}

			   			if exist {
			   				return c.NoContent(http.StatusNotModified)
			   			}
			   		}
		*/

		// Joins faster than Preload(clause.Associations)
		var project dao.Project
		projectQuery := s.db.
			Joins("ProjectLead").
			Where("projects.workspace_id = ?", workspace.ID).
			Set("userId", user.ID).
			Preload("DefaultAssigneesDetails", "is_default_assignee = ?", true).
			Preload("DefaultWatchersDetails", "is_default_watcher = ?", true)

		// Search by id or identifier
		if id, err := uuid.FromString(projectId); err == nil {
			projectQuery = projectQuery.Where("projects.id = ?", id)
		} else {
			projectQuery = projectQuery.Where("projects.identifier = ?", projectId)
		}

		if err := projectQuery.First(&project).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return EErrorDefined(c, apierrors.ErrProjectNotFound)
			}
			return EError(c, err)
		}

		var projectMember dao.ProjectMember
		if err := s.db.Where("project_id = ?", project.ID).Where("member_id = ?", user.ID).First(&projectMember).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return EErrorDefined(c, apierrors.ErrProjectNotFound)
			}
			return EError(c, err)
		}

		project.Workspace = &workspace

		return next(ProjectContext{c.(WorkspaceContext), project, projectMember})
	}
}

func (s *Services) AddProjectServices(g *echo.Group) {
	workspaceGroup := g.Group("workspaces/:workspaceSlug", s.WorkspaceMiddleware)
	workspaceGroup.Use(s.LastVisitedWorkspaceMiddleware)

	projectGroup := workspaceGroup.Group("/projects/:projectId", s.ProjectMiddleware)
	projectGroup.Use(s.ProjectPermissionMiddleware)

	workspaceGroup.Use(s.WorkspacePermissionMiddleware)

	projectAdminGroup := projectGroup.Group("", s.ProjectAdminPermissionMiddleware)

	workspaceGroup.GET("/projects/", s.getProjectList)
	workspaceGroup.POST("/projects/", s.createProject)

	projectGroup.GET("/", s.getProject)
	projectGroup.PATCH("/", s.updateProject)
	projectGroup.DELETE("/", s.deleteProject)

	projectGroup.GET("/activities/", s.getProjectActivityList)

	projectGroup.POST("/logo/", s.updateProjectLogo)
	projectGroup.DELETE("/logo/", s.deleteProjectLogo)

	workspaceGroup.GET("/project-identifiers/", s.checkProjectIdentifierAvailability)

	projectGroup.GET("/members/", s.getProjectMemberList)
	projectGroup.GET("/members/me/", s.getProjectCurrentMembership)
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

	// Rules Script (только для админов проекта)
	projectAdminGroup.GET("/rules-script/", s.getProjectRulesScript)
	projectAdminGroup.PUT("/rules-script/", s.updateProjectRulesScript)
	projectAdminGroup.DELETE("/rules-script/", s.deleteProjectRulesScript)

	// Issue Templates
	projectGroup.GET("/templates/", s.getProjectIssueTemplates)
	projectAdminGroup.POST("/templates/", s.createIssueTemplate)
	projectGroup.GET("/templates/:templateId/", s.getIssueTemplate)
	projectAdminGroup.PATCH("/templates/:templateId/", s.updateIssueTemplate)
	projectAdminGroup.DELETE("/templates/:templateId/", s.deleteIssueTemplate)

	projectGroup.GET("/stats/", s.getProjectStats)

	// Property Templates (шаблоны полей проекта)
	projectGroup.GET("/property-templates/", s.getPropertyTemplateList)
	projectAdminGroup.POST("/property-templates/", s.createPropertyTemplate)
	projectAdminGroup.PATCH("/property-templates/:templateId/", s.updatePropertyTemplate)
	projectAdminGroup.DELETE("/property-templates/:templateId/", s.deletePropertyTemplate)
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
					s.db.Raw("EXISTS(SELECT 1 FROM project_favorites WHERE project_favorites.project_id = projects.id AND user_id = ?)", user.ID)).
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

var allowedFields []string = []string{"name", "description", "description_text", "description_html", "public", "identifier", "default_assignees", "default_watchers", "project_lead_id", "emoji", "cover_image", "rules_script", "hide_fields"}

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
	project.UpdatedById = uuid.NullUUID{UUID: user.ID, Valid: true}
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

	err = tracker.TrackActivity[dao.Project, dao.ProjectActivity](s.tracker, activities.EntityUpdatedActivity, newProjectMap, oldProjectMap, project, user)
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

	if !limiter.Limiter.CanCreateProject(workspace.ID) {
		return EErrorDefined(c, apierrors.ErrProjectLimitExceed)
	}

	project := dao.Project{
		ID:          dao.GenUUID(),
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
		if project.ProjectLeadId == uuid.Nil {
			project.ProjectLeadId = user.ID
		}

		// Create project
		if err := tx.Create(&project).Error; err != nil {
			return err
		}

		return tx.Create(&[]dao.State{
			{
				ID: dao.GenUUID(), ProjectId: project.ID, WorkspaceId: workspace.ID, CreatedById: uuid.NullUUID{UUID: user.ID, Valid: true},
				Name:     "Новая",
				Color:    "#26b5ce",
				Sequence: 15000,
				Group:    "backlog",
				Default:  true,
			},
			{
				ID: dao.GenUUID(), ProjectId: project.ID, WorkspaceId: workspace.ID, CreatedById: uuid.NullUUID{UUID: user.ID, Valid: true},
				Name:     "Открыта",
				Color:    "#f2c94c",
				Sequence: 25000,
				Group:    "unstarted",
			},
			{
				ID: dao.GenUUID(), ProjectId: project.ID, WorkspaceId: workspace.ID, CreatedById: uuid.NullUUID{UUID: user.ID, Valid: true},
				Name:     "В работе",
				Color:    "#5e6ad2",
				Sequence: 35000,
				Group:    "started",
			},
			{
				ID: dao.GenUUID(), ProjectId: project.ID, WorkspaceId: workspace.ID, CreatedById: uuid.NullUUID{UUID: user.ID, Valid: true},
				Name:     "Выполнена",
				Color:    "#4cb782",
				Sequence: 45000,
				Group:    "completed",
			},
			{
				ID: dao.GenUUID(), ProjectId: project.ID, WorkspaceId: workspace.ID, CreatedById: uuid.NullUUID{UUID: user.ID, Valid: true},
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

	err := tracker.TrackActivity[dao.Project, dao.WorkspaceActivity](s.tracker, activities.EntityCreateActivity, nil, nil, project, user)
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
// @Success 200 {object} dto.CheckProjectIdentifierAvailabilityResponse "Статус доступности идентификатора"
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

	response := dto.CheckProjectIdentifierAvailabilityResponse{
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

// getProjectCurrentMembership godoc
// @id getProjectCurrentMembership
// @Summary Проекты (участники): получение информации о текущем членстве в проекте
// @Description Возвращает информацию о членстве текущего пользователя в указанном проекте.
// @Tags Projects
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Success 200 {object} dto.ProjectMember "Информация о членстве пользователя в проекте"
// @Failure 404 {object} apierrors.DefinedError "Членство в проекте не найдено"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/members/me [get]
func (s *Services) getProjectCurrentMembership(c echo.Context) error {
	member := c.(ProjectContext).ProjectMember
	return c.JSON(http.StatusOK, member.ToDTO())
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
	if err := s.db.Model(&dao.WorkspaceMember{}).
		Select("EXISTS(?)",
			s.db.Model(&dao.WorkspaceMember{}).
				Select("1").
				Where("role = ?", types.AdminRole).
				Where("workspace_id = ?", project.WorkspaceId).
				Where("member_id = ?", requestedProjectMember.MemberId),
		).
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

	userID := uuid.NullUUID{UUID: user.ID, Valid: true}
	oldMemberMap := StructToJSONMap(requestedProjectMember)
	requestedProjectMember.Role = data["role"]
	requestedProjectMember.UpdatedById = userID

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if requestedProjectMember.Role == types.GuestRole {
			if requestedProjectMember.IsDefaultAssignee {
				requestedProjectMember.IsDefaultAssignee = false
				var defaultAssignees []uuid.UUID
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

	err := tracker.TrackActivity[dao.Project, dao.ProjectActivity](s.tracker, activities.EntityUpdatedActivity, newMemberMap, oldMemberMap, project, &user)
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

	if projectMember.Role == 0 || projectMember.MemberId == uuid.Nil {
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
	if err := s.db.Model(&dao.ProjectMember{}).
		Select("EXISTS(?)",
			s.db.Model(&dao.ProjectMember{}).
				Select("1").
				Where("member_id = ?", projectMember.MemberId).
				Where("project_id = ?", project.ID),
		).
		Find(&exists).Error; err != nil {
		return EError(c, err)
	}
	if exists {
		return EErrorDefined(c, apierrors.ErrUserAlreadyInProject)
	}

	userID := uuid.NullUUID{UUID: user.ID, Valid: true}
	projectMember.ID = dao.GenUUID()
	projectMember.ProjectId = project.ID
	projectMember.CreatedById = userID
	projectMember.WorkspaceId = workspaceMember.WorkspaceId
	projectMember.ViewProps = types.DefaultViewProps
	projectMember.NotificationAuthorSettingsEmail = types.DefaultProjectMemberNS
	projectMember.NotificationAuthorSettingsApp = types.DefaultProjectMemberNS
	projectMember.NotificationAuthorSettingsTG = types.DefaultProjectMemberNS
	projectMember.NotificationSettingsEmail = types.DefaultProjectMemberNS
	projectMember.NotificationSettingsApp = types.DefaultProjectMemberNS
	projectMember.NotificationSettingsTG = types.DefaultProjectMemberNS

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

	err := tracker.TrackActivity[dao.ProjectMember, dao.ProjectActivity](s.tracker, activities.EntityAddActivity, newMemberMap, nil, projectMember, &user)
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
// @Success 201 {object} dto.JoinProjectsSuccessResponse "Сообщение об успешном подключении к проектам"
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

		userID := uuid.NullUUID{UUID: user.ID, Valid: true}
		memberships = append(memberships, dao.ProjectMember{
			ID:                              dao.GenUUID(),
			ProjectId:                       uuid.Must(uuid.FromString(projectId)),
			MemberId:                        user.ID,
			Role:                            role,
			WorkspaceId:                     workspaceMember.WorkspaceId,
			CreatedById:                     userID,
			CreatedAt:                       time.Now(),
			ViewProps:                       types.DefaultViewProps,
			NotificationAuthorSettingsEmail: types.DefaultProjectMemberNS,
			NotificationAuthorSettingsApp:   types.DefaultProjectMemberNS,
			NotificationAuthorSettingsTG:    types.DefaultProjectMemberNS,
			NotificationSettingsEmail:       types.DefaultProjectMemberNS,
			NotificationSettingsApp:         types.DefaultProjectMemberNS,
			NotificationSettingsTG:          types.DefaultProjectMemberNS,
		})
	}

	if err := s.db.Create(&memberships).Error; err != nil {
		return EError(c, err)
	}

	return c.JSON(http.StatusCreated, dto.JoinProjectsSuccessResponse{Message: "Projects joined successfully"})
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

	userId := user.ID
	userID := uuid.NullUUID{UUID: user.ID, Valid: true}
	project, err := dao.GetProject(s.db, workspace.Slug, userId, projectID)
	if err != nil {
		return EError(c, err)
	}

	projectFavorite := dao.ProjectFavorites{
		Id:          dao.GenUUID(),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		CreatedById: userID,
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

	userId := user.ID
	project, err := dao.GetProject(s.db, workspace.Slug, userId, projectId)
	if err != nil {
		return EError(c, err)
	}

	if err := s.db.Where("project_id = ?", project.ID).
		Where("user_id = ?", userId).
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

	userID := uuid.NullUUID{UUID: user.ID, Valid: true}
	data.Estimate.Id = dao.GenUUID()
	data.Estimate.CreatedAt = time.Now()
	data.Estimate.CreatedById = userID
	data.Estimate.UpdatedAt = time.Now()
	data.Estimate.UpdatedById = userID
	data.Estimate.WorkspaceId = project.WorkspaceId
	data.Estimate.ProjectId = project.ID

	if err := s.db.Create(&data.Estimate).Error; err != nil {
		return EError(c, err)
	}

	for i := 0; i < len(data.EstimatePoints); i++ {
		data.EstimatePoints[i].Id = dao.GenUUID()
		data.EstimatePoints[i].CreatedAt = time.Now()
		data.EstimatePoints[i].CreatedById = userID
		data.EstimatePoints[i].UpdatedAt = time.Now()
		data.EstimatePoints[i].UpdatedById = userID
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

	BlockersList  []uuid.UUID `json:"blockers_list"`
	AssigneesList []uuid.UUID `json:"assignees_list"`
	WatchersList  []uuid.UUID `json:"watchers_list"`
	LabelsList    []uuid.UUID `json:"labels_list"`
	BlocksList    []uuid.UUID `json:"blocks_list"`
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
// @Success 201 {object} dto.NewIssueID "ID созданной задачи"
// @Failure 400 {object} apierrors.DefinedError "Неверные данные задачи"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/issues/ [post]
func (s *Services) createIssue(c echo.Context) error {
	user := *c.(ProjectContext).User
	workspace := c.(ProjectContext).Workspace
	project := c.(ProjectContext).Project

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

		if !limiter.Limiter.CanAddAttachment(workspace.ID) {
			return EErrorDefined(c, apierrors.ErrAssetsLimitExceed)
		}
	}
	if len(strings.TrimSpace(issue.Name)) == 0 {
		return EErrorDefined(c, apierrors.ErrIssueNameEmpty)
	}

	userID := uuid.NullUUID{UUID: user.ID, Valid: true}
	issueNew := dao.Issue{
		ID:                  dao.GenUUID(),
		Name:                issue.Name,
		Priority:            issue.Priority,
		StartDate:           issue.StartDate,
		TargetDate:          issue.TargetDate,
		CompletedAt:         issue.CompletedAt,
		SequenceId:          issue.SequenceId,
		CreatedById:         user.ID,
		ParentId:            issue.ParentId,
		ProjectId:           project.ID,
		StateId:             issue.StateId,
		UpdatedById:         uuid.NullUUID{UUID: user.ID, Valid: true},
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

			if err := notifications.CreateDeadlineNotification(tx, &issueNew, &dateStr, userIds); err != nil {
				return err
			}
		}
		if form != nil {
			// Save inline attachments
			for _, f := range form.File["files"] {
				fileAsset := dao.FileAsset{
					Id:          dao.GenUUID(),
					CreatedAt:   time.Now(),
					CreatedById: userID,
					Name:        f.Filename,
					FileSize:    int(f.Size),
					WorkspaceId: uuid.NullUUID{UUID: issueNew.WorkspaceId, Valid: true},
					IssueId:     uuid.NullUUID{Valid: true, UUID: issueNew.ID},
				}

				if err := s.uploadAssetForm(tx, f, &fileAsset,
					filestorage.Metadata{
						WorkspaceId: issueNew.WorkspaceId.String(),
						ProjectId:   issueNew.ProjectId.String(),
						IssueId:     issueNew.ID.String(),
					}); err != nil {
					return err
				}

				issueNew.InlineAttachments = append(issueNew.InlineAttachments, fileAsset)
			}
		}

		// Fill params

		// Add blockers
		if len(issue.BlockersList) > 0 {
			newBlockers := make([]dao.IssueBlocker, len(issue.BlockersList))
			for i, blocker := range issue.BlockersList {
				newBlockers[i] = dao.IssueBlocker{
					Id:          dao.GenUUID(),
					BlockedById: blocker,
					BlockId:     issueNew.ID,
					ProjectId:   project.ID,
					WorkspaceId: issueNew.WorkspaceId,
					CreatedById: userID,
					UpdatedById: userID,
				}
			}
			if err := tx.CreateInBatches(&newBlockers, 10).Error; err != nil {
				return err
			}
		}

		// Add assignees
		if len(issue.AssigneesList) > 0 {
			issue.AssigneesList = utils.SetToSlice(utils.SliceToSet(issue.AssigneesList))
			newAssignees := make([]dao.IssueAssignee, len(issue.AssigneesList))
			for i, assignee := range issue.AssigneesList {
				newAssignees[i] = dao.IssueAssignee{
					Id:          dao.GenUUID(),
					AssigneeId:  assignee,
					IssueId:     issueNew.ID,
					ProjectId:   project.ID,
					WorkspaceId: issueNew.WorkspaceId,
					CreatedById: userID,
					UpdatedById: userID,
				}
			}
			if err := tx.CreateInBatches(&newAssignees, 10).Error; err != nil {
				return err
			}
		}

		// Add watchers
		if len(issue.WatchersList) > 0 {
			issue.WatchersList = utils.SetToSlice(utils.SliceToSet(issue.WatchersList))
			newWatchers := make([]dao.IssueWatcher, len(issue.WatchersList))
			for i, watcher := range issue.WatchersList {
				newWatchers[i] = dao.IssueWatcher{
					Id:          dao.GenUUID(),
					WatcherId:   watcher,
					IssueId:     issueNew.ID,
					ProjectId:   project.ID,
					WorkspaceId: issueNew.WorkspaceId,
					CreatedById: userID,
					UpdatedById: userID,
				}
			}
			if err := tx.CreateInBatches(&newWatchers, 10).Error; err != nil {
				return err
			}
		}

		// Add labels
		if len(issue.LabelsList) > 0 {
			newLabels := make([]dao.IssueLabel, len(issue.LabelsList))
			for i, label := range issue.LabelsList {
				newLabels[i] = dao.IssueLabel{
					Id:          dao.GenUUID(),
					LabelId:     uuid.Must(uuid.FromString(fmt.Sprint(label))),
					IssueId:     issueNew.ID,
					ProjectId:   project.ID,
					WorkspaceId: issueNew.WorkspaceId,
					CreatedById: userID,
					UpdatedById: userID,
				}
			}
			if err := tx.CreateInBatches(&newLabels, 10).Error; err != nil {
				return err
			}
		}

		// Add blocked
		if len(issue.BlocksList) > 0 {
			newBlocked := make([]dao.IssueBlocker, len(issue.BlocksList))
			for i, block := range issue.BlocksList {
				newBlocked[i] = dao.IssueBlocker{
					Id:          dao.GenUUID(),
					BlockId:     block,
					BlockedById: issueNew.ID,
					ProjectId:   project.ID,
					WorkspaceId: issueNew.WorkspaceId,
					CreatedById: userID,
					UpdatedById: userID,
				}
			}
			if err := tx.CreateInBatches(&newBlocked, 10).Error; err != nil {
				return err
			}
		}
		if issue.Links != nil && len(issue.Links) > 0 {
			newLinks := make([]dao.IssueLink, len(issue.Links))
			for i, link := range issue.Links {
				newLinks[i] = dao.IssueLink{
					Id:          dao.GenUUID(),
					Title:       link.Title,
					Url:         link.Url,
					CreatedById: userID,
					UpdatedById: userID,
					IssueId:     issueNew.ID,
					ProjectId:   project.ID,
					WorkspaceId: project.WorkspaceId,
				}
			}

			if err := tx.CreateInBatches(newLinks, 10).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return EError(c, err)
	}

	issueNew.Project = &project
	err := tracker.TrackActivity[dao.Issue, dao.ProjectActivity](s.tracker, activities.EntityCreateActivity, nil, nil, issueNew, &user)
	if err != nil {
		errStack.GetError(c, err)
	}
	if issueNew.ParentId.Valid {

		data := make(map[string]interface{})
		oldData := make(map[string]interface{})

		oldData["parent"] = uuid.NullUUID{}
		data["parent"] = issueNew.ParentId.UUID

		err := tracker.TrackActivity[dao.Issue, dao.IssueActivity](s.tracker, activities.EntityUpdatedActivity, data, oldData, issueNew, &user)
		if err != nil {
			errStack.GetError(c, err)
		}
	}

	return c.JSON(http.StatusCreated, dto.NewIssueID{Id: issueNew.ID})
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

	label.ID = dao.GenUUID()
	label.CreatedAt = time.Now()
	label.UpdatedAt = time.Now()
	label.CreatedById = uuid.NullUUID{UUID: user.ID, Valid: true}
	label.UpdatedById = uuid.NullUUID{UUID: user.ID, Valid: true}
	label.ProjectId = project.ID
	label.WorkspaceId = project.WorkspaceId

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&label).Error; err != nil {
			if err == gorm.ErrDuplicatedKey {
				return err
			}
			return err
		}

		return nil
	}); err != nil {
		if err == gorm.ErrDuplicatedKey {
			return EErrorDefined(c, apierrors.ErrTagAlreadyExists)
		}
		return EError(c, err)
	}

	err := tracker.TrackActivity[dao.Label, dao.ProjectActivity](s.tracker, activities.EntityCreateActivity, nil, nil, label, &user)
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

	label.UpdatedById = uuid.NullUUID{UUID: user.ID, Valid: true}
	label.UpdatedAt = time.Now()

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		// TODO rate limit
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
		newLabelMap["updateScopeId"] = label.ID

		err := tracker.TrackActivity[dao.Project, dao.ProjectActivity](s.tracker, activities.EntityUpdatedActivity, newLabelMap, oldLabelMap, project, &user)
		if err != nil {
			errStack.GetError(c, err)
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

	var issueExists bool
	if err := s.db.Model(&dao.IssueLabel{}).
		Select("EXISTS(?)",
			s.db.Model(&dao.IssueLabel{}).
				Select("1").
				Where("label_id = ?", label.ID),
		).
		Find(&issueExists).Error; err != nil {
		return EError(c, err)
	}
	if issueExists {
		return EErrorDefined(c, apierrors.ErrLabelNotEmptyCannotDelete)
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		err := tracker.TrackActivity[dao.Label, dao.ProjectActivity](s.tracker, activities.EntityDeleteActivity, nil, nil, label, &user)
		if err != nil {
			errStack.GetError(c, err)
			return err
		}

		return s.db.Delete(&label).Error
	}); err != nil {
		return EError(c, err)
	}

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

	state.ID = dao.GenUUID()
	state.ProjectId = project.ID
	state.WorkspaceId = project.WorkspaceId
	state.CreatedById = uuid.NullUUID{UUID: user.ID, Valid: true}
	state.UpdatedById = uuid.NullUUID{UUID: user.ID, Valid: true}
	state.CreatedAt = time.Now()
	state.UpdatedAt = time.Now()

	if err := updateStatesGroup(s.db, &state, "create"); err != nil {
		return EError(c, err)
	}

	err := tracker.TrackActivity[dao.State, dao.ProjectActivity](s.tracker, activities.EntityCreateActivity, nil, nil, state, &user)
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
	stateId, err := uuid.FromString(c.Param("stateId"))
	if err != nil {
		return EErrorDefined(c, apierrors.ErrProjectStateNotFound)
	}

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
	UpdatedById uuid.UUID `json:"-"`
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
			state.UpdatedById = uuid.NullUUID{UUID: s.UpdatedById, Valid: true}
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
	stateId, err := uuid.FromString(c.Param("stateId"))
	if err != nil {
		return EErrorDefined(c, apierrors.ErrProjectStateNotFound)
	}

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
	//TODO rate limit

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

	err = tracker.TrackActivity[dao.Project, dao.ProjectActivity](s.tracker, activities.EntityUpdatedActivity, newStateMap, oldStateMap, project, &user)
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
	stateId, err := uuid.FromString(c.Param("stateId"))
	if err != nil {
		return EErrorDefined(c, apierrors.ErrProjectStateNotFound)
	}

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
		Select("EXISTS(?)",
			s.db.Model(&dao.Issue{}).
				Select("1").
				Where("state_id = ?", state.ID),
		).
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

		err := tracker.TrackActivity[dao.State, dao.ProjectActivity](s.tracker, activities.EntityDeleteActivity, nil, nil, state, &user)
		if err != nil {
			errStack.GetError(c, err)
			return err
		}

		return tx.Omit(clause.Associations).Delete(&state).Error
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
		CreatedById: user.ID,
		UpdatedById: user.ID,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		WorkspaceId: project.WorkspaceId,
		ProjectId:   project.ID,
		Name:        req.Name,
		Template:    req.Template,
	}

	if err := s.db.Create(&it).Error; err != nil {
		if err == gorm.ErrDuplicatedKey {
			return EErrorDefined(c, apierrors.ErrIssueTemplateDuplicatedName)
		}
		return EError(c, err)
	}

	err := tracker.TrackActivity[dao.IssueTemplate, dao.ProjectActivity](s.tracker, activities.EntityCreateActivity, nil, nil, it, user)
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
	oldTemplateMap["updateScopeId"] = template.Id
	// TODO rate limit
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
		template.UpdatedById = user.ID
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
		newTemplateMap["updateScopeId"] = template.Id

		err := tracker.TrackActivity[dao.Project, dao.ProjectActivity](s.tracker, activities.EntityUpdatedActivity, newTemplateMap, oldTemplateMap, project, user)
		if err != nil {
			errStack.GetError(c, err)
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

		if err := s.db.Transaction(func(tx *gorm.DB) error {
			err := tracker.TrackActivity[dao.IssueTemplate, dao.ProjectActivity](s.tracker, activities.EntityDeleteActivity, nil, nil, template, user)
			if err != nil {
				errStack.GetError(c, err)
				return err
			}

			return s.db.
				Delete(&template).Error
		}); err != nil {
			return EError(c, err)
		}

		return nil
	}); err != nil {
		return EError(c, err)
	}
	return c.NoContent(http.StatusOK)
}

// updateProjectLogo godoc
// @id updateProjectLogo
// @Summary Проекты (логотип): обновление логотипа
// @Description Загружает новый логотип для указанного проекта и обновляет запись в базе данных.
// @Tags Projects
// @Accept multipart/form-data
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param file formData file true "Файл логотипа"
// @Success 200 {object} dto.Project "Обновленный проект"
// @Failure 400 {object} apierrors.DefinedError "Ошибка: неверный формат файла"
// @Failure 403 {object} apierrors.DefinedError "Ошибка: недостаточно прав для обновления логотипа"
// @Failure 404 {object} apierrors.DefinedError "Ошибка: не найдено"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/logo/ [post]
func (s *Services) updateProjectLogo(c echo.Context) error {
	user := c.(ProjectContext).User
	project := c.(ProjectContext).Project

	if !limiter.Limiter.CanAddAttachment(project.WorkspaceId) {
		return EErrorDefined(c, apierrors.ErrAssetsLimitExceed)
	}

	file, err := c.FormFile("file")
	if err != nil {
		return EError(c, err)
	}

	userID := uuid.NullUUID{UUID: user.ID, Valid: true}
	fileAsset := dao.FileAsset{
		Id:          dao.GenUUID(),
		CreatedById: userID,
		WorkspaceId: uuid.NullUUID{UUID: project.WorkspaceId, Valid: true},
	}

	oldLogoId := project.LogoId

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		var oldLogo dao.FileAsset
		if project.LogoId.Valid {
			if err := tx.Where("id = ?", project.LogoId).First(&oldLogo).Error; err != nil {
				if err != gorm.ErrRecordNotFound {
					return err
				}
			}
		}

		if err := s.uploadAssetForm(tx, file, &fileAsset, filestorage.Metadata{
			WorkspaceId: project.WorkspaceId.String(),
			ProjectId:   project.ID.String(),
		}); err != nil {
			return err
		}

		project.LogoId = uuid.NullUUID{UUID: fileAsset.Id, Valid: true}
		if err := tx.Select("logo_id").Updates(&project).Error; err != nil {
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

		err = tracker.TrackActivity[dao.Project, dao.ProjectActivity](s.tracker, activities.EntityUpdatedActivity, newMap, oldMap, project, user)
		if err != nil {
			errStack.GetError(c, err)
		}

		return nil
	}); err != nil {
		return EError(c, err)
	}

	return c.JSON(http.StatusOK, project.ToDTO())
}

// deleteProjectLogo godoc
// @id deleteProjectLogo
// @Summary Проекты (логотип): удаление логотипа проекта
// @Description Удаляет логотип указанного проекта и обновляет запись в базе данных.
// @Tags Projects
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Success 200 {object} dto.Project "Обновленное рабочее пространство"
// @Failure 403 {object} apierrors.DefinedError "Ошибка: недостаточно прав для удаления логотипа"
// @Failure 404 {object} apierrors.DefinedError "Ошибка: не найдено"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/logo/ [delete]
func (s *Services) deleteProjectLogo(c echo.Context) error {
	user := c.(ProjectContext).User
	project := c.(ProjectContext).Project
	oldLogoId := project.LogoId.UUID

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		project.UpdatedById = uuid.NullUUID{UUID: user.ID, Valid: true}
		project.LogoId = uuid.NullUUID{}
		if err := tx.Select("logo_id").Updates(&project).Error; err != nil {
			return err
		}

		if project.LogoId.Valid {
			if err := tx.Where("id = ?", project.LogoId.UUID).Delete(&dao.FileAsset{}).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return EError(c, err)
	}

	//Трекинг активности
	oldMap := map[string]interface{}{
		"logo": oldLogoId.String(),
	}
	newMap := map[string]interface{}{
		"logo": uuid.Nil.String(),
	}

	err := tracker.TrackActivity[dao.Project, dao.ProjectActivity](s.tracker, activities.EntityUpdatedActivity, newMap, oldMap, project, user)
	if err != nil {
		errStack.GetError(c, err)
	}

	return c.JSON(http.StatusOK, project.ToDTO())
}

// @id getProjectStats
// @Summary Проекты: получение статистики проекта
// @Description Возвращает агрегированную статистику проекта: счётчики задач, распределение по статусам,
// приоритетам и группам статусов, информацию о просроченных задачах.
// По умолчанию включены все опциональные секции (исполнители, метки, спринты, временная динамика).
// Для отключения секции передайте соответствующий параметр со значением false.
// @Tags Projects
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта (UUID)"
// @Param include_assignee_stats query bool false "Включить статистику по исполнителям, топ-50 (по умолчанию true)"
// @Param include_label_stats query bool false "Включить статистику по меткам, топ-50 (по умолчанию true)"
// @Param include_sprint_stats query bool false "Включить статистику по спринтам, последние 50 (по умолчанию true)"
// @Param include_timeline query bool false "Включить временную статистику за 12 месяцев (по умолчанию true)"
// @Success 200 {object} dto.ProjectStats "Агрегированная статистика проекта"
// @Failure 400 {object} apierrors.DefinedError "Некорректный запрос"
// @Failure 403 {object} apierrors.DefinedError "Нет доступа к проекту"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/stats/ [get]
func (s *Services) getProjectStats(c echo.Context) error {
	project := c.(ProjectContext).Project

	opts := dto.ProjectStatsRequest{}
	if err := opts.FromHTTPQuery(c); err != nil {
		return EError(c, err)
	}
	stats, err := s.business.GetProjectStats(project.ID, opts)
	if err != nil {
		return EError(c, err)
	}
	return c.JSON(http.StatusOK, stats)
}

// getProjectRulesScript godoc
// @id getProjectRulesScript
// @Summary Проекты: получение скрипта правил
// @Description Возвращает скрипт правил проекта. Доступно только для админов проекта.
// @Tags Projects
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Success 200 {object} dto.RulesScriptResponse "Скрипт правил проекта"
// @Failure 403 {object} apierrors.DefinedError "Нет прав на просмотр скрипта правил"
// @Failure 404 {object} apierrors.DefinedError "Проект не найден"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/rules-script/ [get]
func (s *Services) getProjectRulesScript(c echo.Context) error {
	project := c.(ProjectContext).Project

	response := dto.RulesScriptResponse{
		RulesScript: project.RulesScript,
	}

	return c.JSON(http.StatusOK, response)
}

// updateProjectRulesScript godoc
// @id updateProjectRulesScript
// @Summary Проекты: обновление скрипта правил
// @Description Обновляет скрипт правил проекта. Доступно только для админов проекта.
// @Tags Projects
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param request body dto.UpdateRulesScriptRequest true "Данные для обновления скрипта правил"
// @Success 200 {object} dto.RulesScriptResponse "Обновленный скрипт правил"
// @Failure 400 {object} apierrors.DefinedError "Некорректные данные запроса"
// @Failure 403 {object} apierrors.DefinedError "Нет прав на обновление скрипта правил"
// @Failure 404 {object} apierrors.DefinedError "Проект не найден"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/rules-script/ [put]
func (s *Services) updateProjectRulesScript(c echo.Context) error {
	user := c.(ProjectContext).User
	project := c.(ProjectContext).Project

	var request dto.UpdateRulesScriptRequest
	if err := c.Bind(&request); err != nil {
		return EError(c, err)
	}

	if err := c.Validate(&request); err != nil {
		return EError(c, err)
	}

	// Сохраняем старый скрипт для отслеживания активности
	oldRulesScript := project.RulesScript

	// Обновляем скрипт
	project.RulesScript = request.RulesScript
	project.UpdatedById = uuid.NullUUID{UUID: user.ID, Valid: true}

	// Очищаем неразрывные пробелы
	if project.RulesScript != nil {
		*project.RulesScript = strings.ReplaceAll(*project.RulesScript, "\u00A0", " ")
	}

	// Обновляем в базе данных
	if err := s.db.Model(&dao.Project{}).
		Where("id = ?", project.ID).
		Updates(map[string]interface{}{
			"rules_script":  project.RulesScript,
			"updated_by_id": project.UpdatedById,
			"updated_at":    time.Now(),
		}).Error; err != nil {
		return EError(c, err)
	}

	// Отслеживаем активность
	oldMap := map[string]interface{}{
		"rules_script": oldRulesScript,
	}
	newMap := map[string]interface{}{
		"rules_script": project.RulesScript,
	}

	err := tracker.TrackActivity[dao.Project, dao.ProjectActivity](s.tracker, activities.EntityUpdatedActivity, newMap, oldMap, project, user)
	if err != nil {
		errStack.GetError(c, err)
	}

	response := dto.RulesScriptResponse{
		RulesScript: project.RulesScript,
	}

	return c.JSON(http.StatusOK, response)
}

// deleteProjectRulesScript godoc
// @id deleteProjectRulesScript
// @Summary Проекты: удаление скрипта правил
// @Description Удаляет скрипт правил проекта. Доступно только для админов проекта.
// @Tags Projects
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Success 200 {object} dto.RulesScriptResponse "Пустой скрипт правил"
// @Failure 403 {object} apierrors.DefinedError "Нет прав на удаление скрипта правил"
// @Failure 404 {object} apierrors.DefinedError "Проект не найден"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/rules-script/ [delete]
func (s *Services) deleteProjectRulesScript(c echo.Context) error {
	user := c.(ProjectContext).User
	project := c.(ProjectContext).Project

	// Сохраняем старый скрипт для отслеживания активности
	oldRulesScript := project.RulesScript

	// Удаляем скрипт
	project.RulesScript = nil
	project.UpdatedById = uuid.NullUUID{UUID: user.ID, Valid: true}

	// Обновляем в базе данных
	if err := s.db.Model(&dao.Project{}).
		Where("id = ?", project.ID).
		Updates(map[string]interface{}{
			"rules_script":  nil,
			"updated_by_id": project.UpdatedById,
			"updated_at":    time.Now(),
		}).Error; err != nil {
		return EError(c, err)
	}

	// Отслеживаем активность
	oldMap := map[string]interface{}{
		"rules_script": oldRulesScript,
	}
	newMap := map[string]interface{}{
		"rules_script": nil,
	}

	err := tracker.TrackActivity[dao.Project, dao.ProjectActivity](s.tracker, activities.EntityUpdatedActivity, newMap, oldMap, project, user)
	if err != nil {
		errStack.GetError(c, err)
	}

	response := dto.RulesScriptResponse{
		RulesScript: nil,
	}

	return c.JSON(http.StatusOK, response)
}

// ############# Property Templates methods ###################

// getPropertyTemplateList godoc
// @id getPropertyTemplateList
// @Summary Шаблоны полей: получение списка
// @Description Возвращает список всех шаблонов кастомных полей для проекта.
// @Tags PropertyTemplates
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Success 200 {array} dto.ProjectPropertyTemplate "Список шаблонов полей"
// @Failure 403 {object} apierrors.DefinedError "Нет доступа к проекту"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/property-templates/ [get]
func (s *Services) getPropertyTemplateList(c echo.Context) error {
	project := c.(ProjectContext).Project

	var templates []dao.ProjectPropertyTemplate
	if err := s.db.Where("project_id = ?", project.ID).
		Order("sort_order, created_at").
		Find(&templates).Error; err != nil {
		return EError(c, err)
	}

	result := make([]dto.ProjectPropertyTemplate, 0, len(templates))
	for _, t := range templates {
		if t.Type != "select" {
			t.Options = nil
		}
		result = append(result, *t.ToDTO())
	}

	return c.JSON(http.StatusOK, result)
}

// createPropertyTemplate godoc
// @id createPropertyTemplate
// @Summary Шаблоны полей: создание
// @Description Создает новый шаблон кастомного поля для проекта. Доступно только для админов проекта.
// @Tags PropertyTemplates
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param request body dto.CreatePropertyTemplateRequest true "Данные шаблона поля"
// @Success 201 {object} dto.ProjectPropertyTemplate "Созданный шаблон поля"
// @Failure 400 {object} apierrors.DefinedError "Некорректные данные"
// @Failure 403 {object} apierrors.DefinedError "Нет прав на создание"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/property-templates/ [post]
func (s *Services) createPropertyTemplate(c echo.Context) error {
	user := c.(ProjectContext).User
	project := c.(ProjectContext).Project

	var request dto.CreatePropertyTemplateRequest
	if err := c.Bind(&request); err != nil {
		return EError(c, err)
	}

	// Валидация имени
	if strings.TrimSpace(request.Name) == "" {
		return EErrorDefined(c, apierrors.ErrPropertyTemplateNameRequired)
	}

	// Валидация типа
	validTypes := map[string]bool{"string": true, "boolean": true, "select": true}
	if !validTypes[request.Type] {
		return EErrorDefined(c, apierrors.ErrPropertyTemplateTypeInvalid)
	}

	// Для типа select требуются опции
	var options []string
	if request.Type == "select" {
		if len(request.Options) == 0 {
			return EErrorDefined(c, apierrors.ErrPropertyTemplateOptionsRequired)
		}
		options = request.Options
	}

	template := dao.ProjectPropertyTemplate{
		Id:          dao.GenUUID(),
		ProjectId:   project.ID,
		WorkspaceId: project.WorkspaceId,
		Name:        strings.TrimSpace(request.Name),
		Type:        request.Type,
		Options:     options,
		OnlyAdmin:   request.OnlyAdmin,
		SortOrder:   request.SortOrder,
		CreatedById: uuid.NullUUID{UUID: user.ID, Valid: true},
		UpdatedById: uuid.NullUUID{UUID: user.ID, Valid: true},
	}

	if err := s.db.Create(&template).Error; err != nil {
		return EError(c, err)
	}

	return c.JSON(http.StatusCreated, template.ToDTO())
}

// updatePropertyTemplate godoc
// @id updatePropertyTemplate
// @Summary Шаблоны полей: обновление
// @Description Обновляет шаблон кастомного поля. Доступно только для админов проекта.
// @Tags PropertyTemplates
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param templateId path string true "ID шаблона поля"
// @Param request body dto.UpdatePropertyTemplateRequest true "Данные для обновления"
// @Success 200 {object} dto.ProjectPropertyTemplate "Обновленный шаблон поля"
// @Failure 400 {object} apierrors.DefinedError "Некорректные данные"
// @Failure 403 {object} apierrors.DefinedError "Нет прав на обновление"
// @Failure 404 {object} apierrors.DefinedError "Шаблон не найден"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/property-templates/{templateId}/ [patch]
func (s *Services) updatePropertyTemplate(c echo.Context) error {
	user := c.(ProjectContext).User
	project := c.(ProjectContext).Project
	templateId := c.Param("templateId")

	templateUUID, err := uuid.FromString(templateId)
	if err != nil {
		return EErrorDefined(c, apierrors.ErrPropertyTemplateNotFound)
	}

	var template dao.ProjectPropertyTemplate
	if err := s.db.Where("id = ? AND project_id = ?", templateUUID, project.ID).First(&template).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return EErrorDefined(c, apierrors.ErrPropertyTemplateNotFound)
		}
		return EError(c, err)
	}

	var request dto.UpdatePropertyTemplateRequest
	if err := c.Bind(&request); err != nil {
		return EError(c, err)
	}

	// Применяем обновления напрямую к структуре
	updated := false

	if request.Name != nil {
		name := strings.TrimSpace(*request.Name)
		if name == "" {
			return EErrorDefined(c, apierrors.ErrPropertyTemplateNameRequired)
		}
		template.Name = name
		updated = true
	}

	// Определяем тип для валидации options
	if request.Type != nil {
		validTypes := map[string]bool{"string": true, "boolean": true, "select": true}
		if !validTypes[*request.Type] {
			return EErrorDefined(c, apierrors.ErrPropertyTemplateTypeInvalid)
		}
		template.Type = *request.Type
		updated = true
	}

	// Обработка options
	if request.Options != nil {
		template.Options = *request.Options
		updated = true
	}

	// Для типа select проверяем наличие options
	if template.Type == "select" {
		if len(template.Options) == 0 {
			return EErrorDefined(c, apierrors.ErrPropertyTemplateOptionsRequired)
		}
	}

	if request.OnlyAdmin != nil {
		template.OnlyAdmin = *request.OnlyAdmin
		updated = true
	}
	if request.SortOrder != nil {
		template.SortOrder = *request.SortOrder
		updated = true
	}

	if updated {
		template.UpdatedById = uuid.NullUUID{UUID: user.ID, Valid: true}
		template.UpdatedAt = time.Now()

		if err := s.db.Save(&template).Error; err != nil {
			return EError(c, err)
		}
	}

	// Перезагружаем для актуальных данных
	if err := s.db.First(&template, templateUUID).Error; err != nil {
		return EError(c, err)
	}

	return c.JSON(http.StatusOK, template.ToDTO())
}

// deletePropertyTemplate godoc
// @id deletePropertyTemplate
// @Summary Шаблоны полей: удаление
// @Description Удаляет шаблон кастомного поля и все связанные значения. Доступно только для админов проекта.
// @Tags PropertyTemplates
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param templateId path string true "ID шаблона поля"
// @Success 204 "Шаблон успешно удален"
// @Failure 403 {object} apierrors.DefinedError "Нет прав на удаление"
// @Failure 404 {object} apierrors.DefinedError "Шаблон не найден"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/property-templates/{templateId}/ [delete]
func (s *Services) deletePropertyTemplate(c echo.Context) error {
	project := c.(ProjectContext).Project
	templateId := c.Param("templateId")

	templateUUID, err := uuid.FromString(templateId)
	if err != nil {
		return EErrorDefined(c, apierrors.ErrPropertyTemplateNotFound)
	}

	// Проверяем существование
	var template dao.ProjectPropertyTemplate
	if err := s.db.Where("id = ? AND project_id = ?", templateUUID, project.ID).First(&template).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return EErrorDefined(c, apierrors.ErrPropertyTemplateNotFound)
		}
		return EError(c, err)
	}

	// Удаляем все значения для этого шаблона
	if err := s.db.Where("template_id = ?", templateUUID).Delete(&dao.IssueProperty{}).Error; err != nil {
		return EError(c, err)
	}

	// Удаляем шаблон
	if err := s.db.Delete(&template).Error; err != nil {
		return EError(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}
