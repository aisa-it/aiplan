package member_role

import (
	"fmt"
	"regexp"

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
)

type MemberNotify struct {
	//id  int64
	user *dao.User
	loc  types.TimeZone

	authorProjectSettings *projectMemberNotifies
	memberProjectSettings *projectMemberNotifies

	authorWorkspaceSettings *workspaceMemberNotifies
	memberWorkspaceSettings *workspaceMemberNotifies

	roles Role
}

func (m *MemberNotify) GetLoc() *types.TimeZone {
	return &m.loc
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

//func (u *MemberNotify) I(field string, verb string, entity actField.ActivityField, f isNotifyFunc, authorRole Role)  {
//  u.
//  if f(u, field, verb, entity, u.Has(authorRole)){
//
//  }
//}

//func (u *MemberNotify) Allowed(
//	field string,
//	verb string,
//	entity actField.ActivityField,
//	authorRole Role,
//	settings *MemberSettings,
//) bool {
//	isAuthor := u.Has(authorRole)
//	return settings.Notify(u, field, verb, entity, isAuthor)
//}

//func (ur UserRegistry) AddUser(user *dao.User, roles ...Role) bool {
//	if user == nil {
//		return false
//	}
//	if existing, exists := ur[user.ID]; exists {
//		for _, r := range roles {
//			existing.Add(r)
//		}
//		ur[user.ID] = existing
//		return true
//	}
//
//	tgUser, ok := getUser(user, roles...)
//	if !ok {
//		return false
//	}
//
//	ur[user.ID] = tgUser
//	return true
//}

func (ur UserRegistry) AddUser(user *dao.User, roles ...Role) bool {
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
	}

	return true
}

//	func getUser(user *dao.User, roles ...Role) (MemberNotify, bool) {
//		if user.TelegramId == nil {
//			return MemberNotify{}, false
//		}
//		if !user.CanReceiveNotifications() {
//			return MemberNotify{}, false
//		}
//		if user.Settings.TgNotificationMute {
//			return MemberNotify{}, false
//		}
//		return MemberNotify{id: *user.TelegramId, loc: user.UserTimezone, roles: combineRoles(roles...)}, true
//	}
//
// TODO ref to new notify
func (ur UserRegistry) FilterActivity(field string, verb string, entity actField.ActivityField, f IsNotifyFunc, authorRole Role) []*MemberNotify {
	result := make([]*MemberNotify, 0, len(ur))

	for _, user := range ur {
		//if !f(user, field, verb, entity, user.Has(authorRole)) {
		//	continue
		//}
		result = append(result, user)
	}
	return result
}

// UsersStep helpers
type UsersStep func(tx *gorm.DB, users UserRegistry) error

func AddUserRole(actor *dao.User, role Role) UsersStep {
	return func(tx *gorm.DB, users UserRegistry) error {
		users.AddUser(actor, role)
		return nil
	}
}

func AddOriginalCommentAuthor(act *dao.ActivityEvent) UsersStep {
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

		users.AddUser(act.NewIssueComment.OriginalComment.Actor, IssueCommentCreator)
		return nil
	}
}

func AddProjectAdmin(projectId uuid.UUID) UsersStep {
	return func(tx *gorm.DB, users UserRegistry) error {

		var member []dao.ProjectMember
		if err := tx.Joins("Member").
			Where("project_id = ?", projectId).
			Where("project_members.Role = ?", types.AdminRole).
			Find(&member).Error; err != nil {
			return fmt.Errorf("get project members failed: %v", err)
		}

		for _, v := range member {
			users.AddUser(v.Member, ProjectAdminRole)
		}
		return nil
	}
}

func AddWorkspaceAdmins(workspaceId uuid.UUID) UsersStep {
	return func(tx *gorm.DB, users UserRegistry) error {
		var member []dao.WorkspaceMember
		if err := tx.Joins("Member").
			Where("workspace_id = ?", workspaceId).
			Where("workspace_members.Role = ?", types.AdminRole).
			Find(&member).Error; err != nil {
			return fmt.Errorf("get workspace members failed: %v", err)
		}

		for _, v := range member {
			users.AddUser(v.Member, WorkspaceAdminRole)
		}
		return nil
	}
}

func AddDocMembers(docID uuid.UUID) UsersStep {
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
			users.AddUser(u.Member, roles...)
		}
		return nil
	}
}

func AddDefaultWatchers(projectId uuid.UUID) UsersStep {
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

			users.AddUser(user, ProjectDefaultWatcher)
		}
		return nil
	}
}

func AddIssueUsers(issue *dao.Issue) UsersStep {
	return func(tx *gorm.DB, users UserRegistry) error {
		if issue == nil {
			return nil
		}
		users.AddUser(issue.Author, IssueAuthor)

		if issue.Assignees != nil {
			for _, assignee := range *issue.Assignees {
				users.AddUser(&assignee, IssueAssigner)
			}
		}

		if issue.Watchers != nil {
			for _, watcher := range *issue.Watchers {
				users.AddUser(&watcher, IssueWatcher)
			}
		}
		return nil
	}
}

func AddUsers(from []dao.User, r Role) UsersStep {
	return func(tx *gorm.DB, users UserRegistry) error {
		for _, u := range from {
			users.AddUser(&u, r)
		}
		return nil
	}
}

func AddCommentMentionedUsers[R dao.IRedactorHTML](comment *R) UsersStep {
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
			users.AddUser(&u, CommentMentioned)
		}
		return nil
	}
}
