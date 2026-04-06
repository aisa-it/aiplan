package business

import (
	"log/slog"

	tracker "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/activity-tracker"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/apierrors"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	errStack "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/stack-error"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (b *Business) DeleteProject(user *dao.User, project *dao.Project, workspaceMember *dao.WorkspaceMember) error {
	if !user.IsSuperuser && user.ID != project.ProjectLeadId && workspaceMember.Role != types.AdminRole {
		return apierrors.ErrDeleteProjectForbidden
	}

	err := tracker.TrackEvent(b.ta, types.LayerWorkspace, activities.VerbDeleted, nil, *project, user)
	if err != nil {
		errStack.GetError(nil, err)
		return err
	}

	// Soft-delete project
	{
		// delete DeferredNotifications & activities
		if err := b.db.
			Where("project_id = ?", project.ID).
			Unscoped().
			Delete(&dao.DeferredNotifications{}).Error; err != nil {
			return err
		}

		query := b.db.
			Where("entity_type IN (?)", []types.EntityLayer{types.LayerProject, types.LayerIssue}).
			Where("project_id = ?", project.ID)

		if err := dao.CleanupActivityData(b.db, query, project.ID, types.LayerWorkspace); err != nil {
			return err
		}

	}

	if err := b.db.Session(&gorm.Session{SkipHooks: true}).Omit(clause.Associations).Delete(&project).Error; err != nil {
		return err
	}

	if err := b.db.Session(&gorm.Session{SkipHooks: true}).Omit(clause.Associations).Where("project_id = ?", project.ID).Delete(&dao.Issue{}).Error; err != nil {
		return err
	}

	// Start hard deleting in foreground
	go func(project dao.Project) {
		if err := b.db.Unscoped().Delete(&project).Error; err != nil {
			slog.Error("Hard delete project", "projectId", project.ID, "err", err)
		}
	}(*project)

	return nil
}

func (b *Business) DeleteProjectMember(actor *dao.ProjectMember, requestedMember *dao.ProjectMember, user *dao.User, project *dao.Project, workspaceMember *dao.WorkspaceMember) error {
	var isWorkspaceAdmin bool

	if actor.MemberId != requestedMember.MemberId {
		if err := b.db.Model(&dao.WorkspaceMember{}).
			Select("EXISTS(?)",
				b.db.Model(&dao.WorkspaceMember{}).
					Select("1").
					Where("role = ?", types.AdminRole).
					Where("workspace_id = ?", actor.WorkspaceId).
					Where("member_id = ?", requestedMember.MemberId),
			).
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
			if actorWm.Role != types.AdminRole && actorWm.Workspace.OwnerId != actor.MemberId {
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
				return b.DeleteProject(user, project, workspaceMember)
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
		err := tracker.TrackEvent(b.ta, types.LayerProject, activities.VerbRemoved, tracker.NewTrackerCtx(&data, nil), *requestedMember, actor.Member)
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
