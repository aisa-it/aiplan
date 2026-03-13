package tracker

import (
	"fmt"
	"log/slog"
	"reflect"
	"sync"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	ErrStack "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/stack-error"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func NewTracker(db *gorm.DB) *ActTracker {
	tracker := ActTracker{db: db}
	return &tracker
}

type ActivityCtx struct {
	Tracker         *ActTracker
	RequestedData   DataEntity
	CurrentInstance DataEntity
	Actor           *dao.User
	Layer           types.EntityLayer
}

func (a *ActivityCtx) Requested() Payload {
	return NewPayload(a.RequestedData)
}

func (a *ActivityCtx) Current() Payload {
	return NewPayload(a.CurrentInstance)
}

func (a *ActivityCtx) ResolveField(field actField.ActivityField) actField.ActivityField {
	field = a.applyScope(field)
	field = a.applyFieldLogOverride(field)
	return field
}

func (a *ActivityCtx) applyScope(field actField.ActivityField) actField.ActivityField {
	scope := a.scopeFromCurrent()
	if scope == "" {
		scope = a.scopeFromRequested()
	}

	if scope == "" {
		return field
	}

	return actField.ActivityField(fmt.Sprintf("%s_%s", scope, field))
}

func (a *ActivityCtx) scopeFromCurrent() string {
	scope, _ := GetAs[string](a.CurrentInstance, actField.UpdateScopeKey)
	return scope
}

func (a *ActivityCtx) scopeFromRequested() string {
	scope, _ := GetAs[string](a.RequestedData, actField.UpdateScopeKey)
	return scope
}

func (a *ActivityCtx) applyFieldLogOverride(field actField.ActivityField) actField.ActivityField {
	if override, ok := GetAs[actField.ActivityField](a.RequestedData, actField.FieldLogKey); ok {
		return override
	}

	if override, ok := GetAs[actField.ActivityField](a.RequestedData, field.WithFieldLog()); ok {
		return override
	}

	return field
}

func getActCtx(t *ActTracker, req, cur map[string]interface{}, actor *dao.User, layer types.EntityLayer) *ActivityCtx {
	return &ActivityCtx{Tracker: t, RequestedData: req, CurrentInstance: cur, Actor: actor, Layer: layer}
}

type funcVerb[E dao.IDaoAct] func(c *ActivityCtx, entity E) ([]dao.ActivityEvent, error)

func getVerbFunc[E dao.IDaoAct](activityType string) funcVerb[E] {
	switch activityType {
	case actField.VerbUpdated:
		return update[E]
	case actField.VerbAdded:
		return add[E]
	case actField.VerbRemoved:
		return remove[E]
	case actField.VerbCreated:
		return create[E]
	case actField.VerbDeleted:
		return del[E]

	}
	return nil
}

func TrackEvent[E dao.IDaoAct](tracker *ActTracker, layer types.EntityLayer, activityAction string,
	requestedData DataEntity, currentInstance DataEntity, entity E, actor *dao.User) error {
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

		for _, activity := range activities {
			if err := activity.AfterFind(tracker.db); err != nil {
				return err
			}
			//err := dao.EntityActivityAfterFind(&activity, tracker.db)
			//if err != nil {
			//	ErrStack.GetError(nil, ErrStack.TrackErrorStack(err))
			//	continue
			//}
			activity = confSkipper2(activity, requestedData)
			tracker.RunHandlers(activity)

		}
	}

	return nil
}

func confSkipper2(act dao.ActivityEvent, requestedData map[string]interface{}) dao.ActivityEvent {
	//var result dao.ActivityEvent
	switch act.EntityType {

	case types.LayerIssue:
		if v, ok := requestedData["tg_sender"]; ok {
			if val, intOk := v.(int64); intOk {
				act.SenderTg = val
			}
		}
	case types.LayerDoc:
		if v, ok := requestedData["tg_sender"]; ok {
			if val, intOk := v.(int64); intOk {
				act.SenderTg = val
			}
		}
		//default:
		//	result = act
	}
	return act
}

type ActHandler interface {
	Handle(activity dao.ActivityEvent) error
}
type ActTracker struct {
	db *gorm.DB

	handlers []ActHandler
}

func (t *ActTracker) RunHandlers(activity dao.ActivityEvent) {

	for _, handler := range t.handlers {
		err := handler.Handle(activity)
		if err != nil {
			slog.Error("handler failed", "error", err)
		}
	}
}

func (t *ActTracker) RegisterHandler(handler ActHandler) {
	t.handlers = append(t.handlers, handler)
}

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

func fieldFuncReq[E dao.IDaoAct](rrr string) funcVerb[E] {
	registry := getRegistry[E]()
	return registry[rrr]
}

func buildRegistry[E dao.IDaoAct]() map[string]funcVerb[E] {
	return map[string]funcVerb[E]{

		// simple fields
		actField.Title.Req:      actSingle[E](actField.Title.Field),
		actField.Emoj.Req:       actSingle[E](actField.Emoj.Field),
		actField.Public.Req:     actSingle[E](actField.Public.Field),
		actField.Identifier.Req: actSingle[E](actField.Identifier.Field),
		//actField.ProjectLead.Req: actSingle[E](actField.ProjectLead.Field),
		actField.Priority.Req: actSingle[E](actField.Priority.Field),
		//actField.Role.Req:   actSingle[E](actField.Role.Field),
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
	}
}

// Создает новую активность шаблона.
func NewActivityEvent(verb string, field actField.ActivityField, oldVal *string, newVal string, newId, oldId uuid.NullUUID, actor *dao.User) dao.ActivityEvent {

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
		Actor:         actor,
		Notified:      false,
		Verb:          verb,
		Field:         field,
		OldValue:      oldVal,
		NewValue:      newVal,
		NewIdentifier: newId,
		OldIdentifier: oldId,
	}
}

func SetEntityRefs[E dao.IDaoAct](layer types.EntityLayer, act *dao.ActivityEvent, entity E) error {
	act.EntityType = layer

	switch layer {
	case types.LayerRoot:
		return nil
	case types.LayerWorkspace:
		if e, ok := any(entity).(dao.WorkspaceEntityI); ok {
			act.WorkspaceID = uuid.NullUUID{UUID: e.GetWorkspaceId(), Valid: true}
		} else {
			return fmt.Errorf("entity is not WorkspaceEntity")
		}
	case types.LayerProject:
		if e, ok := any(entity).(dao.ProjectEntityI); ok {
			act.WorkspaceID = uuid.NullUUID{UUID: e.GetWorkspaceId(), Valid: true}
			act.ProjectID = uuid.NullUUID{UUID: e.GetProjectId(), Valid: true}
		} else {
			return fmt.Errorf("entity is not ProjectEntity")
		}
	case types.LayerIssue:
		if e, ok := any(entity).(dao.IssueEntityI); ok {
			act.WorkspaceID = uuid.NullUUID{UUID: e.GetWorkspaceId(), Valid: true}
			act.ProjectID = uuid.NullUUID{UUID: e.GetProjectId(), Valid: true}
			act.IssueID = uuid.NullUUID{UUID: e.GetIssueId(), Valid: true}
		} else {
			return fmt.Errorf("entity is not IssueEntity")
		}
	case types.LayerDoc:
		if e, ok := any(entity).(dao.DocEntityI); ok {
			act.WorkspaceID = uuid.NullUUID{UUID: e.GetWorkspaceId(), Valid: true}
			act.DocID = uuid.NullUUID{UUID: e.GetDocId(), Valid: true}
		} else {
			return fmt.Errorf("entity is not DocEntity")
		}
	case types.LayerForm:
		if e, ok := any(entity).(dao.FormEntityI); ok {
			act.WorkspaceID = uuid.NullUUID{UUID: e.GetWorkspaceId(), Valid: true}
			act.FormID = uuid.NullUUID{UUID: e.GetFormId(), Valid: true}
		} else {
			return fmt.Errorf("entity is not FormEntity")
		}
	case types.LayerSprint:
		if e, ok := any(entity).(dao.SprintEntityI); ok {
			act.WorkspaceID = uuid.NullUUID{UUID: e.GetWorkspaceId(), Valid: true}
			act.SprintID = uuid.NullUUID{UUID: e.GetSprintId(), Valid: true}
		} else {
			return fmt.Errorf("entity is not SprintEntity")
		}
	}
	return nil
}

func (a *ActivityCtx) getDiffData(act actField.FieldMapping) ([]interface{}, []interface{}) {
	f := GetAsOrDefault[string](a.RequestedData, act.Field.WithGetField(), act.Field.String())
	newE := GetAsOrDefault[[]interface{}](a.RequestedData, actField.New(act.Req).Only(), []interface{}{})
	oldE := GetAsOrDefault[[]interface{}](a.CurrentInstance, actField.New(f).Only(), []interface{}{})
	return newE, oldE
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
