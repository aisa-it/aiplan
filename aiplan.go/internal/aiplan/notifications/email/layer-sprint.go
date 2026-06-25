package email

import (
	"fmt"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	member_role "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications/member-role"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

type SprintProcessor struct {
	*BaseProcessor
	plan *emailPlan
}

var sprintFieldConfigs = map[actField.ActivityField]EntityFieldConfig{
	actField.Watchers.Field:    {collectAll, renderSprintWatchers},
	actField.Issue.Field:       {collectAll, renderSprintIssue},
	actField.Description.Field: {collectOne, createFieldRenderer("Описание", BodyField)},
	actField.Name.Field:        {collectOne, createFieldRenderer("Название", StringField)},
	actField.StartDate.Field:   {collectOne, createTargetDateZRender("Начало", targetDateZ)},
	actField.EndDate.Field:     {collectOne, createTargetDateZRender("Конец", targetDateZ)},
}

func NewSprintPipeline() (types.EntityLayer, EmailProcessor) {
	layer := types.LayerSprint
	return layer, &SprintProcessor{
		BaseProcessor: NewBaseProcessor(),
		plan: &emailPlan{
			AuthorRole: member_role.ActionAuthor,
			EntityType: layer,
		},
	}
}

func (sp *SprintProcessor) LoadActivities(tx *gorm.DB) []dao.ActivityEvent {
	var activities []dao.ActivityEvent
	if err := tx.Unscoped().
		Joins("Sprint.CreatedBy").
		Preload("Sprint.Watchers").
		Preload("Sprint.Issues.Project").
		Joins("Actor").
		Joins("Workspace").
		Order("created_at").
		Where("notified = ?", false).
		Where("entity_type = ?", types.LayerSprint).
		Limit(100).
		Find(&activities).Error; err != nil {
		return []dao.ActivityEvent{}
	}
	return activities
}

func (sp *SprintProcessor) GroupActivities(acts []dao.ActivityEvent) ActivityBuckets {
	return GroupActivitiesByLayer(
		acts,
		func(a dao.ActivityEvent) dao.IDaoAct {
			a.Sprint.Workspace = a.Workspace
			a.Sprint.SetUrl()
			return a.Sprint
		},
	)
}

func (sp *SprintProcessor) BuildRecipients(tx *gorm.DB, acts []dao.ActivityEvent, entity dao.IDaoAct) ([]member_role.MemberNotify, EmailContext) {
	sprint, ok := entity.(*dao.Sprint)
	if !ok || sprint == nil {
		return []member_role.MemberNotify{}, EmailContext{}
	}
	steps := []member_role.UsersStep{
		member_role.AddWorkspaceAdmins(sprint.WorkspaceId),
	}

	ctx := EmailContext{
		Plan:     sp.plan,
		Settings: member_role.FromWorkspace(),
		Steps:    steps,
	}

	return BuildRecipientsFromActivities(tx, acts, &ctx)
}

func (sp *SprintProcessor) BuildDigest(tx *gorm.DB, templates *EmailTemplates, acts []dao.ActivityEvent, entity dao.IDaoAct) (map[string]FieldPrerender, int) {
	return renderDigest(tx, templates, acts, entity, sprintFieldConfigs)
}

func (sp *SprintProcessor) BuildSubject(entity dao.IDaoAct) string {
	sprint, ok := entity.(*dao.Sprint)
	if !ok || sprint == nil {
		return ""
	}
	return fmt.Sprintf("Обновления спринта %s", sprint.GetFullName())
}

func (sp *SprintProcessor) BuildHead(templates *EmailTemplates, entity dao.IDaoAct) string {
	sprint, ok := entity.(*dao.Sprint)
	if !ok || sprint == nil {
		return ""
	}
	head := headEntityCtx{
		WorkspaceName: sprint.Workspace.Slug,
		Layer:         "спринт",
		Identifier:    fmt.Sprint(sprint.SequenceId),
		Title:         sprint.GetFullName(),
		Url:           sprint.URL.String(),
		UrlText:       "Посмотреть спринт",
	}
	return templates.RenderHead(head)
}

func getIssueIdFromSprintActivity(a dao.ActivityEvent) uuid.UUID {
	return getUUIDFromActivity(uuidPtrFrom(a.SprintIssuesExtendFields.NewSprintIssue), uuidPtrFrom(a.SprintIssuesExtendFields.OldSprintIssue))
}

func getWatcherIdFromSprintActivity(a dao.ActivityEvent) uuid.UUID {
	return getUUIDFromActivity(uuidPtrFrom(a.NewSprintWatcher), uuidPtrFrom(a.OldSprintWatcher))
}

func renderSprintIssue(tx *gorm.DB, t *EmailTemplates, acts []dao.ActivityEvent, entity dao.IDaoAct) FieldPrerender {
	sprint, ok := entity.(*dao.Sprint)
	if !ok || sprint == nil {
		return FieldPrerender{}
	}

	return renderEntityChange(tx, t, acts, sprint.Issues,
		"Задачи",
		entitySpec[dao.Issue]{
			entityID:    getIssueIdFromSprintActivity,
			entityTitle: func(i dao.Issue) string { return i.FullIssueName() },
			loadRemoved: getRemovedIssues,
		},
	)
}

func renderSprintWatchers(tx *gorm.DB, t *EmailTemplates, acts []dao.ActivityEvent, entity dao.IDaoAct) FieldPrerender {
	sprint, ok := entity.(*dao.Sprint)
	if !ok || sprint == nil {
		return FieldPrerender{}
	}
	return renderEntityChange(tx, t, acts, sprint.Watchers,
		"Наблюдатели",
		entitySpec[dao.User]{
			entityID:    getWatcherIdFromSprintActivity,
			entityTitle: func(i dao.User) string { return i.GetName() },
			loadRemoved: getRemovedMembers,
		})
}
