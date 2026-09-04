package migration

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications/deferred"
	"gorm.io/gorm"
)

type MigrateDeferredNotificationsToSchedule struct {
	db *gorm.DB
}

func NewMigrateDeferredNotificationsToSchedule(db *gorm.DB) *MigrateDeferredNotificationsToSchedule {
	return &MigrateDeferredNotificationsToSchedule{db: db}
}

func (m *MigrateDeferredNotificationsToSchedule) CheckMigrate() (bool, error) {
	if !m.db.Migrator().HasTable("deferred_notifications") {
		return false, nil
	}
	ok, err := CheckRow(m.db, "deferred_notifications")
	if err != nil {
		return false, fmt.Errorf("DeferredNotificationsToSchedule check: %w", err)
	}
	return ok, nil
}

func (m *MigrateDeferredNotificationsToSchedule) Name() string {
	return "DeferredNotificationsToSchedule"
}

func (m *MigrateDeferredNotificationsToSchedule) Execute() error {
	return m.db.Transaction(func(tx *gorm.DB) error {
		var issues []dao.Issue
		if err := tx.
			Where("target_date IS NOT NULL").
			Where("target_date > NOW()").
			Where("completed_at IS NULL").
			Find(&issues).Error; err != nil {
			return fmt.Errorf("DeferredNotificationsToSchedule: select issues: %w", err)
		}

		for i := range issues {
			targetDate := issues[i].TargetDate.Time.Format(time.RFC3339)
			if err := deferred.CreateDeadlineNotification(tx, &issues[i], &targetDate); err != nil {
				return fmt.Errorf("DeferredNotificationsToSchedule: create schedule for issue %s: %w", issues[i].ID, err)
			}
		}

		slog.Info("DeferredNotificationsToSchedule: schedules created", "count", len(issues))

		if err := tx.Exec("DELETE FROM deferred_notifications").Error; err != nil {
			return fmt.Errorf("DeferredNotificationsToSchedule: clear legacy table: %w", err)
		}
		return nil
	})
}
