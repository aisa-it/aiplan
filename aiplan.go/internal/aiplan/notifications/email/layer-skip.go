package email

import (
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	member_role "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications/member-role"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"gorm.io/gorm"
)

var layerSkip types.EntityLayer = -1

type SkipProcessor struct {
	*BaseProcessor
	plan *emailPlan
}

func (s SkipProcessor) LoadActivities(tx *gorm.DB) []dao.ActivityEvent {
	var activities []dao.ActivityEvent
	if err := tx.Unscoped().
		Joins("Workspace").
		Joins("Actor").
		Order("activity_events.created_at").
		Where("entity_type IN (?)", []types.EntityLayer{types.LayerRoot, types.LayerForm}).
		Where("notified = ?", false).
		Limit(100).
		Find(&activities).Error; err != nil {
		return []dao.ActivityEvent{}
	}
	return activities
}

func (s SkipProcessor) GroupActivities(acts []dao.ActivityEvent) ActivityBuckets {
	return GroupActivitiesByLayer(
		acts,
		func(a dao.ActivityEvent) dao.IDaoAct {
			return dao.Workspace{} // группирует все в одну группу
		},
	)
}

func (s SkipProcessor) BuildRecipients(tx *gorm.DB, acts []dao.ActivityEvent, entity dao.IDaoAct) ([]member_role.MemberNotify, EmailContext) {
	ctx := EmailContext{
		Plan:     s.plan,
		Settings: member_role.FromWorkspace(),
		Steps:    []member_role.UsersStep{},
	}
	return BuildRecipientsFromActivities(tx, acts, &ctx)
}

func (s SkipProcessor) BuildDigest(tx *gorm.DB, templates *EmailTemplates, acts []dao.ActivityEvent, entity dao.IDaoAct) (map[string]FieldPrerender, int) {
	return renderDigest(tx, templates, acts, entity, nil)
}

func NewSkipActivitiesPipeline() (types.EntityLayer, EmailProcessor) {
	layer := layerSkip
	return layer, &SkipProcessor{
		BaseProcessor: NewBaseProcessor(),
		plan: &emailPlan{
			AuthorRole: member_role.ActionAuthor,
			EntityType: layer,
		},
	}
}
