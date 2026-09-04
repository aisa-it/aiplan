// Управление уведомлениями для пользователей и задач в системе.
// Содержит функции для создания, очистки и обработки уведомлений различных типов.
//
// Основные возможности:
//   - Создание уведомлений об активности пользователя (например, клонирование задачи).
//   - Создание уведомлений о комментариях к задачам.
//   - Очистка устаревших уведомлений.
//   - Создание уведомлений о приближающихся дедлайнах задач.
package notifications

import (
	"fmt"
	"time"

	"github.com/gofrs/uuid"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	maxRetryAttempts = 3
)

type NotificationCleaner struct {
	db *gorm.DB
}

func NewNotificationCleaner(db *gorm.DB) *NotificationCleaner {
	return &NotificationCleaner{db}
}

func CreateUserNotificationAddComment(tx *gorm.DB, userId uuid.UUID, comment dao.IssueComment) (*dao.UserAppNotify, int, error) {
	var user dao.User

	if err := tx.Where("id = ?", userId).First(&user).Error; err != nil {
		return nil, 0, err
	}

	if !user.CanReceiveNotifications() {
		return nil, 0, fmt.Errorf("user can not receive notify")
	}

	if user.Settings.AppNotificationMute {
		return nil, 0, fmt.Errorf("user off app notify")
	}

	notification := dao.UserAppNotify{
		ID:             dao.GenUUID(),
		UserId:         userId,
		Type:           "comment",
		IssueCommentId: uuid.NullUUID{UUID: comment.Id, Valid: true},
		IssueComment:   &comment,
		WorkspaceId:    uuid.NullUUID{UUID: comment.WorkspaceId, Valid: true},
		IssueId:        uuid.NullUUID{UUID: comment.IssueId, Valid: comment.IssueId != uuid.Nil},
	}

	if err := tx.Omit(clause.Associations).Create(&notification).Error; err != nil {
		return nil, 0, err
	}

	var count int
	if err := tx.Select("count(*)").
		Where("viewed = false").
		Where("user_id = ?", userId).
		Where("deleted_at IS NULL").
		Model(&dao.UserAppNotify{}).
		Find(&count).Error; err != nil {
		return nil, 0, err
	}
	return &notification, count, nil
}

func (nc *NotificationCleaner) Clean() {
	if err := nc.db.Omit(clause.Associations).Unscoped().
		Where("created_at <= ?", time.Now().AddDate(0, -1, 0)).
		Delete(&dao.UserAppNotify{}).Error; err != nil {
		return
	}

	if err := nc.db.Omit(clause.Associations).Unscoped().
		Where("deleted_at is not null").
		Delete(&dao.UserAppNotify{}).Error; err != nil {
		return
	}
	if err := nc.db.Omit(clause.Associations).Unscoped().
		Where("sent_at is not null or (attempt_count = ? and sent_at is null )", maxRetryAttempts).
		Delete(&dao.DeferredNotifications{}).Error; err != nil {
		return
	}
}

type NotificationResponse struct {
	Id     uuid.UUID                  `json:"id,omitempty"`
	Type   string                     `json:"type"`
	Detail NotificationDetailResponse `json:"detail"`
	Data   interface{}                `json:"data"`
	//NewEntity any                        `json:"new_entity,omitempty"`
	//OldEntity any                        `json:"old_entity,omitempty"`
	Viewed    *bool     `json:"viewed,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type NotificationResponseMessage struct {
	Title string `json:"title"`
	Msg   string `json:"msg"`
}

type NotificationDetailResponse struct {
	User         *dto.UserLight         `json:"user,omitempty"`
	IssueComment *dto.IssueCommentLight `json:"issue_comment,omitempty"`
	Issue        *dto.IssueLight        `json:"issue,omitempty"`
	Project      *dto.ProjectLight      `json:"project,omitempty"`
	Workspace    *dto.WorkspaceLight    `json:"workspace,omitempty"`
	Form         *dto.FormLight         `json:"form,omitempty"`
	Doc          *dto.DocLight          `json:"doc,omitempty"`
	Sprint       *dto.SprintLight       `json:"sprint,omitempty"`
}
