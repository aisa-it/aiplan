package business

import (
	"gorm.io/gorm"
	tracker "sheff.online/aiplan/internal/aiplan/activity-tracker"
)

type Business struct {
	db *gorm.DB

	tracker *tracker.ActivitiesTracker
	//emailService         *notifications.EmailService
	//notificationsService *notifications.Notification

	projectCtx   *ProjectCtx
	workspaceCtx *WorkspaceCtx
}

func NewBL(db *gorm.DB, tracker *tracker.ActivitiesTracker) *Business {
	return &Business{
		db:      db,
		tracker: tracker,
		//emailService:         es,
		//notificationsService: ns,
	}
}
