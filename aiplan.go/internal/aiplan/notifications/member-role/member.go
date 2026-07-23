package member_role

import (
	"fmt"
	"regexp"
	"slices"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

type UserRegistry map[uuid.UUID]*MemberNotify

type Role uint

const (
	ProjectAdminRole Role = 1 << iota
	ProjectMemberRole
	ProjectGuestRole

	ProjectDefaultWatcher
	ProjectDefaultAssigner

	IssueAuthor
	IssueWatcher
	IssueAssigner
	IssueCommentCreator

	CommentMentioned

	WorkspaceAdminRole
	WorkspaceMemberRole
	WorkspaceGuestRole

	DocAuthor
	DocWatcher
	DocEditor
	DocReader

	SprintAuthor
	SprintWatcher

	ActionAuthor

	NoAuthor
)

type MemberNotify struct {
	//id  int64
	user *dao.User
	loc  types.TimeZone

	authorProjectSettings *projectMemberNotifies
	memberProjectSettings *projectMemberNotifies

	authorWorkspaceSettings *workspaceMemberNotifies
	memberWorkspaceSettings *workspaceMemberNotifies

	customIdActivities map[uuid.UUID]struct{}
	roles              Role
}

func (m *MemberNotify) GetLoc() *types.TimeZone {
	return &m.loc
}

func (m *MemberNotify) IsActNotify(uuids []uuid.UUID) bool {
	if m.customIdActivities == nil || len(m.customIdActivities) == 0 {
		return true
	}

	return slices.ContainsFunc(uuids, func(id uuid.UUID) bool {
		_, ok := m.customIdActivities[id]
		return ok
	})
}

type projectMemberNotifies struct {
	App   types.ProjectMemberNS
	Tg    types.ProjectMemberNS
	Email types.ProjectMemberNS
}

func (p *projectMemberNotifies) getSettings(nCh types.NotifyChannel) *types.ProjectMemberNS {
	switch nCh {
	case types.TgCh:
		return &p.Tg
	case types.EmailCh:
		return &p.Email
	case types.AppCh:
		return &p.App
	}
	return nil
}

type workspaceMemberNotifies struct {
	App   types.WorkspaceMemberNS
	Tg    types.WorkspaceMemberNS
	Email types.WorkspaceMemberNS
}

func (w *workspaceMemberNotifies) getSettings(nCh types.NotifyChannel) *types.WorkspaceMemberNS {
	switch nCh {
	case types.TgCh:
		return &w.Tg
	case types.EmailCh:
		return &w.Email
	case types.AppCh:
		return &w.App
	}
	return nil
}

func (u *MemberNotify) Has(r Role) bool {
	return u.roles&r != 0
}

func (u *MemberNotify) Add(r Role) {
	u.roles |= r
}

func (u *MemberNotify) Remove(r Role) {
	u.roles &^= r
}

func (u *MemberNotify) Toggle(in Role) {
	u.roles ^= in
}

func (u *MemberNotify) GetUser() *dao.User {
	return u.user
}

func (u *MemberNotify) Allowed(field, verb string, entityType types.EntityLayer,
	authorRole Role, settings *MemberSettings, nCh types.NotifyChannel,
) bool {
	isAuthor := u.Has(authorRole)
	event := dao.ActivityEvent{Field: actField.ActivityField(field), Verb: verb, EntityType: entityType}
	return settings.Notify(*u, &event, isAuthor, nCh)
}

func (ur UserRegistry) AddUser(user *dao.User, conf *memberConfig, roles ...Role) bool {
	if user == nil || !user.CanReceiveNotifications() {
		return false
	}

	m, ok := ur[user.ID]
	if !ok {
		m = &MemberNotify{
			user: user,
			loc:  user.UserTimezone,
		}

		ur[user.ID] = m
	}

	for _, r := range roles {
		m.Add(r)
		if conf != nil && conf.customActivityId.Valid {
			if m.customIdActivities == nil {
				m.customIdActivities = make(map[uuid.UUID]struct{})
			}
			m.customIdActivities[conf.customActivityId.UUID] = struct{}{}
		}
	}

	return true
}

// UsersStep helpers
type UsersStep func(tx *gorm.DB, users UserRegistry) error

func AddUserRole(actor *dao.User, role Role) UsersStep {
	conf := &memberConfig{}
	return func(tx *gorm.DB, users UserRegistry) error {
		users.AddUser(actor, conf, role)
		return nil
	}
}

func AddOriginalCommentAuthor(act *dao.ActivityEvent) UsersStep {
	conf := &memberConfig{}

	return func(tx *gorm.DB, users UserRegistry) error {

		if act.NewIssueComment == nil || !act.NewIssueComment.ReplyToCommentId.Valid {
			return nil
		}

		err := tx.Joins("Actor").
			Where("workspace_id = ? ", act.WorkspaceID).
			Where("project_id = ?", act.ProjectID).
			Where("issue_id = ?", act.IssueID).
			Where("issue_comments.id = ?", act.NewIssueComment.ReplyToCommentId.UUID).
			First(&act.NewIssueComment.OriginalComment).Error

		if err != nil || act.NewIssueComment.OriginalComment.Actor == nil {
			return fmt.Errorf("reply comment not found for issue %d", act.IssueID)
		}

		users.AddUser(act.NewIssueComment.OriginalComment.Actor, conf, IssueCommentCreator)
		return nil
	}
}

func AddProjectAdmin(projectId uuid.UUID) UsersStep {
	conf := &memberConfig{}

	return func(tx *gorm.DB, users UserRegistry) error {

		var member []dao.ProjectMember
		if err := tx.Joins("Member").
			Where("project_id = ?", projectId).
			Where("project_members.Role = ?", types.AdminRole).
			Find(&member).Error; err != nil {
			return fmt.Errorf("get project members failed: %v", err)
		}

		for _, v := range member {
			users.AddUser(v.Member, conf, ProjectAdminRole)
		}
		return nil
	}
}

func AddWorkspaceAdmins(workspaceId uuid.UUID) UsersStep {
	conf := &memberConfig{}

	return func(tx *gorm.DB, users UserRegistry) error {
		var member []dao.WorkspaceMember
		if err := tx.Joins("Member").
			Where("workspace_id = ?", workspaceId).
			Where("workspace_members.Role = ?", types.AdminRole).
			Find(&member).Error; err != nil {
			return fmt.Errorf("get workspace members failed: %v", err)
		}

		for _, v := range member {
			users.AddUser(v.Member, conf, WorkspaceAdminRole)
		}
		return nil
	}
}

func AddDocMembers(docID uuid.UUID) UsersStep {
	conf := &memberConfig{}

	return func(tx *gorm.DB, users UserRegistry) error {
		var docUsers []dao.DocAccessRules
		if err := tx.Joins("Member").
			Where("doc_id = ?", docID).
			Find(&docUsers).Error; err != nil {
			return fmt.Errorf("get doc access rules failed: %v", err)
		}

		for _, u := range docUsers {
			roles := []Role{DocReader}
			if u.Edit {
				roles = append(roles, DocEditor)
			}
			if u.Watch {
				roles = append(roles, DocWatcher)
			}
			users.AddUser(u.Member, conf, roles...)
		}
		return nil
	}
}

func AddDefaultWatchers(projectId uuid.UUID, opts ...MemberOption) UsersStep {
	config := &memberConfig{}
	for _, opt := range opts {
		opt(config)
	}

	return func(tx *gorm.DB, users UserRegistry) error {

		var defaultWatchers []struct {
			ID           uuid.UUID
			TelegramId   *int64
			UserTimezone types.TimeZone
			Settings     types.UserSettings
		}

		err := tx.Model(&dao.ProjectMember{}).
			Select("users.id, users.telegram_id, users.user_timezone, users.settings").
			Joins("JOIN users ON users.id = project_members.member_id").
			Where("project_id = ? AND is_default_watcher = true", projectId).
			Scan(&defaultWatchers).Error

		if err != nil {
			return fmt.Errorf("get default watchers for activity: %v", err)
		}

		for _, w := range defaultWatchers {
			user := &dao.User{
				ID:           w.ID,
				TelegramId:   w.TelegramId,
				UserTimezone: w.UserTimezone,
				Settings:     w.Settings,
			}

			users.AddUser(user, config, ProjectDefaultWatcher)
		}
		return nil
	}
}

type memberConfig struct {
	customActivityId uuid.NullUUID
}

type MemberOption func(*memberConfig)

func WithActivityId(id uuid.UUID) MemberOption {
	return func(config *memberConfig) {
		config.customActivityId = uuid.NullUUID{UUID: id, Valid: true}
	}
}

func AddIssueUsers(issue *dao.Issue, opts ...MemberOption) UsersStep {
	config := &memberConfig{}
	for _, opt := range opts {
		opt(config)
	}
	return func(tx *gorm.DB, users UserRegistry) error {
		if issue == nil {
			return nil
		}
		users.AddUser(issue.Author, config, IssueAuthor)

		if issue.Assignees != nil {
			for _, assignee := range *issue.Assignees {
				users.AddUser(&assignee, config, IssueAssigner)
			}
		}

		if issue.Watchers != nil {
			for _, watcher := range *issue.Watchers {
				users.AddUser(&watcher, config, IssueWatcher)
			}
		}
		return nil
	}
}

func AddUsers(from []dao.User, r Role) UsersStep {
	config := &memberConfig{}
	return func(tx *gorm.DB, users UserRegistry) error {
		for _, u := range from {
			users.AddUser(&u, config, r)
		}
		return nil
	}
}

func AddCommentMentionedUsers[R dao.IRedactorHTML](comment *R) UsersStep {
	config := &memberConfig{}

	return func(tx *gorm.DB, users UserRegistry) error {
		if comment == nil {
			return nil
		}

		reg := regexp.MustCompile(`@(\w+)`)

		res := reg.FindAllStringSubmatch((*comment).GetRedactorHtml().Body, -1)

		if len(res) == 0 {
			return nil
		}

		usernames := make([]string, len(res))
		for i, r := range res {
			usernames[i] = r[1]
		}

		var us []dao.User
		tx.Where("username in (?)", usernames).Find(&us)
		for _, u := range us {
			users.AddUser(&u, config, CommentMentioned)
		}
		return nil
	}
}
