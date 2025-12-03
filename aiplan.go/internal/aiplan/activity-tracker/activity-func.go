// Пакет tracker предоставляет функциональность для создания, обновления, удаления, добавления и перемещения сущностей (entities) в системе.  Он включает в себя различные активности, связанные с сущностями, и генерирует соответствующие записи в журнале изменений (activity log). Он также поддерживает различные типы сущностей и поля для их описания.
//
// Основные возможности:
//   - Создание новой сущности.
//   - Обновление существующей сущности.
//   - Удаление сущности.
//   - Добавление сущности.
//   - Перемещение сущности в другую группу или контекст.
package tracker

import (
	"fmt"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	ErrStack "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/stack-error"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
)

// entityUpdateActivity Обновляет существующую сущность и генерирует запись в журнале активности.
func entityUpdatedActivity[E dao.Entity, A dao.Activity](
	tracker *ActivitiesTracker,
	requestedData map[string]interface{},
	currentInstance map[string]interface{},
	entity E,
	actor dao.User) ([]A, error) {
	result := make([]A, 0)
	for key := range requestedData {
		if f := getFuncUpdate[E, A](actField.ActivityField(key)); f != nil {
			acts, err := f(tracker, requestedData, currentInstance, entity, actor)
			if err != nil {
				return nil, ErrStack.TrackErrorStack(err)
			}
			result = append(result, acts...)
		}
	}
	return result, nil
}

// entityCreateActivity создает новую сущность и генерирует запись в журнале активности.
//
// Параметры:
//   - tracker: экземпляр ActivitiesTracker для сохранения информации об активности.
//   - requestedData: карта данных, передаваемых для создания сущности.
//   - currentInstance: карта текущей конфигурации системы.
//   - entity: экземпляр сущности, которую необходимо создать.
//   - actor: пользователь, выполняющий действие.
//
// Возвращает:
//   - []A: слайс объектов Activity, представляющих созданную активность.
//   - error: ошибка, если при создании активности произошла ошибка.
func entityCreateActivity[E dao.Entity, A dao.Activity](
	tracker *ActivitiesTracker,
	requestedData map[string]interface{},
	currentInstance map[string]interface{},
	entity E,
	actor dao.User) ([]A, error) {

	entityI, ok := any(entity).(dao.IEntity[A])
	if !ok {
		return nil, ErrStack.TrackErrorStack(fmt.Errorf("entity does not implement IEntity[A]"))
	}

	newIdentifier := strToPointer(entityI.GetId())
	if id, ok := requestedData["updateScopeId"].(string); ok {
		newIdentifier = &id
	}

	if e, ok := requestedData["entityParent"].(E); ok {
		entity = e
	}
	verb := actField.VerbCreated
	if e, ok := requestedData["custom_verb"].(string); ok {
		verb = e
	}

	newV := entityI.GetString()
	if newVal, ok := requestedData[fmt.Sprintf("%s_activity_val", newV)]; ok {
		newV = fmt.Sprint(newVal)
	}

	//if scope, ok := currentInstance["updateScope"]; ok {
	//	field = fmt.Sprintf("%s_%s", scope, field)
	//}
	templateActivity := dao.TemplateActivity{
		IdActivity:    dao.GenID(),
		Verb:          verb,
		Field:         strToPointer(entityI.GetEntityType()),
		OldValue:      nil,
		NewValue:      newV,
		Comment:       fmt.Sprintf("%s %s new %s: %s", actor.Email, verb, entityI.GetEntityType(), newV),
		NewIdentifier: newIdentifier,
		OldIdentifier: nil,
		Actor:         &actor,
	}

	if newAct, err := CreateActivity[E, A](entity, templateActivity); err != nil {
		return nil, ErrStack.TrackErrorStack(err).AddContext("comment", templateActivity.Comment)
	} else {
		return []A{*newAct}, nil
	}
}

// Удаляет существующую сущность и генерирует запись в журнале активности.
func entityDeleteActivity[E dao.Entity, A dao.Activity](
	tracker *ActivitiesTracker,
	requestedData map[string]interface{},
	currentInstance map[string]interface{},
	entity E,
	actor dao.User) ([]A, error) {

	entityI, ok := any(entity).(dao.IEntity[A])
	if !ok {
		return nil, ErrStack.TrackErrorStack(fmt.Errorf("entity does not implement IEntity[A]"))
	}

	oldVal := entityI.GetString()
	if oldTitle, ok := requestedData["old_title"]; ok {
		oldVal = fmt.Sprint(oldTitle)
	}

	templateActivity := dao.TemplateActivity{
		IdActivity:    dao.GenID(),
		Verb:          actField.VerbDeleted,
		Field:         strToPointer(entityI.GetEntityType()),
		OldValue:      strToPointer(oldVal),
		Comment:       fmt.Sprintf("%s deleted %s: %s", actor.Email, entityI.GetEntityType(), oldVal),
		NewIdentifier: nil,
		OldIdentifier: nil,
		Actor:         &actor,
	}

	if newAct, err := CreateActivity[E, A](entity, templateActivity); err != nil {
		return nil, ErrStack.TrackErrorStack(err).AddContext("comment", templateActivity.Comment)
	} else {
		return []A{*newAct}, nil
	}
}

// entityAddActivity добавляет новую сущность в систему и генерирует запись в журнале активности.
//
// Параметры:
//   - tracker: экземпляр ActivitiesTracker для сохранения информации об активности.
//   - requestedData: карта данных, передаваемых для создания сущности.
//   - currentInstance: карта текущей конфигурации системы.
//   - entity: экземпляр сущности, которую необходимо создать.
//   - actor: пользователь, выполняющий действие.
//
// Возвращает:
//   - []A: слайс объектов Activity, представляющих созданную активность.
func entityAddActivity[E dao.Entity, A dao.Activity](
	tracker *ActivitiesTracker,
	requestedData map[string]interface{},
	currentInstance map[string]interface{},
	entity E,
	actor dao.User) ([]A, error) {

	entityI, ok := any(entity).(dao.IEntity[A])
	if !ok {
		return nil, ErrStack.TrackErrorStack(fmt.Errorf("entity does not implement IEntity[A]"))
	}

	newIdentifier := strToPointer(entityI.GetId())
	if id, ok := requestedData["updateScopeId"].(string); ok {
		newIdentifier = &id
	}

	if e, ok := requestedData["entityParent"].(E); ok {
		entity = e
	}

	key := entityI.GetEntityType()
	if keyVal, ok := requestedData[fmt.Sprintf("%s_key", entityI.GetEntityType())]; ok {
		key = fmt.Sprint(keyVal)
	}

	newV := entityI.GetString()
	if newVal, ok := requestedData[fmt.Sprintf("%s_activity_val", key)]; ok {
		newV = fmt.Sprint(newVal)
	}
	if newVal, ok := requestedData[fmt.Sprintf("%s_activity_val", newV)]; ok {
		newV = fmt.Sprint(newVal)
	}

	templateActivity := dao.TemplateActivity{
		IdActivity:    dao.GenID(),
		Verb:          actField.VerbAdded,
		Field:         strToPointer(key),
		OldValue:      nil,
		NewValue:      newV,
		Comment:       fmt.Sprintf("%s added %s: %s", actor.Email, key, newV),
		NewIdentifier: newIdentifier,
		OldIdentifier: nil,
		Actor:         &actor,
	}

	if v, ok := requestedData["entity"]; ok {
		entity = v.(E)
	}

	if newAct, err := CreateActivity[E, A](entity, templateActivity); err != nil {
		return nil, ErrStack.TrackErrorStack(err).AddContext("comment", templateActivity.Comment)
	} else {
		return []A{*newAct}, nil
	}
}

// Удаляет существующую сущность и генерирует запись в журнале активности.
func entityRemoveActivity[E dao.Entity, A dao.Activity](
	tracker *ActivitiesTracker,
	requestedData map[string]interface{},
	currentInstance map[string]interface{},
	entity E,
	actor dao.User) ([]A, error) {

	current := entity
	if v, ok := currentInstance["entity"]; ok {
		current = v.(E)
	}

	entityI, ok := any(entity).(dao.IEntity[A])
	if !ok {
		return nil, ErrStack.TrackErrorStack(fmt.Errorf("entity does not implement IEntity[A]"))
	}

	oldIdentifier := strToPointer(entityI.GetId())
	if id, ok := requestedData["updateScopeId"].(string); ok {
		oldIdentifier = &id
	}

	if e, ok := requestedData["entityParent"].(E); ok {
		entity = e
	}

	key := entityI.GetEntityType()
	if keyVal, ok := requestedData[fmt.Sprintf("%s_key", entityI.GetEntityType())]; ok {
		key = fmt.Sprint(keyVal)
	}

	oldV := entityI.GetString()
	if oldVal, ok := requestedData[fmt.Sprintf("%s_activity_val", key)]; ok {
		oldV = fmt.Sprint(oldVal)
	}
	if oldVal, ok := requestedData[fmt.Sprintf("%s_activity_val", oldV)]; ok {
		oldV = fmt.Sprint(oldVal)
	}

	templateActivity := dao.TemplateActivity{
		IdActivity:    dao.GenID(),
		Verb:          actField.VerbRemoved,
		Field:         strToPointer(key),
		OldValue:      &oldV,
		Comment:       fmt.Sprintf("%s remove %s: %s", actor.Email, key, oldV),
		NewIdentifier: nil,
		OldIdentifier: oldIdentifier,
		Actor:         &actor,
	}

	if newAct, err := CreateActivity[E, A](current, templateActivity); err != nil {
		return nil, ErrStack.TrackErrorStack(err).AddContext("comment", templateActivity.Comment)
	} else {
		return []A{*newAct}, nil
	}
}

/*
для переноса в map добавить ключ "parent_key" по которому нужно искать id родительской сущности
*/
// entityMoveActivity Перемещает сущность в другую группу или контекст. Требует указания ключа родительской сущности и новых данных.
//
// Парамметры:
//  - tracker: экземпляр ActivitiesTracker для сохранения информации об активности.
//  - requestedData: карта данных, содержащая информацию о перемещении (ключ родительской сущности, новые данные).
//  - currentInstance: текущая конфигурация системы.
//  - entity: перемещаемая сущность.
//  - actor: пользователь, выполняющий действие.
//
// Возвращает:
//  - []A: слайс объектов Activity, представляющих созданную активность.
//  - error: ошибка, если при создании активности произошла ошибка.
func entityMoveActivity[E dao.Entity, A dao.Activity](
	tracker *ActivitiesTracker,
	requestedData map[string]interface{},
	currentInstance map[string]interface{},
	entity E,
	actor dao.User) ([]A, error) {

	entityI, ok := any(entity).(dao.IEntity[A])
	if !ok {
		return nil, ErrStack.TrackErrorStack(fmt.Errorf("entity does not implement IEntity[A]"))
	}

	var key string
	if v, ok := requestedData["parent_key"]; ok {
		key = v.(string)
	}

	newId := "<nil>"
	oldId := "<nil>"
	var newVal, oldVal string

	if v, ok := requestedData[key]; ok {
		newId = v.(string)
	}

	if v, ok := currentInstance[key]; ok {
		oldId = v.(string)
	}

	if v, ok := requestedData["parent_title"]; ok {
		newVal = v.(string)
	}

	if v, ok := currentInstance["parent_title"]; ok {
		oldVal = v.(string)
	}

	entityTo := entityI.GetEntityType()
	entityFrom := entityI.GetEntityType()

	if v, ok := requestedData["new_entity"]; ok {
		entityTo = v.(string)
	}

	if v, ok := requestedData["old_entity"]; ok {
		entityFrom = v.(string)
	}

	verb := actField.VerbMove
	if v, ok := requestedData["field_move"]; ok {
		verb = fmt.Sprintf("move_%s", v.(string))
	}

	templateActivity := dao.TemplateActivity{
		IdActivity:    dao.GenID(),
		Verb:          verb,
		Field:         strToPointer(entityTo),
		NewValue:      newVal,
		OldValue:      &oldVal,
		Comment:       fmt.Sprintf("%s move %s: from %s[%s] to %s[%s]", actor.Email, entityI.GetEntityType(), oldVal, entityFrom, newVal, entityTo),
		NewIdentifier: strToPointer(newId),
		OldIdentifier: strToPointer(oldId),
		Actor:         &actor,
	}

	if newAct, err := CreateActivity[E, A](entity, templateActivity); err != nil {
		return nil, ErrStack.TrackErrorStack(err).AddContext("comment", templateActivity.Comment)
	} else {
		return []A{*newAct}, nil
	}
}
