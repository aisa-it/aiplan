package notifications

import (
	"fmt"
	"log/slog"

	tracker "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/activity-tracker"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/business"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/config"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

type Notification struct {
	Ws *WebsocketNotificationService
	Tg *TelegramService
	Db *gorm.DB
	//IssueActivityHandler(activity *dao.IssueActivity)
}

func NewNotificationService(cfg *config.Config, db *gorm.DB, tracker *tracker.ActivitiesTracker, bl *business.Business) *Notification {
	return &Notification{
		Ws: NewWebsocketNotificationService(),
		Tg: NewTelegramService(db, cfg, tracker, bl),
		Db: db,
	}
}

type IssueNotification struct {
	Notification
}

func NewIssueNotification(n *Notification) *IssueNotification {
	if n == nil {
		return nil
	}
	return &IssueNotification{
		Notification: *n,
	}
}

func (n *IssueNotification) Handle(activity dao.ActivityI) error {
	a, ok := activity.(dao.IssueActivity)
	if !ok {
		return nil
	}

	if a.Issue == nil {
		if err := n.Db.Unscoped().
			Preload("Author").
			Preload("Assignees").
			Preload("Watchers").
			Preload("Workspace").
			Preload("Project").
			Where("id = ?", a.IssueId).
			Find(&a.Issue).Error; err != nil {
			slog.Error("Get issue for activity", "activityId", a.Id, "err", err)
			return err
		}
	}

	userIdMap := make(map[string]interface{})
	notifyId, countNotify, _ := CreateUserNotificationActivity(n.Db, uuid.FromStringOrNil(a.Issue.CreatedById), a)

	if notifyId != nil {
		n.Ws.Send(uuid.FromStringOrNil(a.Issue.CreatedById), *notifyId, a, countNotify)
	}

	userIdMap[a.Issue.CreatedById] = struct{}{}

	for _, assigneeId := range a.Issue.AssigneeIDs {
		if _, ok := userIdMap[assigneeId]; !ok {
			assigneeUUID := uuid.FromStringOrNil(assigneeId)
			notifyId, countNotify, _ := CreateUserNotificationActivity(n.Db, assigneeUUID, a)
			if notifyId != nil {
				n.Ws.Send(assigneeUUID, *notifyId, a, countNotify)
			}
			userIdMap[assigneeId] = struct{}{}
		}
	}

	for _, watcherId := range a.Issue.WatcherIDs {
		if _, ok := userIdMap[watcherId]; !ok {
			watcherUUID := uuid.FromStringOrNil(watcherId)
			notifyId, countNotify, _ := CreateUserNotificationActivity(n.Db, watcherUUID, a)
			if notifyId != nil {
				n.Ws.Send(watcherUUID, *notifyId, a, countNotify)
			}
			userIdMap[watcherId] = struct{}{}
		}
	}
	return nil
}

type ProjectNotification struct {
	Notification
}

func NewProjectNotification(n *Notification) *ProjectNotification {
	if n == nil {
		return nil
	}
	return &ProjectNotification{
		Notification: *n,
	}
}

func (n *ProjectNotification) Handle(activity dao.ActivityI) error {
	a, ok := activity.(dao.ProjectActivity)
	if !ok {
		return nil
	}

	if a.Project == nil {
		if err := n.Db.Unscoped().
			Joins("Workspace").
			Joins("ProjectLead").
			Where("projects.id = ?", a.ProjectId).
			Find(&a.Project).Error; err != nil {
			slog.Error("Get project for activity", "activityId", a.Id, "err", err)
			return err
		}
	}
	if a.Field != nil && *a.Field == "issue" && a.Verb != "deleted" {
		var issueId string
		if a.NewIdentifier != nil {
			issueId = *a.NewIdentifier
		} else if a.OldIdentifier != nil {
			issueId = *a.OldIdentifier
		} else {
			return nil
		}

		if err := n.Db.Unscoped().
			Joins("Author").
			Joins("Workspace").
			Joins("Project").
			Joins("Parent").
			Preload("Assignees").
			Preload("Watchers").
			Where("issues.id = ?", issueId).
			First(&a.NewIssue).Error; err != nil {
			slog.Error("Get issue for activity", "activityId", a.Id, "err", err)
			return err
		}
	}

	var projectMembers []dao.ProjectMember
	if err := n.Db.Joins("Member").
		Where("project_id = ?", a.ProjectId).
		Find(&projectMembers).Error; err != nil {
		return nil
	}

	userIdMap := make(map[string]interface{})
	actorIdStr := fmt.Sprint(*a.ActorId)
	actorUUID := uuid.FromStringOrNil(actorIdStr)
	notifyId, countNotify, _ := CreateUserNotificationActivity(n.Db, actorUUID, a)

	if notifyId != nil {
		n.Ws.Send(actorUUID, *notifyId, a, countNotify)
	}

	userIdMap[actorIdStr] = struct{}{}

	{ // уведомления по созданию задачи
		if a.NewIssue != nil {
			a.NewIssue.Author = a.Actor
			a.NewIssue.Workspace = a.Workspace

			for _, assigneeId := range a.NewIssue.AssigneeIDs {
				if _, ok := userIdMap[assigneeId]; !ok {
					assigneeUUID := uuid.FromStringOrNil(assigneeId)
					notifyId, countNotify, _ := CreateUserNotificationActivity(n.Db, assigneeUUID, a)
					if notifyId != nil {
						n.Ws.Send(assigneeUUID, *notifyId, a, countNotify)
					}
					userIdMap[assigneeId] = struct{}{}
				}
			}

			for _, watcherId := range a.NewIssue.WatcherIDs {
				if _, ok := userIdMap[watcherId]; !ok {
					watcherUUID := uuid.FromStringOrNil(watcherId)
					notifyId, countNotify, _ := CreateUserNotificationActivity(n.Db, watcherUUID, a)
					if notifyId != nil {
						n.Ws.Send(watcherUUID, *notifyId, a, countNotify)
					}
					userIdMap[watcherId] = struct{}{}
				}
			}

			for _, member := range projectMembers {
				if member.IsDefaultWatcher {
					if _, ok := userIdMap[member.MemberId]; !ok {
						memberUUID := uuid.FromStringOrNil(member.MemberId)
						notifyId, countNotify, _ := CreateUserNotificationActivity(n.Db, memberUUID, a)
						if notifyId != nil {
							n.Ws.Send(memberUUID, *notifyId, a, countNotify)
						}
						userIdMap[member.MemberId] = struct{}{}
					}
				}
			}
		}
	}

	for _, member := range projectMembers {
		if member.Role == types.AdminRole {
			if a.Field != nil && *a.Field == "issue" {
				continue
			}
			if _, ok := userIdMap[member.MemberId]; !ok {
				memberUUID := uuid.FromStringOrNil(member.MemberId)
				notifyId, countNotify, _ := CreateUserNotificationActivity(n.Db, memberUUID, a)
				if notifyId != nil {
					n.Ws.Send(memberUUID, *notifyId, a, countNotify)
				}
				userIdMap[member.MemberId] = struct{}{}
			}
		}
	}
	return nil
}

type DocNotification struct {
	Notification
}

func NewDocNotification(n *Notification) *DocNotification {
	if n == nil {
		return nil
	}
	return &DocNotification{
		Notification: *n,
	}
}

func (n *DocNotification) Handle(activity dao.ActivityI) error {
	a, ok := activity.(dao.DocActivity)
	if !ok {
		return nil
	}

	if a.Doc == nil {
		if err := n.Db.Unscoped().
			Joins("Workspace").
			Joins("Author").
			Joins("ParentDoc").
			Joins("LEFT JOIN doc_access_rules dar ON dar.doc_id = docs.id").
			Where("docs.id = ?", a.DocId).
			Find(&a.Doc).Error; err != nil {
			slog.Error("Get doc for activity", "activityId", a.Id, "err", err)
			return err
		}
	}

	doc := a.Doc

	doc.AfterFind(n.Db)

	authorId := doc.CreatedById
	readerIds := doc.ReaderIDs
	editorsIds := doc.EditorsIDs
	watcherIds := doc.WatcherIDs

	userIds := append(append(append([]string{authorId}, editorsIds...), readerIds...), watcherIds...)

	var workspaceMembers []dao.WorkspaceMember
	if err := n.Db.Joins("Member").
		Where("workspace_id = ?", doc.WorkspaceId).
		Where("workspace_members.member_id IN (?)", userIds).Find(&workspaceMembers).Error; err != nil {
		return err
	}

	userIdMap := make(map[string]interface{})

	for _, member := range workspaceMembers {
		if _, ok := userIdMap[member.MemberId]; !ok {
			memberUUID := uuid.FromStringOrNil(member.MemberId)
			notifyId, countNotify, _ := CreateUserNotificationActivity(n.Db, memberUUID, a)
			if notifyId != nil {
				n.Ws.Send(memberUUID, *notifyId, a, countNotify)
			}
			userIdMap[member.MemberId] = struct{}{}
		}

	}
	return nil
}

type WorkspaceNotification struct {
	Notification
}

func NewWorkspaceNotification(n *Notification) *WorkspaceNotification {
	if n == nil {
		return nil
	}
	return &WorkspaceNotification{
		Notification: *n,
	}
}

func (n *WorkspaceNotification) Handle(activity dao.ActivityI) error {
	a, ok := activity.(dao.WorkspaceActivity)
	if !ok {
		return nil
	}

	if a.Workspace == nil {
		if err := n.Db.Unscoped().
			Joins("Owner").
			Where("workspaces.id = ?", a.WorkspaceId).
			Find(&a.Workspace).Error; err != nil {
			slog.Error("Get project for activity", "activityId", a.Id, "err", err)
			return err
		}
	}

	var workspaceAdminMembers []dao.WorkspaceMember
	if err := n.Db.Joins("Member").
		Where("workspace_id = ?", a.WorkspaceId).
		Where("workspace_members.role = ?", types.AdminRole).Find(&workspaceAdminMembers).Error; err != nil {
		return err
	}

	userIdMap := make(map[string]interface{})

	for _, member := range workspaceAdminMembers {
		if _, ok := userIdMap[member.MemberId]; !ok {
			memberUUID := uuid.FromStringOrNil(member.MemberId)
			notifyId, countNotify, _ := CreateUserNotificationActivity(n.Db, memberUUID, a)
			if notifyId != nil {
				n.Ws.Send(memberUUID, *notifyId, a, countNotify)
			}
			userIdMap[member.MemberId] = struct{}{}
		}

	}
	return nil
}
