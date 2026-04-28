// Пакет предоставляет middleware для проверки прав доступа к форме.
// Middleware проверяет, является ли пользователь администратором рабочего пространства, и разрешает доступ, если это так.
// Если пользователь не является администратором, доступ запрещается.
//
// Основные возможности:
//   - Проверка прав доступа на основе роли пользователя в рабочем пространстве.
//   - Предотвращение доступа к форме для пользователей, не являющихся администраторами.
//   - Обработка ошибок при некорректном контексте.
package aiplan

import (
	"errors"

	apicontext "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/api-context"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/apierrors"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/labstack/echo/v4"
)

func (s *Services) FormPermissionMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		has, err := s.hasFormPermissions(c)
		if err != nil {
			return EError(c, err)
		}
		if !has {
			return EErrorDefined(c, apierrors.ErrFormForbidden)
		}
		return next(c)
	}
}

func (s *Services) hasFormPermissions(c echo.Context) (bool, error) {
	if _, ok := c.(FormContext); !ok {
		return false, errors.New("wrong context")
	}
	apiContext := apicontext.GetContext(c)
	workspaceMember := apiContext.GetWorkspaceMember()
	if apiContext.Error() != nil {
		return false, apiContext.Error()
	}

	// Allow workspace admin all
	if workspaceMember.Role == types.AdminRole {
		return true, nil
	}

	return false, nil
}
