// Пакет для обновления данных сущностей, связанных с активностями.  Обрабатывает как отдельные обновления, так и массовые изменения нескольких сущностей.
//
// Основные возможности:
//   - Обновление значения поля сущности.
//   - Добавление и удаление сущностей из списка.
package tracker

import (
	"fmt"
	"sheff.online/aiplan/internal/aiplan/dao"
	ErrStack "sheff.online/aiplan/internal/aiplan/stack-error"
	"sheff.online/aiplan/internal/aiplan/utils"
)

// entityFieldUpdate Обновляет значение поля сущности, либо добавляет/удаляет сущности из списка.  Обрабатывает как отдельные обновления, так и массовые изменения нескольких сущностей.
//
// Параметры:
//   - field: Имя поля сущности, которое необходимо обновить.
//   - newIdentifier: Идентификатор новой сущности (может быть nil).
//   - oldIdentifier: Идентификатор старой сущности (может быть nil).
//   - tracker: Указатель на ActivitiesTracker для доступа к базе данных.
//   - requestedData:  Содержит данные для обновления поля (новое значение, scope, field_log).
//   - currentInstance: Текущее состояние сущности.
//   - entity: Сущность, которую необходимо обновить.
//   - actor: Пользователь, выполняющий обновление.
//
// Возвращает:
//   - []A: Список созданных Activities.
//   - error: Ошибка, если произошла ошибка при обновлении.
func entityFieldUpdate[E dao.Entity, A dao.Activity](
	field string,
	newIdentifier *string,
	oldIdentifier *string,
	tracker *ActivitiesTracker,
	requestedData map[string]interface{},
	currentInstance map[string]interface{},
	entity E,
	actor dao.User) ([]A, error) {

	result := make([]A, 0)
	oldV := fmt.Sprint(currentInstance[field])
	newV := fmt.Sprint(requestedData[field])
	var old *string

	if oldVal, ok := currentInstance[fmt.Sprintf("%s_activity_val", field)]; ok {
		oldV = fmt.Sprint(oldVal)
	}

	if newVal, ok := requestedData[fmt.Sprintf("%s_activity_val", field)]; ok {
		newV = fmt.Sprint(newVal)
	}

	if oldV == newV {
		return result, nil
	}

	if oldV == "<nil>" {
		old = nil
	} else {
		old = &oldV
	}

	if newV == "<nil>" {
		newV = ""
	}

	if f, ok := requestedData[fmt.Sprintf("%s_func", field)]; ok {
		if ff, ok := f.(func(str string) string); ok {
			newV = ff(newV)
		}
	}

	if f, ok := currentInstance[fmt.Sprintf("%s_func", field)]; ok {
		if ff, ok := f.(func(str string) string); ok && old != nil {
			tmp := ff(*old)
			old = &tmp
		}
	}

	valToComment := newV
	if newV == "" {
		valToComment = "None"
	}

	if id, ok := requestedData["updateScopeId"].(string); ok {
		newIdentifier = &id
	}
	if id, ok := requestedData[fmt.Sprintf("%s_updateScopeId", field)].(string); ok {
		newIdentifier = &id
	}

	if id, ok := currentInstance["updateScopeId"].(string); ok {
		oldIdentifier = &id
	}
	if id, ok := currentInstance[fmt.Sprintf("%s_updateScopeId", field)].(string); ok {
		oldIdentifier = &id
	}

	if scope, ok := currentInstance["updateScope"]; ok {
		field = fmt.Sprintf("%s_%s", scope, field)
	} else if scope, ok := requestedData["updateScope"]; ok {
		field = fmt.Sprintf("%s_%s", scope, field)
	}

	if fieldLog, ok := requestedData[fmt.Sprintf("field_log")]; ok {
		field = fieldLog.(string)
	}

	if fieldLog, ok := requestedData[fmt.Sprintf("%s_field_log", field)]; ok {
		field = fieldLog.(string)
	}

	templateActivity := dao.NewTemplateActivity(dao.ACTIVITY_UPDATED, &field, old, newV, newIdentifier, oldIdentifier, &actor, valToComment)

	if newAct, err := CreateActivity[E, A](entity, templateActivity); err != nil {
		return nil, ErrStack.TrackErrorStack(err).AddContext("comment", templateActivity.Comment)
	} else {
		return []A{*newAct}, nil
	}
}

// entityFiieldsListUpdate Обновляет список сутностей по указанному полю.  Выполняет массовые изменения (добавление и удаление) сутностей, связанных с данным полем.  Использует данные из `requestedData` для обновления или добавления/удаления сутностей.  Работает с несколькими сутностями одновременно.
func entityFieldsListUpdate[E dao.Entity, A dao.Activity, T dao.IDaoAct](
	field string,
	requestedName string,
	tracker *ActivitiesTracker,
	requestedData map[string]interface{},
	currentInstance map[string]interface{},
	entity E,
	actor dao.User) ([]A, error) {

	result := make([]A, 0)

	oldEntities := currentInstance[field].([]interface{})
	newEntities := requestedData[requestedName].([]interface{})
	changes, err := utils.CalculateIDChanges(newEntities, oldEntities)
	if err != nil {
		return nil, ErrStack.TrackErrorStack(err)
	}

	var involvedEntities []T

	query := tracker.db

	if cm, ok := requestedData["current_table"]; ok {
		query = query.Table(fmt.Sprint(cm))
	} else {
		query = query.Model(&entity)
	}

	if _, ok := any(new(T)).(*dao.Issue); ok {
		query = query.Where("issues.id in (?)", changes.InvolvedIds).Joins("Project")
	} else {
		query = query.Where("id in (?)", changes.InvolvedIds)
	}

	if err := query.
		Find(&involvedEntities).Error; err != nil {
		return result, ErrStack.TrackErrorStack(err).AddContext("field", field)
	}

	entityMap := mapEntity(involvedEntities)

	if fieldLog, ok := requestedData["field_log"]; ok {
		field = fieldLog.(string)
	}

	for _, id := range changes.DelIds {
		oldV := entityMap[id].GetString()
		templateActivity := dao.NewTemplateActivity(dao.ACTIVITY_REMOVED, &field, &oldV, "", nil, &id, &actor, oldV)
		if act, err := CreateActivity[E, A](entity, templateActivity); err != nil {
			ErrStack.GetError(nil, ErrStack.TrackErrorStack(err).AddContext("comment", templateActivity.Comment))
			continue
		} else {
			result = append(result, *act)
		}
	}

	for _, id := range changes.AddIds {
		newV := entityMap[id].GetString()
		templateActivity := dao.NewTemplateActivity(dao.ACTIVITY_ADDED, &field, nil, newV, &id, nil, &actor, newV)
		if act, err := CreateActivity[E, A](entity, templateActivity); err != nil {
			ErrStack.GetError(nil, ErrStack.TrackErrorStack(err).AddContext("comment", templateActivity.Comment))
			continue
		} else {
			result = append(result, *act)
		}
	}
	return result, nil
}

func updateEntityRelationsLog[E dao.Entity, A dao.Activity, T dao.IDaoAct](
	field string,
	requestedName string,
	tracker *ActivitiesTracker,
	requestedData map[string]interface{},
	currentInstance map[string]interface{},
	entity E,
	actor dao.User) ([]A, error) {

	result := make([]A, 0)

	oldEntities := currentInstance[field].([]interface{})
	newEntities := requestedData[requestedName].([]interface{})
	changes, err := utils.CalculateIDChanges(newEntities, oldEntities)
	if err != nil {
		return nil, ErrStack.TrackErrorStack(err)
	}

	ie, ok := any(entity).(dao.IDaoAct)
	if !ok {
		return nil, ErrStack.TrackErrorStack(fmt.Errorf("entity does not implement IDaoAct"))
	}

	var involvedEntities []E

	query := tracker.db.Model(&entity)

	if _, ok := any(new(E)).(*dao.Issue); ok {
		query = query.Where("issues.id in (?)", changes.InvolvedIds).Joins("Project")
	} else {
		query = query.Where("id in (?)", changes.InvolvedIds)
	}

	if err := query.
		Find(&involvedEntities).Error; err != nil {
		return result, ErrStack.TrackErrorStack(err)
	}
	iEntityMap := make(map[string]dao.IDaoAct)
	entityMap := make(map[string]E)
	for _, e := range involvedEntities {
		if v, ok := any(e).(dao.IDaoAct); ok {
			iEntityMap[v.GetId()] = v
			entityMap[v.GetId()] = e
		}
	}
	var sourceField, targetField string

	if fieldLog, ok := requestedData[fmt.Sprintf("field_log")]; ok {
		field = fieldLog.(string)
		sourceField = field
		targetField = field
	}

	if fieldLog, ok := requestedData[fmt.Sprintf("field_log_source")]; ok {
		sourceField = fieldLog.(string)
	}
	if fieldLog, ok := requestedData[fmt.Sprintf("field_log_target")]; ok {
		targetField = fieldLog.(string)
	}

	for _, id := range changes.DelIds {
		oldEntity := entityMap[id]
		oldIEntity := iEntityMap[id]

		oldV := oldIEntity.GetString()
		templateActivity := dao.NewTemplateActivity(dao.ACTIVITY_UPDATED, &sourceField, &oldV, "", nil, &id, &actor, oldV)
		if act, err := CreateActivity[E, A](entity, templateActivity); err != nil {
			ErrStack.GetError(nil, ErrStack.TrackErrorStack(err).AddContext("comment", templateActivity.Comment))
			continue
		} else {
			result = append(result, *act)
		}

		oldVTarget := ie.GetString()
		idE := ie.GetId()
		templateActivity = dao.NewTemplateActivity(dao.ACTIVITY_UPDATED, &targetField, &oldVTarget, "", nil, &idE, &actor, oldVTarget)
		if act, err := CreateActivity[E, A](oldEntity, templateActivity); err != nil {
			ErrStack.GetError(nil, ErrStack.TrackErrorStack(err).AddContext("comment", templateActivity.Comment))
			continue
		} else {
			result = append(result, *act)
		}
	}

	for _, id := range changes.AddIds {
		newEntity := entityMap[id]
		newIEntity := iEntityMap[id]

		newV := newIEntity.GetString()
		templateActivity := dao.NewTemplateActivity(dao.ACTIVITY_UPDATED, &sourceField, nil, newV, &id, nil, &actor, newV)
		if act, err := CreateActivity[E, A](entity, templateActivity); err != nil {
			ErrStack.GetError(nil, ErrStack.TrackErrorStack(err).AddContext("comment", templateActivity.Comment))
			continue
		} else {
			result = append(result, *act)
		}

		newV = ie.GetString()
		idE := ie.GetId()
		templateActivity = dao.NewTemplateActivity(dao.ACTIVITY_UPDATED, &targetField, nil, newV, &idE, nil, &actor, newV)
		if act, err := CreateActivity[E, A](newEntity, templateActivity); err != nil {
			ErrStack.GetError(nil, ErrStack.TrackErrorStack(err).AddContext("comment", templateActivity.Comment))
			continue
		} else {
			result = append(result, *act)
		}
	}
	return result, nil
}
