// Пакет предоставляет middleware для защиты API endpoints в приложении AiPlan.
// Он проверяет права доступа пользователей на основе ролей проекта и выдает ошибки, если права отсутствуют.
//
// Основные возможности:
//   - Проверка прав доступа к проектам.
//   - Проверка прав доступа к задачам (issues). Предоставляет различные уровни доступа в зависимости от роли пользователя и метода запроса.
//   - Разграничение прав для администраторов проекта и обычных членов команды.
package aiplan

import (
	"errors"
	"net/http"
	"strings"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/apierrors"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"

	"github.com/labstack/echo/v4"
)

func (s *Services) ProjectPermissionMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		has, err := s.hasProjectPermissions(c)
		if err != nil {
			return EError(c, err)
		}
		if !has {
			return EErrorDefined(c, apierrors.ErrProjectForbidden)
		}
		return next(c)
	}
}

func (s *Services) ProjectAdminPermissionMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		has, err := s.hasProjectAdminPermissions(c)
		if err != nil {
			return EError(c, err)
		}
		if !has {
			return EErrorDefined(c, apierrors.ErrProjectForbidden)
		}
		return next(c)
	}
}

func (s *Services) hasProjectPermissions(c echo.Context) (bool, error) {
	projectContext, ok := c.(ProjectContext)
	if !ok {
		return false, errors.New("wrong context")
	}
	projectMember := projectContext.ProjectMember
	//if projectContext.User.IsSuperuser {
	//	return true, nil
	//}

	// Allow projectMember update notification
	if strings.HasSuffix(c.Path(), "/me/notifications/") && c.Request().Method == http.MethodPost {
		return true, nil
	}

	// Allow workspace admin all
	if projectContext.WorkspaceMember.Role == types.AdminRole {
		return true, nil
	}

	// Allow issue creation to admins and members
	if strings.HasSuffix(c.Path(), "/issues/") && c.Request().Method == http.MethodPost {
		return projectMember.Role > types.GuestRole, nil
	}

	if strings.HasSuffix(c.Path(), "/issue-labels/") && c.Request().Method == http.MethodPost {
		return projectMember.Role > types.GuestRole, nil
	}

	// Allow search to all members
	if strings.HasSuffix(c.Path(), "/issues/search/") && c.Request().Method == http.MethodPost {
		return true, nil
	}

	switch c.Request().Method {
	//Safe methods
	case
		http.MethodGet,
		http.MethodOptions,
		http.MethodHead:
		return true, nil

		// Admin methods
	case
		http.MethodPut,
		http.MethodPost,
		http.MethodPatch,
		http.MethodDelete:
		if strings.Contains(c.Path(), "/project-views/") {
			return true, nil
		}

		if projectMember.Role == 15 || projectContext.Project.ProjectLeadId == projectContext.User.ID {
			return true, nil
		}
	}
	return false, nil
}

func (s *Services) hasProjectAdminPermissions(c echo.Context) (bool, error) {
	projectContext, ok := c.(ProjectContext)
	if !ok {
		return false, errors.New("wrong context")
	}

	// Allow projectMember update notification
	if strings.HasSuffix(c.Path(), "/me/notifications/") && c.Request().Method == http.MethodPost {
		return true, nil
	}

	// Allow project admin all
	if projectContext.ProjectMember.Role == types.AdminRole {
		return true, nil
	}

	return false, nil
}

func (s *Services) IssuePermissionMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		has, err := s.hasIssuePermissions(c)
		if err != nil {
			return EError(c, err)
		}
		if !has {
			return EErrorDefined(c, apierrors.ErrIssueForbidden)
		}
		return next(c)
	}
}

func (s *Services) hasIssuePermissions(c echo.Context) (bool, error) {
	issueContext, ok := c.(IssueContext)
	if !ok {
		return false, errors.New("wrong context")
	}
	projectMember := issueContext.ProjectMember

	if issueContext.User.ID == issueContext.Issue.Author.ID {
		return true, nil
	}
	// Allow workspace admin all
	if issueContext.WorkspaceMember.Role == types.AdminRole {
		return true, nil
	}

	if strings.HasSuffix(c.Path(), "/issue-labels/") && c.Request().Method == http.MethodPost {
		if issueContext.Issue.CreatedById == issueContext.User.ID {
			return true, nil
		}
		for _, user := range *issueContext.Issue.Assignees {
			if user.ID == issueContext.User.ID {
				return true, nil
			}
		}
		return false, nil
	}

	//Safe methods
	switch c.Request().Method {
	case
		http.MethodGet,
		http.MethodOptions,
		http.MethodHead:
		return true, nil
	}

	switch projectMember.Role {
	case types.AdminRole:
		return true, nil
	case types.MemberRole:
		// Allow all edits to issue
		if c.Path() == "/api/auth/workspaces/:workspaceSlug/projects/:projectId/issues/:issueIdOrSeq/" {
			return true, nil
		}

		// Allow all comments operations
		if strings.Contains(c.Path(), "/comments/") {
			return true, nil
		}

		if issueContext.Issue.CreatedById == issueContext.User.ID {
			// If issue author
			return true, nil
		} else {
			return issueContext.Issue.IsAssignee(issueContext.User.ID.String()), nil
		}
	}

	return false, nil
}
