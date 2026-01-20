package tg

import (
	"fmt"
	"regexp"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

type UserRegistry map[uuid.UUID]userTg

type role uint

const (
	projectAdminRole role = 1 << iota
	projectMemberRole
	projectGuestRole

	projectDefaultWatcher
	projectDefaultAssigner

	issueAuthor
	issueWatcher
	issueAssigner
	issueCommentCreator

	commentMentioned

	workspaceAdminRole
	workspaceMemberRole
	workspaceGuestRole

	docAuthor
	docWatcher
	docEditor
	docReader

	sprintAuthor
	sprintWatcher

	actionAuthor
)

type userTg struct {
	id  int64
	loc types.TimeZone

	authorProjectSettings *types.ProjectMemberNS
	memberProjectSettings *types.ProjectMemberNS

	authorWorkspaceSettings *types.WorkspaceMemberNS
	memberWorkspaceSettings *types.WorkspaceMemberNS

	roles role
}

func (u *userTg) Has(r role) bool {
	return u.roles&r != 0
}

func (u *userTg) Add(r role) {
	u.roles |= r
}

func (u *userTg) Remove(r role) {
	u.roles &^= r
}

func (u *userTg) Toggle(in role) {
	u.roles ^= in
}

func (ur UserRegistry) addUser(user *dao.User, roles ...role) bool {
	if user == nil {
		return false
	}
	if existing, exists := ur[user.ID]; exists {
		for _, r := range roles {
			existing.Add(r)
		}
		ur[user.ID] = existing
		return true
	}

	tgUser, ok := getUserTg(user, roles...)
	if !ok {
		return false
	}

	ur[user.ID] = tgUser
	return true
}

func combineRoles(roles ...role) role {
	var result role
	for _, r := range roles {
		result |= r
	}
	return result
}

func (ur UserRegistry) FilterActivity(field string, verb string, entity actField.ActivityField, f isNotifyFunc, authorRole role) []userTg {
	result := make([]userTg, 0, len(ur))

	for _, user := range ur {
		if !f(&user, field, verb, entity, user.Has(authorRole)) {
			continue
		}
		result = append(result, user)
	}
	return result
}

// UsersStep helpers
type UsersStep func(tx *gorm.DB, act dao.ActivityI, users UserRegistry) error

func addUserRole(actor *dao.User, role role) UsersStep {
	return func(tx *gorm.DB, a dao.ActivityI, users UserRegistry) error {
		users.addUser(actor, role)
		return nil
	}
}

func addOriginalCommentAuthor(act *dao.IssueActivity) UsersStep {
	return func(tx *gorm.DB, a dao.ActivityI, users UserRegistry) error {

		if act.NewIssueComment == nil || !act.NewIssueComment.ReplyToCommentId.Valid {
			return nil
		}

		err := tx.Joins("Actor").
			Where("workspace_id = ? ", act.WorkspaceId).
			Where("project_id = ?", act.ProjectId).
			Where("issue_id = ?", act.IssueId).
			Where("issue_comments.id = ?", act.NewIssueComment.ReplyToCommentId.UUID).
			First(&act.NewIssueComment.OriginalComment).Error

		if err != nil || act.NewIssueComment.OriginalComment.Actor == nil {
			return fmt.Errorf("reply comment not found for issue %d", act.IssueId)
		}

		users.addUser(act.NewIssueComment.OriginalComment.Actor, issueCommentCreator)
		return nil
	}
}

func addProjectAdmin(projectId uuid.UUID) UsersStep {
	return func(tx *gorm.DB, a dao.ActivityI, users UserRegistry) error {

		var member []dao.ProjectMember
		if err := tx.Joins("Member").
			Where("project_id = ?", projectId).
			Where("project_members.role = ?", types.AdminRole).
			Find(&member).Error; err != nil {
			return fmt.Errorf("get project members failed: %v", err)
		}

		for _, v := range member {
			users.addUser(v.Member, projectAdminRole)
		}
		return nil
	}
}

func addWorkspaceAdmins(workspaceId uuid.UUID) UsersStep {
	return func(tx *gorm.DB, a dao.ActivityI, users UserRegistry) error {
		var member []dao.WorkspaceMember
		if err := tx.Joins("Member").
			Where("workspace_id = ?", workspaceId).
			Where("workspace_members.role = ?", types.AdminRole).
			Find(&member).Error; err != nil {
			return fmt.Errorf("get workspace members failed: %v", err)
		}

		for _, v := range member {
			users.addUser(v.Member, workspaceAdminRole)
		}
		return nil
	}
}

func addDocMembers(docID uuid.UUID) UsersStep {
	return func(tx *gorm.DB, a dao.ActivityI, users UserRegistry) error {
		var docUsers []dao.DocAccessRules
		if err := tx.Joins("Member").
			Where("doc_id = ?", docID).
			Find(&docUsers).Error; err != nil {
			return fmt.Errorf("get doc access rules failed: %v", err)
		}

		for _, u := range docUsers {
			roles := []role{docReader}
			if u.Edit {
				roles = append(roles, docEditor)
			}
			if u.Watch {
				roles = append(roles, docWatcher)
			}
			users.addUser(u.Member, roles...)
		}
		return nil
	}
}

func addDefaultWatchers(projectId uuid.UUID) UsersStep {
	return func(tx *gorm.DB, a dao.ActivityI, users UserRegistry) error {

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

			users.addUser(user, projectDefaultWatcher)
		}
		return nil
	}
}

func addIssueUsers(issue *dao.Issue) UsersStep {
	return func(tx *gorm.DB, a dao.ActivityI, users UserRegistry) error {
		if issue == nil {
			return nil
		}
		users.addUser(issue.Author, issueAuthor)

		if issue.Assignees != nil {
			for _, assignee := range *issue.Assignees {
				users.addUser(&assignee, issueAssigner)
			}
		}

		if issue.Watchers != nil {
			for _, watcher := range *issue.Watchers {
				users.addUser(&watcher, issueWatcher)
			}
		}
		return nil
	}
}

func addUsers(from []dao.User, r role) UsersStep {
	return func(tx *gorm.DB, a dao.ActivityI, users UserRegistry) error {
		for _, u := range from {
			users.addUser(&u, r)
		}
		return nil
	}
}

func addCommentMentionedUsers[R dao.IRedactorHTML](comment *R) UsersStep {
	return func(tx *gorm.DB, a dao.ActivityI, users UserRegistry) error {
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
			users.addUser(&u, commentMentioned)
		}
		return nil
	}
}
