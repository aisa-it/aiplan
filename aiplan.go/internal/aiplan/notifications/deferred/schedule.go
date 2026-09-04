package deferred

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

func CreateDeadlineNotification(tx *gorm.DB, issue *dao.Issue, deadlineTime *string) error {
	if deadlineTime == nil {
		return deleteDeadlineSchedules(tx, issue.ID)
	}

	targetDate, err := utils.FormatDate(*deadlineTime)
	if err != nil {
		return err
	}

	if err := deleteDeadlineSchedules(tx, issue.ID); err != nil {
		return err
	}

	schedule, err := newDeadlineSchedule(tx, issue, targetDate)
	if err != nil {
		return err
	}
	return tx.Create(schedule).Error
}

func ActivateDeadlineSchedule(tx *gorm.DB, issue *dao.Issue) error {
	var existing dao.NotificationSchedule
	err := tx.Where("issue_id = ?", issue.ID).
		Where("notification_type = ?", dao.NotificationTypeDeadline).
		Order("created_at DESC").
		First(&existing).Error

	switch {
	case err == nil:
		if existing.Status == dao.StatusPending {
			return nil
		}
		return tx.Model(&dao.NotificationSchedule{}).
			Where("id = ?", existing.ID).
			Updates(map[string]interface{}{
				"status":       dao.StatusPending,
				"processed_at": nil,
				"updated_at":   time.Now().UTC(),
			}).Error
	case errors.Is(err, gorm.ErrRecordNotFound):
		schedule, err := newDeadlineSchedule(tx, issue, issue.TargetDate.Time)
		if err != nil {
			return err
		}
		return tx.Create(schedule).Error
	default:
		return err
	}
}

func newDeadlineSchedule(tx *gorm.DB, issue *dao.Issue, targetDate time.Time) (*dao.NotificationSchedule, error) {
	if issue.Project == nil {
		if err := tx.Where("id = ?", issue.ProjectId).First(&issue.Project).Error; err != nil {
			return nil, err
		}
	}

	payload := map[string]any{
		"deadline": targetDate,
		"body":     fmt.Sprintf("Срок выполнения задачи %s-%d истекает %s", issue.Project.Identifier, issue.SequenceId, targetDate.Format("02.01.2006 15:04 MST")),
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal deadline payload: %w", err)
	}

	sendWindow := targetDate.Add(-dao.MaxDeadlineAdvance)

	return &dao.NotificationSchedule{
		ID:               dao.GenUUID(),
		NotificationType: dao.NotificationTypeDeadline,
		IssueID:          uuid.NullUUID{UUID: issue.ID, Valid: true},
		ProjectID:        uuid.NullUUID{UUID: issue.ProjectId, Valid: true},
		WorkspaceID:      uuid.NullUUID{UUID: issue.WorkspaceId, Valid: true},
		ScheduledAt:      targetDate,
		SendWindowStart:  &sendWindow,
		Status:           dao.StatusPending,
		Payload:          payloadBytes,
	}, nil
}

// deleteDeadlineSchedules удаляет все pending-записи дедлайнов для задачи.
func deleteDeadlineSchedules(tx *gorm.DB, issueID uuid.UUID) error {
	return tx.Unscoped().
		Where("issue_id = ?", issueID).
		Where("notification_type = ?", dao.NotificationTypeDeadline).
		Where("status = ?", dao.StatusPending).
		Delete(&dao.NotificationSchedule{}).Error
}

// CreateWorkspaceMessage создаёт запись для рассылки по workspace.
func CreateWorkspaceMessage(
	tx *gorm.DB,
	workspaceID uuid.UUID,
	authorID uuid.UUID,
	title, msg string,
	sendAt time.Time,
	memberIDs []uuid.UUID,
) error {
	payload := map[string]any{
		"title":     title,
		"msg":       msg,
		"author_id": authorID.String(),
	}

	if len(memberIDs) > 0 {
		ids := make([]string, len(memberIDs))
		for i, id := range memberIDs {
			ids[i] = id.String()
		}
		payload["member_ids"] = ids
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal workspace message payload: %w", err)
	}

	sendWindow := sendAt
	schedule := &dao.NotificationSchedule{
		ID:               dao.GenUUID(),
		NotificationType: dao.NotificationTypeWorkspaceMessage,
		AuthorID:         uuid.NullUUID{UUID: authorID, Valid: true},
		WorkspaceID:      uuid.NullUUID{UUID: workspaceID, Valid: true},
		ScheduledAt:      sendAt,
		SendWindowStart:  &sendWindow,
		Status:           dao.StatusPending,
		Payload:          payloadBytes,
	}

	return tx.Create(schedule).Error
}

// CreateServiceMessage создаёт запись для рассылки всем или выбранным пользователям.
func CreateServiceMessage(
	tx *gorm.DB,
	title, msg string,
	sendAt time.Time,
	userIDs []uuid.UUID,
) error {
	payload := map[string]any{
		"title": title,
		"msg":   msg,
	}

	if len(userIDs) > 0 {
		ids := make([]string, len(userIDs))
		for i, id := range userIDs {
			ids[i] = id.String()
		}
		payload["user_ids"] = ids
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal service message payload: %w", err)
	}

	sendWindow := sendAt
	schedule := &dao.NotificationSchedule{
		ID:               dao.GenUUID(),
		NotificationType: dao.NotificationTypeServiceMessage,
		ScheduledAt:      sendAt,
		SendWindowStart:  &sendWindow,
		Status:           dao.StatusPending,
		Payload:          payloadBytes,
	}

	return tx.Create(schedule).Error
}
