package notifications

import (
	"fmt"
	"log/slog"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	member_role "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications/member-role"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

var issueRolesNotified = []member_role.Role{
	member_role.ActionAuthor,
	member_role.IssueAuthor,
	member_role.ProjectDefaultWatcher,
	member_role.IssueWatcher,
	member_role.IssueAssigner,
}

type baseEventHandler struct{}

func (baseEventHandler) FilterRecipients(event *dao.ActivityEvent, recipients []member_role.MemberNotify) []member_role.MemberNotify {
	return recipients
}

func (baseEventHandler) CanHandle(event *dao.ActivityEvent) bool {
	return true
}

// Реализация для слоя issue
type issueEvent struct {
	baseEventHandler
}

func (issueEvent) Preload(tx *gorm.DB, event *dao.ActivityEvent) error {
	issue, err := preloadIssue(tx, event.IssueID.UUID)
	if err != nil {
		return err
	}
	event.Issue = issue

	return nil
}

func (issueEvent) GetRecipientsSteps(event *dao.ActivityEvent) []member_role.UsersStep {
	return []member_role.UsersStep{
		member_role.AddUserRole(event.Issue.Author, member_role.IssueAuthor),
		member_role.AddIssueUsers(event.Issue),
		member_role.AddOriginalCommentAuthor(event),
		member_role.AddCommentMentionedUsers(event.NewIssueComment),
		member_role.AddDefaultWatchers(event.ProjectID.UUID),
	}
}

func (issueEvent) GetSettingsFunc() member_role.IsNotifyFunc {
	return member_role.FromProject()
}

func (issueEvent) AuthorRole() member_role.Role {
	return member_role.IssueAuthor
}

// Реализация для слоя project
type projectEvent struct {
	baseEventHandler
}

func (p projectEvent) FilterRecipients(event *dao.ActivityEvent, recipients []member_role.MemberNotify) []member_role.MemberNotify {
	if event.NewIssue == nil {
		return recipients
	}

	var filtered []member_role.MemberNotify

	for _, u := range recipients {
		for _, r := range issueRolesNotified {
			if u.Has(r) {
				filtered = append(filtered, u)
				break
			}
		}
	}

	return filtered
}

func (p projectEvent) Preload(tx *gorm.DB, event *dao.ActivityEvent) error {
	if err := tx.Unscoped().
		Joins("Workspace").
		Joins("ProjectLead").
		Where("projects.id = ?", event.ProjectID.UUID).
		First(&event.Project).Error; err != nil {
		return fmt.Errorf("preloadProjectActivity: %v", err)
	}
	event.Workspace = event.Project.Workspace

	if event.NewIssue != nil {
		issue, err := preloadIssue(tx, event.NewIssue.ID)
		if err != nil {
			return err
		}
		event.NewIssue = issue
	}

	return nil
}

func (p projectEvent) GetRecipientsSteps(event *dao.ActivityEvent) []member_role.UsersStep {
	return []member_role.UsersStep{
		member_role.AddDefaultWatchers(event.ProjectID.UUID),
		member_role.AddIssueUsers(event.NewIssue),
		member_role.AddProjectAdmin(event.ProjectID.UUID),
	}
}

func (p projectEvent) GetSettingsFunc() member_role.IsNotifyFunc {
	return member_role.FromProject()
}

func (p projectEvent) AuthorRole() member_role.Role {
	return member_role.NoAuthor
}

// Реализация для слоя sprint
type sprintEvent struct {
	baseEventHandler
}

func (s sprintEvent) Preload(tx *gorm.DB, event *dao.ActivityEvent) error {
	if err := tx.Unscoped().
		Joins("Workspace").
		Joins("CreatedBy").
		Preload("Watchers").
		Where("sprints.id = ?", event.SprintID).
		First(&event.Sprint).Error; err != nil {
		return fmt.Errorf("preloadSprintActivity: %v", err)
	}
	var err error
	if event.NewSprintIssue != nil {
		event.NewSprintIssue, err = preloadIssue(tx, event.NewSprintIssue.ID)
		if err != nil {
			return err
		}
	}
	if event.OldSprintIssue != nil {
		event.OldSprintIssue, err = preloadIssue(tx, event.OldSprintIssue.ID)
		if err != nil {
			return err
		}
	}

	event.Workspace = event.Sprint.Workspace

	return nil
}

func (s sprintEvent) GetRecipientsSteps(event *dao.ActivityEvent) []member_role.UsersStep {
	return []member_role.UsersStep{
		member_role.AddUserRole(&event.Sprint.CreatedBy, member_role.SprintAuthor),
		member_role.AddUsers(event.Sprint.Watchers, member_role.SprintWatcher),
	}
}

func (s sprintEvent) GetSettingsFunc() member_role.IsNotifyFunc {
	return member_role.FromWorkspace()
}

func (s sprintEvent) AuthorRole() member_role.Role {
	return member_role.SprintAuthor
}

// Реализация для слоя workspace
type workspaceEvent struct {
	baseEventHandler
}

func (w workspaceEvent) Preload(tx *gorm.DB, event *dao.ActivityEvent) error {
	if err := tx.Unscoped().
		Joins("Owner").
		Where("workspaces.id = ?", event.WorkspaceID.UUID).
		First(&event.Workspace).Error; err != nil {
		slog.Error("Get workspace for activity", "activityId", event.ID, "err", err)
		return fmt.Errorf("preloadWorkspaceActivity: %v", err)
	}

	return nil
}

func (w workspaceEvent) GetRecipientsSteps(event *dao.ActivityEvent) []member_role.UsersStep {
	return []member_role.UsersStep{
		member_role.AddWorkspaceAdmins(event.WorkspaceID.UUID),
	}
}

func (w workspaceEvent) GetSettingsFunc() member_role.IsNotifyFunc {
	return member_role.FromWorkspace()
}

func (w workspaceEvent) AuthorRole() member_role.Role {
	return member_role.NoAuthor
}

// Реализация для слоя doc
type docEvent struct {
	baseEventHandler
}

func (d docEvent) Preload(tx *gorm.DB, event *dao.ActivityEvent) error {
	if err := tx.Unscoped().
		Joins("Workspace").
		Joins("Author").
		Preload("AccessRules.Member").
		Where("docs.id = ?", event.DocID.UUID).
		First(&event.Doc).Error; err != nil {
		return fmt.Errorf("preloadDocActivity: %v", err)
	}
	event.Workspace = event.Doc.Workspace
	event.Doc.AfterFind(tx)

	return nil
}

func (d docEvent) GetRecipientsSteps(event *dao.ActivityEvent) []member_role.UsersStep {
	return []member_role.UsersStep{
		member_role.AddUserRole(event.Doc.Author, member_role.DocAuthor),
		member_role.AddCommentMentionedUsers(event.NewDocComment),
		member_role.AddDocMembers(event.DocID.UUID),
	}
}

func (d docEvent) GetSettingsFunc() member_role.IsNotifyFunc {
	return member_role.FromWorkspace()
}

func (d docEvent) AuthorRole() member_role.Role {
	return member_role.ActionAuthor
}

func preloadIssue(tx *gorm.DB, issueID uuid.UUID) (*dao.Issue, error) {
	var issue dao.Issue
	err := tx.Unscoped().
		Joins("Author").
		Joins("Workspace").
		Joins("Project").
		Joins("Parent").
		Preload("Assignees").
		Preload("Watchers").
		Preload("Parent.Project").
		Preload("Links").
		Where("issues.id = ?", issueID).
		First(&issue).Error
	return &issue, err
}
