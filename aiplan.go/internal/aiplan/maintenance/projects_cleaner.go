// Package maintenance содержит фоновые задачи очистки данных,
// запускаемые периодически по cron-расписанию.
//
// Задачи:
//   - ProjectsCleaner — hard delete проектов с deleted_at != NULL
//   - WorkspacesCleaner — hard delete рабочих пространств
//   - AssetsCleaner — удаление осиротевших файлов из S3/MinIO
//   - UserCleanup — очистка неактивных пользователей
//   - LDAPSync — синхронизация пользователей с LDAP-сервером
//
// Soft delete → Hard delete реализует двухэтапное удаление:
// сначала данные помечаются как удалённые (для возможности восстановления),
// затем физически удаляются фоновой задачей.
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
