package email

import (
	"database/sql"
	"fmt"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	member_role "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications/member-role"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

type ProjectProcessor struct {
	*BaseProcessor
	plan *emailPlan
}

func NewProjectPipeline() (types.EntityLayer, EmailProcessor) {
	layer := types.LayerProject
	return layer, &ProjectProcessor{
		BaseProcessor: NewBaseProcessor(),
		plan: &emailPlan{
			AuthorRole: member_role.ActionAuthor,
			EntityType: layer,
		},
	}
}

var projectFieldConfigs = map[actField.ActivityField]EntityFieldConfig{
	actField.Name.Field:             {collectOne, createFieldRenderer("Имя", StringField)},
	actField.Identifier.Field:       {collectOne, createFieldRenderer("Идентификатор", StringField)},
	actField.Emoj.Field:             {collectOne, createFieldRenderer("Емоджи", EmojiField)},
	actField.Logo.Field:             {collectOne, createFieldRenderer("Логотип", StringField, WithCustomText("изменен логотип проекта"))},
	actField.Public.Field:           {collectOne, createFieldRenderer("Публичность", StringField, WithTranslation(types.ProjectPublicTranslate))},
	actField.DefaultWatchers.Field:  {collectAll, renderProjectDefaultWatchers},
	actField.DefaultAssignees.Field: {collectAll, renderProjectDefaultAssignees},
	actField.ProjectLead.Field:      {collectOne, renderProjectLead},
	actField.Member.Field:           {collectAll, renderProjectMember},
	actField.Role.Field:             {collectOne, renderProjectMemberRole},
	actField.Issue.Field:            {collectAll, renderProjectIssue},
}

func (p ProjectProcessor) LoadActivities(tx *gorm.DB) []dao.ActivityEvent {
	var activities []dao.ActivityEvent
	if err := tx.Unscoped().
		Joins("Project").
		Joins("Workspace").
		Joins("Actor").
		Order("activity_events.created_at").
		Where("activity_events.entity_type = ?", types.LayerProject).
		Where("activity_events.notified = ?", false).
		Limit(100).
		Find(&activities).Error; err != nil {
		return []dao.ActivityEvent{}
	}
	return activities
}

func (p ProjectProcessor) FullLoad(tx *gorm.DB, entity dao.IDaoAct) dao.IDaoAct {
	project := entity.(*dao.Project)
	if err := tx.Unscoped().
		Preload("DefaultAssigneesDetails", "is_default_assignee = ?", true).
		Preload("DefaultWatchersDetails", "is_default_watcher = ?", true).
		First(&project).Error; err != nil {
		return entity
	}
	return project
}

func (p ProjectProcessor) GroupActivities(acts []dao.ActivityEvent) ActivityBuckets {
	return GroupActivitiesByLayer(
		acts,
		func(a dao.ActivityEvent) dao.IDaoAct {
			a.Project.Workspace = a.Workspace
			a.Project.SetUrl()
			return a.Project
		},
	)
}

func issueMemberStep(act dao.ActivityEvent) []member_role.UsersStep {
	return []member_role.UsersStep{
		member_role.AddIssueUsers(act.NewIssue, member_role.WithActivityId(act.ID)),
		member_role.AddDefaultWatchers(act.ProjectID.UUID, member_role.WithActivityId(act.ID)),
	}
}

func (p ProjectProcessor) BuildRecipients(tx *gorm.DB, acts []dao.ActivityEvent, entity dao.IDaoAct) ([]member_role.MemberNotify, EmailContext) {
	steps := []member_role.UsersStep{
		member_role.AddProjectAdmin(entity.GetId()),
	}

	lll := []func(act dao.ActivityEvent) []member_role.UsersStep{
		func(act dao.ActivityEvent) []member_role.UsersStep {
			if act.Field == actField.Issue.Field && act.Verb == actField.VerbCreated && act.NewIssue != nil {
				var issue dao.Issue
				if act.ProjectActivityExtendFields.NewIssue != nil {
					issue = *act.ProjectActivityExtendFields.NewIssue
					if err := tx.Unscoped().
						Joins("Author").
						Preload("Assignees").
						Preload("Watchers").
						First(&issue).Error; err != nil {
						return nil
					}
				}
				issue.Project = act.Project
				*act.ProjectActivityExtendFields.NewIssue = issue
				return issueMemberStep(act)
			}
			return []member_role.UsersStep{}
		},
	}

	ctx := EmailContext{
		Plan:           p.plan,
		Settings:       member_role.FromProject(),
		Steps:          steps,
		CustomRoleFunc: lll,
	}

	return BuildRecipientsFromActivities(tx, acts, &ctx)
}

func (p ProjectProcessor) BuildDigest(tx *gorm.DB, templates *EmailTemplates, acts []dao.ActivityEvent, entity dao.IDaoAct) (map[string]FieldPrerender, int) {
	return renderDigest(tx, templates, acts, entity, projectFieldConfigs)
}

func (p *ProjectProcessor) BuildSubject(entity dao.IDaoAct) string {
	project, ok := entity.(*dao.Project)
	if !ok || project == nil {
		return ""
	}
	return fmt.Sprintf("Обновления проекта %s: (%s)", project.GetString(), project.Name)
}

func (p *ProjectProcessor) BuildHead(templates *EmailTemplates, entity dao.IDaoAct) string {
	project, ok := entity.(*dao.Project)
	if !ok || project == nil {
		return ""
	}
	head := headEntityCtx{
		WorkspaceName: project.Workspace.Slug,
		Layer:         "проект",
		Identifier:    fmt.Sprint(project.Identifier),
		Title:         project.Name,
		Url:           project.URL.String(),
		UrlText:       "Посмотреть проект",
	}
	return templates.RenderHead(head)
}

func renderProjectDefaultWatchers(tx *gorm.DB, t *EmailTemplates, acts []dao.ActivityEvent, entity dao.IDaoAct) FieldPrerender {
	project, ok := entity.(*dao.Project)
	if !ok || project == nil {
		return FieldPrerender{}
	}
	return renderEntityChange(tx, t, acts,
		utils.SliceToSlice(&project.DefaultWatchersDetails, func(t *dao.ProjectMember) dao.User {
			return *t.Member
		}),
		"Наблюдатели по умолчанию",
		entitySpec[dao.User]{
			entityID: func(event dao.ActivityEvent) uuid.UUID {
				return getUUIDFromActivity(uuidPtrFrom(event.NewDefaultWatcher), uuidPtrFrom(event.OldDefaultWatcher))
			},
			entityTitle: func(i dao.User) string { return i.GetName() },
			loadRemoved: getRemovedMembers,
		})
}

func renderProjectDefaultAssignees(tx *gorm.DB, t *EmailTemplates, acts []dao.ActivityEvent, entity dao.IDaoAct) FieldPrerender {
	project, ok := entity.(*dao.Project)
	if !ok || project == nil {
		return FieldPrerender{}
	}
	return renderEntityChange(tx, t, acts,
		utils.SliceToSlice(&project.DefaultAssigneesDetails, func(t *dao.ProjectMember) dao.User {
			return *t.Member
		}),
		"Исполнители по умолчанию",
		entitySpec[dao.User]{
			entityID: func(event dao.ActivityEvent) uuid.UUID {
				return getUUIDFromActivity(uuidPtrFrom(event.NewDefaultAssignee), uuidPtrFrom(event.OldDefaultAssignee))
			},
			entityTitle: func(i dao.User) string { return i.GetName() },
			loadRemoved: getRemovedMembers,
		})
}

func renderProjectLead(_ *gorm.DB, t *EmailTemplates, acts []dao.ActivityEvent, _ dao.IDaoAct) FieldPrerender {
	fp := t.RenderCollectOne(collectOneCtx{
		Key:    "Лидер проекта",
		New:    toValueCtx(nil, utils.ToPtr(acts[0].NewProjectLead.GetName())),
		Old:    toValueCtx(nil, utils.ToPtr(acts[0].OldProjectLead.GetName())),
		Start:  sql.NullTime{Time: acts[0].CreatedAt, Valid: true},
		Author: *acts[0].Actor,
	})
	fp.Verb = acts[0].Verb
	return fp
}

func renderProjectMemberRole(tx *gorm.DB, t *EmailTemplates, acts []dao.ActivityEvent, _ dao.IDaoAct) FieldPrerender {
	return renderEntityChangeComplex(tx, t, acts, "Роль участника",
		WithTitleFunc(func(act *dao.ActivityEvent) *string {
			return utils.ToPtr(act.ProjectMemberExtendFields.NewRole.GetName())
		}),
		WithReplaceFunc(func(str *string) *string {
			if str == nil {
				return nil
			}
			return utils.ToPtr(types.RoleTranslation[*str])
		}),
	)
}
func renderProjectIssue(tx *gorm.DB, t *EmailTemplates, acts []dao.ActivityEvent, entity dao.IDaoAct) FieldPrerender {
	project, ok := entity.(*dao.Project)
	if !ok || project == nil {
		return FieldPrerender{}
	}

	return renderEntityChangeComplex(tx, t, acts, "Задачи",
		WithTitleFunc(func(act *dao.ActivityEvent) *string {
			var issue *dao.Issue
			if act.ProjectActivityExtendFields.NewIssue != nil {
				issue = act.ProjectActivityExtendFields.NewIssue
				if err := tx.Unscoped().
					Joins("Parent").
					Joins("State").
					Joins("Author").
					Preload("Assignees").
					Preload("Watchers").
					Preload("Sprints").
					First(issue).Error; err != nil {
					return nil
				}
				issue.Project = project
				act.ProjectActivityExtendFields.NewIssue = issue
			}
			if issue == nil {
				return utils.ToPtr(act.OldValue)
			}

			return utils.ToPtr(issue.FullIssueName())
		}),
		WithComplexAggregateFunc(issueCreateFunc),
	)
}

func renderProjectMember(tx *gorm.DB, t *EmailTemplates, acts []dao.ActivityEvent, entity dao.IDaoAct) FieldPrerender {
	project, ok := entity.(*dao.Project)
	if !ok || project == nil {
		return FieldPrerender{}
	}

	var members []dao.ProjectMember
	if err := tx.Preload("Member").Where("project_id = ?", project.ID).Find(&members).Error; err != nil {
		return FieldPrerender{}
	}

	memberMap := utils.SliceToMap(&members, func(v *dao.ProjectMember) uuid.UUID {
		return v.MemberId
	})

	return renderEntityChange(tx, t, acts,
		utils.SliceToSlice(&members, func(t *dao.ProjectMember) dao.User {
			return *t.Member
		}),
		"Участники",
		entitySpec[dao.User]{
			entityID: func(event dao.ActivityEvent) uuid.UUID {
				return getUUIDFromActivity(uuidPtrFrom(event.ProjectMemberExtendFields.NewMember), uuidPtrFrom(event.ProjectMemberExtendFields.OldMember))
			},
			entityTitle: func(i dao.User) string {
				return fmt.Sprintf("%s (%s)", i.GetName(), types.TranslateMap(types.RoleTranslation, utils.ToPtr(fmt.Sprint(memberMap[i.GetId()].Role))))
			},
			loadRemoved: getRemovedMembers,
		})
}
