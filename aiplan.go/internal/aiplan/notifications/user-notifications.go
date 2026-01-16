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

	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dto"
	errStack "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/stack-error"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type NotificationCleaner struct {
	db *gorm.DB
}

func NewNotificationCleaner(db *gorm.DB) *NotificationCleaner {
	return &NotificationCleaner{db}
}

func CreateUserNotificationActivity[A dao.Activity](tx *gorm.DB, userId uuid.UUID, activity A) (uuid.UUID, int, error) {
	if userId == uuid.Nil {
		return uuid.Nil, 0, nil
	}

	var notifyId uuid.UUID

	switch a := any(activity).(type) {
	case dao.RootActivity:
		//TODO add activity to notify
		return uuid.Nil, 0, nil

	case dao.WorkspaceActivity:
		var member dao.WorkspaceMember
		if err := tx.
			Joins("Member").
			Where("workspace_id = ?", a.WorkspaceId).
			Where("member_id = ?", userId).
			Where("workspace_members.role = ?", types.AdminRole).
			First(&member).Error; err != nil {
			return uuid.Nil, 0, err
		}
		if !member.Member.CanReceiveNotifications() {
			return uuid.Nil, 0, fmt.Errorf("user can not receive notify")
		}

		if member.Member.Settings.AppNotificationMute {
			return uuid.Nil, 0, fmt.Errorf("user off app notify")
		}

		if member.NotificationSettingsApp.IsNotify(a.Field, actField.Workspace.Field, a.Verb, member.Role) {
			notification := dao.UserNotifications{
				ID:                  dao.GenUUID(),
				UserId:              userId,
				Type:                "activity",
				WorkspaceId:         uuid.NullUUID{UUID: a.WorkspaceId, Valid: true},
				WorkspaceActivityId: uuid.NullUUID{UUID: a.Id, Valid: true},
			}

			if err := tx.Omit(clause.Associations).Create(&notification).Error; err != nil {
				return uuid.Nil, 0, err
			}
			notifyId = notification.ID
		}

	case dao.ProjectActivity:
		var member dao.ProjectMember
		if err := tx.Joins("Member").Where("project_id = ?", a.ProjectId).Where("member_id = ?", userId).First(&member).Error; err != nil {
			return uuid.Nil, 0, err
		}

		if !member.Member.CanReceiveNotifications() {
			return uuid.Nil, 0, fmt.Errorf("user can not receive notify")
		}

		if member.Member.Settings.AppNotificationMute {
			return uuid.Nil, 0, fmt.Errorf("user off app notify")
		}

		var notifyOk, isAuthorNotify, isMemberNotify bool
		isProjectAdm := member.Role == types.AdminRole
		isMemberNotify = member.NotificationSettingsApp.IsNotify(a.Field, "project", a.Verb, member.Role)

		if a.NewIssue != nil {
			isAuthorNotify = a.NewIssue.CreatedById == userId && member.NotificationAuthorSettingsApp.IsNotify(a.Field, "project", a.Verb, member.Role)
			notifyOk = isAuthorNotify || (!isAuthorNotify && isMemberNotify)
		} else {
			notifyOk = isMemberNotify && isProjectAdm
		}

		if (isProjectAdm && isMemberNotify) || (!isProjectAdm && notifyOk) {
			notification := dao.UserNotifications{
				ID:                dao.GenUUID(),
				UserId:            userId,
				Type:              "activity",
				WorkspaceId:       uuid.NullUUID{UUID: a.WorkspaceId, Valid: true},
				ProjectActivityId: uuid.NullUUID{UUID: a.Id, Valid: true},
			}

			if err := tx.Omit(clause.Associations).Create(&notification).Error; err != nil {
				return uuid.Nil, 0, err
			}
			notifyId = notification.ID
		}
	case dao.DocActivity:
		var member dao.WorkspaceMember

		if err := tx.Joins("Member").
			Where("workspace_id = ?", a.WorkspaceId).
			Where("member_id = ?", userId).First(&member).Error; err != nil {
			return uuid.Nil, 0, err
		}
		if !member.Member.CanReceiveNotifications() {
			return uuid.Nil, 0, fmt.Errorf("user can not receive notify")
		}

		if member.Member.Settings.AppNotificationMute {
			return uuid.Nil, 0, fmt.Errorf("user off app notify")
		}

		var authorOK, authorNotifyOk, memberNotifyOK bool

		if a.Doc.CreatedById == userId {
			authorOK = true
		}

		if authorOK && member.NotificationAuthorSettingsApp.IsNotify(a.Field, actField.Doc.Field, a.Verb, member.Role) {
			authorNotifyOk = true
		}
		if member.NotificationSettingsApp.IsNotify(a.Field, actField.Doc.Field, a.Verb, member.Role) {
			memberNotifyOK = true
		}

		if (authorOK && authorNotifyOk) || (!authorOK && memberNotifyOK) {

			notification := dao.UserNotifications{
				ID:            dao.GenUUID(),
				UserId:        userId,
				Type:          "activity",
				WorkspaceId:   uuid.NullUUID{UUID: a.WorkspaceId, Valid: true},
				DocActivityId: uuid.NullUUID{UUID: a.Id, Valid: true},
			}

			//if a.Field != nil && *a.Field == "comment" && a.Verb != "deleted" {
			//	notification.CommentId = a.NewIdentifier
			//}

			if err := tx.Omit(clause.Associations).Create(&notification).Error; err != nil {
				return uuid.Nil, 0, err
			}
			notifyId = notification.ID
		}
	case dao.FormActivity:
		var member dao.WorkspaceMember
		if err := tx.Joins("Member").Where("workspace_id = ?", a.WorkspaceId).Where("member_id = ?", userId).First(&member).Error; err != nil {
			return uuid.Nil, 0, err
		}

		if !member.Member.CanReceiveNotifications() {
			return uuid.Nil, 0, fmt.Errorf("user can not receive notify")
		}

		if member.Member.Settings.AppNotificationMute {
			return uuid.Nil, 0, fmt.Errorf("user off app notify")
		}
		a.Form.CurrentWorkspaceMember = &member

		notification := dao.UserNotifications{
			ID:             dao.GenUUID(),
			UserId:         userId,
			Type:           "activity",
			WorkspaceId:    uuid.NullUUID{UUID: a.WorkspaceId, Valid: true},
			FormActivityId: uuid.NullUUID{UUID: a.Id, Valid: true},
		}

		if err := tx.Omit(clause.Associations).Create(&notification).Error; err != nil {
			return uuid.Nil, 0, err
		}
		notifyId = notification.ID
	case dao.IssueActivity:
		if a.Field != nil && *a.Field == actField.IssueTransfer.Field.String() && a.Verb == "cloned" {
			return uuid.Nil, 0, nil
		}

		var member dao.ProjectMember
		if err := tx.Joins("Member").Where("project_id = ?", a.ProjectId).Where("member_id = ?", userId).First(&member).Error; err != nil {
			return uuid.Nil, 0, err
		}

		if !member.Member.CanReceiveNotifications() {
			return uuid.Nil, 0, fmt.Errorf("user can not receive notify")
		}

		if member.Member.Settings.AppNotificationMute {
			return uuid.Nil, 0, fmt.Errorf("user off app notify")
		}

		var authorOK, authorNotifyOk, memberNotifyOK bool

		if a.Issue != nil {
			if a.Issue.CreatedById == userId {
				authorOK = true
			}

			if authorOK && member.NotificationAuthorSettingsApp.IsNotify(a.Field, "issue", a.Verb, member.Role) {
				authorNotifyOk = true
			}
			if member.NotificationSettingsApp.IsNotify(a.Field, "issue", a.Verb, member.Role) {
				memberNotifyOK = true
			}
		}
		if (authorOK && authorNotifyOk) || (!authorOK && memberNotifyOK) {
			notification := dao.UserNotifications{
				ID:              dao.GenUUID(),
				UserId:          userId,
				Type:            "activity",
				WorkspaceId:     uuid.NullUUID{UUID: a.WorkspaceId, Valid: true},
				IssueId:         uuid.NullUUID{UUID: a.IssueId, Valid: true},
				IssueActivityId: uuid.NullUUID{UUID: a.Id, Valid: true},
			}

			if a.Field != nil && *a.Field == actField.Comment.Field.String() && a.Verb != "deleted" {
				if a.NewIdentifier.Valid {
					notification.CommentId = uuid.NullUUID{UUID: a.NewIdentifier.UUID, Valid: true}
				}
			}

			if err := tx.Omit(clause.Associations).Create(&notification).Error; err != nil {
				return uuid.Nil, 0, err
			}
			notifyId = notification.ID
		}

	case dao.SprintActivity:
		var member dao.WorkspaceMember

		if err := tx.Joins("Member").
			Where("workspace_id = ?", a.WorkspaceId).
			Where("member_id = ?", userId).First(&member).Error; err != nil {
			return uuid.Nil, 0, err
		}
		if !member.Member.CanReceiveNotifications() {
			return uuid.Nil, 0, fmt.Errorf("user can not receive notify")
		}

		if member.Member.Settings.AppNotificationMute {
			return uuid.Nil, 0, fmt.Errorf("user off app notify")
		}

		var authorOK, authorNotifyOk, memberNotifyOK bool

		if a.Sprint.CreatedById == userId {
			authorOK = true
		}

		if authorOK && member.NotificationAuthorSettingsApp.IsNotify(a.Field, actField.Sprint.Field, a.Verb, member.Role) {
			authorNotifyOk = true
		}

		if member.NotificationSettingsApp.IsNotify(a.Field, actField.Sprint.Field, a.Verb, member.Role) {
			memberNotifyOK = true
		}

		if (authorOK && authorNotifyOk) || (!authorOK && memberNotifyOK) {

			notification := dao.UserNotifications{
				ID:               dao.GenUUID(),
				UserId:           userId,
				Type:             "activity",
				WorkspaceId:      uuid.NullUUID{UUID: a.WorkspaceId, Valid: true},
				SprintActivityId: uuid.NullUUID{UUID: a.Id, Valid: true},
			}

			//if a.Field != nil && *a.Field == "comment" && a.Verb != "deleted" {
			//	notification.CommentId = a.NewIdentifier
			//}

			if err := tx.Omit(clause.Associations).Create(&notification).Error; err != nil {
				return uuid.Nil, 0, err
			}
			notifyId = notification.ID
		}

	default:
		return uuid.Nil, 0, errStack.TrackErrorStack(fmt.Errorf("unknown type notify %T", a))
	}

	var count int
	if err := tx.Select("count(*)").
		Where("viewed = false").
		Where("user_id = ?", userId).
		Where("deleted_at IS NULL").
		Model(&dao.UserNotifications{}).
		Find(&count).Error; err != nil {
		return uuid.Nil, 0, err
	}

	if notifyId == uuid.Nil {
		return uuid.Nil, count, nil
	}
	return notifyId, count, nil
}

func CreateUserNotificationAddComment(tx *gorm.DB, userId uuid.UUID, comment dao.IssueComment) (*dao.UserNotifications, int, error) {
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

	notification := dao.UserNotifications{
		ID:          dao.GenUUID(),
		UserId:      userId,
		Type:        "comment",
		CommentId:   uuid.NullUUID{UUID: comment.Id, Valid: true},
		Comment:     &comment,
		WorkspaceId: uuid.NullUUID{UUID: comment.WorkspaceId, Valid: true},
		IssueId:     uuid.NullUUID{UUID: comment.IssueId, Valid: comment.IssueId != uuid.Nil},
	}

	if err := tx.Omit(clause.Associations).Create(&notification).Error; err != nil {
		return nil, 0, err
	}

	var count int
	if err := tx.Select("count(*)").
		Where("viewed = false").
		Where("user_id = ?", userId).
		Where("deleted_at IS NULL").
		Model(&dao.UserNotifications{}).
		Find(&count).Error; err != nil {
		return nil, 0, err
	}
	return &notification, count, nil
}

func CreateUserNotificationAddCFormAnswer(tx *gorm.DB, userId uuid.UUID, answer dao.FormAnswer, activityId uuid.UUID) (*dao.UserNotifications, int, error) {
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

	notification := dao.UserNotifications{
		ID:             dao.GenUUID(),
		UserId:         userId,
		Type:           "notify",
		Title:          fmt.Sprintf("Прохождение формы: \"%s\"", answer.Form.Title),
		Msg:            msg,
		WorkspaceId:    uuid.NullUUID{UUID: answer.WorkspaceId, Valid: true},
		FormActivityId: uuid.NullUUID{UUID: activityId, Valid: true},
	}

	if err := tx.Omit(clause.Associations).Create(&notification).Error; err != nil {
		return nil, 0, err
	}

	var count int
	if err := tx.Select("count(*)").
		Where("viewed = false").
		Where("user_id = ?", userId).
		Where("deleted_at IS NULL").
		Model(&dao.UserNotifications{}).
		Find(&count).Error; err != nil {
		return nil, 0, err
	}
	return &notification, count, nil
}

func (nc *NotificationCleaner) Clean() {
	if err := nc.db.Omit(clause.Associations).Unscoped().
		Where("created_at <= ?", time.Now().AddDate(0, -1, 0)).
		Delete(&dao.UserNotifications{}).Error; err != nil {
		return
	}

	if err := nc.db.Omit(clause.Associations).Unscoped().
		Where("deleted_at is not null").
		Delete(&dao.UserNotifications{}).Error; err != nil {
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
