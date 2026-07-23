package email

import (
	"fmt"
	"strings"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	member_role "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications/member-role"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

type IssueProcessor struct {
	*BaseProcessor
	plan *emailPlan
}

var issueFieldConfigs = map[actField.ActivityField]EntityFieldConfig{
	actField.Name.Field:        {collectOne, createFieldRenderer("Имя", StringField)},
	actField.Description.Field: {collectOne, createFieldRenderer("Описание", BodyField)},
	actField.Priority.Field:    {collectOne, createFieldRenderer("Приоритет", TranslateField, WithTranslation(types.PriorityTranslation))},
	actField.Assignees.Field:   {collectAll, renderIssueAssignee},
	actField.Watchers.Field:    {collectAll, renderIssueWatchers},
	actField.Status.Field:      {collectOne, createFieldRenderer("Статус", StringField)},
	actField.TargetDate.Field:  {collectOne, createTargetDateZRender("Срок исполнения", targetDateTimeZ)},
	actField.Parent.Field:      {collectOne, createFieldRenderer("Родительская задача", StringField)},
	actField.Blocks.Field:      {collectAll, renderIssueBlocked},
	actField.Blocking.Field:    {collectAll, renderIssueBlocker},
	actField.SubIssue.Field:    {collectAll, renderIssueSub},
	actField.Label.Field:       {collectAll, renderIssueLabel},
	actField.Linked.Field:      {collectAll, renderIssueLinked},
	actField.Sprint.Field:      {collectAll, renderIssueSprint},
	actField.Link.Field:        {collectAll, renderIssueLinks},
	actField.Comment.Field:     {collectAll, renderIssueComment},
	actField.Attachment.Field:  {collectAll, renderIssueAttachment},
}

func NewIssuePipeline() (types.EntityLayer, EmailProcessor) {
	layer := types.LayerIssue
	return layer, &IssueProcessor{
		BaseProcessor: NewBaseProcessor(),
		plan: &emailPlan{
			AuthorRole: member_role.IssueAuthor,
			EntityType: layer,
		},
	}
}

func (i IssueProcessor) LoadActivities(tx *gorm.DB) []dao.ActivityEvent {
	var activities []dao.ActivityEvent
	if err := tx.Unscoped().
		Joins("Issue").
		Joins("Actor").
		Joins("Project").
		Joins("Workspace").
		Preload("Issue.Parent").
		Order("activity_events.created_at").
		Where("activity_events.entity_type = ?", types.LayerIssue).
		Where("activity_events.notified = ?", false).
		Limit(100).
		Find(&activities).Error; err != nil {
		return []dao.ActivityEvent{}
	}
	return activities
}

func (i IssueProcessor) FullLoad(tx *gorm.DB, entity dao.IDaoAct) dao.IDaoAct {
	issue := entity.(*dao.Issue)
	issue.FullLoad = true
	if err := tx.Unscoped().
		Joins("Author").
		Preload("Assignees").
		Preload("Watchers").
		Preload("Labels").
		Preload("Links").
		Preload("Sprints").
		Joins("State").
		Joins("Parent").
		Preload("Project.DefaultWatchersDetails", "is_default_watcher = ?", true).
		Preload("Project.DefaultWatchersDetails.Member").
		Set("issueProgress", true).
		Where("issues.id = ?", entity.GetId()).
		First(&issue).
		Error; err != nil {
		return entity
	}
	return issue
}

func (i IssueProcessor) GroupActivities(acts []dao.ActivityEvent) ActivityBuckets {
	return GroupActivitiesByLayer(
		acts,
		func(a dao.ActivityEvent) dao.IDaoAct {
			a.Issue.Workspace = a.Workspace
			a.Issue.Project = a.Project
			if a.Issue.Parent != nil {
				a.Issue.Parent.Project = a.Project
				a.Issue.Parent.Workspace = a.Workspace
			}
			a.Issue.SetUrl()
			return a.Issue
		},
	)
}

func (i IssueProcessor) BuildRecipients(tx *gorm.DB, acts []dao.ActivityEvent, entity dao.IDaoAct) ([]member_role.MemberNotify, EmailContext) {
	issue, ok := entity.(*dao.Issue)
	if !ok {
		return []member_role.MemberNotify{}, EmailContext{}
	}

	steps := []member_role.UsersStep{
		member_role.AddUserRole(issue.Author, member_role.IssueAuthor),
		member_role.AddIssueUsers(issue),
		//member_role.AddOriginalCommentAuthor(i),
		member_role.AddDefaultWatchers(issue.ProjectId),
	}

	ctx := EmailContext{
		Plan:     i.plan,
		Settings: member_role.FromProject(),
		Steps:    steps,
	}

	return BuildRecipientsFromActivities(tx, acts, &ctx)
}

func (i IssueProcessor) BuildDigest(tx *gorm.DB, templates *EmailTemplates, acts []dao.ActivityEvent, entity dao.IDaoAct) (map[string]FieldPrerender, int) {
	return renderDigest(tx, templates, acts, entity, issueFieldConfigs)

}

func (p *IssueProcessor) BuildSubject(entity dao.IDaoAct) string {
	issue, ok := entity.(*dao.Issue)
	if !ok || issue == nil {
		return ""
	}
	return fmt.Sprintf("Обновления задачи %s", issue.FullIssueName())
}

func (p *IssueProcessor) BuildHead(templates *EmailTemplates, entity dao.IDaoAct) string {
	issue, ok := entity.(*dao.Issue)
	if !ok || issue == nil {
		return ""
	}
	head := headEntityCtx{
		WorkspaceName: issue.Workspace.Slug,
		Layer:         "задача",
		Identifier:    fmt.Sprint(issue.String()),
		Title:         issue.Name,
		Url:           issue.URL.String(),
		UrlText:       "Посмотреть задачу",
	}
	return templates.RenderHead(head)
}

func renderIssueAssignee(tx *gorm.DB, t *EmailTemplates, acts []dao.ActivityEvent, entity dao.IDaoAct) FieldPrerender {
	issue, ok := entity.(*dao.Issue)
	if !ok || issue == nil {
		return FieldPrerender{}
	}
	return renderEntityChange(tx, t, acts, *issue.Assignees,
		"Исполнители",
		entitySpec[dao.User]{
			entityID: func(event dao.ActivityEvent) uuid.UUID {
				return getUUIDFromActivity(uuidPtrFrom(event.NewAssignee), uuidPtrFrom(event.OldAssignee))
			},
			entityTitle: func(i dao.User) string { return i.GetName() },
			loadRemoved: getRemovedMembers,
		})
}

func renderIssueWatchers(tx *gorm.DB, t *EmailTemplates, acts []dao.ActivityEvent, entity dao.IDaoAct) FieldPrerender {
	issue, ok := entity.(*dao.Issue)
	if !ok || issue == nil {
		return FieldPrerender{}
	}
	return renderEntityChange(tx, t, acts, *issue.Watchers,
		"Наблюдатели",
		entitySpec[dao.User]{
			entityID: func(event dao.ActivityEvent) uuid.UUID {
				return getUUIDFromActivity(uuidPtrFrom(event.NewWatcher), uuidPtrFrom(event.OldWatcher))
			},
			entityTitle: func(i dao.User) string { return i.GetName() },
			loadRemoved: getRemovedMembers,
		})
}

func renderIssueBlocker(tx *gorm.DB, t *EmailTemplates, acts []dao.ActivityEvent, entity dao.IDaoAct) FieldPrerender {
	issue, ok := entity.(*dao.Issue)
	if !ok || issue == nil {
		return FieldPrerender{}
	}
	return renderEntityChange(tx, t, acts, utils.SliceToSlice(&issue.BlockerIssuesIDs, func(t *dao.IssueBlocker) dao.Issue {
		t.BlockedBy.Project = issue.Project
		t.BlockedBy.Workspace = issue.Workspace
		return *t.BlockedBy
	}),
		"Блокирует",
		entitySpec[dao.Issue]{
			entityID: func(event dao.ActivityEvent) uuid.UUID {
				return getUUIDFromActivity(uuidPtrFrom(event.NewBlockingIssue), uuidPtrFrom(event.OldBlockingIssue))
			},
			entityTitle: func(i dao.Issue) string { return i.GetString() },
			loadRemoved: getRemovedIssues,
		})
}

func renderIssueBlocked(tx *gorm.DB, t *EmailTemplates, acts []dao.ActivityEvent, entity dao.IDaoAct) FieldPrerender {
	issue, ok := entity.(*dao.Issue)
	if !ok || issue == nil {
		return FieldPrerender{}
	}
	return renderEntityChange(tx, t, acts, utils.SliceToSlice(&issue.BlockedIssuesIDs, func(t *dao.IssueBlocker) dao.Issue {
		t.Block.Project = issue.Project
		t.Block.Workspace = issue.Workspace
		return *t.Block
	}),
		"Заблокирована",
		entitySpec[dao.Issue]{
			entityID: func(event dao.ActivityEvent) uuid.UUID {
				return getUUIDFromActivity(uuidPtrFrom(event.NewBlockIssue), uuidPtrFrom(event.OldBlockIssue))
			},
			entityTitle: func(i dao.Issue) string { return i.GetString() },
			loadRemoved: getRemovedIssues,
		})
}

func renderIssueSub(tx *gorm.DB, t *EmailTemplates, acts []dao.ActivityEvent, entity dao.IDaoAct) FieldPrerender {
	issue, ok := entity.(*dao.Issue)
	if !ok || issue == nil {
		return FieldPrerender{}
	}
	var subIssue []dao.Issue
	if err := tx.Unscoped().Joins("Project").Where("issues.parent_id = ?", entity.GetId()).Find(&subIssue).Error; err != nil {
		return FieldPrerender{}
	}

	return renderEntityChange(tx, t, acts, subIssue,
		"Подзадачи",
		entitySpec[dao.Issue]{
			entityID: func(event dao.ActivityEvent) uuid.UUID {
				return getUUIDFromActivity(uuidPtrFrom(event.NewSubIssue), uuidPtrFrom(event.OldSubIssue))
			},
			entityTitle: func(i dao.Issue) string { return i.GetString() },
			loadRemoved: getRemovedIssues,
		})
}

func renderIssueLabel(tx *gorm.DB, t *EmailTemplates, acts []dao.ActivityEvent, entity dao.IDaoAct) FieldPrerender {
	issue, ok := entity.(*dao.Issue)
	if !ok || issue == nil {
		return FieldPrerender{}
	}

	return renderEntityChange(tx, t, acts, *issue.Labels,
		"Теги",
		entitySpec[dao.Label]{
			entityID: func(event dao.ActivityEvent) uuid.UUID {
				return getUUIDFromActivity(uuidPtrFrom(event.IssueActivityExtendFields.NewLabel), uuidPtrFrom(event.IssueActivityExtendFields.OldLabel))
			},
			entityTitle: func(i dao.Label) string { return i.GetString() },
			loadRemoved: func(tx *gorm.DB, uuids []uuid.UUID) map[uuid.UUID]string {
				return getRemovedEntities(tx, uuids, func(a dao.Label) string { return a.GetString() })
			},
		})
}

func renderIssueLinked(tx *gorm.DB, t *EmailTemplates, acts []dao.ActivityEvent, entity dao.IDaoAct) FieldPrerender {
	issue, ok := entity.(*dao.Issue)
	if !ok || issue == nil {
		return FieldPrerender{}
	}

	for i := range issue.LinkedIssues {
		issue.LinkedIssues[i].Project = issue.Project
	}

	return renderEntityChange(tx, t, acts, issue.LinkedIssues,
		"Связанные задачи",
		entitySpec[dao.Issue]{
			entityID: func(event dao.ActivityEvent) uuid.UUID {
				return getUUIDFromActivity(uuidPtrFrom(event.IssueActivityExtendFields.NewIssueLinked), uuidPtrFrom(event.IssueActivityExtendFields.OldIssueLinked))
			},
			entityTitle: func(i dao.Issue) string { return i.GetString() },
			loadRemoved: getRemovedIssues,
		})
}

func renderIssueSprint(tx *gorm.DB, t *EmailTemplates, acts []dao.ActivityEvent, entity dao.IDaoAct) FieldPrerender {
	issue, ok := entity.(*dao.Issue)
	if !ok || issue == nil {
		return FieldPrerender{}
	}

	return renderEntityChange(tx, t, acts, *issue.Sprints,
		"Спринт",
		entitySpec[dao.Sprint]{
			entityID: func(event dao.ActivityEvent) uuid.UUID {
				return getUUIDFromActivity(uuidPtrFrom(event.IssueActivityExtendFields.NewIssueSprint), uuidPtrFrom(event.IssueActivityExtendFields.OldIssueSprint))
			},
			entityTitle: func(i dao.Sprint) string { return i.GetString() },
			loadRemoved: func(tx *gorm.DB, uuids []uuid.UUID) map[uuid.UUID]string {
				return getRemovedEntities(tx, uuids, func(a dao.Sprint) string { return a.GetString() })
			},
		})
}

func renderIssueComment(tx *gorm.DB, t *EmailTemplates, acts []dao.ActivityEvent, entity dao.IDaoAct) FieldPrerender {
	return renderEntityChangeComplex(tx, t, acts, "Комментарии", WithActionTime(targetDateTimeZ), WithReplaceHtml(), WithTitleFunc(getAuthorTitle), WithComplexBlock())
}

func renderIssueAttachment(tx *gorm.DB, t *EmailTemplates, acts []dao.ActivityEvent, entity dao.IDaoAct) FieldPrerender {
	return renderEntityChangeComplex(tx, t, acts, "Вложения")
}

func renderIssueLinks(tx *gorm.DB, t *EmailTemplates, acts []dao.ActivityEvent, entity dao.IDaoAct) FieldPrerender {
	issue, ok := entity.(*dao.Issue)
	if !ok || issue == nil {
		return FieldPrerender{}
	}

	return renderEntityChange(tx, t, acts, *issue.Links,
		"Ссылки",
		entitySpec[dao.IssueLink]{
			entityID: func(event dao.ActivityEvent) uuid.UUID {
				return getUUIDFromActivity(uuidPtrFrom(event.IssueActivityExtendFields.NewLink), uuidPtrFrom(event.IssueActivityExtendFields.OldLink))
			},
			entityTitle: func(i dao.IssueLink) string { return i.GetString() },
			loadRemoved: func(tx *gorm.DB, uuids []uuid.UUID) map[uuid.UUID]string {
				return getRemovedEntities(tx, uuids, func(a dao.IssueLink) string { return a.GetString() })
			},
		})
}

func issueCreateFunc(c *entityChange, act dao.ActivityEvent) {
	var issue dao.Issue
	if act.IssueExtendFields.NewIssue != nil {
		issue = *act.IssueExtendFields.NewIssue
	}

	switch act.Verb {
	case actField.VerbCreated:
		c.Created = true
		var builder strings.Builder

		addField := func(label, value string) {
			if value != "" {
				builder.WriteString("<table width='100%' cellpadding='0' cellspacing='0' style='margin-bottom: 8px;'>")
				builder.WriteString("<tr>")
				builder.WriteString("<td align='right' valign='top' style='white-space: nowrap;'><b>" + label + "</b></td>")
				builder.WriteString("</tr>")

				builder.WriteString("<tr>")
				builder.WriteString("<td align='left' valign='top' style='word-break: break-word;'>" + value + "</td>")
				builder.WriteString("</tr>")
				builder.WriteString("</table>")
			}
		}
		if issue.Author != nil {
			addField("Автор", issue.Author.GetName())
		}

		if issue.DescriptionHtml != "<p></p>" {
			addField("Описание", "<br>"+*htmlReplacer(&issue.DescriptionHtml))
		}

		if issue.State != nil {
			addField("Статус", issue.State.Name)
		}
		if issue.Parent != nil {
			issue.Parent.Project = issue.Project
			addField("Родительская задача", issue.Parent.FullIssueName())
		}

		if issue.Priority != nil {
			addField("Приоритет", types.TranslateMap(types.PriorityTranslation, issue.Priority))
		}

		if issue.Assignees != nil {
			addField("Исполнители", strings.Join(utils.SliceToSlice(issue.Assignees, func(t *dao.User) string { return t.GetName() }), "<br>"))
		}
		if issue.Watchers != nil {
			addField("Наблюдатели", strings.Join(utils.SliceToSlice(issue.Watchers, func(t *dao.User) string { return t.GetName() }), "<br>"))
		}
		if issue.Sprints != nil {
			addField("Спринты", strings.Join(utils.SliceToSlice(issue.Sprints, func(t *dao.Sprint) string { return t.GetString() }), "<br>"))
		}

		c.LastNew = utils.ToPtr(builder.String())
	case actField.VerbDeleted:
		c.Deleted = true
		c.FirstOld = utils.ToPtr("удалена")
	}
}
