package business

import (
	"log/slog"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	tracker "sheff.online/aiplan/internal/aiplan/activity-tracker"
	"sheff.online/aiplan/internal/aiplan/apierrors"
	"sheff.online/aiplan/internal/aiplan/dao"
	errStack "sheff.online/aiplan/internal/aiplan/stack-error"
	"sheff.online/aiplan/internal/aiplan/types"
)

type ProjectCtx struct {
	c               echo.Context
	user            *dao.User
	project         *dao.Project
	projectMember   *dao.ProjectMember
	workspace       *dao.Workspace
	workspaceMember *dao.WorkspaceMember
}

func (b *Business) ProjectCtx(c echo.Context, user *dao.User, project *dao.Project, projectMember *dao.ProjectMember, workspace *dao.Workspace, workspaceMember *dao.WorkspaceMember) {
	b.projectCtx = &ProjectCtx{
		c:               c,
		user:            user,
		project:         project,
		projectMember:   projectMember,
		workspace:       workspace,
		workspaceMember: workspaceMember,
	}
}

func (b *Business) ProjectCtxClean() {
	b.projectCtx = nil
}

func (b *Business) DeleteProject() error {
	if !b.projectCtx.user.IsSuperuser && b.projectCtx.user.ID != b.projectCtx.project.ProjectLeadId && b.projectCtx.workspaceMember.Role != types.AdminRole {
		return apierrors.ErrDeleteProjectForbidden
	}

	err := tracker.TrackActivity[dao.Project, dao.WorkspaceActivity](b.tracker, tracker.ENTITY_DELETE_ACTIVITY, nil, nil, *b.projectCtx.project, b.projectCtx.user)
	if err != nil {
		errStack.GetError(nil, err)
		return err
	}

	// Soft-delete project
	if err := b.db.Session(&gorm.Session{SkipHooks: true}).Omit(clause.Associations).Delete(&b.projectCtx.project).Error; err != nil {
		return err
	}

	// Start hard deleting in foreground
	go func(project dao.Project) {
		if err := b.db.Unscoped().Delete(&project).Error; err != nil {
			slog.Error("Hard delete project", "projectId", project.ID, "err", err)
		}
	}(*b.projectCtx.project)

	return nil
}

func (b *Business) DeleteProjectMember(actor *dao.ProjectMember, requestedMember *dao.ProjectMember) error {
	var isWorkspaceAdmin bool

	if actor.MemberId != requestedMember.MemberId {
		if err := b.db.
			Select("count(*) > 0").
			Where("role = ?", types.AdminRole).
			Where("workspace_id = ?", actor.WorkspaceId).
			Where("member_id = ?", requestedMember.MemberId).
			Model(&dao.WorkspaceMember{}).
			Find(&isWorkspaceAdmin).Error; err != nil {
			return err
		}

		if isWorkspaceAdmin {
			var actorWm dao.WorkspaceMember
			if err := b.db.
				Joins("Workspace").
				Where("workspace_id = ?", actor.WorkspaceId).
				Where("member_id = ?", actor.MemberId).
				Find(&actorWm).Error; err != nil {
				return err
			}
			if actorWm.Role != types.AdminRole && actorWm.Workspace.OwnerId != actor.ID {
				return apierrors.ErrCannotDeleteWorkspaceAdmin
			}
		}
	}

	if requestedMember.Project.ProjectLeadId == requestedMember.MemberId {
		if !actor.Member.IsSuperuser {
			return apierrors.ErrCannotRemoveProjectLead
		}
		if err := b.db.Transaction(func(tx *gorm.DB) error {
			var member dao.ProjectMember
			if err := tx.
				Table("project_members AS pm").
				Joins("JOIN users AS u ON u.id = pm.member_id").
				Where("pm.project_id = ?", actor.ProjectId).
				Where("u.id <> ?", requestedMember.Project.ProjectLeadId).
				Order("pm.role DESC, u.last_active DESC").
				Preload("Member").
				First(&member).Error; err != nil {
				return err
			}

			err := requestedMember.Project.ChangeLead(tx, &member)
			if err != nil {
				return apierrors.ErrChangeProjectLead
			}

			return nil
		}); err != nil {
			if err == gorm.ErrRecordNotFound {
				return b.DeleteProject()
			} else {
				return err
			}
		}
	}

	if actor.MemberId == requestedMember.MemberId && !actor.Member.IsSuperuser {
		return apierrors.ErrCannotRemoveSelfFromProject
	}

	// One cannot remove role higher than his own role
	if actor.Role < requestedMember.Role && !actor.Member.IsSuperuser {
		return apierrors.ErrCannotRemoveHigherRoleUserProject
	} else if requestedMember.Member.IsSuperuser && actor.ID != requestedMember.ID {
		return apierrors.ErrDeleteSuperUser
	}

	// Remove all favorites
	if err := b.db.Exec("delete from project_favorites where user_id = ? and project_id = ?",
		requestedMember.MemberId, requestedMember.ProjectId).Error; err != nil {
		return err
	}

	// Also remove issue from issue assigned
	if err := b.db.Exec("delete from issue_assignees where assignee_id = ? and project_id = ?",
		requestedMember.MemberId, requestedMember.ProjectId).Error; err != nil {
		return err
	}

	// Also remove issue from issue watcher
	if err := b.db.Exec("delete from issue_watchers where watcher_id = ? and project_id = ?",
		requestedMember.MemberId, requestedMember.ProjectId).Error; err != nil {
		return err
	}

	data := map[string]interface{}{
		"updateScopeId": requestedMember.MemberId,
	}

	if err := b.db.Transaction(func(tx *gorm.DB) error {
		err := tracker.TrackActivity[dao.ProjectMember, dao.ProjectActivity](b.tracker, tracker.ENTITY_REMOVE_ACTIVITY, data, nil, *requestedMember, actor.Member)
		if err != nil {
			errStack.GetError(nil, err)
			return err
		}

		return b.db.Delete(&requestedMember).Error
	}); err != nil {
		return err
	}

	return nil
}
