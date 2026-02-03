package email

import (
	"fmt"
	"strings"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	member_role "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications/member-role"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

// pipeline
//func NewSprintPipeline( /*steps []member_role.UsersStep*/ templates *EmailTemplates) LayerPipeline[dao.SprintActivity, *dao.Sprint] {
//
//	return LayerPipeline[dao.SprintActivity, *dao.Sprint]{
//		Load:  loadSprintActivities,
//		Group: groupSprint,
//
//		BuildRecipients: func(tx *gorm.DB, acts []dao.SprintActivity, entity *dao.Sprint) []member_role.MemberNotify {
//			steps := []member_role.UsersStep{
//				member_role.AddWorkspaceAdmins(entity.WorkspaceId),
//			}
//			plan := emailPlan{
//				TableName:  dao.SprintActivity{}.TableName(),
//				settings:   member_role.FromWorkspace(entity.WorkspaceId),
//				AuthorRole: member_role.WorkspaceAdminRole,
//				Steps:      steps,
//			}
//			return BuildRecipientsFromActivities(tx, acts, getSprintActivityAuthor, plan)
//		},
//
//		BuildDigest: func(tx *gorm.DB, acts []dao.SprintActivity, sprint *dao.Sprint) (map[string]fieldPrerender, int) {
//			return renderDigest(tx, templates, acts, sprint, sprintFieldRenderMap, sprintCollectors)
//		},
//
//		Subject: func(s *dao.Sprint) string {
//			return fmt.Sprintf("Обновления спринта %s", s.GetFullName())
//		},
//
//		BuildMessage: buildSprintDigest(emailPlan{
//			settings:   member_role.FromWorkspace(uuid.Nil), // будет перезаписан при реальном использовании
//			AuthorRole: member_role.WorkspaceAdminRole,
//		}),
//		FilterEmpty: true,
//	}
//}

func NewSprintPipeline(templates *EmailTemplates) LayerPipeline[dao.SprintActivity, *dao.Sprint] {

	plan := &emailPlan{
		TableName:  dao.SprintActivity{}.TableName(),
		AuthorRole: member_role.ActionAuthor,
	}

	return LayerPipeline[dao.SprintActivity, *dao.Sprint]{
		Plan:  plan,
		Load:  loadSprintActivities,
		Group: groupSprint,

		BuildRecipients: func(tx *gorm.DB, acts []dao.SprintActivity, sprint *dao.Sprint) []member_role.MemberNotify {
			plan.settings = member_role.FromWorkspace(sprint.WorkspaceId)
			plan.Steps = []member_role.UsersStep{
				member_role.AddWorkspaceAdmins(sprint.WorkspaceId),
			}
			return BuildRecipientsFromActivities(tx, acts, getSprintActivityAuthor, plan)
		},

		BuildDigest: func(tx *gorm.DB, acts []dao.SprintActivity, sprint *dao.Sprint) (map[string]fieldPrerender, int) {
			return renderDigest(tx, templates, acts, sprint, sprintFieldRenderMap, sprintCollectors)
		},

		//BuildMessage: func(b *ActivityBucket[dao.SprintActivity, *dao.Sprint], r Recipient) EmailMessage {
		//  buildSprintDigest(emailPlan{
		//    			settings:   member_role.FromWorkspace(uuid.Nil), // будет перезаписан при реальном использовании
		//    			AuthorRole: member_role.WorkspaceAdminRole,
		//    		}),
		//return buildMessageWithPlan(b, r, plan.(b.Entity.WorkspaceId))
		//},

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

func buildSprintDigest(b *ActivityBucket[dao.SprintActivity, *dao.Sprint], r Recipient) EmailMessage {

	var parts []string
	var cnt int
	for field, html := range b.Prepared {
		r.MemberNotify.Allowed(field, "", actField.Sprint.Field, member_role.ActionAuthor)
		//html.
		//if r.MemberNotify.I()
		//r.
		//r.MemberNotify
		//if !r.MemberNotify.Allowed(field) {
		//	continue
		//}
		parts = append(parts, html.Value)
		cnt += html.Count
	}

	if len(parts) == 0 {
		return EmailMessage{}
	}

	body := strings.Join(parts, "")
	fmt.Println("count", cnt)
	return EmailMessage{
		To:   r.Email,
		HTML: body,
	}
}

var sprintFieldRenderMap = map[actField.ActivityField]FuncFieldRender[dao.SprintActivity, *dao.Sprint]{
	actField.Issue.Field: renderSprintIssue,
	actField.Name.Field:  renderSprintName,
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

func renderSprintName(_ *gorm.DB, t *EmailTemplates, acts []dao.SprintActivity, _ *dao.Sprint) (string, int) {
	return t.RenderCollectOne(collectOneCtx{
		Key: "Название",
		New: toValueCtx(&acts[0].NewValue, nil),
		Old: toValueCtx(acts[0].OldValue, nil),
	})
}

//

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
