// Пакет предоставляет middleware для защиты API endpoints в приложении AiPlan.
// Он проверяет права доступа пользователей на основе ролей проекта и выдает ошибки, если права отсутствуют.
//
// Основные возможности:
//   - Проверка прав доступа к проектам.
//   - Проверка прав доступа к задачам (issues). Предоставляет различные уровни доступа в зависимости от роли пользователя и метода запроса.
//   - Разграничение прав для администраторов проекта и обычных членов команды.
package server

import (
	"errors"
	"net/http"
	"strings"

	apicontext "github.com/aisa-it/aiplan/aiplan.go/pkg/api-context"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/apierrors"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/types"

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
	apiContext := apicontext.GetContext(c)
	if apiContext == nil {
		return false, errors.New("wrong context")
	}

	// Lightweight checks without load
	{
		// Allow projectMember update notification
		if strings.HasSuffix(c.Path(), "/me/notifications/") && c.Request().Method == http.MethodPost {
			return true, nil
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
		case
			http.MethodPut,
			http.MethodPost,
			http.MethodPatch,
			http.MethodDelete:
			if strings.Contains(c.Path(), "/project-views/") {
				return true, nil
			}
		}
	}

	user := apiContext.GetUser()
	workspaceMember := apiContext.GetWorkspaceMember()
	project := apiContext.GetProject()
	projectMember := apiContext.GetProjectMember()
	if apiContext.Error() != nil {
		return false, apiContext.Error()
	}

	// Allow workspace admin all
	if workspaceMember.Role == types.AdminRole {
		return true, nil
	}

	// Allow issue creation to admins and members
	if strings.HasSuffix(c.Path(), "/issues/") && c.Request().Method == http.MethodPost {
		return projectMember.Role > types.GuestRole, nil
	}

	switch c.Request().Method {
	// Admin methods
	case
		http.MethodPut,
		http.MethodPost,
		http.MethodPatch,
		http.MethodDelete:
		if projectMember.Role == 15 || project.ProjectLeadId == user.ID {
			return true, nil
		}
	}
	return false, nil
}

func (s *Services) hasProjectAdminPermissions(c echo.Context) (bool, error) {
	apiContext := apicontext.GetContext(c)
	if apiContext == nil {
		return false, errors.New("wrong context")
	}

	// Lightweight checks without load
	{
		// Allow projectMember update notification
		if strings.HasSuffix(c.Path(), "/me/notifications/") && c.Request().Method == http.MethodPost {
			return true, nil
		}
	}

	projectMember := apiContext.GetProjectMember()
	if apiContext.Error() != nil {
		return false, apiContext.Error()
	}

	// Allow project admin all
	if projectMember.Role == types.AdminRole {
		return true, nil
	}

	return false, nil
}

// IssuePermissionMiddleware проверяет право на действие роута.
//
// Действие берётся из контекста, куда его кладёт регистрация роута,
// поэтому middleware обязан быть роутовым: групповые выполняются раньше
// и действия не увидят. Неразмеченный роут отклоняется — молча пропустить
// запрос без проверки нельзя.
func (s *Services) IssuePermissionMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		action, ok := ActionOf(c)
		if !ok {
			return EErrorDefined(c, apierrors.ErrIssueForbidden)
		}

		apiContext := apicontext.GetContext(c)
		if apiContext == nil {
			return EError(c, errors.New("wrong context"))
		}

		if err := s.policy.Authorize(c.Request().Context(), action, apiContext); err != nil {
			return EError(c, err)
		}
		return next(c)
	}
}
