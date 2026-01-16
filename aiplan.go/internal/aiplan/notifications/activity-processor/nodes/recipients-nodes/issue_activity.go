package recipientsnodes

import (
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

// IssueActivity fields needed: Issue, Project
func GetIssueActivityRecipientsNode(db *gorm.DB, activity dao.IssueActivity) ([]dao.User, error) {
	userIdMap := make(map[uuid.UUID]struct{})
	userIdMap[activity.ActorId.UUID] = struct{}{}

	for _, assigneeId := range activity.Issue.AssigneeIDs {
		userIdMap[assigneeId] = struct{}{}
	}

	for _, watcherId := range activity.Issue.WatcherIDs {
		userIdMap[watcherId] = struct{}{}
	}

	for _, watcherId := range activity.Project.DefaultWatchers {
		userIdMap[watcherId] = struct{}{}
	}

	var users []dao.ProjectMember
	if err := db.Where("member_id in (?)", utils.SetToSlice(userIdMap)).Find(&users).Error; err != nil {
		return nil, err
	}

	var recipients []dao.User
	for _, member := range users {
		if !member.Member.CanReceiveNotifications() {
			continue
		}

		member.NotificationSettingsApp.IsNotify(activity.Field, "issue", activity.Verb, member.Role)
	}

	return recipients, nil
}
