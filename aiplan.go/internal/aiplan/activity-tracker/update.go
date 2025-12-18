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
	return entityFieldsListUpdate[E, A, dao.User](actField.Assignees, tracker, requestedData, currentInstance, entity, actor)
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
	return entityFieldsListUpdate[E, A, dao.User](actField.Watchers, tracker, requestedData, currentInstance, entity, actor)
}

func entityEditorsUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	requestedData["current_table"] = dao.User{}.TableName()
	return entityFieldsListUpdate[E, A, dao.User](actField.Editors, tracker, requestedData, currentInstance, entity, actor)
}

func entityIssuesUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	requestedData["current_table"] = dao.Issue{}.TableName()
	requestedData["field_log"] = actField.Issues.Field

	return entityFieldsListUpdate[E, A, dao.Issue](actField.Issues, tracker, requestedData, currentInstance, entity, actor)
}

func entitySprintUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	requestedData["current_table"] = dao.Issue{}.TableName()
	requestedData["field_log"] = actField.Issues.Field

	return entityFieldsListUpdate[E, A, dao.Issue](actField.Issues, tracker, requestedData, currentInstance, entity, actor)
}

func entityReadersUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	requestedData["current_table"] = dao.User{}.TableName()
	return entityFieldsListUpdate[E, A, dao.User](actField.Readers, tracker, requestedData, currentInstance, entity, actor)
}

func issueLinkedUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	uuidToStr := func(t *interface{}) interface{} {
		if v, ok := (*t).(uuid.UUID); ok {
			return v.String()
		}
		return nil
	}

	r := requestedData[actField.Linked.Req]
	if rSlice, ok := r.([]interface{}); ok {
		requestedData[actField.Linked.Req] = utils.SliceToSlice(&rSlice, uuidToStr)
	}

	c := currentInstance[actField.Linked.Req]
	if cSlice, ok := c.([]interface{}); ok {
		currentInstance[actField.Linked.Req] = utils.SliceToSlice(&cSlice, uuidToStr)
	}

	requestedData["field_log"] = actField.Linked.Field

	return updateEntityRelationsLog[E, A, dao.Issue](actField.Linked.Field, actField.Linked.Req, tracker, requestedData, currentInstance, entity, actor)
}

func issueBlocksListUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	c := currentInstance["blocked_issues"]
	if cSlice, ok := c.([]interface{}); ok {
		currentInstance[actField.Blocks.Req] = utils.SliceToSlice(&cSlice, func(t *interface{}) interface{} {
			if v, ok := (*t).(map[string]interface{}); ok {
				return v["block"].(uuid.UUID).String()
			}
			return nil
		})
	}

	requestedData["field_log_source"] = actField.Blocks.Field
	requestedData["field_log_target"] = actField.Blocking.Field

	return updateEntityRelationsLog[E, A, dao.Issue](actField.Blocks.Field, actField.Blocks.Req, tracker, requestedData, currentInstance, entity, actor)
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
		currentInstance[actField.Blocking.Req] = utils.SliceToSlice(&cSlice, func(t *interface{}) interface{} {
			if v, ok := (*t).(map[string]interface{}); ok {
				return v["blocked_by"].(uuid.UUID).String()
			}
			return nil
		})
	}

	requestedData["field_log_source"] = actField.Blocking.Field
	requestedData["field_log_target"] = actField.Blocks.Field

	return updateEntityRelationsLog[E, A, dao.Issue](actField.Blocking.Field, actField.Blocking.Req, tracker, requestedData, currentInstance, entity, actor)
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
	return entityFieldsListUpdate[E, A, dao.User](actField.DefaultAssignees, tracker, requestedData, currentInstance, entity, actor)
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
	return entityFieldsListUpdate[E, A, dao.User](actField.DefaultWatchers, tracker, requestedData, currentInstance, entity, actor)
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
	return entityFieldUpdate[E, A](actField.Title.Field, uuid.NullUUID{}, uuid.NullUUID{}, tracker, requestedData, currentInstance, entity, actor)
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
	return entityFieldUpdate[E, A](actField.Emoj.Field, uuid.NullUUID{}, uuid.NullUUID{}, tracker, requestedData, currentInstance, entity, actor)
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
	return entityFieldUpdate[E, A](actField.Public.Field, uuid.NullUUID{}, uuid.NullUUID{}, tracker, requestedData, currentInstance, entity, actor)
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
	return entityFieldUpdate[E, A](actField.Identifier.Field, uuid.NullUUID{}, uuid.NullUUID{}, tracker, requestedData, currentInstance, entity, actor)
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
	var newId, oldId uuid.NullUUID
	if pl, exist := requestedData["project_lead"]; exist {
		id, err := uuid.FromString(fmt.Sprint(pl))
		if err != nil {
			return nil, err
		}
		newId = uuid.NullUUID{UUID: id, Valid: true}
	}
	if pl, exist := currentInstance["project_lead"]; exist {
		id, err := uuid.FromString(fmt.Sprint(pl))
		if err != nil {
			return nil, err
		}
		oldId = uuid.NullUUID{UUID: id, Valid: true}
	}

	return entityFieldUpdate[E, A](actField.ProjectLead.Field, newId, oldId, tracker, requestedData, currentInstance, entity, actor)
}

// Обновляет поле приоритета сущности. Принимает данные для обновления приоритета, текущее состояние сущности, объект сущности и пользователя, выполняющего обновление. Возвращает список обновленных Activity и ошибку, если произошла ошибка.
func entityPriorityUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	return entityFieldUpdate[E, A](actField.Priority.Field, uuid.NullUUID{}, uuid.NullUUID{}, tracker, requestedData, currentInstance, entity, actor)
}

// Обновляет роль сущности, принимая идентификатор роли из данных, переданных в параметре requestedData.  Использует общую логику обновления полей, абстрагируясь от конкретного типа сущности.  Возвращает список обновленных Activity и ошибку, если произошла ошибка.
func entityRoleUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	var memberID uuid.NullUUID
	if pl, exist := requestedData["member_id"]; exist {
		id, err := uuid.FromString(fmt.Sprint(pl))
		if err != nil {
			return nil, err
		}
		memberID = uuid.NullUUID{UUID: id, Valid: true}
	}

	return entityFieldUpdate[E, A](actField.Role.Field, memberID, uuid.NullUUID{}, tracker, requestedData, currentInstance, entity, actor)
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
	return entityFieldUpdate[E, A](actField.Name.Field, uuid.NullUUID{}, uuid.NullUUID{}, tracker, requestedData, currentInstance, entity, actor)
}

func entityTemplateUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	return entityFieldUpdate[E, A](actField.Template.Field, uuid.NullUUID{}, uuid.NullUUID{}, tracker, requestedData, currentInstance, entity, actor)
}

func entityLogoUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	return entityFieldUpdate[E, A](actField.Logo.Field, uuid.NullUUID{}, uuid.NullUUID{}, tracker, requestedData, currentInstance, entity, actor)
}

func entityTokenUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	return entityFieldUpdate[E, A](actField.Token.Field, uuid.NullUUID{}, uuid.NullUUID{}, tracker, requestedData, currentInstance, entity, actor)
}

func entityOwnerUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	return entityFieldUpdate[E, A](actField.Owner.Field, uuid.NullUUID{}, uuid.NullUUID{}, tracker, requestedData, currentInstance, entity, actor)
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
	return entityFieldUpdate[E, A](actField.Description.Field, uuid.NullUUID{}, uuid.NullUUID{}, tracker, requestedData, currentInstance, entity, actor)
}

// Обновляет HTML описание сущности, добавляя тег 'comment_html' в данные для обновления.
func entityDescriptionHtmlUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	requestedData["field_log"] = actField.Description.Field
	requestedData[actField.DescriptionHtml.Field.WithActivityValStr()] = requestedData[actField.DescriptionHtml.Req]
	currentInstance[actField.DescriptionHtml.Field.WithActivityValStr()] = currentInstance[actField.DescriptionHtml.Req]

	return entityFieldUpdate[E, A](actField.DescriptionHtml.Field, uuid.NullUUID{}, uuid.NullUUID{}, tracker, requestedData, currentInstance, entity, actor)
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
	return entityFieldUpdate[E, A](actField.Color.Field, uuid.NullUUID{}, uuid.NullUUID{}, tracker, requestedData, currentInstance, entity, actor)
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
	if v, ok := requestedData[actField.TargetDate.Field.String()]; ok && v != nil {
		d, _ := utils.FormatDateStr(v.(string), "2006-01-02T15:04:05Z07:00", nil)
		requestedData[actField.TargetDate.Field.String()] = d
	}
	if v, ok := currentInstance[actField.TargetDate.Field.String()]; ok && v != nil {
		d, _ := utils.FormatDateStr(v.(string), "2006-01-02T15:04:05Z07:00", nil)
		currentInstance[actField.TargetDate.Field.String()] = d
	}
	return entityFieldUpdate[E, A](actField.TargetDate.Field, uuid.NullUUID{}, uuid.NullUUID{}, tracker, requestedData, currentInstance, entity, actor)
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

	requestedData[actField.StartDate.Field.WithFuncStr()] = format
	currentInstance[actField.StartDate.Field.WithFuncStr()] = format

	if v, exists := requestedData[actField.StartDate.Field.String()]; exists {
		if startDate, ok := v.(map[string]interface{}); ok {
			requestedData[actField.StartDate.Field.String()] = startDate["Time"]
		}
	}

	if v, exists := currentInstance[actField.StartDate.Field.String()]; exists {
		if startDate, ok := v.(map[string]interface{}); ok {
			currentInstance[actField.StartDate.Field.String()] = startDate["Time"]
		}
	}

	return entityFieldUpdate[E, A](actField.StartDate.Field, uuid.NullUUID{}, uuid.NullUUID{}, tracker, requestedData, currentInstance, entity, actor)
}

func entityCompletedAtUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	format := func(str string) string {
		if v, err := FormatDate(str, "02.01.2006 15:04 MST", nil); err != nil {
			return ""
		} else {
			return v
		}
	}

	requestedData[actField.CompletedAt.Field.WithFuncStr()] = format
	currentInstance[actField.CompletedAt.Field.WithFuncStr()] = format

	return entityFieldUpdate[E, A](actField.CompletedAt.Field, uuid.NullUUID{}, uuid.NullUUID{}, tracker, requestedData, currentInstance, entity, actor)
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

	requestedData[actField.EndDate.Field.WithFuncStr()] = format
	currentInstance[actField.EndDate.Field.WithFuncStr()] = format

	if v, exists := requestedData[actField.EndDate.Field.String()]; exists {
		if startDate, ok := v.(map[string]interface{}); ok {
			requestedData[actField.EndDate.Field.String()] = startDate["Time"]
		}
	}

	if v, exists := currentInstance[actField.EndDate.Field.String()]; exists {
		if startDate, ok := v.(map[string]interface{}); ok {
			currentInstance[actField.EndDate.Field.String()] = startDate["Time"]
		}
	}

	return entityFieldUpdate[E, A](actField.EndDate.Field, uuid.NullUUID{}, uuid.NullUUID{}, tracker, requestedData, currentInstance, entity, actor)
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
	return entityFieldsListUpdate[E, A, dao.Label](actField.Label, tracker, requestedData, currentInstance, entity, actor)
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
	return entityFieldUpdate[E, A](actField.AuthRequire.Field, uuid.NullUUID{}, uuid.NullUUID{}, tracker, requestedData, currentInstance, entity, actor)
}

// Обновляет произвольные поля сущности, используя общую логику обновления полей.  Функция принимает карту данных для обновления, текущее состояние сущности, объект сущности и пользователя, выполняющего обновление.  Возвращает список обновленных Activity и ошибку, если таковая возникла.
func entityFieldsUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	return entityFieldUpdate[E, A](actField.Fields.Field, uuid.NullUUID{}, uuid.NullUUID{}, tracker, requestedData, currentInstance, entity, actor)
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
	return entityFieldUpdate[E, A](actField.Group.Field, uuid.NullUUID{}, uuid.NullUUID{}, tracker, requestedData, currentInstance, entity, actor)
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
	return entityFieldUpdate[E, A](actField.Default.Field, uuid.NullUUID{}, uuid.NullUUID{}, tracker, requestedData, currentInstance, entity, actor)
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
	return entityFieldUpdate[E, A](actField.EstimatePoint.Field, uuid.NullUUID{}, uuid.NullUUID{}, tracker, requestedData, currentInstance, entity, actor)
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
	return entityFieldUpdate[E, A](actField.Url.Field, uuid.NullUUID{}, uuid.NullUUID{}, tracker, requestedData, currentInstance, entity, actor)
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
	return entityFieldUpdate[E, A](actField.CommentHtml.Field, uuid.NullUUID{}, uuid.NullUUID{}, tracker, requestedData, currentInstance, entity, actor)
}

// Сортирует сущность по полю doc_sort.  Функция принимает объект ActivitiesTracker, данные для обновления, текущее состояние сущности, саму сущность и пользователя, выполняющего обновление. Возвращает список обновленных Activity и ошибку, если таковая произошла.
func entityDocSortUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	switch any(entity).(type) {
	case dao.Doc, dao.Workspace:
	default:
		return nil, nil
	}
	return entityFieldUpdate[E, A](actField.DocSort.Field, uuid.NullUUID{}, uuid.NullUUID{}, tracker, requestedData, currentInstance, entity, actor)
}

func entityReaderRoleUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	return entityFieldUpdate[E, A](actField.ReaderRole.Field, uuid.NullUUID{}, uuid.NullUUID{}, tracker, requestedData, currentInstance, entity, actor)
}

func entityEditorRoleUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	return entityFieldUpdate[E, A](actField.EditorRole.Field, uuid.NullUUID{}, uuid.NullUUID{}, tracker, requestedData, currentInstance, entity, actor)
}

// Обновляет поле статуса сущности, принимая значение из данных для обновления и текущее состояние сущности.  Функция принимает объект ActivitiesTracker, данные для обновления, текущее состояние сущности, саму сущность и пользователя, выполняющего обновление. Возвращает список обновленных Activity и ошибку, если произошла ошибка.
func entityStateUpdate[E dao.Entity, A dao.Activity](tracker *ActivitiesTracker, requestedData map[string]interface{}, currentInstance map[string]interface{}, entity E, actor dao.User) ([]A, error) {
	var newId, oldId uuid.NullUUID
	if stateId, exist := requestedData["state_id"]; exist {
		id, err := uuid.FromString(fmt.Sprint(stateId))
		if err != nil {
			return nil, err
		}
		newId = uuid.NullUUID{UUID: id, Valid: true}
	}

	if stateId, exist := currentInstance["state"]; exist {
		id, err := uuid.FromString(fmt.Sprint(stateId))
		if err != nil {
			return nil, err
		}
		oldId = uuid.NullUUID{UUID: id, Valid: true}
	}

	return entityFieldUpdate[E, A](actField.Status.Field, newId, oldId, tracker, requestedData, currentInstance, entity, actor)
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
	fieldSub := actField.SubIssue.Field

	var result []A
	getNullUidFromUUID := func(id uuid.UUID) uuid.NullUUID {
		if id.IsNil() {
			return uuid.NullUUID{}
		}
		return uuid.NullUUID{UUID: id, Valid: true}
	}

	var newParentId uuid.UUID
	var oldParentId uuid.NullUUID

	if v, exist := currentInstance[field]; exist {
		switch val := v.(type) {
		case uuid.NullUUID:
			oldParentId = val
		case uuid.UUID:
			oldParentId = uuid.NullUUID{UUID: val, Valid: true}
		}
	}

	if v, exists := requestedData[field]; exists {
		switch val := v.(type) {
		case uuid.NullUUID:
			if val.Valid {
				newParentId = val.UUID
			} else {
				newParentId = uuid.Nil
			}
		case string:
			if val == "" || val == "<nil>" {
				newParentId = uuid.Nil
			} else {
				id, err := uuid.FromString(val)
				if err != nil {
					return nil, fmt.Errorf("invalid UUID format for field %s: %w", field, err)
				}
				newParentId = id
			}
		case nil:
			newParentId = uuid.Nil
		default:
			strVal := fmt.Sprint(val)
			if strVal == "" || strVal == "<nil>" {
				newParentId = uuid.Nil
			} else {
				id, err := uuid.FromString(strVal)
				if err != nil {
					return nil, fmt.Errorf("invalid UUID format for field %s: %w", field, err)
				}
				newParentId = id
			}
		}
	}

	if !oldParentId.Valid && newParentId.IsNil() {
		return result, nil
	}

	ids := []uuid.UUID{issue.ID}

	if oldParentId.Valid {
		ids = append(ids, oldParentId.UUID)
	}
	if !newParentId.IsNil() {
		ids = append(ids, newParentId)
	}

	var issues []dao.Issue
	if err := tracker.db.Preload("Project").Where("id in (?)", ids).Find(&issues).Error; err != nil {
		return result, err
	}

	issueMap := make(map[uuid.UUID]dao.IEntity[A], len(issues))
	for _, i := range issues {
		if t, ok := any(i).(dao.IEntity[A]); ok {
			issueMap[i.ID] = t
		}
	}

	var ta dao.TemplateActivity

	e := any(entity).(dao.IDaoAct)
	issueId := e.GetId()

	if !oldParentId.Valid && !newParentId.IsNil() {
		entityI := issueMap[newParentId]
		requestedData[actField.Parent.Field.WithActivityValStr()] = entityI.GetString()
		currentInstance[actField.Parent.Field.WithActivityValStr()] = "<nil>"
		ta = dao.NewTemplateActivity(actField.VerbAdded, fieldSub, nil, e.GetString(), uuid.NullUUID{UUID: issueId, Valid: true}, uuid.NullUUID{}, &actor, e.GetString())
		if act, err := CreateActivity[E, A](any(entityI).(E), ta); err == nil {
			result = append(result, *act)
		}

	} else if newParentId.IsNil() && oldParentId.Valid {
		entityI := issueMap[oldParentId.UUID]
		currentInstance[actField.Parent.Field.WithActivityValStr()] = entityI.GetString()
		ta = dao.NewTemplateActivity(actField.VerbRemoved, fieldSub, strToPointer(e.GetString()), "", uuid.NullUUID{}, uuid.NullUUID{UUID: issueId, Valid: true}, &actor, e.GetString())
		if act, err := CreateActivity[E, A](any(entityI).(E), ta); err == nil {
			result = append(result, *act)
		}

	} else if !newParentId.IsNil() && oldParentId.Valid {
		entityIRem := issueMap[oldParentId.UUID]
		entityIAdd := issueMap[newParentId]
		currentInstance[actField.Parent.Field.WithActivityValStr()] = entityIRem.GetString()
		requestedData[actField.Parent.Field.WithActivityValStr()] = entityIAdd.GetString()
		ta = dao.NewTemplateActivity(actField.VerbRemoved, fieldSub, strToPointer(e.GetString()), "", uuid.NullUUID{}, uuid.NullUUID{UUID: issueId, Valid: true}, &actor, e.GetString())
		if act, err := CreateActivity[E, A](any(entityIRem).(E), ta); err == nil {
			result = append(result, *act)
		}
		ta = dao.NewTemplateActivity(actField.VerbAdded, fieldSub, nil, e.GetString(), uuid.NullUUID{UUID: issueId, Valid: true}, uuid.NullUUID{}, &actor, e.GetString())
		if act, err := CreateActivity[E, A](any(entityIAdd).(E), ta); err == nil {
			result = append(result, *act)
		}
	}

	res, err := entityFieldUpdate[E, A](actField.Parent.Field, getNullUidFromUUID(newParentId), oldParentId, tracker, requestedData, currentInstance, entity, actor)
	if err != nil {
		return nil, err
	}
	result = append(result, res...)

	return result, nil
}

// Преобразует слайс сущностей в map, используя ID сущности в качестве ключа.
func mapEntity[T dao.IDaoAct](arr []T) map[uuid.UUID]T {
	result := make(map[uuid.UUID]T, len(arr))
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
