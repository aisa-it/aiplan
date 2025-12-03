package business

import (
	tracker "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/activity-tracker"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

type WorkspaceCtx struct {
	c               echo.Context
	user            *dao.User
	workspace       *dao.Workspace
	workspaceMember *dao.WorkspaceMember
}

func (b *Business) WorkspaceCtx(c echo.Context, user *dao.User, workspace *dao.Workspace, workspaceMember *dao.WorkspaceMember) {
	b.workspaceCtx = &WorkspaceCtx{
		c:               c,
		user:            user,
		workspace:       workspace,
		workspaceMember: workspaceMember,
	}
}

func (b *Business) WorkspaceCtxClean() {
	b.workspaceCtx = nil
}
func (b *Business) DeleteWorkspaceMember(actor *dao.WorkspaceMember, requestedMember *dao.WorkspaceMember) error {
	if requestedMember.Workspace.OwnerId == requestedMember.MemberId {
		if err := requestedMember.Workspace.ChangeOwner(b.db, actor); err != nil {
			return err
		}
	}

	var projectMembers []dao.ProjectMember
	if err := b.db.
		Joins("Project").
		Joins("Member").
		Where("project_members.workspace_id = ?", requestedMember.Workspace.ID).
		Where("project_members.member_id = ?", requestedMember.MemberId).
		Find(&projectMembers).Error; err != nil {
		return err
	}

	var actorProjectMembers []dao.ProjectMember
	if err := b.db.
		Joins("Project").
		Joins("Member").
		Where("project_members.workspace_id = ?", requestedMember.Workspace.ID).
		Where("project_members.member_id = ?", actor.MemberId).
		Find(&actorProjectMembers).Error; err != nil {
		return err
	}

	actorMap := utils.SliceToMap(&actorProjectMembers, func(v *dao.ProjectMember) string {
		return v.ProjectId
	})

	for _, member := range projectMembers {
		actorPM := actorMap[member.ProjectId]

		b.ProjectCtx(b.workspaceCtx.c, actor.Member, member.Project, &actorPM,
			b.workspaceCtx.workspace, b.workspaceCtx.workspaceMember)

		if err := b.DeleteProjectMember(&actorPM, &member); err != nil {
			b.ProjectCtxClean()
			return err
		}

		b.ProjectCtxClean()
	}

	data := map[string]interface{}{
		"updateScopeId": requestedMember.MemberId,
	}

	if err := b.db.Transaction(func(tx *gorm.DB) error {
		err := tracker.TrackActivity[dao.WorkspaceMember, dao.WorkspaceActivity](
			b.tracker, activities.EntityRemoveActivity, data, nil, *requestedMember, b.workspaceCtx.user)
		if err != nil {
			return err
		}

		return tx.Delete(requestedMember).Error
	}); err != nil {
		return err
	}

	return nil
}
