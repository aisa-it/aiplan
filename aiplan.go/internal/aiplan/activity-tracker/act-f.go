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

func add[E dao.Entity](
	c *ActivityCtx,
	entity E) ([]dao.ActivityEvent, error) {
	result := make([]dao.ActivityEvent, 0)

	//entityI, ok := any(entity).(dao.IEntity[A])
	entityI, ok := any(entity).(dao.IDaoAct)
	if !ok {
		return nil, ErrStack.TrackErrorStack(fmt.Errorf("entity does not implement IEntity[A]"))
	}

	newIdentifier := entityI.GetId()
	if id, ok := c.RequestedData["updateScopeId"].(uuid.UUID); ok {
		newIdentifier = id
	}

	if e, ok := c.RequestedData["entityParent"].(E); ok {
		entity = e
	}

	key := entityI.GetEntityType()
	if keyVal, ok := c.RequestedData[fmt.Sprintf("%s_key", entityI.GetEntityType())]; ok {
		key = fmt.Sprint(keyVal)
	}

	newV := entityI.GetString()
	if newVal, ok := c.RequestedData[fmt.Sprintf("%s_activity_val", key)]; ok {
		newV = fmt.Sprint(newVal)
	}
	if newVal, ok := c.RequestedData[fmt.Sprintf("%s_activity_val", newV)]; ok {
		newV = fmt.Sprint(newVal)
	}

	//templateActivity := dao.TemplateActivity{
	//	IdActivity:    dao.GenUUID(),
	//	Verb:          actField.VerbAdded,
	//	Field:         strToPointer(key),
	//	OldValue:      nil,
	//	NewValue:      newV,
	//	Comment:       fmt.Sprintf("%s added %s: %s", actor.Email, key, newV),
	//	NewIdentifier: uuid.NullUUID{UUID: newIdentifier, Valid: true},
	//	OldIdentifier: uuid.NullUUID{},
	//	Actor:         &actor,
	//}

	if v, ok := c.RequestedData["entity"]; ok {
		entity = v.(E)
	}

	templateActivity := NewTeActy(actField.VerbAdded, actField.ActivityField(key), nil, newV, uuid.NullUUID{UUID: newIdentifier, Valid: true}, uuid.NullUUID{}, c.Actor)
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
func remove[E dao.Entity](
	c *ActivityCtx,
	entity E) ([]dao.ActivityEvent, error) {
	result := make([]dao.ActivityEvent, 0)

	current := entity
	if v, ok := c.CurrentInstance["entity"]; ok {
		current = v.(E)
	}

	entityI, ok := any(entity).(dao.IDaoAct)
	if !ok {
		return nil, ErrStack.TrackErrorStack(fmt.Errorf("entity does not implement IEntity[A]"))
	}

	oldIdentifier := entityI.GetId()
	if id, ok := c.RequestedData["updateScopeId"].(uuid.UUID); ok {
		oldIdentifier = id
	}

	//TODO проверить

	//if e, ok := c.RequestedData["entityParent"].(E); ok {
	//	entity = e
	//}

	key := entityI.GetEntityType()
	if keyVal, ok := c.RequestedData[fmt.Sprintf("%s_key", entityI.GetEntityType())]; ok {
		key = fmt.Sprint(keyVal)
	}

	oldV := entityI.GetString()
	if oldVal, ok := c.RequestedData[fmt.Sprintf("%s_activity_val", key)]; ok {
		oldV = fmt.Sprint(oldVal)
	}
	if oldVal, ok := c.RequestedData[fmt.Sprintf("%s_activity_val", oldV)]; ok {
		oldV = fmt.Sprint(oldVal)
	}

	templateActivity := NewTeActy(actField.VerbRemoved, actField.ActivityField(key), &oldV, "", uuid.NullUUID{}, uuid.NullUUID{UUID: oldIdentifier, Valid: true}, c.Actor)
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
