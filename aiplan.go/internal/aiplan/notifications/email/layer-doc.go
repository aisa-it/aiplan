package email

import (
	"fmt"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	member_role "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications/member-role"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

type DocProcessor struct {
	*BaseProcessor
	plan *emailPlan
}

func NewDocPipeline() (types.EntityLayer, EmailProcessor) {
	layer := types.LayerDoc
	return layer, &DocProcessor{
		BaseProcessor: NewBaseProcessor(),
		plan: &emailPlan{
			AuthorRole: member_role.ActionAuthor,
			EntityType: layer,
		},
	}
}

var docFieldConfigs = map[actField.ActivityField]EntityFieldConfig{
	actField.Title.Field:       {collectOne, createFieldRenderer("Название", StringField)},
	actField.Description.Field: {collectOne, createFieldRenderer("Описание", BodyField)},
	actField.Doc.Field:         {collectAll, makeEntityComplexRenderer("Документы", WithComplexAggregateFunc(docMoveFunc))},
	actField.Readers.Field:     {collectAll, renderDocReaders},
	actField.Editors.Field:     {collectAll, renderDocEditors},
	actField.Watchers.Field:    {collectAll, renderDocWatchers},
	actField.ReaderRole.Field:  {collectOne, createFieldRenderer("Роль просмотра", StringField, WithTranslation(types.RoleTranslation))},
	actField.EditorRole.Field:  {collectOne, createFieldRenderer("Роль редактирования", StringField, WithTranslation(types.RoleTranslation))},

	actField.Comment.Field:    {collectAll, makeEntityComplexRenderer("Комментарии", WithActionTime(targetDateTimeZ), WithReplaceHtml(), WithTitleFunc(getAuthorTitle), WithComplexBlock())},
	actField.Attachment.Field: {collectAll, makeEntityComplexRenderer("Вложения")},
}

func (d DocProcessor) LoadActivities(tx *gorm.DB) []dao.ActivityEvent {
	var activities []dao.ActivityEvent
	if err := tx.Unscoped().
		Joins("Doc").
		Joins("Workspace").
		Joins("Actor").
		Order("activity_events.created_at").
		Where("activity_events.entity_type = ?", types.LayerDoc).
		Where("activity_events.notified = ?", false).
		Limit(100).
		Find(&activities).Error; err != nil {
		return []dao.ActivityEvent{}
	}
	return activities
}

func (d DocProcessor) FullLoad(tx *gorm.DB, entity dao.IDaoAct) dao.IDaoAct {
	doc := entity.(*dao.Doc)
	if err := tx.Unscoped().
		Joins("Author").
		Preload("AccessRules.Member").
		First(&doc).Error; err != nil {
		return entity
	}
	return doc
}

func (d DocProcessor) GroupActivities(acts []dao.ActivityEvent) ActivityBuckets {
	return GroupActivitiesByLayer(
		acts,
		func(a dao.ActivityEvent) dao.IDaoAct {
			a.Doc.Workspace = a.Workspace
			a.Doc.SetUrl()
			return a.Doc
		},
	)
}

func (d DocProcessor) BuildRecipients(tx *gorm.DB, acts []dao.ActivityEvent, entity dao.IDaoAct) ([]member_role.MemberNotify, EmailContext) {
	steps := []member_role.UsersStep{
		member_role.AddWorkspaceAdmins(entity.GetId()),
		member_role.AddDocMembers(entity.GetId()),
	}

	ctx := EmailContext{
		Plan:     d.plan,
		Settings: member_role.FromWorkspace(),
		Steps:    steps,
	}

	return BuildRecipientsFromActivities(tx, acts, &ctx)
}

func (d DocProcessor) BuildDigest(tx *gorm.DB, templates *EmailTemplates, acts []dao.ActivityEvent, entity dao.IDaoAct) (map[string]FieldPrerender, int) {
	return renderDigest(tx, templates, acts, entity, docFieldConfigs)
}

func (d *DocProcessor) BuildSubject(entity dao.IDaoAct) string {
	doc, ok := entity.(*dao.Doc)
	if !ok || doc == nil {
		return ""
	}
	return fmt.Sprintf("Обновления документа %s", doc.Title)
}

func (d *DocProcessor) BuildHead(templates *EmailTemplates, entity dao.IDaoAct) string {
	doc, ok := entity.(*dao.Doc)
	if !ok || doc == nil {
		return ""
	}
	head := headEntityCtx{
		WorkspaceName: doc.Workspace.Slug,
		Layer:         "документ",
		Identifier:    fmt.Sprint(doc.Title),
		Title:         doc.Title,
		Url:           doc.URL.String(),
		UrlText:       "Посмотреть документ",
	}

	return templates.RenderHead(head)
}

func renderDocReaders(tx *gorm.DB, t *EmailTemplates, acts []dao.ActivityEvent, entity dao.IDaoAct) FieldPrerender {
	doc, ok := entity.(*dao.Doc)
	if !ok || doc == nil {
		return FieldPrerender{}
	}
	return renderEntityChange(tx, t, acts,
		*doc.Readers,
		"Читатели",
		entitySpec[dao.User]{
			entityID: func(event dao.ActivityEvent) uuid.UUID {
				return getUUIDFromActivity(uuidPtrFrom(event.NewDocReader), uuidPtrFrom(event.OldDocReader))
			},
			entityTitle: func(i dao.User) string { return i.GetName() },
			loadRemoved: getRemovedMembers,
		})
}

func renderDocWatchers(tx *gorm.DB, t *EmailTemplates, acts []dao.ActivityEvent, entity dao.IDaoAct) FieldPrerender {
	doc, ok := entity.(*dao.Doc)
	if !ok || doc == nil {
		return FieldPrerender{}
	}
	return renderEntityChange(tx, t, acts,
		*doc.Watchers,
		"Наблюдатели",
		entitySpec[dao.User]{
			entityID: func(event dao.ActivityEvent) uuid.UUID {
				return getUUIDFromActivity(uuidPtrFrom(event.NewDocWatcher), uuidPtrFrom(event.OldDocWatcher))
			},
			entityTitle: func(i dao.User) string { return i.GetName() },
			loadRemoved: getRemovedMembers,
		})
}

func renderDocEditors(tx *gorm.DB, t *EmailTemplates, acts []dao.ActivityEvent, entity dao.IDaoAct) FieldPrerender {
	doc, ok := entity.(*dao.Doc)
	if !ok || doc == nil {
		return FieldPrerender{}
	}
	return renderEntityChange(tx, t, acts,
		*doc.Editors,
		"Редакторы",
		entitySpec[dao.User]{
			entityID: func(event dao.ActivityEvent) uuid.UUID {
				return getUUIDFromActivity(uuidPtrFrom(event.NewDocEditor), uuidPtrFrom(event.OldDocEditor))
			},
			entityTitle: func(i dao.User) string { return i.GetName() },
			loadRemoved: getRemovedMembers,
		})
}

func docMoveFunc(c *entityChange, act dao.ActivityEvent) {
	c.Title = utils.ToPtr("текущий")
	c.Info = true
	switch act.Verb {
	case actField.VerbMoveDocWorkspace:
		c.LastNew = utils.ToPtr(fmt.Sprintf("перенесен в корень из \"%s\"", act.OldValue))
	case actField.VerbMoveDocDoc:
		c.LastNew = utils.ToPtr(fmt.Sprintf("перенесен из \"%s\" в \"%s\"", act.OldValue, act.NewValue))
	case actField.VerbMoveWorkspaceDoc:
		c.LastNew = utils.ToPtr(fmt.Sprintf("перенесен из корня в \"%s\"", act.OldValue))
	case actField.VerbAdded:
		c.Info = false
		c.Created = true
		c.Title = utils.ToPtr(act.NewValue)
		c.LastNew = utils.ToPtr(fmt.Sprintf("добавлен"))
	case actField.VerbRemoved:
		c.Info = false
		c.Deleted = true
		c.Title = utils.ToPtr(act.OldValue)
		c.FirstOld = utils.ToPtr(fmt.Sprintf("перенесен"))
	case actField.VerbCreated:
		c.Info = false
		c.Created = true
		c.Title = utils.ToPtr(act.NewValue)
		c.LastNew = utils.ToPtr(fmt.Sprintf("создан вложеный"))
	case actField.VerbDeleted:
		c.Info = false
		c.Deleted = true
		c.Title = utils.ToPtr(act.OldValue)
		c.FirstOld = utils.ToPtr(fmt.Sprintf("удален"))
	}
}
