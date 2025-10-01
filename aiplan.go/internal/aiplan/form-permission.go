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

	"github.com/aisa-it/aiplan/internal/aiplan/apierrors"
	"github.com/aisa-it/aiplan/internal/aiplan/types"
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
	formContext, ok := c.(FormContext)
	if !ok {
		return false, errors.New("wrong context")
	}
	workspaceMember := formContext.WorkspaceMember

	// Allow workspace admin all
	if workspaceMember.Role == types.AdminRole {
		return true, nil
	}

	return false, nil
}
