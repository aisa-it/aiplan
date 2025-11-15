package aiplan

import (
	"errors"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/apierrors"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

func (s *Services) BackupPermissionMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		has, err := s.hasBackupPermissions(c)
		if err != nil {
			return EError(c, err)
		}
		if !has {
			return EErrorDefined(c, apierrors.ErrBackupForbidden)
		}
		return next(c)
	}
}

func (s *Services) hasBackupPermissions(c echo.Context) (bool, error) {
	backupContext, ok := c.(BackupContext)
	if !ok {
		return false, errors.New("wrong context")
	}
	backup := backupContext.Backup
	//user := backupContext.User

	var workspace *dao.Workspace

	if err := s.db.Where("id = ?", backup.WorkspaceId).First(&workspace).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return false, err
		}
	}

	return true, nil
}
