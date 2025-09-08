// Пакет для отслеживания изменений в сущностях (issues, projects, workspaces и т.д.).
// Содержит функции для логирования событий, связанных с созданием, обновлением, удалением и перемещением сущностей.
// Также предоставляет возможность добавления пользовательских обработчиков логов.
//
// Основные возможности:
//   - Логирование изменений сущностей в базе данных.
//   - Предоставление API для добавления пользовательских обработчиков логов.
//   - Поддержка различных типов сущностей и событий.
//   - Интеграция с системой аутентификации пользователей.
package tracker

import (
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"log/slog"
	"sheff.online/aiplan/internal/aiplan/dao"
	ErrStack "sheff.online/aiplan/internal/aiplan/stack-error"
)

type activityFunc func(map[string]interface{}, map[string]interface{}, string, string, *dao.Project, dao.User, *[]dao.EntityActivity) error
type activityFuncGen[E dao.Entity, A dao.Activity] func(*ActivitiesTracker, map[string]interface{}, map[string]interface{}, E, dao.User) ([]A, error)

type ActivityHandler interface {
	Handle(activity dao.ActivityI) error
}
type ActivitiesTracker struct {
	db *gorm.DB

	activitiesMapper  map[string]activityFunc
	fieldUpdateMapper map[string]activityFunc

	activityLogFunc  []func(activity dao.EntityActivity)
	activityLogFuncI []func(activity dao.ActivityI)

	handlers []ActivityHandler
}

func (t *ActivitiesTracker) RunHandlers(activity dao.ActivityI) {

	for _, handler := range t.handlers {
		err := handler.Handle(activity)
		if err != nil {
			slog.Error("handler failed", "error", err)
		}
	}
}

func (t *ActivitiesTracker) RegisterHandler(handler ActivityHandler) {
	t.handlers = append(t.handlers, handler)
}

// attachment (issue(+),)
func getFuncUpdate[E dao.Entity, A dao.Activity](field string) activityFuncGen[E, A] {
	switch field {
	case FIELD_ASSIGNEES: // issue(+)
		return issueAssigneesUpdate[E, A]
	case FIELD_WATCHERS: // issue(+)
		return entityWatchersUpdate[E, A]
	case FIELD_READERS:
		return entityReadersUpdate[E, A]
	case FIELD_EDITORS:
		return entityEditorsUpdate[E, A]
	case FIELD_NAME: // issue(+)
		return entityNameUpdate[E, A]
	case FIELD_TEMPLATE:
		return entityTemplateUpdate[E, A]
	case FIELD_LOGO: // issue(+)
		return entityLogoUpdate[E, A]
	case FIELD_TOKEN:
		return entityTokenUpdate[E, A]
	case FIELD_OWNER:
		return entityOwnerUpdate[E, A]
	case FIELD_TITLE:
		return entityTitleUpdate[E, A]
	case FIELD_EMOJI:
		return entityEmojiUpdate[E, A]
	case FIELD_PUBLIC:
		return entityPublicUpdate[E, A]
	case FIELD_IDENTIFIER:
		return entityIdentifierUpdate[E, A]
	case FIELD_PROJECT_LEAD:
		return entityProjectLeadUpdate[E, A]
	case FIELD_PRIORITY: // issue(+)
		return entityPriorityUpdate[E, A]
	case FIELD_ROLE:
		return entityRoleUpdate[E, A]
	case FIELD_DEFAULT_ASSIGNES:
		return entityDefaultAssigneesUpdate[E, A]
	case FIELD_DEFAULT_WATCHERS:
		return entityDefaultWatchersUpdate[E, A]
	case FIELD_DESCRIPTION: // issue(+)
		return entityDescriptionUpdate[E, A]
	case FIELD_DESCRIPTION_HTML: // issue(+)
		return entityDescriptionHtmlUpdate[E, A]
	case FIELD_COLOR:
		return entityColorUpdate[E, A]
	case FIELD_TARGET_DATE: // issue(+)
		return entityTargetDateUpdate[E, A]
	case FIELD_START_DATE:
		return entityStartDateUpdate[E, A]
	case FIELD_COMPLETED_AT:
		return entityCompletedAtUpdate[E, A]
	case FIELD_END_DATE:
		return entityEndDateUpdate[E, A]
	case FIELD_LABEL: // issue(+)
		return entityLabelUpdate[E, A]
	case FIELD_AUTH_REQUIRE:
		return entityAuthRequireUpdate[E, A]
	case FIELD_FIELDS:
		return entityFieldsUpdate[E, A]
	case FIELD_GROUP:
		return entityGroupUpdate[E, A]
	case FIELD_STATE: // issue(+)
		return entityStateUpdate[E, A]
	case FIELD_PARENT:
		return issueParentUpdate[E, A]
	case FIELD_DEFAULT:
		return entityDefaultUpdate[E, A]
	case FIELD_ESIMATE_POINT:
		return entityEstimatePointUpdate[E, A]
	case FIELD_BLOCKS_LIST:
		return issueBlocksListUpdate[E, A]
	case FIELD_BLOCKERS_LIST:
		return issueBlockersListUpdate[E, A]
	case FIELD_URL: // issue(+)
		return entityUrlUpdate[E, A]
	case FIELD_COMMENT_HTML: // issue(+)
		return entityCommentHtmlUpdate[E, A]
	case FIELD_DOC_SORT:
		return entityDocSortUpdate[E, A]
	case FIELD_LINKED:
		return issueLinkedUpdate[E, A]
	case FIELD_EDITOR_ROLE:
		return entityEditorRoleUpdate[E, A]
	case FIELD_READER_ROLE:
		return entityReaderRoleUpdate[E, A]
	}
	return nil
}

// Возвращает функцию для обработки события активности на основе типа активности.
//
// Параметры:
//   - activityType: Тип активности (например, 'issue.created', 'project.updated').
//
// Возвращает:
//   - activityFunc: Функция, которая принимает параметры и возвращает список dao.EntityActivity.
func getFuncActivity[E dao.Entity, A dao.Activity](activityType string) activityFuncGen[E, A] {
	switch activityType {
	case ENTITY_UPDATED_ACTIVITY:
		return entityUpdatedActivity[E, A]
	case ENTITY_CREATE_ACTIVITY:
		return entityCreateActivity[E, A]
	case ENTITY_DELETE_ACTIVITY:
		return entityDeleteActivity[E, A]
	case ENTITY_ADD_ACTIVITY:
		return entityAddActivity[E, A]
	case ENTITY_REMOVE_ACTIVITY:
		return entityRemoveActivity[E, A]
	case ENTITY_MOVE_ACTIVITY:
		return entityMoveActivity[E, A]
	}
	return nil
}

// CreateActivity создает новую запись об активности сущности (issue, project и т.д.).
// Функция принимает объект сущности и объект шаблона активности, создает новую запись об активности и сохраняет ее в базе данных.
// Возвращает указатель на созданный объект активности и ошибку, если произошла ошибка.
//
// Параметры:
//   - entity: объект сущности, для которой создается активность.
//   - activity: объект шаблона активности, содержащий информацию об активности.
//
// Возвращает:
//   - *A: указатель на созданный объект активности.
//   - error: ошибка, если произошла ошибка при создании активности.
func CreateActivity[E dao.Entity, A dao.Activity](entity E, template dao.TemplateActivity) (*A, error) {
	var result A
	switch a := any(*new(A)).(type) {

	case dao.RootActivity:
		result = any(template.BuildRootActivity(nil)).(A)

	case dao.WorkspaceActivity:
		we, ok := any(entity).(dao.WorkspaceEntityI)
		if !ok {
			return nil,
				ErrStack.TrackErrorStack(fmt.Errorf("not support entity type (%T) for activity (%T)", entity, a)).
					AddContext("entity", fmt.Sprintf("%T", entity)).
					AddContext("activity", fmt.Sprintf("%T", a))
		}
		result = any(template.BuildWorkspaceActivity(we)).(A)

	case dao.ProjectActivity:
		pe, ok := any(entity).(dao.ProjectEntityI)
		if !ok {
			return nil, ErrStack.TrackErrorStack(fmt.Errorf("not support entity type (%T) for activity (%T)", entity, a)).
				AddContext("entity", fmt.Sprintf("%T", entity)).
				AddContext("activity", fmt.Sprintf("%T", a))
		}
		result = any(template.BuildProjectActivity(pe)).(A)

	case dao.IssueActivity:
		ie, ok := any(entity).(dao.IssueEntityI)
		if !ok {
			return nil, ErrStack.TrackErrorStack(fmt.Errorf("not support entity type (%T) for activity (%T)", entity, a)).
				AddContext("entity", fmt.Sprintf("%T", entity)).
				AddContext("activity", fmt.Sprintf("%T", a))
		}
		result = any(template.BuildIssueActivity(ie)).(A)

	case dao.DocActivity:
		de, ok := any(entity).(dao.DocEntityI)
		if !ok {
			return nil, ErrStack.TrackErrorStack(fmt.Errorf("not support entity type (%T) for activity (%T)", entity, a)).
				AddContext("entity", fmt.Sprintf("%T", entity)).
				AddContext("activity", fmt.Sprintf("%T", a))
		}
		result = any(template.BuildDocActivity(de)).(A)

	case dao.FormActivity:
		fe, ok := any(entity).(dao.FormEntityI)
		if !ok {
			return nil, ErrStack.TrackErrorStack(fmt.Errorf("not support entity type (%T) for activity (%T)", entity, a)).
				AddContext("entity", fmt.Sprintf("%T", entity)).
				AddContext("activity", fmt.Sprintf("%T", a))
		}
		result = any(template.BuildFormActivity(fe)).(A)

	default:
		return nil, ErrStack.TrackErrorStack(fmt.Errorf("not support activity (%T)", a)).
			AddContext("activity", fmt.Sprintf("%T", a))

	}
	return &result, nil
}

// Логирует событие изменения сущности в базе данных.  Принимает тип события, данные, текущее состояние, сущность и пользователя.  Вызывает соответствующий обработчик логирования для данного типа события и сохраняет событие в базе данных.
func TrackActivity[E dao.Entity, A dao.Activity](
	tracker *ActivitiesTracker,
	activityAction string,
	requestedData map[string]interface{},
	currentInstance map[string]interface{},
	entity E,
	actor *dao.User) error {
	actFunc := getFuncActivity[E, A](activityAction)
	if actFunc == nil {
		return ErrStack.TrackErrorStack(fmt.Errorf("not activity function")).
			AddContext("activity_action", activityAction).
			AddContext("entity", fmt.Sprintf("%T", entity))
	}

	activities, err := actFunc(tracker, requestedData, currentInstance, entity, *actor)
	if err != nil {
		return ErrStack.TrackErrorStack(err)
	}

	if len(activities) > 0 {
		if err := tracker.db.Omit(clause.Associations).Create(&activities).Error; err != nil {
			return err
		}

		for _, activity := range activities {
			err := dao.EntityActivityAfterFind(&activity, tracker.db)
			if err != nil {
				ErrStack.GetError(nil, ErrStack.TrackErrorStack(err))
				continue
			}
			activity = confSkipper(activity, requestedData)
			if a, ok := any(activity).(dao.ActivityI); ok {
				tracker.RunHandlers(a)
			}
		}
	}

	return nil
}

// NewActivitiesTracker создает новый экземпляр ActivitiesTracker. Этот трекер используется для отслеживания изменений в сущностях (issues, projects, workspaces и т.д.). Он предоставляет API для добавления пользовательских обработчиков логов, а также логирует события изменения сущностей в базу данных.
//
// Параметры:
//   - db: экземпляр gorm.DB, используемый для доступа к базе данных.
//
// Возвращает:
//   - *ActivitiesTracker: новый экземпляр ActivitiesTracker.
func NewActivitiesTracker(db *gorm.DB) *ActivitiesTracker {
	tracker := ActivitiesTracker{db: db}
	return &tracker
}
