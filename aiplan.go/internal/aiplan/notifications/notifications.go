package notifications

import (
	"log/slog"

	tracker "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/activity-tracker"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/business"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/config"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications/tg"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

type Notification struct {
	Ws *WebsocketNotificationService
	Tg *tg.TgService
	Db *gorm.DB
	//IssueActivityHandler(activity *dao.IssueActivity)
}

func NewNotificationService(cfg *config.Config, db *gorm.DB, tracker *tracker.ActivitiesTracker, bl *business.Business) *Notification {
	//NewTelegramService(db, cfg, tracker, bl),
	return &Notification{
		Ws: NewWebsocketNotificationService(),
		Tg: tg.New(db, cfg, tracker, bl),
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

	userIdMap := make(map[uuid.UUID]interface{})
	notifyId, countNotify, _ := CreateUserNotificationActivity(n.Db, a.Issue.CreatedById, a)

	if notifyId != uuid.Nil {
		n.Ws.Send(a.Issue.CreatedById, notifyId, a, countNotify)
	}

	userIdMap[a.Issue.CreatedById] = struct{}{}

	for _, assigneeId := range a.Issue.AssigneeIDs {
		assigneeUUID := uuid.FromStringOrNil(assigneeId)
		if _, ok := userIdMap[assigneeUUID]; !ok {
			notifyId, countNotify, _ := CreateUserNotificationActivity(n.Db, assigneeUUID, a)
			if notifyId != uuid.Nil {
				n.Ws.Send(assigneeUUID, notifyId, a, countNotify)
			}
			userIdMap[assigneeUUID] = struct{}{}
		}
	}

	for _, watcherId := range a.Issue.WatcherIDs {
		watcherUUID := uuid.FromStringOrNil(watcherId)
		if _, ok := userIdMap[watcherUUID]; !ok {
			notifyId, countNotify, _ := CreateUserNotificationActivity(n.Db, watcherUUID, a)
			if notifyId != uuid.Nil {
				n.Ws.Send(watcherUUID, notifyId, a, countNotify)
			}
			userIdMap[watcherUUID] = struct{}{}
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

	userIdMap := make(map[uuid.UUID]interface{})
	actorUUID := a.ActorId.UUID
	notifyId, countNotify, _ := CreateUserNotificationActivity(n.Db, actorUUID, a)

	if notifyId != uuid.Nil {
		n.Ws.Send(actorUUID, notifyId, a, countNotify)
	}

	userIdMap[actorUUID] = struct{}{}

	{ // уведомления по созданию задачи
		if a.NewIssue != nil {
			a.NewIssue.Author = a.Actor
			a.NewIssue.Workspace = a.Workspace

			for _, assigneeId := range a.NewIssue.AssigneeIDs {
				assigneeUUID := uuid.FromStringOrNil(assigneeId)
				if _, ok := userIdMap[assigneeUUID]; !ok {
					notifyId, countNotify, _ := CreateUserNotificationActivity(n.Db, assigneeUUID, a)
					if notifyId != uuid.Nil {
						n.Ws.Send(assigneeUUID, notifyId, a, countNotify)
					}
					userIdMap[assigneeUUID] = struct{}{}
				}
			}

			for _, watcherId := range a.NewIssue.WatcherIDs {
				watcherUUID := uuid.FromStringOrNil(watcherId)
				if _, ok := userIdMap[watcherUUID]; !ok {
					notifyId, countNotify, _ := CreateUserNotificationActivity(n.Db, watcherUUID, a)
					if notifyId != uuid.Nil {
						n.Ws.Send(watcherUUID, notifyId, a, countNotify)
					}
					userIdMap[watcherUUID] = struct{}{}
				}
			}

			for _, member := range projectMembers {
				if member.IsDefaultWatcher {
					if _, ok := userIdMap[member.MemberId]; !ok {
						notifyId, countNotify, _ := CreateUserNotificationActivity(n.Db, member.MemberId, a)
						if notifyId != uuid.Nil {
							n.Ws.Send(member.MemberId, notifyId, a, countNotify)
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
				notifyId, countNotify, _ := CreateUserNotificationActivity(n.Db, member.MemberId, a)
				if notifyId != uuid.Nil {
					n.Ws.Send(member.MemberId, notifyId, a, countNotify)
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

	authorId := doc.CreatedById.String()
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

	userIdMap := make(map[uuid.UUID]interface{})

	for _, member := range workspaceMembers {
		if _, ok := userIdMap[member.MemberId]; !ok {
			notifyId, countNotify, _ := CreateUserNotificationActivity(n.Db, member.MemberId, a)
			if notifyId != uuid.Nil {
				n.Ws.Send(member.MemberId, notifyId, a, countNotify)
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

	userIdMap := make(map[uuid.UUID]interface{})

	for _, member := range workspaceAdminMembers {
		if _, ok := userIdMap[member.MemberId]; !ok {
			notifyId, countNotify, _ := CreateUserNotificationActivity(n.Db, member.MemberId, a)
			if notifyId != uuid.Nil {
				n.Ws.Send(member.MemberId, notifyId, a, countNotify)
			}
			userIdMap[member.MemberId] = struct{}{}
		}

	}
	return nil
}

type FormNotification struct {
	Notification
}

func NewFormNotification(n *Notification) *FormNotification {
	if n == nil {
		return nil
	}
	return &FormNotification{
		Notification: *n,
	}
}

func (n *FormNotification) Handle(activity dao.ActivityI) error {
	a, ok := activity.(dao.FormActivity)
	if !ok {
		return nil
	}

	// only formAnswer BAK-317
	if a.Field != nil && *a.Field != "form_answers" {
		return nil
	}

	if a.Form == nil {
		if err := n.Db.Unscoped().
			Joins("Author").
			Joins("Workspace").
			Where("forms.workspace_id = ?", a.WorkspaceId).
			Where("forms.id = ?", a.FormId).
			Find(&a.Form).Error; err != nil {
			slog.Error("Get form for activity", "activityId", a.Id, "err", err)
			return err
		}
	}

	// only form Author
	if a.Form.NotificationChannels.App {
		if notify, countNotify, err := CreateUserNotificationActivity(n.Db, a.Form.Author.ID, a); err == nil {
			if notify != uuid.Nil {
				n.Ws.Send(a.Form.Author.ID, notify, a, countNotify)
			}
		}
	}

	return nil
}
