// Управление базой данных при импорте данных в систему. Содержит функции для создания различных сущностей (пользователей, проектов, задач и т.д.) с использованием GORM и обработки конфликтов при создании.
package db

import (
	"fmt"
	"slices"

	"github.com/aisa-it/aiplan/internal/aiplan/types"

	"github.com/aisa-it/aiplan/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/internal/aiplan/issues-import/context"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var dbSteps []func(*context.ImportContext, *gorm.DB) error = []func(*context.ImportContext, *gorm.DB) error{
	CreateAvatars,
	CreateUser,
	CreateWorkspaceMembers,
	CreateProject,
	CreateProjectMembers,
	CreateStates,
	CreateLabels,
	CreateIssues,
	CreateLinkedIssues,
	CreateLinks,
	CreateComments,
	CreateIssueLabelsLinks,
	CreateBlockers,
	CreateAssignees,
	CreateWatchers,
	CreateFileAssets,
	CreateAttachments,
}

func RunDBSteps(context *context.ImportContext, db *gorm.DB) error {
	context.Log.Info("Save project to workspace")
	context.Stage = "db"
	context.Counters.TotalDBStages = len(dbSteps)
	return db.Transaction(func(tx *gorm.DB) error {
		for _, fn := range dbSteps {
			if context.Finished {
				return nil
			}

			if err := fn(context, tx); err != nil {
				return err
			}
			context.Counters.CurrentDBStage++
		}
		return nil
	})
}

func CreateAvatars(c *context.ImportContext, tx *gorm.DB) error {
	if !c.IgnoreAttachments {
		c.Log.Info("Create avatars", "count", c.Attachments.Len())
		if err := tx.CreateInBatches(c.AvatarAssets, 100).Error; err != nil {
			return err
		}
	}
	return nil
}

func CreateUser(context *context.ImportContext, tx *gorm.DB) error {
	context.Log.Info("Create users", "count", len(context.UsersToCreate))
	for k, u := range context.UsersToCreate {
		pass := dao.GenPassword()
		u.Password = dao.GenPasswordHash(pass)
		u.Theme = types.DefaultTheme
		if err := tx.Save(&u).Error; err != nil {
			if err == gorm.ErrDuplicatedKey {
				context.Log.Warn("Duplicated user", "id", k)
				continue
			}
			return err
		}
		u.Password = pass
		context.UsersToCreate[k] = u
	}
	return nil
}

func CreateWorkspaceMembers(context *context.ImportContext, tx *gorm.DB) error {
	context.Log.Info("Create workspace members", "count", len(context.WorkspaceMembers))
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(context.WorkspaceMembers, 100).Error; err != nil {
		for _, m := range context.WorkspaceMembers {
			fmt.Printf("%+v\n", m)
		}
		return err
	}
	return nil
}

func CreateProject(context *context.ImportContext, tx *gorm.DB) error {
	context.Log.Info("Create project")
	return tx.Create(&context.Project).Error
}

func CreateProjectMembers(context *context.ImportContext, tx *gorm.DB) error {
	context.Log.Info("Create project members", "count", len(context.ProjectMembers))
	return tx.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(context.ProjectMembers, 100).Error
}

func CreateStates(context *context.ImportContext, tx *gorm.DB) error {
	context.Log.Info("Create states", "count", context.States.Len())
	uniqName := make(map[string]struct{}, context.States.Len())
	context.States.Range(func(k string, v dao.State) {
		if _, ok := uniqName[v.Name]; ok {
			v.Color = "#199600"
			context.States.PutNoLock(k, v)
		} else {
			uniqName[v.Name] = struct{}{}
		}
	})

	return tx.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(context.States.Array(), 100).Error
}

func CreateLabels(context *context.ImportContext, tx *gorm.DB) error {
	context.Log.Info("Create labels", "count", context.Labels.Len())

	context.ReleasesTags.Range(func(key string, value dao.Label) {
		context.Labels.Put(value.ID, value)
	})

	return tx.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(context.Labels.Array(), 100).Error
}

func CreateIssues(context *context.ImportContext, tx *gorm.DB) error {
	issueArr := context.Issues.Array()

	slices.SortFunc(issueArr, func(a dao.Issue, b dao.Issue) int {
		if !a.ParentId.Valid {
			return -1
		} else if !b.ParentId.Valid {
			return 1
		}

		if a.ParentId.UUID == b.ID {
			return 1
		} else if b.ParentId.UUID == a.ID {
			return -1
		}

		return a.SequenceId - b.SequenceId
	})

	context.Log.Info("Create issues", "count", len(issueArr))
	for _, i := range issueArr {
		if err := tx.Create(&i).Error; err != nil {
			fmt.Println(i.SequenceId, i.ParentId)
			return err
		}
	}

	return nil
}

func CreateLinkedIssues(context *context.ImportContext, tx *gorm.DB) error {
	context.Log.Info("Create linked issues", "count", len(context.LinkedIssues))
	return tx.CreateInBatches(context.LinkedIssues, 100).Error
}

func CreateLinks(context *context.ImportContext, tx *gorm.DB) error {
	context.Log.Info("Create links", "count", context.IssueLinks.Len())
	return tx.CreateInBatches(context.IssueLinks.Array(), 100).Error
}

func CreateComments(context *context.ImportContext, tx *gorm.DB) error {
	context.Log.Info("Create comments", "count", context.IssueComments.Len())
	return tx.CreateInBatches(context.IssueComments.ValueArray(), 100).Error
}

func CreateIssueLabelsLinks(context *context.ImportContext, tx *gorm.DB) error {
	context.Log.Info("Create issue labels links", "count", context.IssueLabels.Len())
	return tx.CreateInBatches(context.IssueLabels.Array(), 100).Error
}

func CreateBlockers(context *context.ImportContext, tx *gorm.DB) error {
	context.Log.Info("Create blockers", "count", len(context.Blockers.ValueArray()))
	for _, v := range context.Blockers.ValueArray() {
		if err := tx.Save(v).Error; err != nil {
			return err
		}
	}
	return nil
}

func CreateAssignees(context *context.ImportContext, tx *gorm.DB) error {
	context.Log.Info("Create issue assignees", "count", context.IssueAssignees.Len())
	return tx.CreateInBatches(context.IssueAssignees.ValueArray(), 100).Error
}

func CreateWatchers(context *context.ImportContext, tx *gorm.DB) error {
	context.Log.Info("Create issue watchers", "count", context.IssueWatchers.Len())
	return tx.CreateInBatches(context.IssueWatchers.ValueArray(), 100).Error
}

func CreateFileAssets(context *context.ImportContext, tx *gorm.DB) error {
	if context.IgnoreAttachments {
		return nil
	}
	context.Log.Info("Create file assets", "count", context.FileAssets.Len())
	return tx.CreateInBatches(context.FileAssets.Array(), 200).Error
}

func CreateAttachments(context *context.ImportContext, tx *gorm.DB) error {
	if context.IgnoreAttachments {
		return nil
	}
	context.Log.Info("Create attachments", "count", context.Attachments.Len())
	for _, attachment := range context.Attachments.Array() {
		if attachment.IssueAttachment != nil {
			if err := tx.Create(attachment.IssueAttachment).Error; err != nil {
				return err
			}
		}
	}
	return nil
}
