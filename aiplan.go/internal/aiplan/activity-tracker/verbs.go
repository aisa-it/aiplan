package tracker

import (
	"fmt"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	ErrStack "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/stack-error"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/gofrs/uuid"
)

func update[E dao.IDaoAct](c *ActivityCtx, en E) ([]dao.ActivityEvent, error) {
	result := make([]dao.ActivityEvent, 0)
	for key := range c.RequestedData {
		if f := fieldFuncReq[E](key); f != nil {
			acts, err := f(c, en)
			if err != nil {
				return nil, ErrStack.TrackErrorStack(err)
			}
			result = append(result, acts...)
		}
	}
	return result, nil
}

func add[E dao.IDaoAct](c *ActivityCtx, entity E) ([]dao.ActivityEvent, error) {
	entity = GetAsOrDefault[E](c.RequestedData, actField.ParentKey, entity)
	entity = GetAsOrDefault[E](c.RequestedData, actField.EntityKey, entity)

	key := entity.GetEntityType()
	key = GetAsOrDefault[actField.ActivityField](c.RequestedData, key.WithKey(), key)

	newV := entity.GetString()
	newV = GetAsOrDefault[string](c.RequestedData, key.AsLogValue(), newV)
	newV = GetAsOrDefault[string](c.RequestedData, actField.New(newV).AsLogValue(), newV)

	newIdentifier := GetAsOrDefault[uuid.UUID](c.RequestedData, actField.NewKey(actField.KindScopeID), entity.GetId())

	change := activityChange[E]{verb: actField.VerbAdded, field: key, newVal: newV, newID: ToNullUUID(newIdentifier), entity: entity}

	return buildEvents(c, []activityChange[E]{change})
}

func remove[E dao.IDaoAct](c *ActivityCtx, entity E) ([]dao.ActivityEvent, error) {
	current := GetAsOrDefault[E](c.CurrentInstance, actField.EntityKey, entity)

	oldIdentifier := GetAsOrDefault[uuid.UUID](c.RequestedData, actField.NewKey(actField.KindScopeID), entity.GetId())

	key := entity.GetEntityType()
	key = GetAsOrDefault[actField.ActivityField](c.RequestedData, key.WithKey(), key)

	oldV := entity.GetString()
	oldV = GetAsOrDefault[string](c.RequestedData, key.AsLogValue(), oldV)
	oldV = GetAsOrDefault[string](c.RequestedData, actField.New(oldV).AsLogValue(), oldV)

	change := activityChange[E]{verb: actField.VerbRemoved, field: key, oldVal: &oldV, oldID: ToNullUUID(oldIdentifier), entity: current}

	return buildEvents(c, []activityChange[E]{change})
}

func create[E dao.IDaoAct](c *ActivityCtx, entity E) ([]dao.ActivityEvent, error) {
	newIdentifier := GetAsOrDefault[uuid.UUID](c.RequestedData, actField.NewKey(actField.KindScopeID), entity.GetId())

	verb := GetAsOrDefault[string](c.RequestedData, actField.CustomVerbKey, actField.VerbCreated)

	newVal := GetAsOrDefault[string](c.RequestedData, actField.New(entity.GetEntityType()).AsLogValue(), entity.GetString())
	newVal = GetAsOrDefault[string](c.RequestedData, actField.New(newVal).AsLogValue(), newVal)

	entity = GetAsOrDefault[E](c.RequestedData, actField.EntityParentKey, entity)
	change := activityChange[E]{
		verb: verb, field: entity.GetEntityType(), newVal: newVal, newID: ToNullUUID(newIdentifier), entity: entity}

	return buildEvents(c, []activityChange[E]{change})
}

func del[E dao.IDaoAct](c *ActivityCtx, entity E) ([]dao.ActivityEvent, error) {
	oldVal := GetAsOrDefault[string](c.RequestedData, actField.OldTitleKey, entity.GetString())
	change := activityChange[E]{verb: actField.VerbDeleted, field: entity.GetEntityType(), oldVal: &oldVal, entity: entity}
	return buildEvents(c, []activityChange[E]{change})
}

// move — перемещение сущности в другую родительскую сущность.
//
//	Ожидаемые ключи в RequestedData:
//	 - parent_key: ключ поля родительской сущности (обязательно)
//	 - entity: дополнительный модификатор verb (опционально)
//	 - field_log: переопределённое имя поля (опционально)
//	 - updateScopeId: ID новой родительской сущности (опционально)
func move[E dao.IDaoAct](c *ActivityCtx, entity E) ([]dao.ActivityEvent, error) {
	parentKey, ok := GetAs[string](c.RequestedData, actField.ParentKey)
	if !ok || parentKey == "" {
		return nil, ErrStack.TrackErrorStack(fmt.Errorf("parent_key key is required for move"))
	}

	key := GetAsOrDefault[actField.ActivityField](c.RequestedData, actField.NewKey(actField.KindLogOverride), actField.ActivityField(parentKey))

	verb := actField.VerbMove
	if v, ok := GetAs[string](c.RequestedData, actField.FieldMoveKey); ok {
		verb = fmt.Sprintf("move_%s", v)
	}
	field := GetAsOrDefault[actField.ActivityField](c.RequestedData, actField.NewEntityKey, entity.GetEntityType())

	newId := GetAsOrDefault[uuid.UUID](c.RequestedData, key.AsKey(), uuid.Nil)
	oldId := GetAsOrDefault[uuid.UUID](c.CurrentInstance, key.AsKey(), uuid.Nil)

	newVal := GetAsOrDefault[string](c.RequestedData, actField.ParentTitleKey, "")
	oldVal := GetAsOrDefault[string](c.CurrentInstance, actField.ParentTitleKey, "")

	change := activityChange[E]{verb: verb, field: field, oldVal: &oldVal, newVal: newVal, newID: ToNullUUID(newId), oldID: ToNullUUID(oldId), entity: entity}

	return buildEvents(c, []activityChange[E]{change})
}
