package email

import (
	"bytes"
	"text/template"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	member_role "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications/member-role"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

// 1. group activities by sprint
// 2. resolve recipients
// 3. build digest
// 4. render templates
// 5. filter empty

var sprintCollectors = map[actField.ActivityField]activityFieldCollector[dao.SprintActivity]{
	actField.Watchers.Field:    collectAll[dao.SprintActivity],
	actField.Issue.Field:       collectAll[dao.SprintActivity],
	actField.Description.Field: collectLast[dao.SprintActivity],
	actField.StartDate.Field:   collectLast[dao.SprintActivity],
	actField.EndDate.Field:     collectLast[dao.SprintActivity],
}

func NewSprintPipeline( /*steps []member_role.UsersStep*/ ) LayerPipeline[dao.SprintActivity, *dao.Sprint] {
	return LayerPipeline[dao.SprintActivity, *dao.Sprint]{
		Load:  loadSprintActivities,
		Group: groupSprint,
		BuildRecipients: func(tx *gorm.DB, acts []dao.SprintActivity, entity *dao.Sprint) []member_role.MemberNotify {
			steps := []member_role.UsersStep{
				member_role.AddWorkspaceAdmins(entity.WorkspaceId),
			}
			return BuildRecipientsFromActivities(tx, acts, steps, getSprintActivityAuthor)
		},
		BuildDigest: func(tx *gorm.DB, acts []dao.SprintActivity, sprint *dao.Sprint) (map[string]string, int) {
			templates := LoadTemplates(tx)
			return renderSprintDigest(tx, templates, acts, sprint)
		},

		FilterEmpty: true,
	}
}

func getSprintActivityAuthor(act dao.SprintActivity) *dao.User {
	return act.Actor
}

func groupSprint(acts []dao.SprintActivity) ActivityBuckets[dao.SprintActivity, *dao.Sprint] {
	return GroupActivitiesByLayer(
		acts,
		func(a dao.SprintActivity) uuid.UUID { return a.SprintId },
		func(a dao.SprintActivity) *dao.Sprint { return a.Sprint },
	)
}

// type SprintActivityBuckets map[uuid.UUID]ActivityBucket
type ActivityBuckets[A dao.ActivityI, E dao.IDaoAct] map[uuid.UUID]*ActivityBucket[A, E]

type ActivityBucket[A dao.ActivityI, E dao.IDaoAct] struct {
	Entity     E
	Activities []A

	Recipients []member_role.MemberNotify
	Prepared   map[string]string // field -> rendered html

	FirstAt time.Time
	LastAt  time.Time
}

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

//func (m SprintActivityBuckets) Add(act dao.SprintActivity) {
//	v, ok := m[act.SprintId]
//	if !ok {
//		m[act.SprintId] = ActivityBucket{
//			Entity:     act.Sprint,
//			Activities: []dao.SprintActivity{act},
//			FirstAt:    act.CreatedAt,
//			LastAt:     act.CreatedAt,
//		}
//		return
//	}
//
//	v.Activities = append(v.Activities, act)
//
//	if act.CreatedAt.Before(v.FirstAt) {
//		v.FirstAt = act.CreatedAt
//	}
//	if act.CreatedAt.After(v.LastAt) {
//		v.LastAt = act.CreatedAt
//	}
//
//	m[act.SprintId] = v
//}

//func ProcessSprintActivities(
//	tx *gorm.DB,
//	acts []dao.SprintActivity,
//	steps []member_role.UsersStep,
//) SprintActivityBuckets {
//
//	result := make(SprintActivityBuckets)
//	templates := LoadTemplates(tx)
//
//	for _, act := range acts {
//		result.Add(act)
//	}
//
//	for id, sprint := range result {
//
//		sprint.Recipients = BuildRecipientsFromActivities(
//			tx, sprint.Activities, steps,
//			func(a dao.SprintActivity) *dao.User {
//				return a.Actor
//			})
//
//		prepared, changes := renderSprintDigest(
//			tx,
//			templates,
//			sprint.Activities,
//			sprint.Sprint,
//		)
//
//		if changes == 0 {
//			continue // нечего слать
//		}
//
//		sprint.Prepared = prepared
//		result[id] = sprint
//	}
//
//	return result
//}

func renderSprintDigest(
	tx *gorm.DB,
	templates EmailTemplates,
	activities []dao.SprintActivity,
	sprint *dao.Sprint,
) (map[string]string, int) {

	result := make(map[string]string)
	totalChanges := 0

	digest := CollectActivitiesByField(activities, sprintCollectors)

	for field, acts := range digest {
		switch field {
		case actField.Issue.Field.String():
			views, count := BuildEntityChangeDigest(
				tx,
				acts,
				sprint.Issues,
				sprintIssues,
				sprintActIsAdded,
				sprintActIsRemoved,
				func(i dao.Issue) string {
					return i.FullIssueName()
				},
				getRemovedIssues,
			)
			context := struct {
				Key   string
				Views []DigestView
			}{
				"Задачи",
				views,
			}
			var buf bytes.Buffer
			if err := templates.Combine.Execute(&buf, context); err != nil {
				continue
			}

			result[field] = buf.String()
			totalChanges += count
		}
	}

	return result, totalChanges
}

type EmailTemplates struct {
	Combine *template.Template
	Single  template.Template
}

func LoadTemplates(tx *gorm.DB) EmailTemplates {
	names := []string{
		"v2_combine_element",
	}
	var templates []dao.Template
	if err := tx.Where("name in (?)", names).Find(&templates).Error; err != nil {
		return EmailTemplates{}
	}

	var res EmailTemplates
	for _, t := range templates {
		switch t.Name {
		case "v2_combine_element":
			res.Combine = t.ParsedTemplate
		}
	}
	return res
}

func sprintIssues(a dao.SprintActivity) uuid.UUID {
	switch a.Verb {
	case actField.VerbAdded:
		return a.NewSprintIssue.ID
	case actField.VerbRemoved:
		return a.OldSprintIssue.ID
	default:
		return uuid.Nil
	}
}

func sprintActIsAdded(a dao.SprintActivity) bool {
	return a.Verb == actField.VerbAdded
}

func sprintActIsRemoved(a dao.SprintActivity) bool {
	return a.Verb == actField.VerbRemoved
}

func getRemovedIssues(tx *gorm.DB, ids []uuid.UUID) map[uuid.UUID]string {
	var issues []dao.Issue
	res := make(map[uuid.UUID]string)

	if err := tx.Joins("Project").
		Where("issues.id IN (?)", ids).
		Find(&issues).Error; err != nil {
		return res
	}

	for _, i := range issues {
		res[i.ID] = i.FullIssueName()
	}

	return res
}
