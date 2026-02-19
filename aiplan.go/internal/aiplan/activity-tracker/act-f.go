package tracker

import (
	"fmt"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	ErrStack "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/stack-error"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/gofrs/uuid"
)

func update[E dao.Entity](c *ActivityCtx, en E) ([]dao.ActivityEvent, error) {
	result := make([]dao.ActivityEvent, 0)
	for key := range c.RequestedData {
		if f := gggg[E](key); f != nil {
			acts, err := f(c, en)
			if err != nil {
				return nil, ErrStack.TrackErrorStack(err)
			}
			result = append(result, acts...)
		}
	}
	return result, nil
}

func add[E dao.Entity](c *ActivityCtx, entity E) ([]dao.ActivityEvent, error) {
	result := make([]dao.ActivityEvent, 0)

	entityI, ok := any(entity).(dao.IDaoAct)
	if !ok {
		return nil, ErrStack.TrackErrorStack(fmt.Errorf("entity does not implement IEntity[A]"))
	}

	entity = GetAsOrDefault[E](c.RequestedData, ValueKey("entityParent"), entity)
	entity = GetAsOrDefault[E](c.RequestedData, ValueKey("entity"), entity)

	key := entityI.GetEntityType()
	key = GetAsOrDefault[actField.ActivityField](c.RequestedData, FieldWithKey(key), key)

	newV := entityI.GetString()
	newV = GetAsOrDefault[string](c.RequestedData, ValueKey(key), newV)
	newV = GetAsOrDefault[string](c.RequestedData, ValueKey(newV), newV)

	newIdentifier := GetAsOrDefault[uuid.UUID](c.RequestedData, ValueKey("updateScopeId"), entityI.GetId())

	templateActivity := NewTeActy(actField.VerbAdded, key, nil, newV, ToNullUUID(newIdentifier), uuid.NullUUID{}, c.Actor)
	if err := Gettt(c.Layer, &templateActivity, entity); err != nil {
		return result, nil
	}
	result = append(result, templateActivity)
	return result, nil

	//if newAct, err := CreateActivity[E, A](entity, templateActivity); err != nil {
	//	return nil, ErrStack.TrackErrorStack(err).AddContext("comment", templateActivity.Comment)
	//} else {
	//	return []dao.{*newAct}, nil
	//}
}

// Удаляет существующую сущность и генерирует запись в журнале активности.
func remove[E dao.Entity](c *ActivityCtx, entity E) ([]dao.ActivityEvent, error) {
	result := make([]dao.ActivityEvent, 0)

	current := GetAsOrDefault[E](c.RequestedData, ValueKey("entity"), entity)

	entityI, ok := any(entity).(dao.IDaoAct)
	if !ok {
		return nil, ErrStack.TrackErrorStack(fmt.Errorf("entity does not implement IEntity[A]"))
	}

	oldIdentifier := GetAsOrDefault[uuid.UUID](c.RequestedData, ValueKey("updateScopeId"), entityI.GetId())
	//entity = types.GetAsOrDefault[E](c.RequestedData, types.ActivityValKey("entityParent"), entity)

	key := entityI.GetEntityType()
	key = GetAsOrDefault[actField.ActivityField](c.RequestedData, FieldWithKey(key), key)

	oldV := entityI.GetString()
	oldV = GetAsOrDefault[string](c.RequestedData, ValueKey(key), oldV)
	oldV = GetAsOrDefault[string](c.RequestedData, ValueKey(oldV), oldV)

	templateActivity := NewTeActy(actField.VerbRemoved, key, &oldV, "", uuid.NullUUID{}, uuid.NullUUID{UUID: oldIdentifier, Valid: true}, c.Actor)
	if err := Gettt(c.Layer, &templateActivity, current); err != nil {
		return result, nil
	}
	result = append(result, templateActivity)
	return result, nil

	//templateActivity := dao.TemplateActivity{
	//	IdActivity:    dao.GenUUID(),
	//	Verb:          actField.VerbRemoved,
	//	Field:         strToPointer(key),
	//	OldValue:      &oldV,
	//	Comment:       fmt.Sprintf("%s remove %s: %s", actor.Email, key, oldV),
	//	NewIdentifier: uuid.NullUUID{},
	//	OldIdentifier: uuid.NullUUID{UUID: oldIdentifier, Valid: true},
	//	Actor:         &actor,
	//}
	//
	//if newAct, err := CreateActivity[E, A](current, templateActivity); err != nil {
	//	return nil, ErrStack.TrackErrorStack(err).AddContext("comment", templateActivity.Comment)
	//} else {
	//	return []A{*newAct}, nil
	//}
}
