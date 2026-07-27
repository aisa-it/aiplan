package email

import (
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"time"

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
	actField.Name.Field:              {collectOne, createFieldRenderer("Имя", StringField)},
	actField.Identifier.Field:        {collectOne, createFieldRenderer("Идентификатор", StringField)},
	actField.Emoj.Field:              {collectOne, createFieldRenderer("Емоджи", EmojiField)},
	actField.Logo.Field:              {collectOne, createFieldRenderer("Логотип", StringField, WithCustomText("изменен логотип проекта"))},
	actField.Public.Field:            {collectOne, createFieldRenderer("Публичность", StringField, WithTranslation(types.ProjectPublicTranslate))},
	actField.DefaultWatchers.Field:   {collectAll, renderProjectDefaultWatchers},
	actField.DefaultAssignees.Field:  {collectAll, renderProjectDefaultAssignees},
	actField.ProjectLead.Field:       {collectOne, renderProjectLead},
	actField.Member.Field:            {collectAll, renderProjectMember},
	actField.Role.Field:              {collectOne, renderProjectMemberRole},
	actField.Issue.Field:             {collectAll, renderProjectIssue},
	actField.Status.Field:            {collectCompositeField("status"), renderProjectState},
	actField.StatusName.Field:        {collectCompositeField("status"), renderProjectState},
	actField.StatusColor.Field:       {collectCompositeField("status"), renderProjectState},
	actField.StatusDescription.Field: {collectCompositeField("status"), renderProjectState},
	actField.StatusDefault.Field:     {collectCompositeField("status"), renderProjectState},
	actField.StatusGroup.Field:       {collectCompositeField("status"), renderProjectState},
	actField.Label.Field:             {collectCompositeField("label"), renderProjectLabel},
	actField.LabelName.Field:         {collectCompositeField("label"), renderProjectLabel},
	actField.LabelColor.Field:        {collectCompositeField("label"), renderProjectLabel},
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
						Joins("Parent").
						Joins("State").
						Preload("Assignees").
						Preload("Watchers").
						Preload("Sprints").
						First(&issue).Error; err != nil {
						return nil
					}
				}
				issue.Project = act.Project
				issue.Workspace = act.Workspace
				issue.SetUrl()
				*act.ProjectActivityExtendFields.NewIssue = issue
				return []member_role.UsersStep{
					member_role.AddIssueUsers(act.NewIssue, member_role.WithActivityId(act.ID)),
					member_role.AddDefaultWatchers(act.ProjectID.UUID, member_role.WithActivityId(act.ID)),
				}
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

	result, totalChanges := renderDigest(tx, templates, acts, entity, projectFieldConfigs)

	if len(acts) > 0 {
		allIDs := make([]uuid.UUID, len(acts))
		for i, act := range acts {
			allIDs[i] = act.ID
		}
		for key, fp := range result {
			if key != actField.Issue.Field.String() {
				fp.ActivityIds = allIDs
				result[key] = fp
			}
		}
	}

	var issueActs []dao.ActivityEvent
	for _, act := range acts {
		if act.Field == actField.Issue.Field && slices.Contains([]string{actField.VerbCreated, actField.VerbCopied, actField.VerbAdded}, act.Verb) {
			issueActs = append(issueActs, act)
		}
	}

	customBodyFP := buildProjectCustomBody(tx, templates, issueActs, entity)
	if customBodyFP.Count > 0 {
		result[customBodyFieldKey] = customBodyFP
		totalChanges += customBodyFP.Count
	}

	return result, totalChanges
}

func renderProjectIssue(tx *gorm.DB, t *EmailTemplates, acts []dao.ActivityEvent, entity dao.IDaoAct) FieldPrerender {
	views := make([]DigestView, 0, len(acts))
	authors := make(map[uuid.UUID]dao.User)
	var start, end time.Time

	for _, act := range acts {
		if act.Verb == actField.VerbDeleted || act.Verb == actField.VerbRemoved {
			// только удаленные и перемещенные
			views = append(views, DigestView{Title: act.OldValue, IsGone: true})

			if act.Actor != nil {
				authors[act.Actor.ID] = *act.Actor
			}
			if start.IsZero() || act.CreatedAt.Before(start) {
				start = act.CreatedAt
			}
			if end.IsZero() || act.CreatedAt.After(end) {
				end = act.CreatedAt
			}
		}
	}

	if len(views) == 0 {
		return FieldPrerender{}
	}

	fp := t.RenderCollectAll(collectAllCtx{
		Key:    "Задачи",
		Views:  views,
		Start:  sql.NullTime{Time: start, Valid: true},
		End:    sql.NullTime{Time: end, Valid: true},
		Author: authors,
	}, len(views))
	fp.Verb = actField.VerbDeleted
	return fp
}

func buildProjectCustomBody(tx *gorm.DB, templates *EmailTemplates, issueActs []dao.ActivityEvent, entity dao.IDaoAct) FieldPrerender {
	if len(issueActs) == 0 {
		return FieldPrerender{}
	}

	project, ok := entity.(*dao.Project)
	if !ok {
		return FieldPrerender{}
	}

	customBodyData := make(map[uuid.UUID]string)
	var actIds []uuid.UUID
	replaceMap := make(map[string]any)

	for _, act := range issueActs {
		issue := act.ProjectActivityExtendFields.NewIssue
		if issue == nil {
			continue
		}
		headCtx := headEntityCtx{
			WorkspaceName: project.Workspace.Slug,
			Layer:         "задача",
			Identifier:    issue.String(),
			Title:         issue.Name,
			Url:           issue.URL.String(),
			UrlText:       "Посмотреть задачу",
		}
		headHTML := templates.RenderHead(headCtx)

		fieldsHTML := renderIssueFieldsFromIssue(tx, templates, issue, act.CreatedAt, act.Actor, replaceMap)
		activityHTML := templates.RenderActivity(activityBodyCtx{
			Body:           fieldsHTML,
			ActivityActors: "",
			CustomBody:     "",
		})

		customBodyData[act.ID] = `<hr style="border:none;border-top:1px solid #dde2ea;margin:24px 0;">` + headHTML + activityHTML
		actIds = append(actIds, act.ID)
	}

	if len(customBodyData) == 0 {
		return FieldPrerender{}
	}

	placeholder := ApplyCustomReplaceText(customIssueBody, replaceMap)

	fp := FieldPrerender{
		CustomBodyValue: customBodyData,
		Verb:            actField.VerbCreated,
		Field:           actField.Issue.Field,
		Count:           len(customBodyData),
		ActivityIds:     actIds,
		Value:           placeholder,
		Replace:         replaceMap,
		Authors:         collectIssueAuthors(issueActs),
	}

	fp.Add(optCustomBody)
	return fp
}

func collectIssueAuthors(acts []dao.ActivityEvent) []dao.User {
	seen := make(map[uuid.UUID]bool)
	var authors []dao.User
	for _, act := range acts {
		if act.Actor != nil && !seen[act.Actor.ID] {
			seen[act.Actor.ID] = true
			authors = append(authors, *act.Actor)
		}
	}
	return authors
}

func renderIssueFieldsFromIssue(tx *gorm.DB, templates *EmailTemplates, issue *dao.Issue, actTime time.Time, actor *dao.User, replaceMap map[string]any) string {
	if issue == nil {
		return ""
	}

	var synActs []dao.ActivityEvent

	if issue.Name != "" {
		synActs = append(synActs, addScalar(actTime, actor, actField.Name.Field, issue.Name))
	}
	if issue.DescriptionHtml != "" && issue.DescriptionHtml != "<p></p>" {
		synActs = append(synActs, addScalar(actTime, actor, actField.Description.Field, issue.DescriptionHtml))
	}
	if issue.Priority != nil {
		synActs = append(synActs, addScalar(actTime, actor, actField.Priority.Field, *issue.Priority))
	}
	if issue.State != nil {
		synActs = append(synActs, addScalar(actTime, actor, actField.Status.Field, issue.State.Name))
	}
	if issue.Parent != nil {
		issue.Parent.Project = issue.Project
		synActs = append(synActs, addScalar(actTime, actor, actField.Parent.Field, issue.Parent.FullIssueName()))
	}
	if issue.TargetDate != nil && !issue.TargetDate.Time.IsZero() {
		synActs = append(synActs, addScalar(actTime, actor, actField.TargetDate.Field, issue.TargetDate.Time.Format(time.RFC3339)))
	}

	if issue.Assignees != nil {
		synActs = append(synActs, addCollection(actTime, actor, actField.Assignees.Field, *issue.Assignees,
			func(u *dao.User) dao.IssueActivityExtendFields {
				return dao.IssueActivityExtendFields{
					IssueAssigneeExtendFields: dao.IssueAssigneeExtendFields{NewAssignee: u},
				}
			})...)
	}
	if issue.Watchers != nil {
		synActs = append(synActs, addCollection(actTime, actor, actField.Watchers.Field, *issue.Watchers,
			func(u *dao.User) dao.IssueActivityExtendFields {
				return dao.IssueActivityExtendFields{
					IssueWatchersExtendFields: dao.IssueWatchersExtendFields{NewWatcher: u},
				}
			})...)
	}
	if issue.Sprints != nil {
		synActs = append(synActs, addCollection(actTime, actor, actField.Sprint.Field, *issue.Sprints,
			func(s *dao.Sprint) dao.IssueActivityExtendFields {
				return dao.IssueActivityExtendFields{
					IssueSprintExtendFields: dao.IssueSprintExtendFields{NewIssueSprint: s},
				}
			})...)
	}

	if len(synActs) == 0 {
		return ""
	}

	result, _ := renderDigest(tx, templates, synActs, issue, issueFieldConfigs)

	orderedFields := []actField.ActivityField{
		actField.Name.Field, actField.Description.Field, actField.Status.Field,
		actField.Parent.Field, actField.Priority.Field, actField.Assignees.Field,
		actField.Watchers.Field, actField.Sprint.Field, actField.TargetDate.Field,
	}

	var parts []string
	for _, field := range orderedFields {
		if fp, ok := result[field.String()]; ok && fp.Value != "" {
			for k, v := range fp.Replace {
				if replaceMap != nil {
					replaceMap[k] = v
				}
			}
			parts = append(parts, fp.Value)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n")
}

func addScalar(actTime time.Time, actor *dao.User, field actField.ActivityField, value string) dao.ActivityEvent {
	return dao.ActivityEvent{
		CreatedAt: actTime,
		Actor:     actor,
		Field:     field,
		Verb:      actField.VerbCreated,
		NewValue:  value,
	}
}

func addCollection[T any](actTime time.Time, actor *dao.User, field actField.ActivityField, items []T, extFn func(*T) dao.IssueActivityExtendFields) []dao.ActivityEvent {
	var acts []dao.ActivityEvent
	for i := range items {
		acts = append(acts, dao.ActivityEvent{
			CreatedAt:                 actTime,
			Actor:                     actor,
			Field:                     field,
			Verb:                      actField.VerbAdded,
			IssueActivityExtendFields: extFn(&items[i]),
		})
	}
	return acts
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
			if v, ok := types.RoleTranslation[*str]; ok {
				return utils.ToPtr(v)
			}
			return str
		}),
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

func renderProjectState(tx *gorm.DB, t *EmailTemplates, acts []dao.ActivityEvent, entity dao.IDaoAct) FieldPrerender {
	return renderWithDeletedSeparated(tx, t, acts, entity, "Статус",
		func(c *entityChange, act dao.ActivityEvent) {
			c.Deleted = true
			c.FirstOld = utils.ToPtr("удалена")
		},
		func(act *dao.ActivityEvent) *string {
			if act.ProjectActivityExtendFields.NewState != nil {
				return utils.ToPtr(act.ProjectActivityExtendFields.NewState.Name)

			} else if act.ProjectActivityExtendFields.OldState != nil {
				return utils.ToPtr(act.ProjectActivityExtendFields.OldState.Name)
			}
			return utils.ToPtr(act.OldValue)
		},
		stateComplexFunc,
	)
}

func stateComplexFunc(c *entityChange, act dao.ActivityEvent) {
	var newState dao.State
	if act.ProjectActivityExtendFields.NewState != nil {
		newState = *act.ProjectActivityExtendFields.NewState
	}

	cFields := c.CompositeFields != nil

	switch act.Verb {
	case actField.VerbCreated:
		c.Created = true
		if cFields {
			c.CompositeFields["Группа"] = compositeFields{
				transitionFlags: transitionFlags{Created: true},
				New:             new(types.TranslateMap(types.StatusTranslation, new(newState.Group))),
			}
			if newState.Description != "" {
				c.CompositeFields["Описание"] = compositeFields{
					transitionFlags: transitionFlags{Created: true},
					New:             htmlReplacer(&newState.Description),
				}
			}
			if newState.Default {
				c.CompositeFields["По умолчанию"] = compositeFields{
					transitionFlags: transitionFlags{Created: true},
					New:             new("установлен"),
				}
			}
		}
	case actField.VerbDeleted:
		c.Deleted = true
		c.FirstOld = utils.ToPtr("удалена")
	case actField.VerbUpdated:
		c.Updated = true
		switch act.Field {
		case actField.StatusColor.Field:
			cf := c.CompositeFields["Цвет"]
			cf.Created = true
			cf.New = new("Обновлен")
			c.CompositeFields["Цвет"] = cf
		case actField.StatusDescription.Field:
			cf := c.CompositeFields["Описание"]
			cf.Updated = true
			if cf.Old == nil {
				cf.Old = new(act.OldValue)
			}
			cf.New = new(act.NewValue)
			c.CompositeFields["Описание"] = cf
		case actField.StatusName.Field:
			cf := c.CompositeFields["Имя"]
			cf.Updated = true
			if cf.Old == nil {
				cf.Old = new(act.OldValue)
			}
			cf.New = new(act.NewValue)
			c.CompositeFields["Имя"] = cf
		case actField.StatusGroup.Field:
			cf := c.CompositeFields["Группа"]
			cf.Updated = true
			if cf.Old == nil {
				cf.Old = new(types.TranslateMap(types.StatusTranslation, new(act.OldValue)))

			}
			cf.New = new(types.TranslateMap(types.StatusTranslation, new(act.NewValue)))
			c.CompositeFields["Группа"] = cf
		case actField.StatusDefault.Field:
			cf := c.CompositeFields["По умолчанию"]
			cf.Created = true
			cf.New = new("Установлен")
			c.CompositeFields["По умолчанию"] = cf
		}
	}
}

func renderProjectLabel(tx *gorm.DB, t *EmailTemplates, acts []dao.ActivityEvent, entity dao.IDaoAct) FieldPrerender {
	return renderWithDeletedSeparated(tx, t, acts, entity, "Тег",
		func(c *entityChange, act dao.ActivityEvent) {
			c.Deleted = true
			c.FirstOld = utils.ToPtr("удален")
		},
		func(act *dao.ActivityEvent) *string {
			if act.ProjectActivityExtendFields.NewLabel != nil {
				return utils.ToPtr(act.ProjectActivityExtendFields.NewLabel.Name)

			} else if act.ProjectActivityExtendFields.OldLabel != nil {
				return utils.ToPtr(act.ProjectActivityExtendFields.OldLabel.Name)
			}
			return utils.ToPtr(act.OldValue)
		},
		labelComplexFunc,
	)
}

func labelComplexFunc(c *entityChange, act dao.ActivityEvent) {
	switch act.Verb {
	case actField.VerbCreated:
		c.Created = true
		c.LastNew = new("создан")
	case actField.VerbDeleted:
		c.Deleted = true
		c.FirstOld = new("удален")
	case actField.VerbUpdated:
		c.Updated = true
		switch act.Field {
		case actField.LabelColor.Field:
			cf := c.CompositeFields["Цвет"]
			cf.Created = true
			cf.New = new("Обновлен")
			c.CompositeFields["Цвет"] = cf
		case actField.LabelName.Field:
			cf := c.CompositeFields["Имя"]
			cf.Updated = true
			if cf.Old == nil {
				cf.Old = new(act.OldValue)
			}
			cf.New = new(act.NewValue)
			c.CompositeFields["Имя"] = cf
		}
	}
}
