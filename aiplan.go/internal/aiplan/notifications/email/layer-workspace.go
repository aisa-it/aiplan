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

type WorkspaceProcessor struct {
	*BaseProcessor
	plan *emailPlan
}

var workspaceFieldConfigs = map[actField.ActivityField]EntityFieldConfig{
	actField.Name.Field:        {collectOne, createFieldRenderer("Имя", StringField)},
	actField.Description.Field: {collectOne, createFieldRenderer("Описание", BodyField)},
	actField.Project.Field:     {collectAll, makeEntityComplexRenderer("Проекты")},
	actField.Doc.Field:         {collectAll, makeEntityComplexRenderer("Документы")},
	actField.Form.Field:        {collectAll, makeEntityComplexRenderer("Формы")},
	actField.Sprint.Field:      {collectAll, makeEntityComplexRenderer("Спринты")},

	actField.Token.Field:  {collectOne, createFieldRenderer("Токен", StringField, WithCustomText("изменен токен пространства"))},
	actField.Member.Field: {collectAll, renderWorkspaceMember},

	actField.Logo.Field: {collectOne, createFieldRenderer("Логотип", StringField, WithCustomText("изменен логотип пространства"))},
	actField.Role.Field: {collectAll, renderWorkspaceMemberRole},
}

func NewWorkspacePipeline() (types.EntityLayer, EmailProcessor) {
	layer := types.LayerWorkspace
	return layer, &WorkspaceProcessor{
		BaseProcessor: NewBaseProcessor(),
		plan: &emailPlan{
			AuthorRole: member_role.ActionAuthor,
			EntityType: layer,
		},
	}
}

func (w WorkspaceProcessor) LoadActivities(tx *gorm.DB) []dao.ActivityEvent {
	var activities []dao.ActivityEvent
	if err := tx.Unscoped().
		Joins("Workspace").
		Joins("Actor").
		Order("activity_events.created_at").
		Where("entity_type = ?", types.LayerWorkspace).
		Where("notified = ?", false).
		Limit(100).
		Find(&activities).Error; err != nil {
		return []dao.ActivityEvent{}
	}
	return activities
}

func (w WorkspaceProcessor) GroupActivities(acts []dao.ActivityEvent) ActivityBuckets {
	return GroupActivitiesByLayer(
		acts,
		func(a dao.ActivityEvent) dao.IDaoAct {
			a.Workspace.SetUrl()
			return a.Workspace
		},
	)
}

func (w WorkspaceProcessor) BuildRecipients(tx *gorm.DB, acts []dao.ActivityEvent, entity dao.IDaoAct) ([]member_role.MemberNotify, EmailContext) {
	steps := []member_role.UsersStep{
		member_role.AddWorkspaceAdmins(entity.GetId()),
	}

	ctx := EmailContext{
		Plan:     w.plan,
		Settings: member_role.FromWorkspace(),
		Steps:    steps,
	}

	return BuildRecipientsFromActivities(tx, acts, &ctx)
}

func (w WorkspaceProcessor) BuildDigest(tx *gorm.DB, templates *EmailTemplates, acts []dao.ActivityEvent, entity dao.IDaoAct) (map[string]FieldPrerender, int) {
	return renderDigest(tx, templates, acts, entity, workspaceFieldConfigs)

}

func (w *WorkspaceProcessor) BuildSubject(entity dao.IDaoAct) string {
	workspace, ok := entity.(*dao.Workspace)
	if !ok || workspace == nil {
		return ""
	}
	return fmt.Sprintf("Обновления пространства %s: (%s)", workspace.Slug, workspace.Name)
}

func (w *WorkspaceProcessor) BuildHead(templates *EmailTemplates, entity dao.IDaoAct) string {
	workspace, ok := entity.(*dao.Workspace)
	if !ok || workspace == nil {
		return ""
	}
	head := headEntityCtx{
		WorkspaceName: workspace.Slug,
		Layer:         "пространство",
		Identifier:    "",
		Title:         workspace.Name,
		Url:           workspace.URL.String(),
		UrlText:       "Посмотреть пространство",
	}
	return templates.RenderHead(head)
}

func renderWorkspaceMember(tx *gorm.DB, t *EmailTemplates, acts []dao.ActivityEvent, entity dao.IDaoAct) FieldPrerender {
	workspace, ok := entity.(*dao.Workspace)
	if !ok || workspace == nil {
		return FieldPrerender{}
	}

	var members []dao.WorkspaceMember
	if err := tx.Preload("Member").Where("workspace_id = ?", workspace.ID).Find(&members).Error; err != nil {
		return FieldPrerender{}
	}

	memberMap := utils.SliceToMap(&members, func(v *dao.WorkspaceMember) uuid.UUID {
		return v.MemberId
	})

	return renderEntityChange(tx, t, acts,
		utils.SliceToSlice(&members, func(t *dao.WorkspaceMember) dao.User {
			return *t.Member
		}),
		"Участники",
		entitySpec[dao.User]{
			entityID: func(event dao.ActivityEvent) uuid.UUID {
				return getUUIDFromActivity(uuidPtrFrom(event.WorkspaceActivityExtendFields.NewMember), uuidPtrFrom(event.WorkspaceActivityExtendFields.OldMember))
			},
			entityTitle: func(i dao.User) string {
				return fmt.Sprintf("%s (%s)", i.GetName(), types.TranslateMap(types.RoleTranslation, utils.ToPtr(fmt.Sprint(memberMap[i.GetId()].Role))))
			},
			loadRemoved: getRemovedMembers,
		})
}

func renderWorkspaceMemberRole(tx *gorm.DB, t *EmailTemplates, acts []dao.ActivityEvent, entity dao.IDaoAct) FieldPrerender {
	return renderEntityChangeComplex(tx, t, acts, "Роль участника",
		WithTitleFunc(func(act *dao.ActivityEvent) *string {
			return utils.ToPtr(act.EntityMemberExtendFields.NewRole.GetName())
		}),
		WithReplaceFunc(func(str *string) *string {
			if str == nil {
				return nil
			}
			return utils.ToPtr(types.RoleTranslation[*str])
		}),
	)
}
