package aiplan

import (
	"errors"
	"net/http"
	"strings"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/apierrors"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/labstack/echo/v4"
)

func (s *Services) SprintPermissionMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		has, err := s.hasSprintPermissions(c)
		if err != nil {
			return EError(c, err)
		}
		if !has {
			return EErrorDefined(c, apierrors.ErrSprintForbidden)
		}
		return next(c)
	}
}

func (s *Services) hasSprintPermissions(c echo.Context) (bool, error) {
	sprintContext, ok := c.(SprintContext)
	if !ok {
		return false, errors.New("wrong context")
	}
	workspaceMember := sprintContext.WorkspaceMember
	sprint := sprintContext.Sprint
	user := sprintContext.User

	// Allow Author
	if user.ID == sprint.CreatedById {
		return true, nil
	}

	if sprintContext.User.IsSuperuser {
		return true, nil
	}

	if onlyReadMethod(c) {
		if workspaceMember.Role >= types.MemberRole {
			return true, nil
		}
		return false, nil
	}

	if strings.HasSuffix(c.Path(), "/issues/search/") && c.Request().Method == http.MethodPost {
		return workspaceMember.Role > types.GuestRole, nil
	}
	if strings.HasSuffix(c.Path(), "/sprint-view/") && c.Request().Method == http.MethodPost {
		return workspaceMember.Role > types.GuestRole, nil
	}

	return false, nil
}

func (s *Services) SprintAdminPermissionMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		has, err := s.hasSprintAdminPermissions(c)
		if err != nil {
			return EError(c, err)
		}
		if !has {
			return EErrorDefined(c, apierrors.ErrSprintForbidden)
		}
		return next(c)
	}
}

func (s *Services) hasSprintAdminPermissions(c echo.Context) (bool, error) {
	sprintContext, ok := c.(SprintContext)
	if !ok {
		return false, errors.New("wrong context")
	}

	// Allow project admin all
	if sprintContext.WorkspaceMember.Role == types.AdminRole {
		return true, nil
	}

	return false, nil
}
