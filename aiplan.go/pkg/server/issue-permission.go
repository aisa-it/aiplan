package server

import (
	"github.com/aisa-it/aiplan/aiplan.go/pkg/apierrors"
	"github.com/labstack/echo/v4"
)

// IssuePermissionMiddleware проверяет право на действие роута задачи.
// Права на проект, пространство, спринт, документ и форму проверяются тем же
// permissionMiddleware через помощники регистрации роутов (route-actions.go).
func (s *Services) IssuePermissionMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return s.permissionMiddleware(apierrors.ErrIssueForbidden)(next)
}
