// Реестр фоновых cron-задач сервера.
package server

import (
	"fmt"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/config"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/cronmanager"
	filestorage "github.com/aisa-it/aiplan/aiplan.go/pkg/file-storage"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/maintenance"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/notifications"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/notifications/email"
	"gorm.io/gorm"
)

// defaultCronJobs — набор задач ядра.
func defaultCronJobs(
	c *config.Config,
	db *gorm.DB,
	storage filestorage.FileStorage,
	np *notifications.NotificationProcessor,
	es *email.EmailService,
) cronmanager.JobRegistry {
	return cronmanager.JobRegistry{
		"notification_processing": cronmanager.Job{
			Func:     np.ProcessNotifications,
			Schedule: "*/1 * * * *", // every minute
		},

		"email_processing": cronmanager.Job{
			Func:     es.EmailActivity,
			Schedule: fmt.Sprintf("*/%d * * * *", c.NotificationsSleep),
		},
		/*"delete_inactive_users": cronmanager.Job{
			Func:     maintenance.NewUserCleaner(db).DeleteInactiveUsers,
			Schedule: "0 0 * * *", // daily at midnight
		},*/
		"assets_clean": cronmanager.Job{
			Func:     maintenance.NewAssetCleaner(db, storage).CleanAssets,
			Schedule: "0 1 * * *", // daily at 01:00
		},
		"user_notification_clean": cronmanager.Job{
			Func:     notifications.NewNotificationCleaner(db).Clean,
			Schedule: "30 1 * * *", // daily at 01:30
		},
		"workspaces_clean": cronmanager.Job{
			Func:     maintenance.NewWorkspacesCleaner(db).CleanWorkspaces,
			Schedule: "0 2 * * *", // daily at 02:00
		},
		"projects_clean": cronmanager.Job{
			Func:     maintenance.NewProjectsCleaner(db).CleanProjects,
			Schedule: "0 3 * * *", // daily at 03:00
		},
	}
}

// initCron создаёт менеджер задач и загружает реестр.
func initCron(registry cronmanager.JobRegistry) (*cronmanager.CronManager, error) {
	cronManager := cronmanager.NewCronManager(registry)
	if err := cronManager.LoadJobs(); err != nil {
		return nil, fmt.Errorf("load cron jobs: %w", err)
	}
	return cronManager, nil
}
