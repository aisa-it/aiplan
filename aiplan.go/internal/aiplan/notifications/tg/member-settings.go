package tg

import (
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

type memberSettings struct {
	EntityID uuid.UUID
	Load     funcLoadSettings
	Notify   isNotifyFunc
}

func fromProject(id uuid.UUID) memberSettings {
	return memberSettings{
		EntityID: id,
		Load:     LoadProjectSettings,
		Notify:   shouldProjectNotify,
	}
}

func fromWorkspace(id uuid.UUID) memberSettings {
	return memberSettings{
		EntityID: id,
		Load:     loadWorkspaceSettings,
		Notify:   shouldWorkspaceNotify,
	}
}

type isNotifyFunc func(u *userTg, field string, verb string, entity actField.ActivityField, isAuthor bool) bool

func shouldProjectNotify(u *userTg, field string, verb string, entity actField.ActivityField, isAuthor bool) bool {
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

func shouldWorkspaceNotify(u *userTg, field string, verb string, entity actField.ActivityField, isAuthor bool) bool {
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

// Load member notify settings
type funcLoadSettings func(tx *gorm.DB, workspaceId uuid.UUID, r role, ur UserRegistry) error

func LoadProjectSettings(tx *gorm.DB, projectID uuid.UUID, r role, ur UserRegistry) error {
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

func loadWorkspaceSettings(tx *gorm.DB, workspaceId uuid.UUID, r role, ur UserRegistry) error {
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
