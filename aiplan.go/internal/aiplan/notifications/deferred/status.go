package deferred

import (
	"log/slog"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
)

func (np *NotificationProcessor) markCompleted(s *dao.NotificationSchedule) {
	now := time.Now().UTC()

	np.db.Model(&dao.NotificationSchedule{}).
		Where("id = ?", s.ID).
		Updates(map[string]interface{}{
			"status":       dao.StatusCompleted,
			"processed_at": now,
			"updated_at":   now,
		})
}

func (np *NotificationProcessor) markCancelled(s *dao.NotificationSchedule) {
	now := time.Now().UTC()

	np.db.Model(&dao.NotificationSchedule{}).
		Where("id = ?", s.ID).
		Updates(map[string]interface{}{
			"status":       dao.StatusCancelled,
			"processed_at": now,
			"updated_at":   now,
		})
}

func (np *NotificationProcessor) updateDelivery(s *dao.NotificationSchedule) {
	if err := np.db.Model(&dao.NotificationSchedule{}).
		Where("id = ?", s.ID).
		Update("payload", s.Payload).Error; err != nil {
		slog.Error("update delivery", "id", s.ID, "err", err)
	}
}
