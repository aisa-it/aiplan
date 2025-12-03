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
	"log/slog"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	ErrStack "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/stack-error"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type activityFuncGen[E dao.Entity, A dao.Activity] func(*ActivitiesTracker, map[string]interface{}, map[string]interface{}, E, dao.User) ([]A, error)

type ActivityHandler interface {
	Handle(activity dao.ActivityI) error
}
type ActivitiesTracker struct {
	db *gorm.DB

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
func getFuncUpdate[E dao.Entity, A dao.Activity](field actField.ActivityField) activityFuncGen[E, A] {
	switch field {
	case actField.ReqAssignees: // issue(+)
		return issueAssigneesUpdate[E, A]
	case actField.ReqWatchers: // issue(+)
		return entityWatchersUpdate[E, A]
	case actField.ReqReaders:
		return entityReadersUpdate[E, A]
	case actField.ReqEditors:
		return entityEditorsUpdate[E, A]
	case actField.ReqIssues:
		return entityIssuesUpdate[E, A]
	case actField.ReqSprint:
		return entitySprintUpdate[E, A]
	case actField.ReqName: // issue(+)
		return entityNameUpdate[E, A]
	case actField.ReqTemplate:
		return entityTemplateUpdate[E, A]
	case actField.ReqLogo: // issue(+)
		return entityLogoUpdate[E, A]
	case actField.ReqToken:
		return entityTokenUpdate[E, A]
	case actField.ReqOwner:
		return entityOwnerUpdate[E, A]
	case actField.ReqTitle:
		return entityTitleUpdate[E, A]
	case actField.ReqEmoj:
		return entityEmojiUpdate[E, A]
	case actField.ReqPublic:
		return entityPublicUpdate[E, A]
	case actField.ReqIdentifier:
		return entityIdentifierUpdate[E, A]
	case actField.ReqProjectLead:
		return entityProjectLeadUpdate[E, A]
	case actField.ReqPriority: // issue(+)
		return entityPriorityUpdate[E, A]
	case actField.ReqRole:
		return entityRoleUpdate[E, A]
	case actField.ReqDefaultAssignees:
		return entityDefaultAssigneesUpdate[E, A]
	case actField.ReqDefaultWatchers:
		return entityDefaultWatchersUpdate[E, A]
	case actField.ReqDescription: // issue(+)
		return entityDescriptionUpdate[E, A]
	case actField.ReqDescriptionHtml: // issue(+)
		return entityDescriptionHtmlUpdate[E, A]
	case actField.ReqColor:
		return entityColorUpdate[E, A]
	case actField.ReqTargetDate: // issue(+)
		return entityTargetDateUpdate[E, A]
	case actField.ReqStartDate:
		return entityStartDateUpdate[E, A]
	case actField.ReqCompletedAt:
		return entityCompletedAtUpdate[E, A]
	case actField.ReqEndDate:
		return entityEndDateUpdate[E, A]
	case actField.ReqLabel: // issue(+)
		return entityLabelUpdate[E, A]
	case actField.ReqAuthRequire:
		return entityAuthRequireUpdate[E, A]
	case actField.ReqFields:
		return entityFieldsUpdate[E, A]
	case actField.ReqGroup:
		return entityGroupUpdate[E, A]
	case actField.ReqState: // issue(+)
		return entityStateUpdate[E, A]
	case actField.ReqParent:
		return issueParentUpdate[E, A]
	case actField.ReqDefault:
		return entityDefaultUpdate[E, A]
	case actField.ReqEstimatePoint:
		return entityEstimatePointUpdate[E, A]
	case actField.ReqBlocksList:
		return issueBlocksListUpdate[E, A]
	case actField.ReqBlockersList:
		return issueBlockersListUpdate[E, A]
	case actField.ReqUrl: // issue(+)
		return entityUrlUpdate[E, A]
	case actField.ReqCommentHtml: // issue(+)
		return entityCommentHtmlUpdate[E, A]
	case actField.ReqDocSort:
		return entityDocSortUpdate[E, A]
	case actField.ReqLinked:
		return issueLinkedUpdate[E, A]
	case actField.ReqEditorRole:
		return entityEditorRoleUpdate[E, A]
	case actField.ReqReaderRole:
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
	case actField.EntityUpdatedActivity:
		return entityUpdatedActivity[E, A]
	case actField.EntityCreateActivity:
		return entityCreateActivity[E, A]
	case actField.EntityDeleteActivity:
		return entityDeleteActivity[E, A]
	case actField.EntityAddActivity:
		return entityAddActivity[E, A]
	case actField.EntityRemoveActivity:
		return entityRemoveActivity[E, A]
	case actField.EntityMoveActivity:
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
	case dao.SprintActivity:
		se, ok := any(entity).(dao.SprintEntityI)
		if !ok {
			return nil,
				ErrStack.TrackErrorStack(fmt.Errorf("not support entity type (%T) for activity (%T)", entity, a)).
					AddContext("entity", fmt.Sprintf("%T", entity)).
					AddContext("activity", fmt.Sprintf("%T", a))
		}
		result = any(template.BuildSprintActivity(se)).(A)
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
