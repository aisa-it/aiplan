// Пакет для удаления неактивных пользователей.
//
// Основные возможности:
//   - Определяет неактивных пользователей на основе периода неактивности.
//   - Исключает суперпользователей, ботов и интеграции из удаления.
//   - Проверяет членство в рабочих пространствах и права владельца перед удалением.
//   - Логирует ошибки при удалении пользователей.
package maintenance

import (
	"log"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"gorm.io/gorm"
)

type UserCleaner struct {
	db *gorm.DB
}

func NewUserCleaner(db *gorm.DB) *UserCleaner {
	return &UserCleaner{
		db: db,
	}
}

func (uc *UserCleaner) DeleteInactiveUsers() {
	// Inactivity threshold period
	thresholdDuration := 180 * 24 * time.Hour // 180 days

	// Cutoff date
	cutoffDate := time.Now().Add(-thresholdDuration)

	// Find users inactive since the cutoff date
	var users []dao.User

	err := uc.db.Where(
		"(is_active = ?) AND (last_active IS NULL AND created_at <= ?)",
		true, cutoffDate,
	).Find(&users).Error

	if err != nil {
		log.Println("Error retrieving inactive users:", err)
		return
	}

	for _, user := range users {
		// Skip superusers, bots, and integrations
		if user.IsSuperuser || user.IsBot || user.IsIntegration {
			continue
		}

		// Check if user is a member in workspace_members
		var memberCount int64
		if err := uc.db.Model(&dao.WorkspaceMember{}).Where("member_id = ?", user.ID).Count(&memberCount).Error; err != nil {
			log.Printf("Error checking workspace membership for user %s: %v", user.ID, err)
			continue
		}
		if memberCount > 0 {
			continue
		}

		// Check if user is an owner in workspaces
		var ownerCount int64
		if err := uc.db.Model(&dao.Workspace{}).Where("owner_id = ?", user.ID).Count(&ownerCount).Error; err != nil {
			log.Printf("Error checking workspace ownership for user %s: %v", user.ID, err)
			continue
		}
		if ownerCount > 0 {
			continue
		}

		// Delete user
		if err := uc.db.Delete(&user).Error; err != nil {
			log.Printf("Error deleting user %s: %v", user.ID, err)
		} else {
			log.Printf("Inactive user %s deleted", user.ID)
		}
	}

}
