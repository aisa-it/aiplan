package tracker

import (
	"fmt"
	"log/slog"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	ErrStack "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/stack-error"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ActHandler interface {
	Handle(activity dao.ActivityEvent) error
}
type ActTracker struct {
	db       *gorm.DB
	handlers []ActHandler
}

func NewTracker(db *gorm.DB) *ActTracker {
	tracker := ActTracker{db: db}
	return &tracker
}

func (t *ActTracker) RegisterHandler(handler ActHandler) {
	t.handlers = append(t.handlers, handler)
}

func (t *ActTracker) RunHandlers(activity dao.ActivityEvent) {

	for _, handler := range t.handlers {
		err := handler.Handle(activity)
		if err != nil {
			slog.Error("handler failed", "error", err)
		}
	}
}

func TrackEvent[E dao.IDaoAct](tracker *ActTracker, layer types.EntityLayer, activityAction string,
	ctx *Ctx, entity E, actor *dao.User) error {
	c := NewActCtx(tracker, ctx, actor, layer)
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
			activity = confSkipper(activity, c.RequestedData)
			tracker.RunHandlers(activity)
		}
	}

	return nil
}

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
	case actField.VerbMove:
		return move[E]

	}
	return nil
}

func confSkipper(act dao.ActivityEvent, requestedData map[string]interface{}) dao.ActivityEvent {
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
	default:
		return act
	}
	return act
}
