package business

import (
	"log/slog"

	tracker "github.com/aisa-it/aiplan/internal/aiplan/activity-tracker"
	"github.com/aisa-it/aiplan/internal/aiplan/dao"
	"gorm.io/gorm"
)

type Business struct {
	db *gorm.DB

	tracker *tracker.ActivitiesTracker

	projectCtx   *ProjectCtx
	workspaceCtx *WorkspaceCtx
}

func NewBL(db *gorm.DB, tracker *tracker.ActivitiesTracker) (*Business, error) {
	b := &Business{
		db:      db,
		tracker: tracker,
	}

	slog.Info("Populate users table FKs")
	if err := b.PopulateUserFKs(); err != nil {
		slog.Error("Fail to populate users FKs. Users merge will not working", "err", err)
	}

	if err := b.db.Where("username = ?", "deleted_user").First(&deletedServiceUser).Error; err != nil {
		if err != gorm.ErrRecordNotFound {
			slog.Error("Fetch deleted service user", "err", err)
			return nil, err
		}
		username := "deleted_user"
		deletedServiceUser = &dao.User{
			ID:            dao.GenID(),
			Username:      &username,
			FirstName:     "Пользователь",
			LastName:      "Удаленный",
			Email:         "deleted.user@aiplan.ru",
			IsActive:      true,
			IsIntegration: true,
		}
		if err := b.db.Create(&deletedServiceUser).Error; err != nil {
			slog.Error("Created deleted service user", "err", err)
			return nil, err
		}
	}

	return b, nil
}
