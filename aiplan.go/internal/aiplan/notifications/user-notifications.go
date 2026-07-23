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
	"encoding/json"
	"fmt"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

func CreateUserNotificationAddCFormAnswer(tx *gorm.DB, userId uuid.UUID, answer dao.FormAnswer, activityId uuid.UUID) (*dao.UserAppNotify, int, error) {
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

	var msg string
	if answer.Responder != nil {
		msg = answer.Responder.GetName()
	} else {
		msg = "Анонимный пользователь"
	}

	msg += " отправил ответ на форму"
	// TODO check
	notification := dao.UserAppNotify{
		ID:              dao.GenUUID(),
		UserId:          userId,
		Type:            "notify",
		Title:           fmt.Sprintf("Прохождение формы: \"%s\"", answer.Form.Title),
		Msg:             msg,
		WorkspaceId:     uuid.NullUUID{UUID: answer.WorkspaceId, Valid: true},
		ActivityEventId: uuid.NullUUID{UUID: activityId, Valid: true},
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

func CreateDeadlineNotification(tx *gorm.DB, issue *dao.Issue, deadlineTime *string, assignees []uuid.UUID) error {
	if err := tx.Unscoped().
		Where("issue_id = ?", issue.ID).
		Where("notification_type = ?", "deadline_notification").
		Delete(&dao.DeferredNotifications{}).Error; err != nil {
		return err
	}

	if deadlineTime == nil || assignees == nil {
		return nil
	}

	targetDate, err := utils.FormatDate(*deadlineTime)
	if err != nil {
		return err
	}
	var notifications []dao.DeferredNotifications

	if issue.Project == nil {
		if err := tx.Where("id = ?", issue.ProjectId).First(&issue.Project).Error; err != nil {
			return err
		}
	}

	var issueAssignees []dao.User

	if err := tx.Where("id IN (?)", assignees).Find(&issueAssignees).Error; err != nil {
		return err
	}

	if len(issueAssignees) == 0 {
		return nil
	}

	for _, user := range issueAssignees {
		loc := time.Location(user.UserTimezone)
		targetDateTime := truncateToDay(targetDate.In(&loc))
		payload := map[string]interface{}{
			"deadline": targetDateTime,
			"body":     fmt.Sprintf("Срок выполнения задачи %s-%d истекает %s", issue.Project.Identifier, issue.SequenceId, targetDateTime.Format("02.01.2006 15:04 MST")),
		}
		deadlineNotification, err := createDeadlineDeferredNotification(targetDate, user, issue, payload)
		if err != nil {
			return err
		}
		deadlineNotification.DeliveryMethod = "telegram"
		deadlineNotification.ID = dao.GenUUID()
		notifications = append(notifications, *deadlineNotification)
		deadlineNotification.ID = dao.GenUUID()
		deadlineNotification.DeliveryMethod = "email"
		notifications = append(notifications, *deadlineNotification)
		deadlineNotification.ID = dao.GenUUID()
		deadlineNotification.DeliveryMethod = "app"
		notifications = append(notifications, *deadlineNotification)
	}

	if len(notifications) > 0 {
		if err := tx.Create(&notifications).Error; err != nil {
			return err
		}
	}

	return nil
}

func createDeadlineDeferredNotification(targetDate time.Time, user dao.User, issue *dao.Issue, payload map[string]interface{}) (*dao.DeferredNotifications, error) {
	tz := time.Location(user.UserTimezone)
	userTime := truncateToDay(targetDate.In(&tz))
	userTime = userTime.Add(-1 * user.Settings.DeadlineNotification)
	timeUTC := userTime.UTC()
	payload["id"] = dao.GenID()
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	notify := dao.DeferredNotifications{
		ID:                  dao.GenUUID(),
		UserID:              user.ID,
		IssueID:             uuid.NullUUID{UUID: issue.ID, Valid: true},
		ProjectID:           uuid.NullUUID{UUID: issue.ProjectId, Valid: true},
		WorkspaceID:         uuid.NullUUID{UUID: issue.WorkspaceId, Valid: true},
		NotificationType:    "deadline_notification",
		DeliveryMethod:      "telegram",
		TimeSend:            &timeUTC,
		AttemptCount:        0,
		NotificationPayload: payloadBytes,
	}
	return &notify, nil
}

func truncateToDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), 0, 0, t.Location())
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
