package migration

import (
	"fmt"
	"log/slog"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"gorm.io/gorm"
)

func DeduplicateSprintFolders(db *gorm.DB) error {
	type dupRow struct {
		WorkspaceID string
		Name        string
	}
	var duplicates []dupRow
	if err := db.Raw(`SELECT workspace_id::text, name FROM sprint_folders GROUP BY workspace_id, name HAVING COUNT(*) > 1`).Scan(&duplicates).Error; err != nil {
		return err
	}
	if len(duplicates) == 0 {
		return nil
	}
	for _, d := range duplicates {
		var folders []dao.SprintFolder
		if err := db.Where("workspace_id = ? AND name = ?", d.WorkspaceID, d.Name).
			Order("created_at ASC").Find(&folders).Error; err != nil {
			return err
		}
		for i, dup := range folders[1:] {
			dup.Name = fmt.Sprintf("%s-%d", dup.Name, i+1)
			if err := db.Save(&dup).Error; err != nil {
				return err
			}
		}
	}
	slog.Info("Sprint folders deduplicated successfully", "duplicate_groups", len(duplicates))
	return nil
}
