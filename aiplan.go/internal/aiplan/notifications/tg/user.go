package tg

import (
	"log/slog"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
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

func (ur UserRegistry) LoadProjectSettings(tx *gorm.DB, projectID uuid.UUID, r role) error {
	if len(ur) == 0 {
		return nil
	}

	var members []dao.ProjectMember
	err := tx.
		Where("project_id = ? AND member_id IN ?",
			projectID,
			utils.MapToSlice(ur, func(k uuid.UUID, v userTg) uuid.UUID { return k })).
		Find(&members).Error

	if err != nil {
		return err
	}

	for _, member := range members {
		user := ur[member.MemberId]

		if user.Has(r) {
			user.authorProjectSettings = &member.NotificationAuthorSettingsTG
		} else {
			user.memberProjectSettings = &member.NotificationSettingsTG
		}
		ur[member.MemberId] = user
	}

	return nil
}

func (ur UserRegistry) FilterActivity(field *string, verb, entity string, f isNotifyFunc, authorRole role) []userTg {
	result := make([]userTg, 0, len(ur))

	for _, user := range ur {
		if !f(&user, field, verb, entity, user.Has(authorRole)) {
			continue
		}
		result = append(result, user)
	}
	return result
}

type isNotifyFunc func(u *userTg, field *string, verb, entity string, isAuthor bool) bool

func shouldProjectNotify(u *userTg, field *string, verb, entity string, isAuthor bool) bool {
	var settings *types.ProjectMemberNS

	if isAuthor {
		settings = u.authorProjectSettings
	} else {
		settings = u.memberProjectSettings
	}

	if settings == nil {
		return false
	}

	return settings.IsNotify(field, entity, verb, u.getProjectRole())
}

func shouldWorkspaceNotify(u *userTg, field *string, verb, entity string, isAuthor bool) bool {
	var settings *types.WorkspaceMemberNS

	if isAuthor {
		settings = u.authorWorkspaceSettings
	} else {
		settings = u.memberWorkspaceSettings
	}

	if settings == nil {
		return false
	}

	return settings.IsNotify(field, entity, verb, u.getWorkspaceRole())
}

func (u *userTg) getProjectRole() int {
	if u.Has(projectAdminRole) {
		return types.AdminRole
	}
	if u.Has(projectMemberRole) {
		return types.MemberRole
	}
	if u.Has(projectGuestRole) {
		return types.GuestRole
	}
	return 0
}

func (u *userTg) getWorkspaceRole() int {
	if u.Has(workspaceAdminRole) {
		return types.AdminRole
	}
	if u.Has(workspaceMemberRole) {
		return types.MemberRole
	}
	if u.Has(workspaceGuestRole) {
		return types.GuestRole
	}
	return 0
}

func (ur UserRegistry) LoadWorkspaceSettings(tx *gorm.DB, workspaceId uuid.UUID, r role) error {
	if len(ur) == 0 {
		return nil
	}

	var members []dao.WorkspaceMember
	err := tx.
		Where("workspace_id = ? AND member_id IN ?",
			workspaceId,
			utils.MapToSlice(ur, func(k uuid.UUID, v userTg) uuid.UUID { return k })).
		Find(&members).Error

	if err != nil {
		return err
	}

	for _, member := range members {
		user := ur[member.MemberId]

		if user.Has(r) {
			user.authorWorkspaceSettings = &member.NotificationAuthorSettingsTG
		} else {
			user.memberWorkspaceSettings = &member.NotificationSettingsTG
		}
		ur[member.MemberId] = user
	}

	return nil
}

func addOriginalCommentAuthor(tx *gorm.DB, act *dao.IssueActivity, users UserRegistry) {
	if act.NewIssueComment == nil || !act.NewIssueComment.ReplyToCommentId.Valid {
		return
	}

	err := tx.Joins("Actor").
		Where("workspace_id = ? ", act.WorkspaceId).
		Where("project_id = ?", act.ProjectId).
		Where("issue_id = ?", act.IssueId).
		Where("issue_comments.id = ?", act.NewIssueComment.ReplyToCommentId.UUID).
		First(&act.NewIssueComment.OriginalComment).Error

	if err != nil || act.NewIssueComment.OriginalComment.Actor == nil {
		return
	}

	users.addUser(act.NewIssueComment.OriginalComment.Actor, issueCommentCreator)
}

func addProjectAdmin(tx *gorm.DB, projectId uuid.UUID, users UserRegistry) {
	var member []dao.ProjectMember
	if err := tx.Joins("Member").
		Where("project_id = ?", projectId).
		Where("project_members.role = ?", types.AdminRole).
		Find(&member).Error; err != nil {
		return
	}

	for _, v := range member {
		users.addUser(v.Member, projectAdminRole)
	}
}

func addWorkspaceAdmin(tx *gorm.DB, workspaceId uuid.UUID, users UserRegistry) {
	var member []dao.WorkspaceMember
	if err := tx.Joins("Member").
		Where("workspace_id = ?", workspaceId).
		Where("workspace_members.role = ?", types.AdminRole).
		Find(&member).Error; err != nil {
		return
	}

	for _, v := range member {
		users.addUser(v.Member, workspaceAdminRole)
	}
}

func addDocMembers(tx *gorm.DB, docId uuid.UUID, users UserRegistry) {
	var docUsers []dao.DocAccessRules
	if err := tx.Joins("Member").
		Where("doc_id = ?", docId).
		Find(&docUsers).Error; err != nil {
		return
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
}

func addDefaultWatchers(tx *gorm.DB, projectId uuid.UUID, users UserRegistry) {
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
		slog.Error("Fetch default watchers for activity", "project_id", projectId, "err", err)
		return
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
}

func addIssueUsers(issue *dao.Issue, users UserRegistry) {
	if issue == nil {
		return
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
}
