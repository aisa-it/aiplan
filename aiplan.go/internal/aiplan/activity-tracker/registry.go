package tracker

import (
	"reflect"
	"sync"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"gorm.io/gorm"
)

var registryCache sync.Map // key: reflect.Type

func getRegistry[E dao.IDaoAct]() map[string]funcVerb[E] {
	t := reflect.TypeOf((*E)(nil)).Elem()

	if v, ok := registryCache.Load(t); ok {
		return v.(map[string]funcVerb[E])
	}

	reg := buildRegistry[E]()
	registryCache.LoadOrStore(t, reg)
	return reg
}

type funcVerb[E dao.IDaoAct] func(c *ActivityCtx, entity E) ([]dao.ActivityEvent, error)

func fieldFuncReq[E dao.IDaoAct](rrr string) funcVerb[E] {
	registry := getRegistry[E]()
	return registry[rrr]
}

func buildRegistry[E dao.IDaoAct]() map[string]funcVerb[E] {
	return map[string]funcVerb[E]{

		// simple fields
		actField.Title.Req:           actSingle[E](actField.Title.Field),
		actField.Emoj.Req:            actSingle[E](actField.Emoj.Field),
		actField.Public.Req:          actSingle[E](actField.Public.Field),
		actField.Identifier.Req:      actSingle[E](actField.Identifier.Field),
		actField.ProjectLead.Req:     actSingle[E](actField.ProjectLead.Field),
		actField.Priority.Req:        actSingle[E](actField.Priority.Field),
		actField.Role.Req:            actSingle[E](actField.Role.Field),
		actField.Name.Req:            actSingle[E](actField.Name.Field),
		actField.Template.Req:        actSingle[E](actField.Template.Field),
		actField.Logo.Req:            actSingle[E](actField.Logo.Field),
		actField.Token.Req:           actSingle[E](actField.Token.Field),
		actField.Owner.Req:           actSingle[E](actField.Owner.Field), // TODO CHECK
		actField.Description.Req:     actSingle[E](actField.Description.Field),
		actField.DescriptionHtml.Req: actSingleMappedField[E](actField.Description.Field, actField.DescriptionHtml),
		actField.Color.Req:           actSingle[E](actField.Color.Field),

		actField.TargetDate.Req:  actDateField[E](DateFieldConfig{Field: actField.TargetDate.Field, FormatLayout: "2006-01-02T15:04:05Z07:00"}),
		actField.StartDate.Req:   actDateField[E](DateFieldConfig{Field: actField.StartDate.Field, FormatLayout: "02.01.2006 15:04 MST", UnwrapTimeMap: true}),
		actField.CompletedAt.Req: actDateField[E](DateFieldConfig{Field: actField.CompletedAt.Field, FormatLayout: "02.01.2006 15:04 MST", UnwrapTimeMap: true, SkipIfNil: true}),
		actField.EndDate.Req:     actDateField[E](DateFieldConfig{Field: actField.EndDate.Field, FormatLayout: "02.01.2006 15:04 MST", UnwrapTimeMap: true}),

		actField.AuthRequire.Req:   actSingle[E](actField.AuthRequire.Field),
		actField.Fields.Req:        actSingle[E](actField.Fields.Field),
		actField.Group.Req:         actSingle[E](actField.Group.Field),
		actField.Default.Req:       actSingle[E](actField.Default.Field),
		actField.EstimatePoint.Req: actSingle[E](actField.EstimatePoint.Field),
		actField.Url.Req:           actSingle[E](actField.Url.Field),
		actField.Comment.Req:       actSingle[E](actField.CommentHtml.Field),
		actField.DocSort.Req:       actSingle[E](actField.DocSort.Field), //TODO check
		actField.ReaderRole.Req:    actSingle[E](actField.ReaderRole.Field),
		actField.EditorRole.Req:    actSingle[E](actField.EditorRole.Field),
		actField.Status.Req:        actSingle[E](actField.Status.Field),

		actField.Assignees.Req:        actSeveral[E, dao.User](actField.Assignees),
		actField.Watchers.Req:         actSeveral[E, dao.User](actField.Watchers),
		actField.Editors.Req:          actSeveral[E, dao.User](actField.Editors),
		actField.Readers.Req:          actSeveral[E, dao.User](actField.Readers),
		actField.DefaultAssignees.Req: actSeveral[E, dao.User](actField.DefaultAssignees),
		actField.DefaultWatchers.Req:  actSeveral[E, dao.User](actField.DefaultWatchers),

		actField.Label.Req:  actSeveral[E, dao.Label](actField.Label),
		actField.Issues.Req: actSeveral[E, dao.Issue](actField.Issues),
		actField.Sprint.Req: actSeveral[E, dao.Issue](actField.Issues),

		// linked
		actField.Linked.Req:   actLinked[E](actField.Linked, &actField.Linked.Field, &actField.Linked.Field, nil),
		actField.Blocks.Req:   actLinked[E](actField.Blocks, &actField.Blocks.Field, &actField.Blocking.Field, &keyExtractor{"blocked_issues", "block"}),
		actField.Blocking.Req: actLinked[E](actField.Blocking, &actField.Blocking.Field, &actField.Blocks.Field, &keyExtractor{"blocker_issues", "blocked_by"}),

		actField.Parent.Req: parentUpdate[E](actField.Parent.Field),
	}
}

func getEntities[T dao.IDaoAct](tx *gorm.DB, changes *utils.IDChangeSet) ([]T, error) {
	var involvedEntities []T

	query := tx.Model(new(T))
	if _, ok2 := any(new(T)).(*dao.Issue); ok2 {
		query = query.Where("issues.id in (?)", changes.InvolvedIds).Joins("Project")
	} else {
		query = query.Where("id in (?)", changes.InvolvedIds)
	}
	if err := query.
		Find(&involvedEntities).Error; err != nil {
		return nil, err
		//return nil, ErrStack.TrackErrorStack(err).AddContext("field", act.Field.String())
	}
	return involvedEntities, nil
}
