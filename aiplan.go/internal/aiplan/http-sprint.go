package aiplan

import (
	"errors"
	"net/http"

	tracker "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/activity-tracker"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/apierrors"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dto"
	errStack "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/stack-error"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SprintContext struct {
	WorkspaceContext
	Sprint dao.Sprint
}

func (s *Services) SprintMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		sprintId := c.Param("sprintId")
		workspace := c.(WorkspaceContext).Workspace
		//user := c.(WorkspaceContext).User

		var sprint dao.Sprint
		query := s.db.
			Joins("Workspace").
			Joins("CreatedBy").
			Joins("UpdatedBy").
			Preload("Watchers").
			Preload("Issues").
			Where("sprints.workspace_id = ?", workspace.ID).
			Set("issueProgress", true)

		if val, err := uuid.FromString(sprintId); err != nil {
			query = query.Where("sprints.sequence_id = ?", sprintId)
		} else {
			query = query.Where("sprints.id = ?", val.String())
		}

		if err := query.First(&sprint).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return EErrorDefined(c, apierrors.ErrSprintNotFound)
			}
			return EError(c, err)
		}

		sprint.Stats.AllIssues = len(sprint.Issues)
		for _, issue := range sprint.Issues {
			switch issue.IssueProgress.Status {
			case types.InProgress:
				sprint.Stats.InProgress++
			case types.Pending:
				sprint.Stats.Pending++
			case types.Cancelled:
				sprint.Stats.Cancelled++
			case types.Completed:
				sprint.Stats.Completed++
			}
		}

		// Для получения списка задач спринта отсортированных по sequence_id
		//err := s.db.
		//  Preload("State").
		//  Joins("JOIN sprint_issues ON issues.id = sprint_issues.issue_id").
		//  Where("sprint_issues.sprint_id = ?", sprint.Id).
		//  Order("sprint_issues.position ASC").
		//  Find(&sprint.Issues).Error

		//if err != nil {
		//  return EError(c, err)
		//}
		return next(SprintContext{c.(WorkspaceContext), sprint})
	}
}

func (s *Services) AddSprintServices(g *echo.Group) {
	workspaceGroup := g.Group("workspaces/:workspaceSlug", s.WorkspaceMiddleware)
	workspaceGroup.Use(s.LastVisitedWorkspaceMiddleware)
	workspaceGroup.Use(s.WorkspacePermissionMiddleware)

	sprintGroup := workspaceGroup.Group("/sprints/:sprintId", s.SprintMiddleware)
	sprintGroup.Use(s.SprintPermissionMiddleware)

	sprintAdminGroup := sprintGroup.Group("", s.SprintAdminPermissionMiddleware)

	workspaceGroup.GET("/sprints/", s.getSprintList)
	workspaceGroup.POST("/sprints/", s.createSprint)

	sprintAdminGroup.PATCH("/", s.updateSprint)
	sprintAdminGroup.DELETE("/", s.deleteSprint)

	sprintAdminGroup.POST("/issues/", s.sprintIssuesUpdate)
	sprintAdminGroup.POST("/watchers/", s.sprintWatchersUpdate)

	sprintGroup.GET("/activities/", s.getSpringActivityList)
	sprintGroup.GET("/", s.GetSprint)

}

// getSprintList godoc
// @id getSprintList
// @Summary Спринты: получения списка спринтов
// @Description Возвращает список всех спринтов в рабочем пространстве.
// @Tags Sprint
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Success 200 {array} dto.SprintLight "Список спринтов"
// @Failure 400 {object} apierrors.DefinedError "Ошибка запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/sprints/ [get]
func (s *Services) getSprintList(c echo.Context) error {
	workspace := c.(WorkspaceContext).Workspace

	var sprints []dao.Sprint
	if err := s.db.
		Set("issueProgress", true).
		Preload("Issues").
		Preload("Issues.State").
		Where("workspace_id = ?", workspace.ID).
		Find(&sprints).Error; err != nil {
		return EError(c, err)
	}

	for i := range sprints {
		sprints[i].Stats.AllIssues = len(sprints[i].Issues)
		for _, issue := range sprints[i].Issues {
			switch issue.IssueProgress.Status {
			case types.InProgress:
				sprints[i].Stats.InProgress++
			case types.Pending:
				sprints[i].Stats.Pending++
			case types.Cancelled:
				sprints[i].Stats.Cancelled++
			case types.Completed:
				sprints[i].Stats.Completed++
			}
		}
	}

	return c.JSON(
		http.StatusOK,
		utils.SliceToSlice(&sprints, func(s *dao.Sprint) dto.SprintLight { return *s.ToLightDTO() }))
	//utils.SliceToSlice(&sprint, func(p *dao.ProjectWithCount) dto.ProjectLight { return *p.ToLightDTO() }))
}

// createSprint godoc
// @id createSprint
// @Summary Спринты: создание спринта
// @Description Создает новый спринт в рабочем пространстве.
// @Tags Sprint
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param request body requestSprint true "Информация о спринте"
// @Success 200 {object} dto.Sprint "Созданный спринт"
// @Failure 400 {object} apierrors.DefinedError "Ошибка запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/sprints/ [post]
func (s *Services) createSprint(c echo.Context) error {
	var req requestSprint
	user := c.(WorkspaceContext).User

	err := c.Bind(&req)
	if err != nil {
		return EError(c, apierrors.ErrSprintBadRequest)
	}
	if req.Name == "" {
		return EErrorDefined(c, apierrors.ErrSprintRequestValidate)
	}

	if err := c.Validate(req); err != nil {
		return EErrorDefined(c, apierrors.ErrSprintRequestValidate)
	}

	sprint, err := req.toDao(c)
	if err != nil {
		return EError(c, err)
	}

	if err := s.db.Create(&sprint).Error; err != nil {
		return EError(c, err)
	}

	err = tracker.TrackActivity[dao.Sprint, dao.WorkspaceActivity](s.tracker, tracker.ENTITY_CREATE_ACTIVITY, nil, nil, *sprint, user)
	if err != nil {
		errStack.GetError(c, err)
	}
	sprint.CreatedBy = *user

	return c.JSON(http.StatusCreated, sprint.ToDTO())
}

// GetSprint godoc
// @id GetSprint
// @Summary Спринты: получение информации о спринте
// @Description Получение информации о спринте.
// @Tags Sprint
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param sprintId path string true "Идентификатор или номер последовательности спринта"
// @Success 200 {object} dto.Sprint "Спринт"
// @Failure 400 {object} apierrors.DefinedError "Ошибка запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Спринт не найден"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/sprints/{sprintId}/ [get]
func (s *Services) GetSprint(c echo.Context) error {
	sprint := c.(SprintContext).Sprint
	return c.JSON(http.StatusOK, sprint.ToDTO())
}

// updateSprint godoc
// @id updateSprint
// @Summary Спринты: обновление информации о спринте
// @Description Обновление информации о спринте.
// @Tags Sprint
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param sprintId path string true "Идентификатор или номер последовательности спринта"
// @Param request body requestSprint true "Информация о спринте"
// @Success 200 {object} dto.Sprint "Спринт"
// @Failure 400 {object} apierrors.DefinedError "Ошибка запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Спринт не найден"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/sprints/{sprintId}/ [patch]
func (s *Services) updateSprint(c echo.Context) error {
	sprint := c.(SprintContext).Sprint
	user := c.(SprintContext).User
	oldSprintMap := StructToJSONMap(sprint)

	var req requestSprint
	fields, err := BindData(c, "", &req)
	if err != nil {
		return EError(c, err)
	}

	for _, field := range fields {
		switch field {
		case "name":
			sprint.Name = req.Name
		case "description":
			sprint.Description = req.Description
		case "start_date":
			sprint.StartDate = req.StartDate.ToNullTime()
		case "end_date":
			sprint.EndDate = req.EndDate.ToNullTime()
		}
	}

	if len(fields) > 0 {
		var userUUID uuid.UUID
		if val, err := uuid.FromString(user.ID); err != nil {
			return EError(c, err)
		} else {
			userUUID = val
		}
		sprint.UpdatedById = uuid.NullUUID{UUID: userUUID, Valid: true}
		sprint.UpdatedBy = user
		fields = append(fields, "updated_by_id")
		if err := s.db.Omit(clause.Associations).Select(fields).Updates(&sprint).Error; err != nil {
			return EError(c, err)
		}
	}
	newSprintMap := StructToJSONMap(sprint)

	err = tracker.TrackActivity[dao.Sprint, dao.SprintActivity](s.tracker, tracker.ENTITY_UPDATED_ACTIVITY, newSprintMap, oldSprintMap, sprint, user)
	if err != nil {
		errStack.GetError(c, err)
	}

	return c.JSON(http.StatusOK, sprint.ToDTO())
}

// sprintIssuesUpdate godoc
// @id sprintIssuesUpdate
// @Summary Спринты: Изменяет задачи в спринте
// @Description Изменяет список задач в спринте.
// @Tags Sprint
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param sprintId path string true "Идентификатор или номер последовательности спринта"
// @Param request body requestIssueIdList true "Список id задач"
// @Success 200  "Задачи добавлены"
// @Failure 400 {object} apierrors.DefinedError "Ошибка запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Спринт не найден"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/sprints/{sprintId}/issues/ [post]
func (s *Services) sprintIssuesUpdate(c echo.Context) error {
	workspace := c.(SprintContext).Workspace
	sprint := c.(SprintContext).Sprint
	user := c.(SprintContext).User

	oldIssueIds := utils.SliceToSlice(&sprint.Issues, func(t *dao.Issue) interface{} { return t.ID.String() })

	workspaceUUID := uuid.Must(uuid.FromString(workspace.ID))
	userUUID := uuid.Must(uuid.FromString(user.ID))

	var req requestIssueIdList

	err := c.Bind(&req)
	if err != nil {
		return EError(c, apierrors.ErrSprintBadRequest)
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		var issues []dao.Issue

		if err := s.db.
			Where("workspace_id = ?", workspace.ID).
			Where("sprint_id = ?", sprint.Id).
			Where("issue_id IN (?)", req.IssuesRemove).
			Delete(&dao.SprintIssue{}).Error; err != nil {
			return EError(c, err)
		}

		if err := tx.
			Where("workspace_id", workspace.ID).
			Where("id in (?)", req.IssuesAdd).
			Where("id not in (?)",
				tx.
					Select("issue_id::text").
					Where("workspace_id", workspace.ID).
					Where("sprint_id = ?", sprint.Id).
					Model(&dao.SprintIssue{})).
			Find(&issues).Error; err != nil {
			return err
		}

		var maxPosition int
		if err := tx.Model(&dao.SprintIssue{}).
			Unscoped().
			Where("workspace_id = ? AND sprint_id = ?", workspaceUUID, sprint.Id).
			Select("COALESCE(MAX(position), 0)").
			Scan(&maxPosition).Error; err != nil {
			return err
		}

		var sprintIssues []dao.SprintIssue
		for i, issue := range issues {

			projectUUID := uuid.Must(uuid.FromString(issue.ProjectId))

			sprintIssues = append(sprintIssues, dao.SprintIssue{
				Id: dao.GenUUID(),

				SprintId:    sprint.Id,
				IssueId:     issue.ID,
				ProjectId:   projectUUID,
				WorkspaceId: workspaceUUID,
				CreatedById: userUUID,
				Position:    maxPosition + i + 1,
			})
		}

		if err := tx.CreateInBatches(&sprintIssues, 10).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		return EError(c, err)
	}

	if err := s.db.
		Where("id IN (?)",
			s.db.Select("issue_id").
				Where("workspace_id = ?", workspace.ID).
				Where("sprint_id = ?", sprint.Id).
				Model(&dao.SprintIssue{})).
		Find(&sprint.Issues).Error; err != nil {
		return EError(c, err)
	}

	newIssuesIds := utils.SliceToSlice(&sprint.Issues, func(t *dao.Issue) interface{} { return t.ID.String() })
	reqData := map[string]interface{}{
		"issue_list": newIssuesIds,
	}
	currentInstance := map[string]interface{}{
		"issues": oldIssueIds,
	}

	{ // reg activity
		err = tracker.TrackActivity[dao.Sprint, dao.SprintActivity](s.tracker, tracker.ENTITY_UPDATED_ACTIVITY, reqData, currentInstance, sprint, user)
		if err != nil {
			errStack.GetError(c, err)
		}

		changes, err := utils.CalculateIDChanges(newIssuesIds, oldIssueIds)
		if err != nil {
			return EError(c, err)
		}
		var issues []dao.Issue
		if err := s.db.Where("workspace_id = ?", workspace.ID).Where("id IN (?)", changes.InvolvedIds).Find(&issues).Error; err != nil {
			return EError(c, err)
		}

		issueMap := utils.SliceToMap(&issues, func(t *dao.Issue) string { return t.ID.String() })

		data := map[string]interface{}{
			"issue_key":           "sprint",
			"sprint_activity_val": sprint.Name,
			"updateScopeId":       sprint.Id.String(),
		}

		for _, id := range changes.AddIds {
			err = tracker.TrackActivity[dao.Issue, dao.IssueActivity](s.tracker, tracker.ENTITY_ADD_ACTIVITY, data, nil, issueMap[id], user)
			if err != nil {
				errStack.GetError(c, err)
			}
		}
		for _, id := range changes.DelIds {
			err = tracker.TrackActivity[dao.Issue, dao.IssueActivity](s.tracker, tracker.ENTITY_REMOVE_ACTIVITY, data, nil, issueMap[id], user)
			if err != nil {
				errStack.GetError(c, err)
			}
		}
	}

	return c.NoContent(http.StatusOK)
}

// deleteSprint godoc
// @id deleteSprint
// @Summary Спринты: Удалить спринт
// @Description Удаляет спринт.
// @Tags Sprint
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param sprintId path string true "Идентификатор или номер последовательности спринта"
// @Success 200  "Спринт удален"
// @Failure 400 {object} apierrors.DefinedError "Ошибка запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Спринт не найден"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/sprints/{sprintId}/ [delete]
func (s *Services) deleteSprint(c echo.Context) error {
	sprint := c.(SprintContext).Sprint
	user := c.(SprintContext).User

	err := tracker.TrackActivity[dao.Sprint, dao.WorkspaceActivity](s.tracker, tracker.ENTITY_DELETE_ACTIVITY, nil, nil, sprint, user)
	if err != nil {
		errStack.GetError(c, err)
		return err
	}

	if err := s.db.Delete(&sprint).Error; err != nil {
		return EError(c, err)
	}
	return c.NoContent(http.StatusOK)
}

// SprintWatchersUpdate godoc
// @id SprintWatchersUpdate
// @Summary Спринты: Изменение наблюдателей в спринте
// @Description Изменение наблюдателей в спринте.
// @Tags Sprint
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param sprintId path string true "Идентификатор или номер последовательности спринта"
// @Param request body requestUserIdList true "Список id user"
// @Success 200  "ок"
// @Failure 400 {object} apierrors.DefinedError "Ошибка запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Спринт не найден"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/sprints/{sprintId}/watchers/ [post]
func (s *Services) sprintWatchersUpdate(c echo.Context) error {
	workspace := c.(SprintContext).Workspace
	sprint := c.(SprintContext).Sprint
	user := c.(SprintContext).User

	oldMemberIds := utils.SliceToSlice(&sprint.Watchers, func(t *dao.User) interface{} { return t.ID })

	workspaceUUID := uuid.Must(uuid.FromString(workspace.ID))
	userUUID := uuid.Must(uuid.FromString(user.ID))

	var req requestUserIdList

	err := c.Bind(&req)
	if err != nil {
		return EError(c, apierrors.ErrSprintBadRequest)
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		var workspaceMembers []dao.WorkspaceMember

		if err := tx.
			Where("workspace_id = ?", workspace.ID).
			Where("sprint_id = ?", sprint.Id).
			Where("watcher_id IN (?)", req.MembersRemove).
			Delete(&dao.SprintWatcher{}).Error; err != nil {
			return err
		}

		if err := tx.
			Where("workspace_id", workspace.ID).
			Where("member_id in (?)", req.MembersAdd).
			Where("member_id not in (?)",
				tx.
					Select("watcher_id::text").
					Where("workspace_id", workspace.ID).
					Where("sprint_id = ?", sprint.Id).
					Model(&dao.SprintWatcher{})).
			Find(&workspaceMembers).Error; err != nil {
			return err
		}

		var sprintWatchers []dao.SprintWatcher
		for _, member := range workspaceMembers {
			memberUUID := uuid.Must(uuid.FromString(member.MemberId))
			sprintWatchers = append(sprintWatchers, dao.SprintWatcher{
				Id:          dao.GenUUID(),
				CreatedById: userUUID,
				WatcherId:   memberUUID,
				SprintId:    sprint.Id,
				WorkspaceId: workspaceUUID,
			})
		}

		if err := tx.CreateInBatches(&sprintWatchers, 10).Error; err != nil {
			return err
		}

		return nil
	}); err != nil {
		return EError(c, err)
	}

	if err := s.db.
		Where("id IN (?)",
			s.db.Select("watcher_id").
				Where("workspace_id = ?", workspace.ID).
				Where("sprint_id = ?", sprint.Id).
				Model(&dao.SprintWatcher{})).
		Find(&sprint.Watchers).Error; err != nil {
		return EError(c, err)
	}

	reqData := map[string]interface{}{
		"watchers_list": utils.SliceToSlice(&sprint.Watchers, func(t *dao.User) interface{} { return t.ID }),
	}
	currentInstance := map[string]interface{}{
		"watchers": oldMemberIds,
	}

	err = tracker.TrackActivity[dao.Sprint, dao.SprintActivity](s.tracker, tracker.ENTITY_UPDATED_ACTIVITY, reqData, currentInstance, sprint, user)
	if err != nil {
		errStack.GetError(c, err)
	}

	return c.NoContent(http.StatusOK)
}

// getSpringActivityList godoc
// @id getSpringActivityList
// @Summary Спринты: получение активностей спринта
// @Description Возвращает список активностей для указанного спринта с возможностью пагинации.
// @Tags Sprint
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param sprintId path string true "Идентификатор или номер последовательности спринта"
// @Param offset query int false "Смещение для пагинации" default(0)
// @Param limit query int false "Количество записей на странице" default(100)
// @Success 200 {object} dao.PaginationResponse{result=[]dto.EntityActivityFull} "Список активностей спринта"
// @Failure 400 {object} apierrors.DefinedError "Ошибка запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Спринт не найден"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/sprints/{sprintId}/activities/ [get]
func (s *Services) getSpringActivityList(c echo.Context) error {
	sprintId := c.(SprintContext).Sprint.Id
	workspaceId := c.(SprintContext).Workspace.ID

	offset := -1
	limit := 100

	if err := echo.QueryParamsBinder(c).
		Int("offset", &offset).
		Int("limit", &limit).BindError(); err != nil {
		return EError(c, err)
	}

	var sprint dao.SprintActivity
	sprint.UnionCustomFields = "'sprint' AS entity_type"

	unionTable := dao.BuildUnionSubquery(s.db, "union_activities", dao.FullActivity{}, sprint)

	query := unionTable.
		Joins("Sprint").
		Joins("Workspace").
		Joins("Actor").
		Order("union_activities.created_at desc").
		Where("union_activities.workspace_id = ?", workspaceId).
		Where("union_activities.sprint_id = ?", sprintId)

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

//

type requestSprint struct {
	Name        string             `json:"name,omitempty"`
	Description types.RedactorHTML `json:"description,omitempty" swaggertype:"string"`
	StartDate   *types.TargetDate  `json:"start_date,omitempty" extensions:"x-nullable" swaggertype:"string"`
	EndDate     *types.TargetDate  `json:"end_date,omitempty" extensions:"x-nullable" swaggertype:"string"`
}

type requestIssueIdList struct {
	IssuesAdd    []string `json:"issues_add,omitempty"`
	IssuesRemove []string `json:"issues_remove,omitempty"`
}

type requestUserIdList struct {
	MembersAdd    []string `json:"members_add,omitempty"`
	MembersRemove []string `json:"members_remove,omitempty"`
}

func (rs *requestSprint) toDao(ctx echo.Context) (*dao.Sprint, error) {
	var workspaceMember dao.WorkspaceMember
	var workspace dao.Workspace
	switch v := ctx.(type) {
	case WorkspaceContext:
		workspaceMember = v.WorkspaceMember
		workspace = v.Workspace
	case SprintContext:
		workspaceMember = v.WorkspaceMember
		workspace = v.Workspace
	}

	userUUID := uuid.Must(uuid.FromString(workspaceMember.MemberId))
	workspaceUUID := uuid.Must(uuid.FromString(workspace.ID))

	return &dao.Sprint{
		Id:          dao.GenUUID(),
		CreatedById: userUUID,

		WorkspaceId: workspaceUUID,
		CreatedBy:   dao.User{},
		Name:        rs.Name,
		Description: rs.Description,
		StartDate:   rs.StartDate.ToNullTime(),
		EndDate:     rs.EndDate.ToNullTime(),
	}, nil
}
