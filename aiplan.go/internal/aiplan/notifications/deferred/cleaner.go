package deferred

import (
	"log/slog"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"gorm.io/gorm"
)

type NotificationScheduleCleaner struct {
	db *gorm.DB
}

func NewNotificationScheduleCleaner(db *gorm.DB) *NotificationScheduleCleaner {
	return &NotificationScheduleCleaner{db: db}
}

func (c *NotificationScheduleCleaner) Clean() {
	if err := c.db.Unscoped().
		Where("status IN ?", []string{dao.StatusCompleted, dao.StatusCancelled}).
		Where("updated_at <= ?", time.Now().AddDate(0, -1, 0)).
		Delete(&dao.NotificationSchedule{}).Error; err != nil {
		slog.Error("notification_schedules clean", "err", err)
	}
}
