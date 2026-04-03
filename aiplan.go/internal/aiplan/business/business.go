package business

import (
	"log/slog"

	tracker "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/activity-tracker"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"gorm.io/gorm"
)

type Business struct {
	db *gorm.DB

	ta *tracker.ActTracker
}

func NewBL(db *gorm.DB, ta *tracker.ActTracker) (*Business, error) {
	b := &Business{
		db: db,
		ta: ta,
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
			ID:            dao.GenUUID(),
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
	var noAuthServiceUser *dao.User
	if err := b.db.Where("username = ?", "no_auth_user").First(&noAuthServiceUser).Error; err != nil {
		if err != gorm.ErrRecordNotFound {
			slog.Error("Fetch noAuth service user", "err", err)
			return nil, err
		}
		username := "no_auth_user"
		noAuthServiceUser = &dao.User{
			ID:            dao.GenUUID(),
			Username:      &username,
			FirstName:     "Пользователь",
			LastName:      "Анонимный",
			Email:         "no.auth.user@aiplan.ru",
			IsActive:      true,
			IsIntegration: true,
		}
		if err := b.db.Create(&noAuthServiceUser).Error; err != nil {
			slog.Error("Created deleted service user", "err", err)
			return nil, err
		}
	}

	return b, nil
}

func (b *Business) GetTrackerEvent() *tracker.ActTracker {
	return b.ta
}
