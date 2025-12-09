// Обработка разрешений для доступа к рабочим пространствам.
// Проверяет права пользователя на выполнение определенных действий в рабочем пространстве.
//
// Основные возможности:
//   - Проверка прав доступа на основе роли пользователя и владельца рабочего пространства.
//   - Разрешение доступа для суперпользователей.
//   - Разрешение доступа для действий с файлами 'favorites'.
//   - Разрешение доступа для административных форм.
//   - Разрешение доступа для операций с документами (doc).
//   - Разделение на безопасные и административные методы запросов.
package aiplan

import (
	"errors"
	"net/http"
	"strings"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/apierrors"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"

	"github.com/labstack/echo/v4"
)

// WorkspacePermissionMiddleware WorkspacePermissionHandler godoc
// @Summary Проверка разрешений рабочего пространства
// @Description Проверяет, есть ли у пользователя разрешение на выполнение действия в рабочем пространстве.
// @Tags Workspace
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Failure 403 {object} apierrors.DefinedError "Ошибка: доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
func (s *Services) WorkspacePermissionMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		has, err := s.hasWorkspacePermission(c)
		if err != nil {
			return EError(c, err)
		}
		if !has {
			return EErrorDefined(c, apierrors.ErrWorkspaceForbidden)
		}
		return next(c)
	}
}

func (s *Services) hasWorkspacePermission(c echo.Context) (bool, error) {
	workspaceContext, ok := c.(WorkspaceContext)
	if !ok {
		return false, errors.New("wrong context")
	}
	workspaceMember := workspaceContext.WorkspaceMember

	if strings.HasSuffix(c.Path(), "/me/notifications/") && c.Request().Method == http.MethodPost {
		return true, nil
	}

	if strings.Contains(c.Path(), "/backups/") && workspaceMember.Role != 15 && workspaceContext.Workspace.OwnerId != workspaceContext.User.ID.String() {
		return false, nil
	}

	// Allow favorites edit
	if strings.Contains(c.Path(), "/user-favorite-projects/") {
		return true, nil
	}
	if strings.Contains(c.Path(), "/user-favorite-docs/") {
		return true, nil
	}

	// Allow admin form
	if c.Path() == "/api/auth/workspaces/:workspaceSlug/forms/" {
		if workspaceMember.Role == types.AdminRole {
			return true, nil
		} else {
			return false, nil
		}
	}

	// Allow doc all (look at doc-permission)
	if strings.Contains(c.Path(), "/api/auth/workspaces/:workspaceSlug/doc/") {
		return true, nil
	}

	// Allow sprint member & admin (look at sprint-permission)
	if strings.Contains(c.Path(), "/api/auth/workspaces/:workspaceSlug/sprints/:sprintId/issues/search/") {
		return workspaceMember.Role > types.GuestRole, nil
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
		if workspaceMember.Role == 15 ||
			workspaceContext.Workspace.OwnerId == workspaceContext.User.ID.String() {
			return true, nil
		}
	}
	return false, nil
}
