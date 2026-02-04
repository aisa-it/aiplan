package email

import (
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

func getRemovedIssues(tx *gorm.DB, ids []uuid.UUID) map[uuid.UUID]string {
	var issues []dao.Issue
	res := make(map[uuid.UUID]string)

	if err := tx.Joins("Project").
		Where("issues.id IN (?)", ids).
		Find(&issues).Error; err != nil {
		return res
	}

	for _, i := range issues {
		res[i.ID] = i.FullIssueName()
	}

	return res
}

func getRemovedMember(tx *gorm.DB, ids []uuid.UUID) map[uuid.UUID]string {
	var users []dao.User
	res := make(map[uuid.UUID]string)

	if err := tx.
		Where("id IN (?)", ids).
		Find(&users).Error; err != nil {
		return res
	}

	for _, i := range users {
		res[i.ID] = i.GetName()
	}

	return res
}
