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

func CreateUserNotificationActivity[A dao.Activity](tx *gorm.DB, userId string, activity A) (*string, int, error) {
	if userId == "" {
		return nil, 0, nil
	}

	var notifyId string

	switch a := any(activity).(type) {
	case dao.RootActivity:
		//TODO add activity to notify
		return nil, 0, nil

	case dao.WorkspaceActivity:
		var member dao.WorkspaceMember
		if err := tx.
			Joins("Member").
			Where("workspace_id = ?", a.WorkspaceId).
			Where("member_id = ?", userId).
			Where("workspace_members.role = ?", types.AdminRole).
			First(&member).Error; err != nil {
			return nil, 0, err
		}
		if !member.Member.CanReceiveNotifications() {
			return nil, 0, fmt.Errorf("user can not receive notify")
		}

		if member.Member.Settings.AppNotificationMute {
			return nil, 0, fmt.Errorf("user off app notify")
		}

		if member.NotificationSettingsApp.IsNotify(a.Field, "workspace", a.Verb, member.Role) {
			notification := dao.UserNotifications{
				ID:                  dao.GenID(),
				UserId:              userId,
				Type:                "activity",
				WorkspaceId:         &a.WorkspaceId,
				WorkspaceActivityId: &a.Id,
			}

			if err := tx.Omit(clause.Associations).Create(&notification).Error; err != nil {
				return nil, 0, err
			}
			notifyId = notification.ID
		}

	case dao.ProjectActivity:
		var member dao.ProjectMember
		if err := tx.Joins("Member").Where("project_id = ?", a.ProjectId).Where("member_id = ?", userId).First(&member).Error; err != nil {
			return nil, 0, err
		}

		if !member.Member.CanReceiveNotifications() {
			return nil, 0, fmt.Errorf("user can not receive notify")
		}

		if member.Member.Settings.AppNotificationMute {
			return nil, 0, fmt.Errorf("user off app notify")
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
				ID:                dao.GenID(),
				UserId:            userId,
				Type:              "activity",
				WorkspaceId:       &a.WorkspaceId,
				ProjectActivityId: &a.Id,
			}

			if err := tx.Omit(clause.Associations).Create(&notification).Error; err != nil {
				return nil, 0, err
			}
			notifyId = notification.ID
		}
	case dao.DocActivity:
		var member dao.WorkspaceMember

		if err := tx.Joins("Member").
			Where("workspace_id = ?", a.WorkspaceId).
			Where("member_id = ?", userId).First(&member).Error; err != nil {
			return nil, 0, err
		}
		if !member.Member.CanReceiveNotifications() {
			return nil, 0, fmt.Errorf("user can not receive notify")
		}

		if member.Member.Settings.AppNotificationMute {
			return nil, 0, fmt.Errorf("user off app notify")
		}

		var authorOK, authorNotifyOk, memberNotifyOK bool

		if a.Doc.CreatedById == userId {
			authorOK = true
		}

		if authorOK && member.NotificationAuthorSettingsApp.IsNotify(a.Field, "doc", a.Verb, member.Role) {
			authorNotifyOk = true
		}
		if member.NotificationSettingsApp.IsNotify(a.Field, "doc", a.Verb, member.Role) {
			memberNotifyOK = true
		}

		if (authorOK && authorNotifyOk) || (!authorOK && memberNotifyOK) {

			notification := dao.UserNotifications{
				ID:            dao.GenID(),
				UserId:        userId,
				Type:          "activity",
				WorkspaceId:   &a.WorkspaceId,
				DocActivityId: &a.Id,
			}

			//if a.Field != nil && *a.Field == "comment" && a.Verb != "deleted" {
			//	notification.CommentId = a.NewIdentifier
			//}

			if err := tx.Omit(clause.Associations).Create(&notification).Error; err != nil {
				return nil, 0, err
			}
			notifyId = notification.ID
		}
	case dao.FormActivity:
		//TODO add activity to notify
		return nil, 0, nil
	case dao.IssueActivity:
		if a.Field != nil && *a.Field == "issue_transfer" && a.Verb == "cloned" {
			return nil, 0, nil
		}

		var member dao.ProjectMember
		if err := tx.Joins("Member").Where("project_id = ?", a.ProjectId).Where("member_id = ?", userId).First(&member).Error; err != nil {
			return nil, 0, err
		}

		if !member.Member.CanReceiveNotifications() {
			return nil, 0, fmt.Errorf("user can not receive notify")
		}

		if member.Member.Settings.AppNotificationMute {
			return nil, 0, fmt.Errorf("user off app notify")
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
				ID:              dao.GenID(),
				UserId:          userId,
				Type:            "activity",
				WorkspaceId:     &a.WorkspaceId,
				IssueId:         &a.IssueId,
				IssueActivityId: &a.Id,
			}

			if a.Field != nil && *a.Field == "comment" && a.Verb != "deleted" {
				notification.CommentId = a.NewIdentifier
			}

			if err := tx.Omit(clause.Associations).Create(&notification).Error; err != nil {
				return nil, 0, err
			}
			notifyId = notification.ID
		}

	default:
		return nil, 0, errStack.TrackErrorStack(fmt.Errorf("unknown type notify %T", a))
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

	if notifyId == "" {
		return nil, count, nil
	}
	return &notifyId, count, nil
}

func CreateUserNotificationAddComment(tx *gorm.DB, userId string, comment dao.IssueComment) (*dao.UserNotifications, int, error) {
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
		ID:          dao.GenID(),
		UserId:      userId,
		Type:        "comment",
		CommentId:   &comment.Id,
		Comment:     &comment,
		WorkspaceId: &comment.WorkspaceId,
		IssueId:     &comment.IssueId,
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

func CreateDeadlineNotification(tx *gorm.DB, issue *dao.Issue, deadlineTime *string, assignees *[]string) error {
	if err := tx.Unscoped().
		Where("issue_id = ?", issue.ID).
		Where("notification_type = ?", "deadline_notification").
		Delete(&dao.DeferredNotifications{}).Error; err != nil {
		return err
	}

	if deadlineTime == nil || assignees == nil {
		return nil
	}

	targetDate, err := time.Parse("2006-01-02", *deadlineTime)
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

	if err := tx.Where("id IN (?)", *assignees).Find(&issueAssignees).Error; err != nil {
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
		deadlineNotification.ID = dao.GenID()
		notifications = append(notifications, *deadlineNotification)
		deadlineNotification.ID = dao.GenID()
		deadlineNotification.DeliveryMethod = "email"
		notifications = append(notifications, *deadlineNotification)
		deadlineNotification.ID = dao.GenID()
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
	issueId := issue.ID.String()
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
		ID:                  dao.GenID(),
		UserID:              user.ID,
		IssueID:             &issueId,
		ProjectID:           &issue.ProjectId,
		WorkspaceID:         &issue.WorkspaceId,
		NotificationType:    "deadline_notification",
		DeliveryMethod:      "telegram",
		TimeSend:            &timeUTC,
		AttemptCount:        0,
		NotificationPayload: payloadBytes,
	}
	return &notify, nil
}

func truncateToDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 9, 0, 0, 0, t.Location())
}

type NotificationResponse struct {
	Id     string                     `json:"id,omitempty"`
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
}
