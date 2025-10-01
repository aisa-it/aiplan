package business

import (
	"time"

	tracker "github.com/aisa-it/aiplan/internal/aiplan/activity-tracker"
	"github.com/aisa-it/aiplan/internal/aiplan/dao"
	errStack "github.com/aisa-it/aiplan/internal/aiplan/stack-error"
	"github.com/aisa-it/aiplan/internal/aiplan/types"
	"github.com/aisa-it/aiplan/internal/aiplan/utils"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
	// Change workspace owner on demand
	if requestedMember.Workspace.OwnerId == requestedMember.MemberId {
		if err := requestedMember.Workspace.ChangeOwner(b.db, actor); err != nil {
			return err
		}
	}

	// Change role to admin for new owner
	if err := b.db.Model(requestedMember).UpdateColumn("role", types.AdminRole).Error; err != nil {
		return err
	}

	// Update memberships in projects
	{
		var projects []dao.Project
		if err := b.db.Where("workspace_id = ?", requestedMember.Workspace.ID).Find(&projects).Error; err != nil {
			return err
		}

		for _, project := range projects {
			if err := b.db.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "project_id"}, {Name: "member_id"}},
				DoUpdates: clause.Assignments(map[string]interface{}{"role": types.AdminRole, "updated_at": time.Now(), "updated_by_id": actor.MemberId}),
			}).Create(&dao.ProjectMember{
				ID:          dao.GenID(),
				CreatedAt:   time.Now(),
				CreatedById: &actor.MemberId,
				WorkspaceId: requestedMember.Workspace.ID,
				ProjectId:   project.ID,
				Role:        types.AdminRole,
				MemberId:    requestedMember.MemberId,
			}).Error; err != nil {
				return err
			}
		}
	}

	// Migrate projects leaders to current user
	if err := b.db.
		Model(&dao.Project{}).
		Where(&dao.Project{
			WorkspaceId:   requestedMember.Workspace.ID,
			ProjectLeadId: requestedMember.MemberId,
		}).
		Updates(dao.Project{
			ProjectLeadId: actor.MemberId,
		}).Error; err != nil {
		return err
	}

	var actorProjectsMembers []dao.ProjectMember
	if err := b.db.
		Joins("Project").
		Joins("Member").
		Where("project_members.workspace_id = ?", requestedMember.Workspace.ID).
		Where("member_id = ?", actor.MemberId).
		Find(&actorProjectsMembers).Error; err != nil {
		return err
	}

	actorMap := utils.SliceToMap(&actorProjectsMembers, func(v *dao.ProjectMember) string {
		return v.ProjectId
	})

	var delProjectsMembers []dao.ProjectMember
	if err := b.db.
		Joins("Project").
		Joins("Member").
		Where("project_members.workspace_id = ?", requestedMember.Workspace.ID).
		Where("member_id = ?", requestedMember.MemberId).
		Find(&delProjectsMembers).Error; err != nil {
		return err
	}

	for _, member := range delProjectsMembers {
		pm := actorMap[member.ProjectId]
		b.ProjectCtx(b.workspaceCtx.c, actor.Member, member.Project, &pm, b.workspaceCtx.workspace, b.workspaceCtx.workspaceMember)
		if err := b.DeleteProjectMember(&pm, &member); err != nil {
			return err
		}
		b.ProjectCtxClean()
	}

	data := map[string]interface{}{
		"updateScopeId": requestedMember.MemberId,
	}

	if err := b.db.Transaction(func(tx *gorm.DB) error {
		err := tracker.TrackActivity[dao.WorkspaceMember, dao.WorkspaceActivity](b.tracker, tracker.ENTITY_REMOVE_ACTIVITY, data, nil, *requestedMember, b.workspaceCtx.user)
		if err != nil {
			errStack.GetError(b.workspaceCtx.c, err)
			return err
		}

		return b.db.Omit(clause.Associations).Delete(requestedMember).Error
	}); err != nil {
		return err
	}

	return nil
}
