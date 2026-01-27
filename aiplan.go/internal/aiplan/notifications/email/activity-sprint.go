package email

import (
	"bytes"
	"fmt"
	"log/slog"
	"strings"
	"text/template"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	member_role "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications/member-role"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

var sprintCollectors = map[actField.ActivityField]activityFieldCollector[dao.SprintActivity]{
	actField.Watchers.Field:    collectAll[dao.SprintActivity],
	actField.Issue.Field:       collectAll[dao.SprintActivity],
	actField.Description.Field: collectOne[dao.SprintActivity],
	actField.Name.Field:        collectOne[dao.SprintActivity],
	actField.StartDate.Field:   collectOne[dao.SprintActivity],
	actField.EndDate.Field:     collectOne[dao.SprintActivity],
}

func (s *EmailService) ProcessSprint() {
	s.sending = true

	defer func() {
		s.sending = false
	}()

	pipeline := NewSprintPipeline()
	buckets := RunLayerPipeline(s.db, pipeline)
	if len(buckets) == 0 {
		return
	}
	messages := BuildEmailMessages(buckets, pipeline)
	if len(messages) == 0 {
		return
	}

	for _, m := range messages {
		if err := s.Send(m); err != nil {
			slog.Error("send email", "to", m.To, "err", err)
		}
	}

	return
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

		BuildDigest: func(tx *gorm.DB, acts []dao.SprintActivity, sprint *dao.Sprint) (map[string]fieldPrerender, int) {
			templates := LoadTemplates(tx)
			return renderSprintDigest(tx, templates, acts, sprint)
		},

		Subject: func(s *dao.Sprint) string {
			return fmt.Sprintf("Обновления спринта %s", s.GetFullName())
		},

		BuildMessage: buildSprintDigest,

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

func buildSprintDigest(b *ActivityBucket[dao.SprintActivity, *dao.Sprint], r Recipient) EmailMessage {

	var parts []string
	var cnt int
	for _, html := range b.Prepared {
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

// type SprintActivityBuckets map[uuid.UUID]ActivityBucket
type ActivityBuckets[A dao.ActivityI, E dao.IDaoAct] map[uuid.UUID]*ActivityBucket[A, E]

type ActivityBucket[A dao.ActivityI, E dao.IDaoAct] struct {
	Entity     E
	Activities []A

	MemberNotify []member_role.MemberNotify
	Prepared     map[string]fieldPrerender

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

type fieldPrerender struct {
	Value string
	Count int
}

func renderSprintDigest(
	tx *gorm.DB,
	templates EmailTemplates,
	activities []dao.SprintActivity,
	sprint *dao.Sprint,
) (map[string]fieldPrerender, int) {

	result := make(map[string]fieldPrerender)
	totalChanges := 0

	digest := CollectActivitiesByField(activities, sprintCollectors)

	for field, acts := range digest {
		switch field {
		case actField.Issue.Field.String():
			if val, cnt := templates.sprintIssueRender(tx, acts, sprint); cnt > 0 {
				result[field] = fieldPrerender{
					Value: val,
					Count: cnt,
				}
				totalChanges += cnt
			}
		case actField.Name.Field.String():
			if val, cnt := templates.sprintNameRender(tx, acts, sprint); cnt > 0 {
				result[field] = fieldPrerender{
					Value: val,
					Count: cnt,
				}
				totalChanges += cnt
			}
		}
	}
	return result, totalChanges
}

func (t *EmailTemplates) sprintIssueRender(tx *gorm.DB, acts []dao.SprintActivity, sprint *dao.Sprint) (string, int) {
	f := entitySpec[dao.SprintActivity, dao.Issue]{
		entityID:    getIssueIdFromSprintActivity,
		isAdded:     defaultIsAdded[dao.SprintActivity],
		isRemoved:   defaultActIsRemoved[dao.SprintActivity],
		entityTitle: func(i dao.Issue) string { return i.FullIssueName() },
		loadRemoved: getRemovedIssues,
	}

	views, count := BuildEntityChangeDigest(tx, acts, sprint.Issues, f)
	context := collectAllCtx{
		"Задачи",
		views,
	}
	var buf bytes.Buffer
	if err := t.CollectAll.Execute(&buf, context); err != nil {
		return "", 0
	}
	return buf.String(), count
}

func (t *EmailTemplates) sprintNameRender(tx *gorm.DB, acts []dao.SprintActivity, sprint *dao.Sprint) (string, int) {
	ff := func(s *string) *actValueCtx {
		if s != nil {
			return &actValueCtx{
				Value: s,
				Body:  nil,
			}
		}
		return nil
	}
	context := collectOneCtx{
		Key: "Название",
		New: ff(&acts[0].NewValue),
		Old: ff(acts[0].OldValue),
	}
	var buf bytes.Buffer
	if err := t.CollectOne.Execute(&buf, context); err != nil {
		return "", 0
	}
	return buf.String(), 1
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

type EmailTemplates struct {
	CollectAll *template.Template
	CollectOne *template.Template
}

func LoadTemplates(tx *gorm.DB) EmailTemplates {
	names := []string{
		"v2_collect_all",
		"v2_collect_one",
	}
	var templates []dao.Template
	if err := tx.Where("name in (?)", names).Find(&templates).Error; err != nil {
		return EmailTemplates{}
	}

	var res EmailTemplates
	for _, t := range templates {
		switch t.Name {
		case "v2_collect_all":
			res.CollectAll = t.ParsedTemplate
		case "v2_collect_one":
			res.CollectOne = t.ParsedTemplate
		}
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

func defaultIsAdded[A dao.ActivityI](a A) bool {
	return a.GetVerb() == actField.VerbAdded
}

func defaultActIsRemoved[A dao.ActivityI](a A) bool {
	return a.GetVerb() == actField.VerbRemoved
}
