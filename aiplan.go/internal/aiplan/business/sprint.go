package business

import (
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

func GetSprintStatsByWorkspace(tx *gorm.DB, workspaceID uuid.UUID) (map[uuid.UUID]types.SprintStats, error) {
	var rows []struct {
		SprintID   uuid.UUID
		AllIssues  int
		Pending    int
		InProgress int
		Completed  int
		Cancelled  int
	}

	if err := tx.
		Model(&dao.SprintIssue{}).
		Select(`
			sprint_issues.sprint_id as sprint_id,
			COUNT(*) as all_issues,
			SUM(CASE WHEN s.group = 'backlog' OR s.group = 'unstarted' THEN 1 ELSE 0 END) as pending,
			SUM(CASE WHEN s.group = 'started' THEN 1 ELSE 0 END) as in_progress,
			SUM(CASE WHEN s.group = 'completed' THEN 1 ELSE 0 END) as completed,
			SUM(CASE WHEN s.group = 'cancelled' THEN 1 ELSE 0 END) as cancelled
		`).
		Joins("JOIN issues i ON i.id = sprint_issues.issue_id").
		Joins("JOIN states s ON s.id = i.state_id").
		Where("sprint_issues.workspace_id = ?", workspaceID).
		Group("sprint_issues.sprint_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	result := make(map[uuid.UUID]types.SprintStats, len(rows))
	for _, r := range rows {
		result[r.SprintID] = types.SprintStats{
			AllIssues:  r.AllIssues,
			Pending:    r.Pending,
			InProgress: r.InProgress,
			Completed:  r.Completed,
			Cancelled:  r.Cancelled,
		}
	}
	return result, nil
}
