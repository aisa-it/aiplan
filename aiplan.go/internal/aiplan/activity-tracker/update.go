// Package tracker
// Пакет предоставляет функции для обновления различных полей сущностей в системе отслеживания задач (tracker).
// Он включает в себя функции для обновления полей, связанных с назначением, наблюдателями, статусом, информацией о проекте и т.д.
// Также предоставляет функции для работы со списками и связями между сущностями.
// Функции используют общую логику обновления полей, абстрагируясь от конкретных типов сущностей.
// Включает в себя функции для работы с блоками и блокирующими задачами, а также для обновления информации о датах и времени.
package tracker

import (
	"fmt"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
)

// issueAssigneesUpdate Обновляет список назначенных пользователей для сущности. Использует общую логику обновления полей, абстрагируясь от конкретного типа сущности.
//
// Параметры:
//   - tracker: экземпляр ActivitiesTracker для доступа к данным.
//   - requestedData: карта с данными, которые необходимо обновить.
//   - currentInstance: карта с текущими данными сущности.
//   - entity: сущность, поля которой необходимо обновить.
//   - actor: пользователь, выполняющий обновление.
//
// Возвращает:
//   - []A: список обновленных Activity (если произошла ошибка, возвращает nil и ошибку).
//   - error: ошибка, произошедшая при обновлении (если произошла ошибка, возвращает nil).
func issueAssigneesUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	requestedData["current_table"] = dao.User{}.TableName()
	return entityFieldsListUpdate[E, A, dao.User](actField.FieldAssignees, actField.ReqFieldAssignees, tracker, requestedData, currentInstance, entity, actor)
}

// entityWatchersUpdate Обновляет список наблюдателей для сущности. Использует общую логику обновления полей, абстрагируясь от конкретного типа сущности.
//
// Параметры:
//   - tracker: экземпляр ActivitiesTracker для доступа к данным.
//   - requestedData: карта с данными, которые необходимо обновить.
//   - currentInstance: карта с текущими данными сущности.
//   - entity: сущность, поля которой необходимо обновить.
//   - actor: пользователь, выполняющий обновление.
//
// Возвращает:
//   - []A: список обновленных Activity (если произошла ошибка, возвращает nil и ошибку).
//   - error: ошибка, произошедшая при обновлении (если произошла ошибка, возвращает nil).
func entityWatchersUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	requestedData["current_table"] = dao.User{}.TableName()
	return entityFieldsListUpdate[E, A, dao.User](actField.FieldWatchers, actField.ReqFieldWatchers, tracker, requestedData, currentInstance, entity, actor)
}

func entityEditorsUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	requestedData["current_table"] = dao.User{}.TableName()
	return entityFieldsListUpdate[E, A, dao.User](actField.FieldEditors, actField.ReqFieldEditors, tracker, requestedData, currentInstance, entity, actor)
}

func entityIssuesUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	requestedData["current_table"] = dao.Issue{}.TableName()
	requestedData["field_log"] = actField.FieldIssues

	return entityFieldsListUpdate[E, A, dao.Issue](actField.FieldIssues, actField.ReqFieldIssues, tracker, requestedData, currentInstance, entity, actor)
}

func entitySprintUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	requestedData["current_table"] = dao.Issue{}.TableName()
	requestedData["field_log"] = actField.FieldIssues

	return entityFieldsListUpdate[E, A, dao.Issue](actField.FieldIssues, actField.ReqFieldIssues, tracker, requestedData, currentInstance, entity, actor)
}

func entityReadersUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	requestedData["current_table"] = dao.User{}.TableName()
	return entityFieldsListUpdate[E, A, dao.User](actField.FieldReaders, actField.ReqFieldReaders, tracker, requestedData, currentInstance, entity, actor)
}

func issueLinkedUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	uuidToStr := func(t *interface{}) interface{} {
		if v, ok := (*t).(uuid.UUID); ok {
			return v.String()
		}
		return nil
	}

	r := requestedData[actField.ReqFieldLinked]
	if rSlice, ok := r.([]interface{}); ok {
		requestedData[actField.ReqFieldLinked] = utils.SliceToSlice(&rSlice, uuidToStr)
	}

	c := currentInstance[actField.ReqFieldLinked]
	if cSlice, ok := c.([]interface{}); ok {
		currentInstance[actField.ReqFieldLinked] = utils.SliceToSlice(&cSlice, uuidToStr)
	}

	requestedData["field_log"] = actField.FieldLinked

	return updateEntityRelationsLog[E, A, dao.Issue](actField.FieldLinked, actField.ReqFieldLinked, tracker, requestedData, currentInstance, entity, actor)
}

func issueBlocksListUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	c := currentInstance["blocked_issues"]
	if cSlice, ok := c.([]interface{}); ok {
		currentInstance[actField.ReqFieldBlocksList] = utils.SliceToSlice(&cSlice, func(t *interface{}) interface{} {
			if v, ok := (*t).(map[string]interface{}); ok {
				return v["block"].(uuid.UUID).String()
			}
			return nil
		})
	}

	requestedData["field_log_source"] = actField.FieldBlocks
	requestedData["field_log_target"] = actField.FieldBlocking

	return updateEntityRelationsLog[E, A, dao.Issue](actField.FieldBlocks, actField.ReqFieldBlocksList, tracker, requestedData, currentInstance, entity, actor)
}

// issueBlockersListUpdate Обновляет список заблокированных сущностей.  Функция принимает объект ActivitiesTracker, данные для обновления, текущее состояние сущности, саму сущность и пользователя, выполняющего обновление. Возвращает список обновленных Activity и ошибку, если произошла ошибка.
//
// Параметры:
//   - tracker: экземпляр ActivitiesTracker для доступа к данным.
//   - requestedData: карта с данными, которые необходимо обновить, включая список заблокированных сущностей.
//   - currentInstance: карта с текущими данными сущности.
//   - entity: сущность, поля которой необходимо обновить.
//   - actor: пользователь, выполняющий обновление.
//
// Возвращает:
//   - []A: список обновленных Activity (если произошла ошибка, возвращает nil и ошибку).
//   - error: ошибка, произошедшая при обновлении (если произошла ошибка, возвращает nil).
func issueBlockersListUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	c := currentInstance["blocker_issues"]
	if cSlice, ok := c.([]interface{}); ok {
		currentInstance[actField.ReqFieldBlockersList] = utils.SliceToSlice(&cSlice, func(t *interface{}) interface{} {
			if v, ok := (*t).(map[string]interface{}); ok {
				return v["blocked_by"].(uuid.UUID).String()
			}
			return nil
		})
	}

	requestedData["field_log_source"] = actField.FieldBlocking
	requestedData["field_log_target"] = actField.FieldBlocks

	return updateEntityRelationsLog[E, A, dao.Issue](actField.FieldBlocking, actField.ReqFieldBlockersList, tracker, requestedData, currentInstance, entity, actor)
}

// entityDefaultAssigneesUpdate Обновляет поле дефолтных назначенных пользователей для сущности. Использует общую логику обновления полей, абстрагируясь от конкретного типа сущности.
//
// Параметры:
//   - tracker: экземпляр ActivitiesTracker для доступа к данным.
//   - requestedData: карта с данными, которые необходимо обновить.
//   - currentInstance: карта с текущими данными сущности.
//   - entity: сущность, поля которой необходимо обновить.
//   - actor: пользователь, выполняющий обновление.
//
// Возвращает:
//   - []A: список обновленных Activity (если произошла ошибка, возвращает nil и ошибку).
//   - error: ошибка, произошедшая при обновлении (если произошла ошибка, возвращает nil).
func entityDefaultAssigneesUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	requestedData["current_table"] = dao.User{}.TableName()
	return entityFieldsListUpdate[E, A, dao.User](actField.FieldDefaultAssignees, actField.ReqFieldDefaultAssignees, tracker, requestedData, currentInstance, entity, actor)
}

// entityDefaultWatchersUpdate Обновляет список дефолтных наблюдателей для сущности. Использует общую логику обновления полей, абстрагируясь от конкретного типа сущности.
//
// Параметры:
//   - tracker: экземпляр ActivitiesTracker для доступа к данным.
//   - requestedData: карта с данными, которые необходимо обновить.
//   - currentInstance: карта с текущими данными сущности.
//   - entity: сущность, поля которой необходимо обновить.
//   - actor: пользователь, выполняющий обновление.
//
// Возвращает:
//   - []A: список обновленных Activity (если произошла ошибка, возвращает nil и ошибку).
//   - error: ошибка, произошедшая при обновлении (если произошла ошибка, возвращает nil).
func entityDefaultWatchersUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	requestedData["current_table"] = dao.User{}.TableName()
	return entityFieldsListUpdate[E, A, dao.User](actField.FieldDefaultWatchers, actField.ReqFieldDefaultWatchers, tracker, requestedData, currentInstance, entity, actor)
}

// entityTitleUpdate Обновляет заголовок сущности. Использует общую логику обновления полей, абстрагируясь от конкретного типа сущности.
//
// Параметры:
//   - tracker: экземпляр ActivitiesTracker для доступа к данным.
//   - requestedData: карта с данными, которые необходимо обновить.
//   - currentInstance: карта с текущими данными сущности.
//   - entity: сущность, поля которой необходимо обновить.
//   - actor: пользователь, выполняющий обновление.
//
// Возвращает:
//   - []A: список обновленных Activity (если произошла ошибка, возвращает nil и ошибку).
//   - error: ошибка, произошедшая при обновлении (если произошла ошибка, возвращает nil).
func entityTitleUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	return entityFieldUpdate[E, A](actField.FieldTitle, nil, nil, tracker, requestedData, currentInstance, entity, actor)
}

// entityEmojiUpdate Обновляет эмодзи сущности. Использует общую логику обновления полей, абстрагируясь от конкретного типа сущности.
//
// Параметры:
//   - tracker: экземпляр ActivitiesTracker для доступа к данным.
//   - requestedData: карта с данными, которые необходимо обновить.
//   - currentInstance: карта с текущими данными сущности.
//   - entity: сущность, поля которой необходимо обновить.
//   - actor: пользователь, выполняющий обновление.
//
// Возвращает:
//   - []A: список обновленных Activity (если произошла ошибка, возвращает nil и ошибку).
//   - error: ошибка, произошедшая при обновлении (если произошла ошибка, возвращает nil).
func entityEmojiUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	return entityFieldUpdate[E, A](actField.FieldEmoj, nil, nil, tracker, requestedData, currentInstance, entity, actor)
}

// entityPublicUpdate Обновляет поле публичности сущности. Позволяет установить, видна ли сущность посторонним пользователям.
//
// Параметры:
//   - tracker: экземпляр ActivitiesTracker для доступа к данным.
//   - requestedData: карта с данными, которые необходимо обновить.
//   - currentInstance: карта с текущими данными сущности.
//   - entity: сущность, поля которой необходимо обновить.
//   - actor: пользователь, выполняющий обновление.
//
// Возвращает:
//   - []A: список обновленных Activity (если произошла ошибка, возвращает nil и ошибку).
//   - error: ошибка, произошедшая при обновлении (если произошла ошибка, возвращает nil).
func entityPublicUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	return entityFieldUpdate[E, A](actField.FieldPublic, nil, nil, tracker, requestedData, currentInstance, entity, actor)
}

// entityIdentifierUpdate Обновляет идентификатор сущности. Использует общую логику обновления полей, абстрагируясь от конкретного типа сущности.
//
// Параметры:
//   - tracker: экземпляр ActivitiesTracker для доступа к данным.
//   - requestedData: карта с данными, которые необходимо обновить.
//   - currentInstance: карта с текущими данными сущности.
//   - entity: сущность, поля которой необходимо обновить.
//   - actor: пользователь, выполняющий обновление.
//
// Возвращает:
//   - []A: список обновленных Activity (если произошла ошибка, возвращает nil и ошибку).
//   - error: ошибка, произошедшая при обновлении (если произошла ошибка, возвращает nil).
func entityIdentifierUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	return entityFieldUpdate[E, A](actField.FieldIdentifier, nil, nil, tracker, requestedData, currentInstance, entity, actor)
}

// entityProjectLeadUpdate Обновляет поле руководителя проекта сущности.
//
// Параметры:
//   - tracker: экземпляр ActivitiesTracker для доступа к данным.
//   - requestedData: карта с данными, которые необходимо обновить.
//   - currentInstance: карта с текущими данными сущности.
//   - entity: сущность, поля которой необходимо обновить.
//   - actor: пользователь, выполняющий обновление.
//
// Возвращает:
//   - []A: список обновленных Activity (если произошла ошибка, возвращает nil и ошибку).
//   - error: ошибка, произошедшая при обновлении (если произошла ошибка, возвращает nil).
func entityProjectLeadUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	return entityFieldUpdate[E, A](actField.FieldProjectLead, strToPointer(fmt.Sprint(requestedData["project_lead"])), strToPointer(fmt.Sprint(currentInstance["project_lead"])), tracker, requestedData, currentInstance, entity, actor)
}

// Обновляет поле приоритета сущности. Принимает данные для обновления приоритета, текущее состояние сущности, объект сущности и пользователя, выполняющего обновление. Возвращает список обновленных Activity и ошибку, если произошла ошибка.
func entityPriorityUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	return entityFieldUpdate[E, A](actField.FieldPriority, nil, nil, tracker, requestedData, currentInstance, entity, actor)
}

// Обновляет роль сущности, принимая идентификатор роли из данных, переданных в параметре requestedData.  Использует общую логику обновления полей, абстрагируясь от конкретного типа сущности.  Возвращает список обновленных Activity и ошибку, если произошла ошибка.
func entityRoleUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	memberID := fmt.Sprint(requestedData["member_id"])
	return entityFieldUpdate[E, A](actField.FieldRole, &memberID, nil, tracker, requestedData, currentInstance, entity, actor)
}

// entityNameUpdate Обновляет имя сущности. Использует общую логику обновления полей, абстрагируясь от конкретного типа сущности.
//
// Параметры:
//   - tracker: экземпляр ActivitiesTracker для доступа к данным.
//   - requestedData: карта с данными, которые необходимо обновить.
//   - currentInstance: карта с текущими данными сущности.
//   - entity: сущность, поля которой необходимо обновить.
//   - actor: пользователь, выполняющий обновление.
//
// Возвращает:
//   - []A: список обновленных Activity (если произошла ошибка, возвращает nil и ошибку).
//   - error: ошибка, произошедшая при обновлении (если произошла ошибка, возвращает nil).
func entityNameUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	return entityFieldUpdate[E, A](actField.FieldName, nil, nil, tracker, requestedData, currentInstance, entity, actor)
}

func entityTemplateUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	return entityFieldUpdate[E, A](actField.FieldTemplate, nil, nil, tracker, requestedData, currentInstance, entity, actor)
}

func entityLogoUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	return entityFieldUpdate[E, A](actField.FieldLogo, nil, nil, tracker, requestedData, currentInstance, entity, actor)
}

func entityTokenUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	return entityFieldUpdate[E, A](actField.FieldToken, nil, nil, tracker, requestedData, currentInstance, entity, actor)
}

func entityOwnerUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	return entityFieldUpdate[E, A](actField.FieldOwner, nil, nil, tracker, requestedData, currentInstance, entity, actor)
}

// entityDescriptionUpdate Обновляет описание сущности. Использует общую логику обновления полей, абстрагируясь от конкретного типа сущности.
//
// Параметры:
//   - tracker: экземпляр ActivitiesTracker для доступа к данным.
//   - requestedData: карта с данными, которые необходимо обновить.
//   - currentInstance: карта с текущими данными сущности.
//   - entity: сущность, поля которой необходимо обновить.
//   - actor: пользователь, выполняющий обновление.
//
// Возвращает:
//   - []A: список обновленных Activity (если произошла ошибка, возвращает nil и ошибку).
//   - error: ошибка, произошедшая при обновлении (если произошла ошибка, возвращает nil).
func entityDescriptionUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	return entityFieldUpdate[E, A](actField.FieldDescription, nil, nil, tracker, requestedData, currentInstance, entity, actor)
}

// Обновляет HTML описание сущности, добавляя тег 'comment_html' в данные для обновления.
func entityDescriptionHtmlUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	requestedData["field_log"] = actField.FieldDescription
	return entityFieldUpdate[E, A](actField.FieldDescriptionHtml, nil, nil, tracker, requestedData, currentInstance, entity, actor)
}

// Обновляет цвет сущности, получая значение из данных, переданных в параметре requestedData.  Использует общую логику обновления полей, абстрагируясь от конкретного типа сущности.  Цвет может быть представлен в виде строки, например, 'red', 'blue' и т.д.  Если цвет не указан, поле остается без изменений.
//
// Параметры:
//   - tracker: экземпляр ActivitiesTracker для доступа к данным.
//   - requestedData: карта с данными, которые необходимо обновить, включая цвет.
//   - currentInstance: карта с текущими данными сущности.
//   - entity: сущность, поля которой необходимо обновить.
//   - actor: пользователь, выполняющий обновление.
//
// Возвращает:
//   - []A: список обновленных Activity (если произошла ошибка, возвращает nil и ошибку).
//   - error: ошибка, произошедшая при обновлении (если произошла ошибка, возвращает nil).
func entityColorUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	return entityFieldUpdate[E, A](actField.FieldColor, nil, nil, tracker, requestedData, currentInstance, entity, actor)
}

// entityTargetDateUpdate Обновляет поле даты старта сущности. Использует общую логику обновления полей, абстрагируясь от конкретного типа сущности.
//
// Параметры:
//   - tracker: экземпляр ActivitiesTracker для доступа к данным.
//   - requestedData: карта с данными, которые необходимо обновить, включая дату старта.
//   - currentInstance: карта с текущими данными сущности.
//   - entity: сущность, поля которой необходимо обновить.
//   - actor: пользователь, выполняющий обновление.
//
// Возвращает:
//   - []A: список обновленных Activity (если произошла ошибка, возвращает nil и ошибку).
//   - error: ошибка, произошедшая при обновлении (если произошла ошибка, возвращает nil).
func entityTargetDateUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	if v, ok := requestedData[actField.FieldTargetDate.String()]; ok && v != nil {
		d, _ := utils.FormatDateStr(v.(string), "2006-01-02T15:04:05Z07:00", nil)
		requestedData[actField.FieldTargetDate.String()] = d
	}
	if v, ok := currentInstance[actField.FieldTargetDate.String()]; ok && v != nil {
		d, _ := utils.FormatDateStr(v.(string), "2006-01-02T15:04:05Z07:00", nil)
		currentInstance[actField.FieldTargetDate.String()] = d
	}
	return entityFieldUpdate[E, A](actField.FieldTargetDate, nil, nil, tracker, requestedData, currentInstance, entity, actor)
}

// entityStartDateUpdate Обновляет поле даты старта сущности. Использует общую логику обновления полей, абстрагируясь от конкретного типа сущности.
//
// Параметры:
//   - tracker: экземпляр ActivitiesTracker для доступа к данным.
//   - requestedData: карта с данными, которые необходимо обновить, включая дату старта.
//   - currentInstance: карта с текущими данными сущности.
//   - entity: сущность, поля которой необходимо обновить.
//   - actor: пользователь, выполняющий обновление.
//
// Возвращает:
//   - []A: список обновленных Activity (если произошла ошибка, возвращает nil и ошибку).
//   - error: ошибка, произошедшая при обновлении (если произошла ошибка, возвращает nil).
func entityStartDateUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	format := func(str string) string {
		if dateStr, err := utils.FormatDateStr(str, "02.01.2006 15:04 MST", &actor.UserTimezone); err != nil {
			return ""
		} else {
			return dateStr
		}
	}

	requestedData[actField.FieldStartDate.WithFuncStr()] = format
	currentInstance[actField.FieldStartDate.WithFuncStr()] = format

	if v, exists := requestedData[actField.FieldStartDate.String()]; exists {
		if startDate, ok := v.(map[string]interface{}); ok {
			requestedData[actField.FieldStartDate.String()] = startDate["Time"]
		}
	}

	if v, exists := currentInstance[actField.FieldStartDate.String()]; exists {
		if startDate, ok := v.(map[string]interface{}); ok {
			currentInstance[actField.FieldStartDate.String()] = startDate["Time"]
		}
	}

	return entityFieldUpdate[E, A](actField.FieldStartDate, nil, nil, tracker, requestedData, currentInstance, entity, actor)
}

func entityCompletedAtUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	format := func(str string) string {
		if v, err := FormatDate(str, "02.01.2006 15:04 MST", nil); err != nil {
			return ""
		} else {
			return v
		}
	}

	requestedData[actField.FieldCompletedAt.WithFuncStr()] = format
	currentInstance[actField.FieldCompletedAt.WithFuncStr()] = format

	return entityFieldUpdate[E, A](actField.FieldCompletedAt, nil, nil, tracker, requestedData, currentInstance, entity, actor)
}

// entityEndDateUpdate Обновляет поле даты окончания сущности. Использует общую логику обновления полей, абстрагируясь от конкретного типа сущности.
//
// Параметры:
//   - tracker: экземпляр ActivitiesTracker для доступа к данным.
//   - requestedData: карта с данными, которые необходимо обновить, включая дату окончания.
//   - currentInstance: карта с текущими данными сущности.
//   - entity: сущность, поля которой необходимо обновить.
//   - actor: пользователь, выполняющий обновление.
//
// Возвращает:
//   - []A: список обновленных Activity (если произошла ошибка, возвращает nil и ошибку).
//   - error: ошибка, произошедшая при обновлении (если произошла ошибка, возвращает nil).
func entityEndDateUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	format := func(str string) string {
		if dateStr, err := utils.FormatDateStr(str, "02.01.2006 15:04 MST", &actor.UserTimezone); err != nil {
			return ""
		} else {
			return dateStr
		}
	}

	requestedData[actField.FieldEndDate.WithFuncStr()] = format
	currentInstance[actField.FieldEndDate.WithFuncStr()] = format

	if v, exists := requestedData[actField.FieldEndDate.String()]; exists {
		if startDate, ok := v.(map[string]interface{}); ok {
			requestedData[actField.FieldEndDate.String()] = startDate["Time"]
		}
	}

	if v, exists := currentInstance[actField.FieldEndDate.String()]; exists {
		if startDate, ok := v.(map[string]interface{}); ok {
			currentInstance[actField.FieldEndDate.String()] = startDate["Time"]
		}
	}

	return entityFieldUpdate[E, A](actField.FieldEndDate, nil, nil, tracker, requestedData, currentInstance, entity, actor)
}

// entityLabelUpdate Обновляет список тегов сущности. Использует общую логику обновления полей, абстрагируясь от конкретного типа сущности.
//
// Параметры:
//   - tracker: экземпляр ActivitiesTracker для доступа к данным.
//   - requestedData: карта с данными, которые необходимо обновить.
//   - currentInstance: карта с текущими данными сущности.
//   - entity: сущность, поля которой необходимо обновить.
//   - actor: пользователь, выполняющий обновление.
//
// Возвращает:
//   - []A: список обновленных Activity (если произошла ошибка, возвращает nil и ошибку).
//   - error: ошибка, произошедшая при обновлении (если произошла ошибка, возвращает nil).
func entityLabelUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	requestedData["current_table"] = dao.Label{}.TableName()
	return entityFieldsListUpdate[E, A, dao.Label](actField.FieldLabel, actField.ReqFieldLabel, tracker, requestedData, currentInstance, entity, actor)
}

// entityAuthRequireUpdate Обновляет поле авторизации сущности.  Устанавливает, требуется ли авторизация для доступа к сущности.
//
// Параметры:
//   - tracker: экземпляр ActivitiesTracker для доступа к данным.
//   - requestedData: карта с данными, которые необходимо обновить.
//   - currentInstance: карта с текущими данными сущности.
//   - entity: сущность, поля которой необходимо обновить.
//   - actor: пользователь, выполняющий обновление.
//
// Возвращает:
//   - []A: список обновленных Activity (если произошла ошибка, возвращает nil и ошибку).
//   - error: ошибка, произошедшая при обновлении (если произошла ошибка, возвращает nil).
func entityAuthRequireUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	return entityFieldUpdate[E, A](actField.FieldAuthRequire, nil, nil, tracker, requestedData, currentInstance, entity, actor)
}

// Обновляет произвольные поля сущности, используя общую логику обновления полей.  Функция принимает карту данных для обновления, текущее состояние сущности, объект сущности и пользователя, выполняющего обновление.  Возвращает список обновленных Activity и ошибку, если таковая возникла.
func entityFieldsUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	return entityFieldUpdate[E, A](actField.FieldFields, nil, nil, tracker, requestedData, currentInstance, entity, actor)
}

// entityGroupUpdate Обновляет поле группы сущности. Использует общую логику обновления полей, абстрагируясь от конкретного типа сущности.
//
// Параметры:
//   - tracker: экземпляр ActivitiesTracker для доступа к данным.
//   - requestedData: карта с данными, которые необходимо обновить.
//   - currentInstance: карта с текущими данными сущности.
//   - entity: сущность, поля которой необходимо обновить.
//   - actor: пользователь, выполняющий обновление.
//
// Возвращает:
//   - []A: список обновленных Activity (если произошла ошибка, возвращает nil и ошибку).
//   - error: ошибка, произошедшая при обновлении (если произошла ошибка, возвращает nil).
func entityGroupUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	return entityFieldUpdate[E, A](actField.FieldGroup, nil, nil, tracker, requestedData, currentInstance, entity, actor)
}

// Обновляет поле дефолтного значения сущности. Использует общую логику обновления полей, абстрагируясь от конкретного типа сущности.
//
// Параметры:
//   - tracker: экземпляр ActivitiesTracker для доступа к данным.
//   - requestedData: карта с данными, которые необходимо обновить.
//   - currentInstance: карта с текущими данными сущности.
//   - entity: сущность, поля которой необходимо обновить.
//   - actor: пользователь, выполняющий обновление.
//
// Возвращает:
//   - []A: список обновленных Activity (если произошла ошибка, возвращает nil и ошибку).
//   - error: ошибка, произошедшая при обновлении (если произошла ошибка, возвращает nil).
func entityDefaultUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	return entityFieldUpdate[E, A](actField.FieldDefault, nil, nil, tracker, requestedData, currentInstance, entity, actor)
}

// entityEstimatePointUpdate Обновляет поле оценки в сущности. Использует общую логику обновления полей, абстрагируясь от конкретного типа сущности.
//
// Параметры:
//   - tracker: экземпляр ActivitiesTracker для доступа к данным.
//   - requestedData: карта с данными, которые необходимо обновить, включая значение оценки.
//   - currentInstance: карта с текущими данными сущности.
//   - entity: сущность, поля которой необходимо обновить.
//   - actor: пользователь, выполняющий обновление.
//
// Возвращает:
//   - []A: список обновленных Activity (если произошла ошибка, возвращает nil и ошибку).
//   - error: ошибка, произошедшая при обновлении (если произошла ошибка, возвращает nil).
func entityEstimatePointUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	return entityFieldUpdate[E, A](actField.FieldEstimatePoint, nil, nil, tracker, requestedData, currentInstance, entity, actor)
}

// entityUrlUpdate Обновляет URL сущности. Использует общую логику обновления полей, абстрагируясь от конкретного типа сущности.
//
// Параметры:
//   - tracker: экземпляр ActivitiesTracker для доступа к данным.
//   - requestedData: карта с данными, которые необходимо обновить, включая URL.
//   - currentInstance: карта с текущими данными сущности.
//   - entity: сущность, поля которой необходимо обновить.
//   - actor: пользователь, выполняющий обновление.
//
// Возвращает:
//   - []A: список обновленных Activity (если произошла ошибка, возвращает nil и ошибку).
//   - error: ошибка, произошедшая при обновлении (если произошла ошибка, возвращает nil).
func entityUrlUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	return entityFieldUpdate[E, A](actField.FieldUrl, nil, nil, tracker, requestedData, currentInstance, entity, actor)
}

// entityCommentHtmlUpdate Обновляет поле HTML комментария сущности.  Использует общую логику обновления полей, абстрагируясь от конкретного типа сущности.
//
// Параметры:
//   - tracker: экземпляр ActivitiesTracker для доступа к данным.
//   - requestedData: карта с данными, которые необходимо обновить, включая HTML комментарий.
//   - currentInstance: карта с текущими данными сущности.
//   - entity: сущность, поля которой необходимо обновить.
//   - actor: пользователь, выполняющий обновление.
//
// Возвращает:
//   - []A: список обновленных Activity (если произошла ошибка, возвращает nil и ошибку).
//   - error: ошибка, произошедшая при обновлении (если произошла ошибка, возвращает nil).
func entityCommentHtmlUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	return entityFieldUpdate[E, A](actField.FieldCommentHtml, nil, nil, tracker, requestedData, currentInstance, entity, actor)
}

// Сортирует сущность по полю doc_sort.  Функция принимает объект ActivitiesTracker, данные для обновления, текущее состояние сущности, саму сущность и пользователя, выполняющего обновление. Возвращает список обновленных Activity и ошибку, если таковая произошла.
func entityDocSortUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	switch any(entity).(type) {
	case dao.Doc, dao.Workspace:
	default:
		return nil, nil
	}
	return entityFieldUpdate[E, A](actField.FieldDocSort, nil, nil, tracker, requestedData, currentInstance, entity, actor)
}

func entityReaderRoleUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	return entityFieldUpdate[E, A](actField.FieldReaderRole, nil, nil, tracker, requestedData, currentInstance, entity, actor)
}

func entityEditorRoleUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	return entityFieldUpdate[E, A](actField.FieldEditorRole, nil, nil, tracker, requestedData, currentInstance, entity, actor)
}

// Обновляет поле статуса сущности, принимая значение из данных для обновления и текущее состояние сущности.  Функция принимает объект ActivitiesTracker, данные для обновления, текущее состояние сущности, саму сущность и пользователя, выполняющего обновление. Возвращает список обновленных Activity и ошибку, если произошла ошибка.
func entityStateUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	newId := fmt.Sprint(requestedData["state_id"])
	oldId := fmt.Sprint(currentInstance["state"])
	return entityFieldUpdate[E, A](actField.FieldStatus, &newId, &oldId, tracker, requestedData, currentInstance, entity, actor)
}

// issueParentUpdate Обновляет поле родительской сущности.  Функция принимает объект ActivitiesTracker, данные для обновления, текущее состояние сущности, саму сущность и пользователя, выполняющего обновление.  Возвращает список обновленных Activity и ошибку, если таковая произошла.
//
// Параметры:
//   - tracker: экземпляр ActivitiesTracker для доступа к данным.
//   - requestedData: карта с данными, которые необходимо обновить, включая ID родительской сущности.
//   - currentInstance: карта с текущими данными сущности.
//   - entity: сущность, поля которой необходимо обновить.
//   - actor: пользователь, выполняющий обновление.
//
// Возвращает:
//   - []A: список обновленных Activity (если произошла ошибка, возвращает nil и ошибку).
//   - error: ошибка, произошедшая при обновлении (если произошла ошибка, возвращает nil).
func issueParentUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	issue, ok := any(entity).(dao.Issue)
	if !ok {
		return nil, nil
	}

	field := "parent"
	fieldSub := actField.FieldSubIssue

	var result []A

	oldParentId := strToPointer(fmt.Sprint(currentInstance[field]))
	newParentId := strToPointer(fmt.Sprint(requestedData[field]))
	if oldParentId == nil && newParentId == nil {
		return result, nil
	}

	ids := []string{issue.ID.String()}

	if oldParentId != nil {
		ids = append(ids, *oldParentId)
	}
	if newParentId != nil {
		ids = append(ids, *newParentId)
	}

	var issues []dao.Issue
	if err := tracker.db.Preload("Project").Where("id in (?)", ids).Find(&issues).Error; err != nil {
		return result, err
	}

	issueMap := make(map[string]dao.IEntity[A], len(issues))
	for _, i := range issues {
		if t, ok := any(i).(dao.IEntity[A]); ok {
			issueMap[i.ID.String()] = t
		}
	}

	var ta dao.TemplateActivity

	e := any(entity).(dao.IDaoAct)
	issueId := strToPointer(e.GetId())

	if oldParentId == nil && newParentId != nil {
		entityI := issueMap[*newParentId]
		requestedData[actField.FieldParent.WithActivityValStr()] = entityI.GetString()
		ta = dao.NewTemplateActivity(dao.ACTIVITY_ADDED, fieldSub, nil, e.GetString(), issueId, nil, &actor, e.GetString())
		if act, err := CreateActivity[E, A](any(entityI).(E), ta); err == nil {
			result = append(result, *act)
		}

	} else if newParentId == nil && oldParentId != nil {
		entityI := issueMap[*oldParentId]
		currentInstance[actField.FieldParent.WithActivityValStr()] = entityI.GetString()
		ta = dao.NewTemplateActivity(dao.ACTIVITY_REMOVED, fieldSub, strToPointer(e.GetString()), "", nil, issueId, &actor, e.GetString())
		if act, err := CreateActivity[E, A](any(entityI).(E), ta); err == nil {
			result = append(result, *act)
		}

	} else if newParentId != nil && oldParentId != nil {
		entityIRem := issueMap[*oldParentId]
		entityIAdd := issueMap[*newParentId]
		currentInstance[actField.FieldParent.WithActivityValStr()] = entityIRem.GetString()
		requestedData[actField.FieldParent.WithActivityValStr()] = entityIAdd.GetString()
		ta = dao.NewTemplateActivity(dao.ACTIVITY_REMOVED, fieldSub, strToPointer(e.GetString()), "", nil, issueId, &actor, e.GetString())
		if act, err := CreateActivity[E, A](any(entityIRem).(E), ta); err == nil {
			result = append(result, *act)
		}
		ta = dao.NewTemplateActivity(dao.ACTIVITY_ADDED, fieldSub, nil, e.GetString(), issueId, nil, &actor, e.GetString())
		if act, err := CreateActivity[E, A](any(entityIAdd).(E), ta); err == nil {
			result = append(result, *act)
		}
	}

	res, err := entityFieldUpdate[E, A](actField.FieldParent, newParentId, oldParentId, tracker, requestedData, currentInstance, entity, actor)
	if err != nil {
		return nil, err
	}
	result = append(result, res...)

	return result, nil
}

// Преобразует слайс сущностей в map, используя ID сущности в качестве ключа.
func mapEntity[T dao.IDaoAct](arr []T) map[string]T {
	result := make(map[string]T, len(arr))
	for _, a := range arr {
		result[a.GetId()] = a
	}
	return result
}

// Преобразует строку в указатель на строку. Если входная строка равна '<nil>', возвращает nil.
func strToPointer(str string) *string {
	if str != "<nil>" {
		return &str
	}
	return nil
}
