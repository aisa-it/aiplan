package aiplan

import (
	"github.com/gofrs/uuid"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm/clause"
	"net/http"
	"sheff.online/aiplan/internal/aiplan/apierrors"
	"sheff.online/aiplan/internal/aiplan/dao"
	"sheff.online/aiplan/internal/aiplan/dto"
	"sheff.online/aiplan/internal/aiplan/types"
	"sheff.online/aiplan/internal/aiplan/utils"
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
			Where("sprints.workspace_id = ?", workspace.ID)

		if val, err := uuid.FromString(sprintId); err != nil {
			query = query.Where("sprints.sequence_id = ?", sprintId)
		} else {
			query = query.Where("sprints.id = ?", val.String())
		}

		if err := query.First(&sprint).Error; err != nil {
			return EError(c, err)
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

	sprintAdminGroup.POST("/issues/add/", s.addIssuesToSprint)
	sprintAdminGroup.DELETE("/issues/remove/", s.removeIssuesFromSprint)
	sprintAdminGroup.POST("/members/add/", s.addSprintWatchers)
	sprintAdminGroup.DELETE("/members/remove/", s.removeSprintWatchers)

	sprintGroup.GET("/", s.GetSprint)

}

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

func (s *Services) createSprint(c echo.Context) error {
	var req requestSprint

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

	sprint, err := req.toDao(nil, c)
	if err != nil {
		return EError(c, err)
	}

	if err := s.db.Create(&sprint).Error; err != nil {
		return EError(c, err)
	}
	return c.JSON(http.StatusCreated, sprint.ToDTO())
}

func (s *Services) GetSprint(c echo.Context) error {
	sprint := c.(SprintContext).Sprint
	return c.JSON(http.StatusOK, sprint.ToDTO())
}

func (s *Services) updateSprint(c echo.Context) error {
	sprint := c.(SprintContext).Sprint
	user := c.(SprintContext).User

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

	return c.JSON(http.StatusOK, sprint.ToDTO())
}

func (s *Services) addIssuesToSprint(c echo.Context) error {
	workspace := c.(SprintContext).Workspace
	sprint := c.(SprintContext).Sprint
	user := c.(SprintContext).User

	workspaceUUID, err := utils.UuidFromId(workspace.ID)
	if err != nil {
		return EError(c, err)
	}

	userUUID, err := utils.UuidFromId(user.ID)
	if err != nil {
		return EError(c, err)
	}

	var req requestIssueIdList

	err = c.Bind(&req)
	if err != nil {
		return EError(c, apierrors.ErrSprintBadRequest)
	}

	var issues []dao.Issue

	if err := s.db.
		Where("workspace_id", workspace.ID).
		Where("id in (?)", req.Issues).
		Where("id not in (?)",
			s.db.
				Select("issue_id::text").
				Where("workspace_id", workspace.ID).
				Where("sprint_id = ?", sprint.Id).
				Model(&dao.SprintIssue{})).
		Find(&issues).Error; err != nil {
		return EError(c, err)
	}

	var maxPosition int
	if err := s.db.Model(&dao.SprintIssue{}).
		Unscoped().
		Where("workspace_id = ? AND sprint_id = ?", workspaceUUID, sprint.Id).
		Select("COALESCE(MAX(position), 0)").
		Scan(&maxPosition).Error; err != nil {
		return EError(c, err)
	}

	var sprintIssues []dao.SprintIssue
	for i, issue := range issues {

		projectUUID, err := utils.UuidFromId(issue.ProjectId)
		if err != nil {
			return EError(c, err)
		}
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

	if err := s.db.CreateInBatches(&sprintIssues, 10).Error; err != nil {
		return err
	}

	return c.NoContent(http.StatusUpgradeRequired)
}

func (s *Services) removeIssuesFromSprint(c echo.Context) error {
	workspace := c.(SprintContext).Workspace
	sprint := c.(SprintContext).Sprint

	var req requestIssueIdList

	err := c.Bind(&req)
	if err != nil {
		return EError(c, apierrors.ErrSprintBadRequest)
	}

	if err := s.db.
		Where("workspace_id = ?", workspace.ID).
		Where("sprint_id = ?", sprint.Id).
		Where("issue_id IN (?)", req.Issues).
		Delete(&dao.SprintIssue{}).Error; err != nil {
		return EError(c, err)
	}

	return c.NoContent(http.StatusOK)
}

func (s *Services) deleteSprint(c echo.Context) error {
	sprint := c.(SprintContext).Sprint
	if err := s.db.Delete(&sprint).Error; err != nil {
		return EError(c, err)
	}
	return c.NoContent(http.StatusOK)
}

func (s *Services) addSprintWatchers(c echo.Context) error {
	workspace := c.(SprintContext).Workspace
	sprint := c.(SprintContext).Sprint
	user := c.(SprintContext).User

	workspaceUUID, err := utils.UuidFromId(workspace.ID)
	if err != nil {
		return EError(c, err)
	}

	userUUID, err := utils.UuidFromId(user.ID)
	if err != nil {
		return EError(c, err)
	}

	var req requestUserIdList

	err = c.Bind(&req)
	if err != nil {
		return EError(c, apierrors.ErrSprintBadRequest)
	}

	var workspaceMembers []dao.WorkspaceMember

	if err := s.db.
		Where("workspace_id", workspace.ID).
		Where("member_id in (?)", req.Members).
		Where("member_id not in (?)",
			s.db.
				Select("watcher_id::text").
				Where("workspace_id", workspace.ID).
				Where("sprint_id = ?", sprint.Id).
				Model(&dao.SprintWatcher{})).
		Find(&workspaceMembers).Error; err != nil {
		return EError(c, err)
	}

	var sprintWatchers []dao.SprintWatcher
	for _, member := range workspaceMembers {
		memberUUID, err := utils.UuidFromId(member.MemberId)
		if err != nil {
			return EError(c, err)
		}
		sprintWatchers = append(sprintWatchers, dao.SprintWatcher{
			Id:          dao.GenUUID(),
			CreatedById: userUUID,
			WatcherId:   memberUUID,
			SprintId:    sprint.Id,
			WorkspaceId: workspaceUUID,
		})
	}

	if err := s.db.CreateInBatches(&sprintWatchers, 10).Error; err != nil {
		return err
	}

	return c.NoContent(http.StatusUpgradeRequired)
}

func (s *Services) removeSprintWatchers(c echo.Context) error {
	workspace := c.(SprintContext).Workspace
	sprint := c.(SprintContext).Sprint

	var req requestUserIdList

	err := c.Bind(&req)
	if err != nil {
		return EError(c, apierrors.ErrSprintBadRequest)
	}

	if err := s.db.
		Where("workspace_id = ?", workspace.ID).
		Where("sprint_id = ?", sprint.Id).
		Where("watcher_id IN (?)", req.Members).
		Delete(&dao.SprintWatcher{}).Error; err != nil {
		return EError(c, err)
	}

	return c.NoContent(http.StatusOK)
}

//

type requestSprint struct {
	Name        string             `json:"name,omitempty"`
	Description types.RedactorHTML `json:"description,omitempty" swaggertype:"string"`
	StartDate   *types.TargetDate  `json:"start_date,omitempty" extensions:"x-nullable" swaggertype:"string"`
	EndDate     *types.TargetDate  `json:"end_date,omitempty" extensions:"x-nullable" swaggertype:"string"`
}

type requestIssueIdList struct {
	Issues []string `json:"issues,omitempty"`
}

type requestUserIdList struct {
	Members []string `json:"members,omitempty"`
}

func (rs *requestSprint) toDao(sprint *dao.Sprint, ctx echo.Context) (*dao.Sprint, error) {
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

	userUUID, err := utils.UuidFromId(workspaceMember.MemberId)
	if err != nil {
		return nil, err
	}

	workspaceUUID, err := utils.UuidFromId(workspace.ID)
	if err != nil {
		return nil, err
	}

	if sprint == nil {
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
	} else {
		//TODO add update
		return nil, nil
	}
}
