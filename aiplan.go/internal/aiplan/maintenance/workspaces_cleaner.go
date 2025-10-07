// Удаляет мягко удаленные рабочие пространства из базы данных.
package maintenance

import (
	"log/slog"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"gorm.io/gorm"
)

type WorkspacesCleaner struct {
	db *gorm.DB
}

func NewWorkspacesCleaner(db *gorm.DB) *WorkspacesCleaner {
	return &WorkspacesCleaner{db: db}
}

func (pc *WorkspacesCleaner) CleanWorkspaces() {
	slog.Info("Start hard delete workspaces")
	var workspaces []dao.Workspace
	if err := pc.db.Unscoped().Where("deleted_at is not NULL").Limit(5).Find(&workspaces).Error; err != nil {
		slog.Error("Get soft deleted workspaces", "err", err)
		return
	}

	for _, space := range workspaces {
		if err := pc.db.Unscoped().Delete(&space).Error; err != nil {
			slog.Error("Hard delete workspace", "workspaceId", space.ID, "err", err)
		}
	}
	slog.Info("Finish hard delete workspaces")
}
