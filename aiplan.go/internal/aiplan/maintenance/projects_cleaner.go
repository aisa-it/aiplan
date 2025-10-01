// Пакет для очистки данных о проектах, помеченных как удаленные (soft-deleted). Выполняет их окончательное удаление из базы данных для освобождения ресурсов и обеспечения целостности данных.
//
// Основные возможности:
//   - Находит мягко удаленные проекты.
//   - Выполняет их жесткое удаление из базы данных.
package maintenance

import (
	"log/slog"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"gorm.io/gorm"
)

type ProjectsCleaner struct {
	db *gorm.DB
}

func NewProjectsCleaner(db *gorm.DB) *ProjectsCleaner {
	return &ProjectsCleaner{db: db}
}

func (pc *ProjectsCleaner) CleanProjects() {
	slog.Info("Start hard delete projects")
	var projects []dao.Project
	if err := pc.db.Unscoped().Where("deleted_at is not NULL").Limit(5).Find(&projects).Error; err != nil {
		slog.Error("Get soft deleted project", "err", err)
		return
	}

	for _, project := range projects {
		if err := pc.db.Unscoped().Delete(&project).Error; err != nil {
			slog.Error("Hard delete project", "projectId", project.ID, "err", err)
		}
	}
	slog.Info("Finish hard delete projects")
}
