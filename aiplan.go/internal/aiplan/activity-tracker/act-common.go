package tracker

import (
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
)

func actSingleWithoutIdentifier[E dao.Entity](field actField.ActivityField) func(c *ActivityCtx, entity E) ([]dao.ActivityEvent, error) {
	return func(c *ActivityCtx, entity E) ([]dao.ActivityEvent, error) {
		return fieldUpdate(c, field, uuid.NullUUID{}, uuid.NullUUID{}, entity)
	}
}

func fieldUpdate[E dao.Entity](
	c *ActivityCtx, field actField.ActivityField,
	newIdentifier uuid.NullUUID, oldIdentifier uuid.NullUUID,
	entity E,
) ([]dao.ActivityEvent, error) {

	result := make([]dao.ActivityEvent, 0)

	oldV := c.getOldValue(field)
	newV := c.getNewValue(field)

	newIdentifier = c.getNewId(newIdentifier, field)
	oldIdentifier = c.getOldId(newIdentifier, field)

	c.getUpdateScope(&field)

	if oldV == newV {
		return result, nil
	}

	templateActivity := NewTeActy(actField.VerbUpdated, field, utils.ToPtr(oldV), newV, newIdentifier, oldIdentifier, c.Actor)
	if err := Gettt(c.Layer, &templateActivity, entity); err != nil {
		return result, nil
	}

	return []dao.ActivityEvent{templateActivity}, nil
}

func acteeee[E dao.Entity, T dao.IDaoAct](field actField.FieldMapping, fieldLog *actField.ActivityField) func(c *ActivityCtx, entity E) ([]dao.ActivityEvent, error) {
	return func(c *ActivityCtx, entity E) ([]dao.ActivityEvent, error) {
		if _, ok := c.RequestedData["field_log"]; !ok && fieldLog != nil {
			c.RequestedData["field_log"] = *fieldLog
		}
		return fieldListUpdate[E, T](c, field, entity)
	}
}

func fieldListUpdate[E dao.Entity, T dao.IDaoAct](
	c *ActivityCtx, act actField.FieldMapping, entity E,
) ([]dao.ActivityEvent, error) {

	result := make([]dao.ActivityEvent, 0)
	changes := utils.CalculateIDChanges(c.getEntities(act))
	involvedEntities, err := getEntities[T](c.Tracker.db, changes)
	if err != nil {
		return nil, err
	}

	//if err := query.
	//	Find(&involvedEntities).Error; err != nil {
	//	return result, ErrStack.TrackErrorStack(err).AddContext("field", act.Field.String())
	//}

	entityMap := mapEntity(involvedEntities)

	if fieldLog, ok := c.RequestedData["field_log"]; ok {
		act.Field = fieldLog.(actField.ActivityField)
	}

	for _, id := range changes.DelIds {

		oldV := entityMap[id].GetString()
		templateActivity := NewTeActy(actField.VerbRemoved, act.Field, &oldV, "", uuid.NullUUID{}, uuid.NullUUID{UUID: id, Valid: true}, c.Actor)
		if err := Gettt(c.Layer, &templateActivity, entity); err != nil {
			return result, nil
		}
		//if act, err := CreateActivity[E, A](entity, templateActivity); err != nil {
		//	ErrStack.GetError(nil, ErrStack.TrackErrorStack(err).AddContext("comment", templateActivity.Comment))
		//	continue
		//} else {
		result = append(result, templateActivity)
		//}
	}

	for _, id := range changes.AddIds {

		newV := entityMap[id].GetString()
		templateActivity := NewTeActy(actField.VerbAdded, act.Field, nil, newV, uuid.NullUUID{UUID: id, Valid: true}, uuid.NullUUID{}, c.Actor)
		if err := Gettt(c.Layer, &templateActivity, entity); err != nil {
			return result, nil
		}
		//templateActivity := dao.NewTemplateActivity(actField.VerbAdded, act.Field, nil, newV, uuid.NullUUID{UUID: newId, Valid: true}, uuid.NullUUID{}, c.Actor, newV)

		//if act, err := CreateActivity[E, A](entity, templateActivity); err != nil {
		//	ErrStack.GetError(nil, ErrStack.TrackErrorStack(err).AddContext("comment", templateActivity.Comment))
		//	continue
		//} else {
		result = append(result, templateActivity)
		//}
	}
	return result, nil
}
