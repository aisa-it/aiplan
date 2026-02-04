package email

import (
	"fmt"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	member_role "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications/member-role"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

func loadSprintActivities(tx *gorm.DB) []dao.SprintActivity {
	var activities []dao.SprintActivity
	if err := tx.Unscoped().
		Joins("Sprint.CreatedBy").
		Preload("Sprint.Watchers").
		Preload("Sprint.Issues.Project").
		Joins("Actor").
		Joins("Workspace").
		Order("created_at").
		Where("notified = ?", false).
		Limit(100).
		Find(&activities).Error; err != nil {
		//slog.Error("Get activities", slog.Int("interval", e.service.cfg.NotificationsSleep), "err", err)
		return []dao.SprintActivity{}
	}
	return activities
}

// pipeline
func NewSprintPipeline(templates *EmailTemplates) LayerPipeline[dao.SprintActivity, *dao.Sprint] {

	plan := &emailPlan{
		TableName:  dao.SprintActivity{}.TableName(),
		AuthorRole: member_role.ActionAuthor,
		Entity:     actField.Sprint.Field,
	}

	return LayerPipeline[dao.SprintActivity, *dao.Sprint]{
		Plan:  plan,
		Load:  loadSprintActivities,
		Group: groupSprint,

		BuildRecipients: func(tx *gorm.DB, acts []dao.SprintActivity, sprint *dao.Sprint) ([]member_role.MemberNotify, EmailContext) {
			steps := []member_role.UsersStep{
				member_role.AddWorkspaceAdmins(sprint.WorkspaceId),
			}

			ctx := EmailContext{
				Plan:     plan,
				Settings: member_role.FromWorkspace(sprint.WorkspaceId),
				Steps:    steps,
			}

			return BuildRecipientsFromActivities(tx, acts, getSprintActivityAuthor, &ctx)
		},

		BuildDigest: func(tx *gorm.DB, acts []dao.SprintActivity, sprint *dao.Sprint) (map[string]fieldPrerender, int) {
			return renderDigest(tx, templates, acts, sprint, sprintFieldRenderMap, sprintCollectors)
		},

		Subject: func(s *dao.Sprint) string {
			return fmt.Sprintf("Обновления спринта %s", s.GetFullName())
		},

		FilterEmpty: true,
	}
}

func groupSprint(acts []dao.SprintActivity) ActivityBuckets[dao.SprintActivity, *dao.Sprint] {
	return GroupActivitiesByLayer(
		acts,
		func(a dao.SprintActivity) uuid.UUID { return a.SprintId },
		func(a dao.SprintActivity) *dao.Sprint { return a.Sprint },
	)
}

func getSprintActivityAuthor(act dao.SprintActivity) *dao.User {
	return act.Actor
}

// collectors
var sprintCollectors = map[actField.ActivityField]activityFieldCollector[dao.SprintActivity]{
	actField.Watchers.Field:    collectAll[dao.SprintActivity],
	actField.Issue.Field:       collectAll[dao.SprintActivity],
	actField.Description.Field: collectOne[dao.SprintActivity],
	actField.Name.Field:        collectOne[dao.SprintActivity],
	actField.StartDate.Field:   collectOne[dao.SprintActivity],
	actField.EndDate.Field:     collectOne[dao.SprintActivity],
}

var sprintFieldRenderMap = map[actField.ActivityField]FuncFieldRender[dao.SprintActivity, *dao.Sprint]{
	actField.Issue.Field:    renderSprintIssue,
	actField.Name.Field:     renderSprintName,
	actField.Watchers.Field: renderSprintWatcher,
}

func renderSprintIssue(tx *gorm.DB, t *EmailTemplates, acts []dao.SprintActivity, sprint *dao.Sprint) (string, int) {
	spec := entitySpec[dao.SprintActivity, dao.Issue]{
		entityID:    getIssueIdFromSprintActivity,
		entityTitle: func(i dao.Issue) string { return i.FullIssueName() },
		loadRemoved: getRemovedIssues,
	}

	views, count := BuildEntityChangeDigest(tx, acts, sprint.Issues, spec)

	ctx := collectAllCtx{
		Key:   "Задачи",
		Views: views,
	}

	return t.RenderCollectAll(ctx, count)
}

func renderSprintWatcher(tx *gorm.DB, t *EmailTemplates, acts []dao.SprintActivity, sprint *dao.Sprint) (string, int) {
	spec := entitySpec[dao.SprintActivity, dao.User]{
		entityID:    getWatcherIdFromSprintActivity,
		entityTitle: func(i dao.User) string { return i.GetName() },
		loadRemoved: getRemovedMember,
	}

	views, count := BuildEntityChangeDigest(tx, acts, sprint.Watchers, spec)

	ctx := collectAllCtx{
		Key:   "Наблюдатели",
		Views: views,
	}

	return t.RenderCollectAll(ctx, count)
}

func renderSprintName(_ *gorm.DB, t *EmailTemplates, acts []dao.SprintActivity, _ *dao.Sprint) (string, int) {
	return t.RenderCollectOne(collectOneCtx{
		Key: "Название",
		New: toValueCtx(&acts[0].NewValue, nil),
		Old: toValueCtx(acts[0].OldValue, nil),
	})
}

func getIssueIdFromSprintActivity(a dao.SprintActivity) uuid.UUID {
	switch a.Verb {
	case actField.VerbAdded:
		return a.NewSprintIssue.ID
	case actField.VerbRemoved:
		return a.OldSprintIssue.ID
	default:
		return uuid.Nil
	}
}

func getWatcherIdFromSprintActivity(a dao.SprintActivity) uuid.UUID {
	switch a.Verb {
	case actField.VerbAdded:
		return a.NewSprintWatcher.ID
	case actField.VerbRemoved:
		return a.OldSprintWatcher.ID
	default:
		return uuid.Nil
	}
}
