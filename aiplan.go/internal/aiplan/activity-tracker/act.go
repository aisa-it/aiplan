package tracker

import (
	"fmt"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	ErrStack "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/stack-error"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ActivityCtx struct {
	Tracker         *ActivitiesTracker
	RequestedData   DataEntity
	CurrentInstance DataEntity
	Actor           *dao.User
	Layer           types.EntityLayer
}

//type funcVerb[E dao.Entity] func(c *ActivityCtx, entity E) ([]dao.ActivityEvent, error)

func getActCtx(t *ActivitiesTracker, req, cur map[string]interface{}, actor *dao.User, layer types.EntityLayer) *ActivityCtx {
	return &ActivityCtx{
		Tracker:         t,
		RequestedData:   req,
		CurrentInstance: cur,
		Actor:           actor,
		Layer:           layer,
	}
}

type funcVerb[E dao.Entity] func(c *ActivityCtx, entity E) ([]dao.ActivityEvent, error)

func getVerbFunc[E dao.Entity](activityType string) funcVerb[E] {
	switch activityType {
	case actField.VerbUpdated:
		return update[E]
	case actField.VerbAdded:
		return add[E]
	case actField.VerbRemoved:
		return remove[E]
	}
	return nil
}

func TrackAct[E dao.Entity](
	tracker *ActivitiesTracker,
	layer types.EntityLayer, activityAction string,
	requestedData DataEntity, currentInstance DataEntity,
	entity E, actor *dao.User,
) error {
	c := getActCtx(tracker, requestedData, currentInstance, actor, layer)
	verbFunc := getVerbFunc[E](activityAction)

	if verbFunc == nil {
		return ErrStack.TrackErrorStack(fmt.Errorf("not activity function")).
			AddContext("activity_action", activityAction).
			AddContext("entity", fmt.Sprintf("%T", entity))
	}

	activities, err := verbFunc(c, entity)
	if err != nil {
		return ErrStack.TrackErrorStack(err)
	}

	if len(activities) > 0 {
		if err := tracker.db.Omit(clause.Associations).Create(&activities).Error; err != nil {
			return err
		}

		//for _, activity := range activities {
		//	err := dao.EntityActivityAfterFind(&activity, tracker.db)
		//	if err != nil {
		//		ErrStack.GetError(nil, ErrStack.TrackErrorStack(err))
		//		continue
		//	}
		//	activity = confSkipper(activity, requestedData)
		//	if a, ok := any(activity).(dao.ActivityI); ok {
		//		tracker.RunHandlers(a)
		//	}
		//}
	}

	return nil
}

func gggg[E dao.Entity](rrr string) funcVerb[E] {
	switch rrr {
	case actField.Title.Req:
		return actSingleWithoutIdentifier[E](actField.Title.Field)
	case actField.Emoj.Req:
		return actSingleWithoutIdentifier[E](actField.Emoj.Field)
	case actField.Public.Req:
		return actSingleWithoutIdentifier[E](actField.Public.Field)
	case actField.Identifier.Req:
		return actSingleWithoutIdentifier[E](actField.Identifier.Field)
	case actField.Priority.Req:
		return actSingleWithoutIdentifier[E](actField.Priority.Field)
	case actField.Name.Req:
		return actSingleWithoutIdentifier[E](actField.Name.Field)
	case actField.Template.Req:
		return actSingleWithoutIdentifier[E](actField.Template.Field)
	case actField.Logo.Req:
		return actSingleWithoutIdentifier[E](actField.Logo.Field)
	case actField.Token.Req:
		return actSingleWithoutIdentifier[E](actField.Token.Field)
	case actField.Owner.Req:
		return actSingleWithoutIdentifier[E](actField.Owner.Field)
	case actField.Description.Req:
		return actSingleWithoutIdentifier[E](actField.Description.Field)
	case actField.Color.Req:
		return actSingleWithoutIdentifier[E](actField.Color.Field)
	case actField.AuthRequire.Req:
		return actSingleWithoutIdentifier[E](actField.AuthRequire.Field)
	case actField.Fields.Req:
		return actSingleWithoutIdentifier[E](actField.Fields.Field)
	case actField.Group.Req:
		return actSingleWithoutIdentifier[E](actField.Group.Field)
	case actField.Default.Req:
		return actSingleWithoutIdentifier[E](actField.Default.Field)
	case actField.EstimatePoint.Req:
		return actSingleWithoutIdentifier[E](actField.EstimatePoint.Field)
	case actField.Url.Req:
		return actSingleWithoutIdentifier[E](actField.Url.Field)
	case actField.CommentHtml.Req:
		return actSingleWithoutIdentifier[E](actField.CommentHtml.Field)
	case actField.ReaderRole.Req:
		return actSingleWithoutIdentifier[E](actField.ReaderRole.Field)
	case actField.EditorRole.Req:
		return actSingleWithoutIdentifier[E](actField.EditorRole.Field)

	case actField.DescriptionHtml.Req:
		return func(c *ActivityCtx, entity E) ([]dao.ActivityEvent, error) {
			c.RequestedData["field_log"] = actField.Description.Field
			c.RequestedData[actField.DescriptionHtml.Field.WithActivityValStr()] = c.RequestedData[actField.DescriptionHtml.Req]
			c.CurrentInstance[actField.DescriptionHtml.Field.WithActivityValStr()] = c.CurrentInstance[actField.DescriptionHtml.Req]
			return actSingleWithoutIdentifier[E](actField.DescriptionHtml.Field)(c, entity)
		}

		/////////////////
	case actField.Assignees.Req:
		return acteeee[E, dao.User](actField.Assignees, nil)
	case actField.Watchers.Req:
		return acteeee[E, dao.User](actField.Watchers, nil)
	case actField.Editors.Req:
		return acteeee[E, dao.User](actField.Editors, nil)
	case actField.Readers.Req:
		return acteeee[E, dao.User](actField.Readers, nil)
	case actField.DefaultAssignees.Req:
		return acteeee[E, dao.User](actField.DefaultAssignees, nil)
	case actField.DefaultWatchers.Req:
		return acteeee[E, dao.User](actField.DefaultWatchers, nil)

	case actField.Label.Req:
		return acteeee[E, dao.Label](actField.Label, nil)

	case actField.Issues.Req:
		return acteeee[E, dao.Issue](actField.Issues, nil) // TODO проверить actField.Issues.Field
	case actField.Sprint.Req:
		return acteeee[E, dao.Issue](actField.Issues, nil) // TODO проверить actField.Issues.Field
	}
	return nil
}

// Создает новую активность шаблона.
func NewTeActy(verb string, field actField.ActivityField, oldVal *string, newVal string, newId, oldId uuid.NullUUID, actor *dao.User) dao.ActivityEvent {

	return dao.ActivityEvent{
		ID:            dao.GenUUID(),
		WorkspaceID:   uuid.NullUUID{},
		ProjectID:     uuid.NullUUID{},
		IssueID:       uuid.NullUUID{},
		DocID:         uuid.NullUUID{},
		FormID:        uuid.NullUUID{},
		SprintID:      uuid.NullUUID{},
		EntityType:    0,
		ActorID:       actor.ID,
		Notified:      false,
		Verb:          verb,
		Field:         field,
		OldValue:      oldVal,
		NewValue:      newVal,
		NewIdentifier: newId,
		OldIdentifier: oldId,
	}
}

func Gettt[E dao.Entity](layer types.EntityLayer, act *dao.ActivityEvent, entity E) error {
	act.EntityType = layer

	switch layer {
	case types.EntityRoot:
		return nil
	case types.EntityWorkspace:
		if e, ok := any(entity).(dao.WorkspaceEntityI); ok {
			act.WorkspaceID = uuid.NullUUID{UUID: e.GetWorkspaceId(), Valid: true}
		} else {
			return fmt.Errorf("entity is not WorkspaceEntity")
		}
	case types.EntityProject:
		if e, ok := any(entity).(dao.ProjectEntityI); ok {
			act.WorkspaceID = uuid.NullUUID{UUID: e.GetWorkspaceId(), Valid: true}
			act.ProjectID = uuid.NullUUID{UUID: e.GetProjectId(), Valid: true}
		} else {
			return fmt.Errorf("entity is not ProjectEntity")
		}
	case types.EntityIssue:
		if e, ok := any(entity).(dao.IssueEntityI); ok {
			act.WorkspaceID = uuid.NullUUID{UUID: e.GetWorkspaceId(), Valid: true}
			act.ProjectID = uuid.NullUUID{UUID: e.GetProjectId(), Valid: true}
			act.IssueID = uuid.NullUUID{UUID: e.GetIssueId(), Valid: true}
		} else {
			return fmt.Errorf("entity is not IssueEntity")
		}
	case types.EntityDoc:
		if e, ok := any(entity).(dao.DocEntityI); ok {
			act.WorkspaceID = uuid.NullUUID{UUID: e.GetWorkspaceId(), Valid: true}
			act.DocID = uuid.NullUUID{UUID: e.GetDocId(), Valid: true}
		} else {
			return fmt.Errorf("entity is not DocEntity")
		}
	case types.EntityForm:
		if e, ok := any(entity).(dao.FormEntityI); ok {
			act.WorkspaceID = uuid.NullUUID{UUID: e.GetWorkspaceId(), Valid: true}
			act.FormID = uuid.NullUUID{UUID: e.GetFormId(), Valid: true}
		} else {
			return fmt.Errorf("entity is not FormEntity")
		}
	case types.EntitySprint:
		if e, ok := any(entity).(dao.SprintEntityI); ok {
			act.WorkspaceID = uuid.NullUUID{UUID: e.GetWorkspaceId(), Valid: true}
			act.SprintID = uuid.NullUUID{UUID: e.GetSprintId(), Valid: true}
		} else {
			return fmt.Errorf("entity is not SprintEntity")
		}
	}
	return nil
}

func (a *ActivityCtx) getEntities(act actField.FieldMapping) ([]interface{}, []interface{}) {
	f := act.Field.String()
	if v, ok := a.RequestedData[act.Field.WithGetFieldStr()]; ok {
		f = v.(string)
	}
	return a.RequestedData[act.Req].([]interface{}), a.CurrentInstance[f].([]interface{})
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
