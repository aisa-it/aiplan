package member_role

import (
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

type notifySetting int

const (
	TgSettings notifySetting = iota
	EmailSettings
	AppSettings
)

type MemberSettings struct {
	EntityID uuid.UUID
	Load     funcLoadSettings
	Notify   isNotifyFunc
}

func FromProject(id uuid.UUID) MemberSettings {
	return MemberSettings{
		EntityID: id,
		Load:     loadProjectSettings,
		Notify:   shouldProjectNotify,
	}
}

func FromWorkspace(id uuid.UUID) MemberSettings {
	return MemberSettings{
		EntityID: id,
		Load:     loadWorkspaceSettings,
		Notify:   shouldWorkspaceNotify,
	}
}

type isNotifyFunc func(u *MemberNotify, field string, verb string, entity actField.ActivityField, isAuthor bool) bool

func shouldProjectNotify(u *MemberNotify, field string, verb string, entity actField.ActivityField, isAuthor bool) bool {
	var settings *types.ProjectMemberNS

	if isAuthor {
		settings = u.authorProjectSettings
	} else {
		settings = u.memberProjectSettings
	}

	if settings == nil {
		return false
	}

	if field == "" {
		return false
	}

	return settings.IsNotify(utils.ToPtr(field), entity, verb, u.getProjectRole())
}

func shouldWorkspaceNotify(u *MemberNotify, field string, verb string, entity actField.ActivityField, isAuthor bool) bool {
	var settings *types.WorkspaceMemberNS

	if isAuthor {
		settings = u.authorWorkspaceSettings
	} else {
		settings = u.memberWorkspaceSettings
	}

	if settings == nil {
		return false
	}
	if field == "" {
		return false
	}

	return settings.IsNotify(utils.ToPtr(field), entity, verb, u.getWorkspaceRole())
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
type funcLoadSettings func(tx *gorm.DB, workspaceId uuid.UUID, r Role, ur UserRegistry, settings notifySetting) error

func loadProjectSettings(tx *gorm.DB, projectID uuid.UUID, r Role, ur UserRegistry, settings notifySetting) error {
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

		var authorSettings, memberSettings *types.ProjectMemberNS
		switch settings {
		case TgSettings:
			authorSettings = &member.NotificationAuthorSettingsTG
			memberSettings = &member.NotificationSettingsTG
		case EmailSettings:
			authorSettings = &member.NotificationAuthorSettingsEmail
			memberSettings = &member.NotificationSettingsEmail
		case AppSettings:
			authorSettings = &member.NotificationAuthorSettingsApp
			memberSettings = &member.NotificationSettingsApp
		}

		if user.Has(r) {
			user.authorProjectSettings = authorSettings
		} else {
			user.memberProjectSettings = memberSettings
		}

		ur[member.MemberId] = user
	}

	return nil
}

func loadWorkspaceSettings(tx *gorm.DB, workspaceId uuid.UUID, r Role, ur UserRegistry, settings notifySetting) error {
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

		if user.Has(r) {
			user.authorWorkspaceSettings = &member.NotificationAuthorSettingsTG
		} else {
			user.memberWorkspaceSettings = &member.NotificationSettingsTG
		}
		ur[member.MemberId] = user
	}

	return nil
}

//func (ur UserRegistry) FilterActivity(field string, verb string, entity actField.ActivityField, f isNotifyFunc, authorRole Role) []MemberNotify {
//  result := make([]MemberNotify, 0, len(ur))
//
//  for _, user := range ur {
//    if !f(user, field, verb, entity, user.Has(authorRole)) {
//      continue
//    }
//    result = append(result, *user)
//  }
//  return result
//}
