package member_role

import (
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

type MemberSettings struct {
	Notify IsNotifyFunc
}

func FromProject() IsNotifyFunc {
	return shouldProjectNotify

}

func FromWorkspace() IsNotifyFunc {
	return shouldWorkspaceNotify
}

type IsNotifyFunc func(u MemberNotify, event *dao.ActivityEvent, isAuthor bool, nCh types.NotifyChannel) bool

func shouldProjectNotify(u MemberNotify, event *dao.ActivityEvent, isAuthor bool, nCh types.NotifyChannel) bool {
	var settings *types.ProjectMemberNS

	if isAuthor {
		settings = u.authorProjectSettings.getSettings(nCh)
	} else {
		settings = u.memberProjectSettings.getSettings(nCh)
	}

	if settings == nil {
		return false
	}

	return settings.IsNotify(event.Field, event.EntityType, event.Verb, u.getProjectRole())
}

func shouldWorkspaceNotify(u MemberNotify, event *dao.ActivityEvent, isAuthor bool, nCh types.NotifyChannel) bool {
	var settings *types.WorkspaceMemberNS

	if isAuthor {
		settings = u.authorWorkspaceSettings.getSettings(nCh)
	} else {
		settings = u.memberWorkspaceSettings.getSettings(nCh)
	}

	if settings == nil {
		return false
	}

	return settings.IsNotify(event.Field, event.EntityType, event.Verb, u.getWorkspaceRole())
}

func (u *MemberNotify) getProjectRole() int {
	if u.Has(ProjectAdminRole) {
		return types.AdminRole
	}
	if u.Has(ProjectMemberRole) {
		return types.MemberRole
	}
	if u.Has(ProjectGuestRole) {
		return types.GuestRole
	}
	return 0
}

func (u *MemberNotify) getWorkspaceRole() int {
	if u.Has(WorkspaceAdminRole) {
		return types.AdminRole
	}
	if u.Has(WorkspaceMemberRole) {
		return types.MemberRole
	}
	if u.Has(WorkspaceGuestRole) {
		return types.GuestRole
	}
	return 0
}

// Load member notify settings
type funcLoadSettings func(tx *gorm.DB, workspaceId uuid.UUID, ur UserRegistry) error

func LoadProjectSettings(tx *gorm.DB, projectID uuid.UUID, ur UserRegistry) error {
	if len(ur) == 0 {
		return nil
	}

	var members []dao.ProjectMember
	err := tx.
		Where("project_id = ? AND member_id IN ?",
			projectID,
			utils.MapToSlice(ur, func(k uuid.UUID, v *MemberNotify) uuid.UUID { return k })).
		Find(&members).Error

	if err != nil {
		return err
	}

	for _, member := range members {
		user := ur[member.MemberId]

		user.authorProjectSettings = &projectMemberNotifies{
			App:   member.NotificationAuthorSettingsApp,
			Tg:    member.NotificationAuthorSettingsTG,
			Email: member.NotificationAuthorSettingsEmail,
		}
		user.memberProjectSettings = &projectMemberNotifies{
			App:   member.NotificationSettingsApp,
			Tg:    member.NotificationSettingsTG,
			Email: member.NotificationSettingsEmail,
		}

		ur[member.MemberId] = user
	}

	return nil
}

func LoadWorkspaceSettings(tx *gorm.DB, workspaceId uuid.UUID, ur UserRegistry) error {
	if len(ur) == 0 {
		return nil
	}

	var members []dao.WorkspaceMember
	err := tx.
		Where("workspace_id = ? AND member_id IN ?",
			workspaceId,
			utils.MapToSlice(ur, func(k uuid.UUID, v *MemberNotify) uuid.UUID { return k })).
		Find(&members).Error

	if err != nil {
		return err
	}

	for _, member := range members {
		user := ur[member.MemberId]

		user.authorWorkspaceSettings = &workspaceMemberNotifies{
			App:   member.NotificationAuthorSettingsApp,
			Tg:    member.NotificationAuthorSettingsTG,
			Email: member.NotificationAuthorSettingsEmail,
		}
		user.memberWorkspaceSettings = &workspaceMemberNotifies{
			App:   member.NotificationSettingsApp,
			Tg:    member.NotificationSettingsTG,
			Email: member.NotificationSettingsEmail,
		}

		ur[member.MemberId] = user
	}

	return nil
}
