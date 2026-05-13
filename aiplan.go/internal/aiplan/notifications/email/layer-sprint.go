package email

import (
	"database/sql"
	"fmt"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	member_role "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications/member-role"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

// pipeline
func NewSprintPipeline(templates *EmailTemplates) LayerPipeline[*dao.Sprint] {
	plan := &emailPlan{
		AuthorRole: member_role.ActionAuthor,
		EntityType: types.LayerSprint,
	}

	return LayerPipeline[*dao.Sprint]{
		Plan:  plan,
		Load:  loadSprintActivities,
		Group: groupSprint,

		BuildRecipients: func(tx *gorm.DB, acts []dao.ActivityEvent, sprint *dao.Sprint) ([]member_role.MemberNotify, EmailContext) {
			steps := []member_role.UsersStep{
				member_role.AddWorkspaceAdmins(sprint.WorkspaceId),
			}

			ctx := EmailContext{
				Plan:     plan,
				Settings: member_role.FromWorkspace(),
				Steps:    steps,
			}

			return BuildRecipientsFromActivities(tx, acts, getSprintActivityAuthor, &ctx)
		},

		BuildDigest: func(tx *gorm.DB, acts []dao.ActivityEvent, sprint *dao.Sprint) (map[string]FieldPrerender, int) {
			return renderDigest(tx, templates, acts, sprint, sprintFieldRenderMap, sprintCollectors)
		},

		BuildHead: func(sprint *dao.Sprint) string {
			rr := headEntityCtx{
				WorkspaceName: sprint.Workspace.Slug,
				Layer:         "спринт",
				Identifier:    fmt.Sprint(sprint.SequenceId),
				Title:         sprint.GetFullName(),
				Url:           sprint.URL.String(),
				UrlText:       "Посмотреть спринт",
			}
			return renderHead(templates, rr)
		},

		Subject: func(s *dao.Sprint) string {
			return fmt.Sprintf("Обновления спринта %s", s.GetFullName())
		},

		FilterEmpty: true,
	}
}

func loadSprintActivities(tx *gorm.DB) []dao.ActivityEvent {
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

func groupSprint(acts []dao.ActivityEvent) ActivityBuckets[*dao.Sprint] {
	return GroupActivitiesByLayer(
		acts,
		func(a dao.ActivityEvent) uuid.UUID { return a.SprintID.UUID },
		func(a dao.ActivityEvent) *dao.Sprint {
			a.Sprint.Workspace = a.Workspace
			a.Sprint.SetUrl()
			return a.Sprint
		},
	)
}

func getSprintActivityAuthor(act dao.ActivityEvent) *dao.User {
	return act.Actor
}

func getIssueIdFromSprintActivity(a dao.ActivityEvent) uuid.UUID {
	return getUUIDFromActivity(a, uuidPtrFrom(a.NewSprintIssue), uuidPtrFrom(a.OldSprintIssue), nil, nil)
}

func getWatcherIdFromSprintActivity(a dao.ActivityEvent) uuid.UUID {
	return getUUIDFromActivity(a, uuidPtrFrom(a.NewSprintWatcher), uuidPtrFrom(a.OldSprintWatcher), nil, nil)
}

// collectors
var sprintCollectors = map[actField.ActivityField]activityFieldCollector{
	actField.Watchers.Field:    collectAll,
	actField.Issue.Field:       collectAll,
	actField.Description.Field: collectOne,
	actField.Name.Field:        collectOne,
	actField.StartDate.Field:   collectOne,
	actField.EndDate.Field:     collectOne,
}

var sprintFieldRenderMap = map[actField.ActivityField]FuncFieldRender[*dao.Sprint]{
	actField.Watchers.Field:    renderSprintWatcher,
	actField.Issue.Field:       renderSprintIssue,
	actField.Description.Field: renderSprintDescription,
	actField.Name.Field:        renderSprintName,
	actField.StartDate.Field:   renderSprintStartDate,
	actField.EndDate.Field:     renderSprintEndDate,
}

func renderSprintIssue(tx *gorm.DB, t *EmailTemplates, acts []dao.ActivityEvent, sprint *dao.Sprint) FieldPrerender {
	return renderEntityChange(tx, t, acts, sprint.Issues,
		"Задачи",
		entitySpec[dao.Issue]{
			entityID:    getIssueIdFromSprintActivity,
			entityTitle: func(i dao.Issue) string { return i.FullIssueName() },
			loadRemoved: getRemovedIssues,
			//getAuthor:   getSprintActivityAuthor,
		},
	)
}

func renderSprintWatcher(tx *gorm.DB, t *EmailTemplates, acts []dao.ActivityEvent, sprint *dao.Sprint) FieldPrerender {
	fp := renderEntityChange(tx, t, acts, sprint.Watchers,
		"Наблюдатели",
		entitySpec[dao.User]{
			entityID:    getWatcherIdFromSprintActivity,
			entityTitle: func(i dao.User) string { return i.GetName() },
			loadRemoved: getRemovedMember,
			//getAuthor:   getSprintActivityAuthor,
		})
	fp.Verb = acts[0].Verb
	return fp
}

func renderSprintName(_ *gorm.DB, t *EmailTemplates, acts []dao.ActivityEvent, _ *dao.Sprint) FieldPrerender {
	fp := t.RenderCollectOne(collectOneCtx{
		Key:    "Название",
		New:    toValueCtx(&acts[0].NewValue, nil),
		Old:    toValueCtx(&acts[0].OldValue, nil),
		Start:  sql.NullTime{Time: acts[0].CreatedAt, Valid: true},
		Author: *acts[0].Actor,
	})
	fp.Verb = acts[0].Verb
	return fp
}

func renderSprintDescription(_ *gorm.DB, t *EmailTemplates, acts []dao.ActivityEvent, _ *dao.Sprint) FieldPrerender {
	fp := t.RenderCollectOne(collectOneCtx{
		Key:    "Описание",
		New:    toValueCtx(nil, &acts[0].NewValue),
		Old:    toValueCtx(nil, &acts[0].OldValue),
		Start:  sql.NullTime{Time: acts[0].CreatedAt, Valid: true},
		Author: *acts[0].Actor,
	})
	fp.Verb = acts[0].Verb

	return fp
}

func renderSprintEndDate(_ *gorm.DB, t *EmailTemplates, acts []dao.ActivityEvent, _ *dao.Sprint) FieldPrerender {
	replace := make(map[string]any)
	fp := t.RenderCollectOne(collectOneCtx{
		Key:    "Конец",
		New:    collectDate(&acts[0].NewValue, targetDateZ+"_new", replace),
		Old:    collectDate(&acts[0].OldValue, targetDateZ+"_old", replace),
		Start:  sql.NullTime{Time: acts[0].CreatedAt, Valid: true},
		Author: *acts[0].Actor,
	})
	fp.Replace = replace
	fp.Verb = acts[0].Verb

	return fp
}

func renderSprintStartDate(_ *gorm.DB, t *EmailTemplates, acts []dao.ActivityEvent, _ *dao.Sprint) FieldPrerender {
	replace := make(map[string]any)

	fp := t.RenderCollectOne(collectOneCtx{
		Key:    "Начало",
		New:    collectDate(&acts[0].NewValue, targetDateZ+"_new", replace),
		Old:    collectDate(&acts[0].OldValue, targetDateZ+"_old", replace),
		Start:  sql.NullTime{Time: acts[0].CreatedAt, Valid: true},
		Author: *acts[0].Actor,
	})
	fp.Replace = replace
	fp.Verb = acts[0].Verb
	return fp
}
