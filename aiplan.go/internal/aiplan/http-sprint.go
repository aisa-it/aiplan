package aiplan

import (
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"

	tracker "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/activity-tracker"
	apicontext "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/api-context"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/apierrors"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dto"
	errStack "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/stack-error"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *Services) SprintMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		apiContext := apicontext.GetContext(c)
		workspace := apiContext.GetWorkspace()
		if apiContext.Error() != nil {
			return EError(c, apiContext.Error())
		}

		exists, err := dao.IsSprintExists(s.db, workspace.ID, c.Param("sprintId"))
		if err != nil {
			return EError(c, err)
		}
		if !exists {
			return EErrorDefined(c, apierrors.ErrSprintNotFound)
		}

		return next(c.(AuthContext))
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

	workspaceGroup.POST("/sprints-folder/", s.addSprintFolders)

	workspaceGroup.PATCH("/sprints-folder/:sprintFolderId", s.updateSprintFolders)
	workspaceGroup.DELETE("/sprints-folder/:sprintFolderId", s.deleteSprintFolders)

	sprintAdminGroup.PATCH("/", s.updateSprint)
	sprintAdminGroup.DELETE("/", s.deleteSprint)

	sprintAdminGroup.POST("/issues/", s.sprintIssuesUpdate)
	sprintAdminGroup.POST("/watchers/", s.sprintWatchersUpdate)

	sprintGroup.GET("/activities/", s.getSpringActivityList)
	sprintGroup.GET("/", s.GetSprint)

	sprintGroup.POST("/sprint-view/", s.updateSprintView)

	sprintGroup.POST("/issues/search/", s.getIssueList)

	sprintGroup.GET("/states/", s.getSprintStates)
}

// getSprintList godoc
// @id getSprintList
// @Summary Спринты: получение директорий спринтов
// @Description Возвращает список всех директорий спринтов, с вложенными спринтами.
// @Tags Sprint
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Success 200 {array} dto.SprintFolder "Список директорий спринтов"
// @Failure 400 {object} apierrors.DefinedError "Ошибка запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Hе найден"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/sprints/ [get]
func (s *Services) getSprintList(c echo.Context) error {
	apiContext := apicontext.GetContext(c)
	workspace := apiContext.GetWorkspace()
	if apiContext.Error() != nil {
		return EError(c, apiContext.Error())
	}

	var sprints []dao.Sprint
	if err := s.db.
		Set("issueProgress", true).
		Joins("SprintFolder").
		Preload("Issues.State").
		Where("sprints.workspace_id = ?", workspace.ID).
		Order("start_date DESC").
		Find(&sprints).Error; err != nil {
		return EError(c, err)
	}

	for i := range sprints {
		sprints[i].UpdateStats()
	}

	var folders []dao.SprintFolder
	if err := s.db.
		Where("workspace_id = ?", workspace.ID).
		Find(&folders).Error; err != nil {
		return EError(c, err)
	}

	folderMap := make(map[uuid.UUID]*dao.SprintFolder, len(folders))
	for i := range folders {
		folderMap[folders[i].Id] = &folders[i]
	}

	var unassignedSprints []dao.Sprint
	for i := range sprints {
		if sprints[i].SprintFolderId.Valid {
			if folder, ok := folderMap[sprints[i].SprintFolderId.UUID]; ok {
				folder.Sprints = append(folder.Sprints, sprints[i])
			}
		} else {
			unassignedSprints = append(unassignedSprints, sprints[i])

		}
	}

	result := make([]dao.SprintFolder, 0, len(folderMap)+1)

	for _, folder := range folderMap {
		result = append(result, *folder)
	}

	if len(unassignedSprints) != 0 {
		result = append(result, dao.SprintFolder{
			Id:      uuid.Nil,
			Sprints: unassignedSprints,
		})
	}

	slices.SortFunc(result, func(a, b dao.SprintFolder) int {
		if a.Id == uuid.Nil && b.Id != uuid.Nil {
			return 1
		}
		if a.Id != uuid.Nil && b.Id == uuid.Nil {
			return -1
		}
		if a.Id == uuid.Nil && b.Id == uuid.Nil {
			return 0
		}

		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return c.JSON(http.StatusOK, utils.SliceToSlice(&result, func(t *dao.SprintFolder) *dto.SprintFolder { return t.ToDTO() }))
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
// @Param request body dto.RequestSprint true "Информация о спринте"
// @Success 200 {object} dto.Sprint "Созданный спринт"
// @Failure 400 {object} apierrors.DefinedError "Ошибка запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/sprints/ [post]
func (s *Services) createSprint(c echo.Context) error {
	var req dto.RequestSprint
	user := apicontext.GetContext(c).GetUser()

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

	sprintReq := sprintDto{
		req,
	}
	sprint, err := sprintReq.toDao(c)
	if err != nil {
		return EError(c, err)
	}
	if sprint.EndDate.Valid && sprint.StartDate.Valid {
		if !sprint.EndDate.Time.After(sprint.StartDate.Time) {
			return EErrorDefined(c, apierrors.ErrInvalidSprintTimeWindow)
		}
	}

	if sprint.SprintFolderId.Valid {
		if err := s.db.Where("workspace_id = ?", sprint.WorkspaceId).
			Where("id = ?", sprintReq.RequestSprint.Folder).
			First(&sprint.SprintFolder).Error; err != nil {
			sprint.SprintFolderId = uuid.NullUUID{}
			sprint.SprintFolder = nil
		}
	}

	if err := s.db.Create(&sprint).Error; err != nil {
		return EError(c, err)
	}

	err = tracker.TrackActivity[dao.Sprint, dao.WorkspaceActivity](s.tracker, activities.EntityCreateActivity, nil, nil, *sprint, user)
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
	apiContext := apicontext.GetContext(c)
	sprint := apiContext.GetSprint(apicontext.WithSprintAll())
	if apiContext.Error() != nil {
		return EError(c, apiContext.Error())
	}
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
// @Param request body dto.RequestSprint true "Информация о спринте"
// @Success 200 {object} dto.Sprint "Спринт"
// @Failure 400 {object} apierrors.DefinedError "Ошибка запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Спринт не найден"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/sprints/{sprintId}/ [patch]
func (s *Services) updateSprint(c echo.Context) error {
	apiContext := apicontext.GetContext(c)
	sprint := apiContext.GetSprint(
		apicontext.WithSprintCreatedBy(),
		apicontext.WithSprintUpdatedBy(),
		apicontext.WithSprintFolder(),
	)
	if apiContext.Error() != nil {
		return EError(c, apiContext.Error())
	}
	user := apiContext.GetUser()
	oldSprintMap := StructToJSONMap(*sprint)

	var req dto.RequestSprint
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
		case "sprint_folder_id":
			var folder *dao.SprintFolder
			if eerr := s.db.Where("workspace_id = ?", sprint.WorkspaceId).
				Where("id = ?", req.Folder).
				First(&folder).Error; eerr != nil {
				sprint.SprintFolderId = uuid.NullUUID{}
				sprint.SprintFolder = nil
			} else {
				sprint.SprintFolderId = req.Folder
				sprint.SprintFolder = folder
			}
		}
	}

	if len(fields) > 0 {
		sprint.UpdatedById = uuid.NullUUID{UUID: user.ID, Valid: true}
		sprint.UpdatedBy = user
		fields = append(fields, "updated_by_id")
		if sprint.EndDate.Valid && sprint.StartDate.Valid {
			if !sprint.EndDate.Time.After(sprint.StartDate.Time) {
				return EErrorDefined(c, apierrors.ErrInvalidSprintTimeWindow)
			}
		}
		if err := s.db.Omit(clause.Associations).Select(fields).Updates(sprint).Error; err != nil {
			return EError(c, err)
		}
	}
	newSprintMap := StructToJSONMap(*sprint)

	err = tracker.TrackActivity[dao.Sprint, dao.SprintActivity](s.tracker, activities.EntityUpdatedActivity, newSprintMap, oldSprintMap, *sprint, user)
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
// @Param request body dto.RequestIssueIdList true "Список id задач"
// @Success 200  "Задачи добавлены"
// @Failure 400 {object} apierrors.DefinedError "Ошибка запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Спринт не найден"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/sprints/{sprintId}/issues/ [post]
func (s *Services) sprintIssuesUpdate(c echo.Context) error {
	apiContext := apicontext.GetContext(c)
	workspace := apiContext.GetWorkspace()
	sprint := apiContext.GetSprint(apicontext.WithSprintIssues())
	if apiContext.Error() != nil {
		return EError(c, apiContext.Error())
	}
	user := apiContext.GetUser()

	oldIssueIds := utils.SliceToSlice(&sprint.Issues, func(t *dao.Issue) interface{} { return t.ID })

	workspaceUUID := workspace.ID
	userUUID := user.ID

	var req dto.RequestIssueIdList

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
					Select("issue_id").
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

			projectUUID := issue.ProjectId

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

	newIssuesIds := utils.SliceToSlice(&sprint.Issues, func(t *dao.Issue) interface{} { return t.ID })
	reqData := map[string]interface{}{
		"issue_list": newIssuesIds,
		"field_log":  activities.Issue.Field,
	}
	currentInstance := map[string]interface{}{
		"issues": oldIssueIds,
	}

	{ // reg activity
		err = tracker.TrackActivity[dao.Sprint, dao.SprintActivity](s.tracker, activities.EntityUpdatedActivity, reqData, currentInstance, *sprint, user)
		if err != nil {
			errStack.GetError(c, err)
		}

		changes := utils.CalculateIDChanges(newIssuesIds, oldIssueIds)
		var issues []dao.Issue
		if err := s.db.Where("workspace_id = ?", workspace.ID).Where("id IN (?)", changes.InvolvedIds).Find(&issues).Error; err != nil {
			return EError(c, err)
		}

		issueMap := utils.SliceToMap(&issues, func(t *dao.Issue) uuid.UUID { return t.ID })

		data := map[string]interface{}{
			"issue_key":           "sprint",
			"sprint_activity_val": sprint.Name,
			"updateScopeId":       sprint.Id,
		}

		for _, id := range changes.AddIds {
			err = tracker.TrackActivity[dao.Issue, dao.IssueActivity](s.tracker, activities.EntityAddActivity, data, nil, issueMap[id], user)
			if err != nil {
				errStack.GetError(c, err)
			}
		}
		for _, id := range changes.DelIds {
			err = tracker.TrackActivity[dao.Issue, dao.IssueActivity](s.tracker, activities.EntityRemoveActivity, data, nil, issueMap[id], user)
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
	apiContext := apicontext.GetContext(c)
	sprint := apiContext.GetSprint()
	if apiContext.Error() != nil {
		return EError(c, apiContext.Error())
	}
	user := apiContext.GetUser()

	err := tracker.TrackActivity[dao.Sprint, dao.WorkspaceActivity](s.tracker, activities.EntityDeleteActivity, nil, nil, *sprint, user)
	if err != nil {
		errStack.GetError(c, err)
		return err
	}

	if err := s.db.Delete(sprint).Error; err != nil {
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
// @Param request body dto.RequestUserIdList true "Список id user"
// @Success 200  "ок"
// @Failure 400 {object} apierrors.DefinedError "Ошибка запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Спринт не найден"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/sprints/{sprintId}/watchers/ [post]
func (s *Services) sprintWatchersUpdate(c echo.Context) error {
	apiContext := apicontext.GetContext(c)
	workspace := apiContext.GetWorkspace()
	sprint := apiContext.GetSprint(apicontext.WithSprintWatchers())
	if apiContext.Error() != nil {
		return EError(c, apiContext.Error())
	}
	user := apiContext.GetUser()

	oldMemberIds := utils.SliceToSlice(&sprint.Watchers, func(t *dao.User) interface{} { return t.ID })

	workspaceUUID := workspace.ID
	userUUID := user.ID

	var req dto.RequestUserIdList

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
					Select("watcher_id").
					Where("workspace_id", workspace.ID).
					Where("sprint_id = ?", sprint.Id).
					Model(&dao.SprintWatcher{})).
			Find(&workspaceMembers).Error; err != nil {
			return err
		}

		var sprintWatchers []dao.SprintWatcher
		for _, member := range workspaceMembers {
			memberUUID := member.MemberId
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

	err = tracker.TrackActivity[dao.Sprint, dao.SprintActivity](s.tracker, activities.EntityUpdatedActivity, reqData, currentInstance, *sprint, user)
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
	apiContext := apicontext.GetContext(c)
	workspace := apiContext.GetWorkspace()
	sprint := apiContext.GetSprint()
	if apiContext.Error() != nil {
		return EError(c, apiContext.Error())
	}
	sprintId := sprint.Id
	workspaceId := workspace.ID

	offset := -1
	limit := 100

	if err := echo.QueryParamsBinder(c).
		Int("offset", &offset).
		Int("limit", &limit).BindError(); err != nil {
		return EError(c, err)
	}

	var sprintActivity dao.SprintActivity
	sprintActivity.UnionCustomFields = "'sprint' AS entity_type"

	unionTable := dao.BuildUnionSubquery(s.db, "union_activities", dao.FullActivity{}, sprintActivity)

	query := unionTable.
		Joins("Sprint").
		Joins("Workspace").
		Joins("Actor").
		Order("union_activities.created_at desc").
		Where("union_activities.workspace_id = ?", workspaceId).
		Where("union_activities.sprint_id = ?", sprintId)

	var acts []dao.FullActivity

	resp, err := dao.PaginationRequest(
		offset,
		limit,
		query,
		&acts,
	)
	if err != nil {
		return EError(c, err)
	}

	resp.Result = utils.SliceToSlice(resp.Result.(*[]dao.FullActivity), func(pa *dao.FullActivity) dto.EntityActivityFull { return *pa.ToDTO() })

	return c.JSON(http.StatusOK, resp)
}

// updateSprintView godoc
// @id updateSprintView
// @Summary Спринты: установка фильтров для просмотра
// @Description Позволяет пользователю установить настройки просмотра для конкретного спринта.
// @Tags Sprint
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param sprintId path string true "Идентификатор или номер последовательности спринта"
// @Param view_props body types.ViewProps true "Настройки просмотра проекта"
// @Success 204 {string} string "Настройки просмотра успешно обновлены"
// @Failure 400 {object} apierrors.DefinedError "Ошибка при установке настроек просмотра"
// @Failure 404 {object} apierrors.DefinedError "Проект не найден"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/sprints/{sprintId}/sprint-view/ [post]
func (s *Services) updateSprintView(c echo.Context) error {
	apiContext := apicontext.GetContext(c)
	sprint := apiContext.GetSprint()
	if apiContext.Error() != nil {
		return EError(c, apiContext.Error())
	}
	user := apiContext.GetUser()

	var viewProps types.ViewProps

	if err := c.Bind(&viewProps); err != nil {
		return EError(c, err)
	}

	if err := c.Validate(viewProps); err != nil {
		return EErrorDefined(c, apierrors.ErrInvalidSprintViewProps.WithFormattedMessage(err))
	}

	view := dao.SprintViews{
		Id:        dao.GenUUID(),
		SprintId:  sprint.Id,
		MemberId:  user.ID,
		ViewProps: viewProps,
	}

	if err := s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "sprint_id"}, {Name: "member_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"view_props", "updated_at"}),
	}).Create(&view).Error; err != nil {
		return EError(c, err)
	}

	return c.NoContent(http.StatusOK)
}

// getSprintStates godoc
// @id getSprintStates
// @Summary Спринты: получение состояний задач в спринте
// @Description Возвращает список всех состояний задач, которые используются в задачах текущего спринта.
// @Tags Sprint
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param sprintId path string true "Идентификатор или номер последовательности спринта"
// @Success 200 {array} dto.StateLight "Список состояний задач"
// @Failure 400 {object} apierrors.DefinedError "Ошибка запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Спринт не найден"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/sprints/{sprintId}/states/ [get]
func (s *Services) getSprintStates(c echo.Context) error {
	apiContext := apicontext.GetContext(c)
	sprint := apiContext.GetSprint(apicontext.WithSprintIssues())
	if apiContext.Error() != nil {
		return EError(c, apiContext.Error())
	}

	projectMap := make(map[uuid.UUID]struct{})

	for _, i := range sprint.Issues {
		projectId := i.ProjectId
		projectMap[projectId] = struct{}{}
	}

	var states []dao.State
	if err := s.db.
		Where("project_id in (?)", slices.Collect(maps.Keys(projectMap))).
		Order("sequence").
		Find(&states).Error; err != nil {
		return EError(c, err)
	}
	return c.JSON(http.StatusOK, utils.SliceToSlice(&states, func(t *dao.State) *dto.StateLight { return t.ToLightDTO() }))
}

// addSprintFolders godoc
// @id addSprintFolders
// @Summary Спринты: добавление директории спринтов
// @Description Создает новую директорию для спринтов.
// @Tags Sprint
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param data body dto.RequestSprintFolder true "Данные папки спринтов"
// @Success 200 {object} dto.SprintFolder "Новая директория спринтов"
// @Failure 400 {object} apierrors.DefinedError "Ошибка запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Пространство не найдено"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/sprints-folder/ [post]
func (s *Services) addSprintFolders(c echo.Context) error {
	var req dto.RequestSprintFolder
	apiContext := apicontext.GetContext(c)
	workspace := apiContext.GetWorkspace()
	if apiContext.Error() != nil {
		return EError(c, apiContext.Error())
	}
	user := apiContext.GetUser()

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

	folder := &dao.SprintFolder{
		Id: dao.GenUUID(),

		CreatedById: user.ID,
		WorkspaceId: workspace.ID,
		CreatedBy:   *user,
		Workspace:   workspace,
		Name:        req.Name,
	}

	if err := s.db.Create(&folder).Error; err != nil {
		return EError(c, err)
	}

	return c.JSON(http.StatusCreated, folder.ToDTO())
}

// updateSprintFolders godoc
// @id updateSprintFolders
// @Summary Спринты: обновление директорий спринтов
// @Description Обновляет директорию спринта.
// @Tags Sprint
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param sprintFolderId path string true "Идентификатор директории спринта"
// @Param data body dto.RequestSprintFolder true "Данные папки спринтов"
// @Success 200 {array} dto.SprintFolder "Обновленная директория спринтов"
// @Failure 400 {object} apierrors.DefinedError "Ошибка запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Не найдено"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/sprints-folder/{sprintFolderId}/ [patch]
func (s *Services) updateSprintFolders(c echo.Context) error {
	var req dto.RequestSprintFolder
	apiContext := apicontext.GetContext(c)
	workspace := apiContext.GetWorkspace()
	if apiContext.Error() != nil {
		return EError(c, apiContext.Error())
	}
	user := apiContext.GetUser()
	sprintFolderId := strings.TrimSuffix(c.Param("sprintFolderId"), "/")

	var folder dao.SprintFolder
	if err := s.db.Where("workspace_id = ?", workspace.ID).
		Where("id = ?", sprintFolderId).First(&folder).Error; err != nil {
		return EError(c, err)
	}

	err := c.Bind(&req)
	if err != nil {
		return EError(c, apierrors.ErrSprintBadRequest)
	}
	if req.Name == "" || req.Name == folder.Name {
		return EErrorDefined(c, apierrors.ErrSprintRequestValidate)
	}

	if err := c.Validate(req); err != nil {
		return EErrorDefined(c, apierrors.ErrSprintRequestValidate)
	}

	folder.Name = req.Name
	folder.UpdatedById = uuid.NullUUID{UUID: user.ID, Valid: true}
	folder.UpdatedBy = user

	if err := s.db.Updates(&folder).Error; err != nil {
		return EError(c, err)
	}

	return c.JSON(http.StatusCreated, folder.ToDTO())
}

// deleteSprintFolders godoc
// @id deleteSprintFolders
// @Summary Спринты: удаление директорий спринтов
// @Description Удаляет директорию спринта.
// @Tags Sprint
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param sprintFolderId path string true "Идентификатор директории спринта"
// @Success 204 "Директория успешно удалена"
// @Failure 400 {object} apierrors.DefinedError "Ошибка запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Не найдено"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/sprints-folder/{sprintFolderId}/ [delete]
func (s *Services) deleteSprintFolders(c echo.Context) error {
	apiContext := apicontext.GetContext(c)
	workspace := apiContext.GetWorkspace()
	if apiContext.Error() != nil {
		return EError(c, apiContext.Error())
	}
	sprintFolderId := strings.TrimSuffix(c.Param("sprintFolderId"), "/")
	if err := s.db.Transaction(func(tx *gorm.DB) error {

		var exists bool
		if err := s.db.Model(&dao.Sprint{}).
			Select("EXISTS(?)",
				s.db.Model(&dao.Sprint{}).
					Select("1").
					Where("sprint_folder_id = ?", sprintFolderId),
			).
			Find(&exists).Error; err != nil {
			return err
		}
		if exists {
			return apierrors.ErrSprintFolderDelete
		}

		if err := s.db.Where("workspace_id = ?", workspace.ID).
			Where("id = ?", sprintFolderId).Delete(&dao.SprintFolder{}).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		return EError(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}

type sprintDto struct {
	dto.RequestSprint
}

func (rs *sprintDto) toDao(ctx echo.Context) (*dao.Sprint, error) {
	if rs == nil {
		return nil, fmt.Errorf("empty sprint")
	}
	apiContext := apicontext.GetContext(ctx)
	workspace := apiContext.GetWorkspace()
	workspaceMember := apiContext.GetWorkspaceMember()
	if apiContext.Error() != nil {
		return nil, apiContext.Error()
	}

	return &dao.Sprint{
		Id:          dao.GenUUID(),
		CreatedById: workspaceMember.MemberId,

		WorkspaceId:    workspace.ID,
		CreatedBy:      dao.User{},
		Name:           rs.Name,
		Description:    rs.Description,
		StartDate:      rs.StartDate.ToNullTime(),
		EndDate:        rs.EndDate.ToNullTime(),
		SprintFolderId: rs.Folder,
	}, nil
}
