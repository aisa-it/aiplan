package business

import (
	"strings"

	tracker "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/activity-tracker"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/apierrors"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	errStack "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/stack-error"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (b *Business) DeleteWorkspace(user *dao.User, workspace *dao.Workspace) error {
	if !user.IsSuperuser && user.ID != workspace.OwnerId {
		return apierrors.ErrDeleteWorkspaceForbidden
	}

	err := tracker.TrackActivity[dao.Workspace, dao.RootActivity](b.tracker, activities.EntityDeleteActivity, nil, nil, *workspace, user)
	if err != nil {
		errStack.GetError(nil, err)
		return err
	}

	{
		// delete DeferredNotifications & activities
		if err := b.db.
			Where("workspace_id = ?", workspace.ID).
			Unscoped().
			Delete(&dao.DeferredNotifications{}).Error; err != nil {
			return err
		}

		activityTables := []dao.UnionableTable{
			&dao.WorkspaceActivity{},
			&dao.DocActivity{},
			&dao.FormActivity{},
			&dao.ProjectActivity{},
			&dao.IssueActivity{},
		}

		q := utils.SliceToSlice(&activityTables, func(a *dao.UnionableTable) string {
			tn := strings.Split((*a).TableName(), "_")
			return tn[0] + "_activity_id"
		})

		queryString := strings.Join(q, " IN (?) OR ") + " IN (?)"

		var queries []interface{}

		for _, model := range activityTables {
			queries = append(queries, b.db.Select("id").
				Where("workspace_id = ?", workspace.ID).
				Model(&model))
		}

		if err := b.db.Where(queryString, queries...).
			Unscoped().Delete(&dao.UserNotifications{}).Error; err != nil {
			return err
		}

		for _, model := range activityTables {
			if err := b.db.
				Where("workspace_id = ?", workspace.ID).
				Unscoped().
				Delete(model).Error; err != nil {
				return err
			}
		}

		cleanId := map[string]interface{}{"new_identifier": nil, "old_identifier": nil}
		if err := b.db.Model(&dao.RootActivity{}).Where("new_identifier = ? OR old_identifier = ?", workspace.ID, workspace.ID).Updates(cleanId).Error; err != nil {
			return err
		}
	}

	// Soft-delete projects
	if err := b.db.Session(&gorm.Session{SkipHooks: true}).Omit(clause.Associations).Where("workspace_id = ?", workspace.ID).Delete(&dao.Project{}).Error; err != nil {
		return err
	}

	// Soft-delete workspace
	if err := b.db.Session(&gorm.Session{SkipHooks: true}).Omit(clause.Associations).Delete(workspace).Error; err != nil {
		return err
	}

	// Soft-delete issues
	if err := b.db.Session(&gorm.Session{SkipHooks: true}).Omit(clause.Associations).Where("workspace_id = ?", workspace.ID).Delete(&dao.Issue{}).Error; err != nil {
		return err
	}

	/*// Start hard deleting in foreground
	go func(workspace dao.Workspace) {
		if err := b.db.Unscoped().Omit(clause.Associations).Delete(&workspace).Error; err != nil {
			slog.Error("Hard delete workspace", "workspaceId", workspace.ID, "err", err)
		}
	}(*workspace)
	*/

	return nil
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

	actorMap := utils.SliceToMap(&actorProjectMembers, func(v *dao.ProjectMember) uuid.UUID {
		return v.ProjectId
	})

	for _, member := range projectMembers {
		actorPM := actorMap[member.ProjectId]

		if err := b.DeleteProjectMember(&actorPM, &member, actor.Member, member.Project, actor); err != nil {
			return err
		}
	}

	data := map[string]interface{}{
		"updateScopeId": requestedMember.MemberId,
	}

	if err := b.db.Transaction(func(tx *gorm.DB) error {
		err := tracker.TrackActivity[dao.WorkspaceMember, dao.WorkspaceActivity](
			b.tracker, activities.EntityRemoveActivity, data, nil, *requestedMember, actor.Member)
		if err != nil {
			return err
		}

		return tx.Delete(requestedMember).Error
	}); err != nil {
		return err
	}

	return nil
}
