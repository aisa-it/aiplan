package aiplan

import (
	"errors"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
	"net/http"
	"sheff.online/aiplan/internal/aiplan/apierrors"
	"sheff.online/aiplan/internal/aiplan/dao"
)

type SprintContext struct {
	WorkspaceContext
	Sprint dao.Sprint
}

func (s *Services) SprintMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		sprintId := c.Param("sprintId")
		workspace := c.(WorkspaceContext).Workspace
		user := c.(WorkspaceContext).User

		var sprint dao.Sprint
		if err := s.db.
			Joins("Workspace").
			Where("sprints.workspace_id = ?", workspace.ID).
			Where("sprints.id = ? OR sprints.SequenceId = ?", sprintId, sprintId). // Search by id or SequenceId
			Set("userId", user.ID).
			First(&sprint).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return EErrorDefined(c, apierrors.ErrProjectNotFound)
			}
			return EError(c, err)
		}

		return next(SprintContext{c.(WorkspaceContext), sprint})
	}
}

func (s *Services) AddSprintServices(g *echo.Group) {
	workspaceGroup := g.Group("workspaces/:workspaceSlug", s.WorkspaceMiddleware)
	workspaceGroup.Use(s.LastVisitedWorkspaceMiddleware)
	workspaceGroup.Use(s.WorkspacePermissionMiddleware)

	sprintGroup := workspaceGroup.Group("/sprints/:sprintId", s.SprintMiddleware)
	sprintGroup.Use(s.SprintPermissionMiddleware)

	//sprintAdminGroup := sprintGroup.Group("", s.SprintAdminPermissionMiddleware)

	workspaceGroup.GET("/sprints/", s.getSprintList)

}

func (s *Services) getSprintList(c echo.Context) error {
	//user := c.(SprintContext).User
	workspace := c.(SprintContext).Workspace
	//workspaceMember := c.(SprintContext).WorkspaceMember

	//searchQuery := ""
	//if err := echo.QueryParamsBinder(c).
	//	String("search_query", &searchQuery).
	//	BindError(); err != nil {
	//	return EError(c, err)
	//}

	var sprints []dao.Sprint
	query := s.db.
		Joins("Workspace").
		Joins("Workspace.Owner").
		Where("sprints.workspace_id = ?", workspace.ID).
		Order("sequence_id")

	if err := query.Find(&sprints).Error; err != nil {
		return EError(c, err)
	}
	return c.JSON(
		http.StatusOK,
		sprints)
	//utils.SliceToSlice(&sprint, func(p *dao.ProjectWithCount) dto.ProjectLight { return *p.ToLightDTO() }))
}
