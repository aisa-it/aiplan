package dao

import (
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dto"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

type ActivityEvent struct {
	ID uuid.UUID `gorm:"column:id;primaryKey;type:uuid"`

	CreatedAt time.Time `gorm:"column:created_at;not null;index:,type:brin"`

	ActorID  uuid.UUID `gorm:"column:actor_id;type:uuid;not null;index:idx_activity_actor_entity_created,priority:1"`
	Notified bool      `gorm:"column:notified;default:false;index:idx_activity_notified,priority:1,where:notified=false"`

	Verb          string
	Field         actField.ActivityField
	OldValue      *string
	NewValue      string
	NewIdentifier uuid.NullUUID `gorm:"type:uuid"`
	OldIdentifier uuid.NullUUID `gorm:"type:uuid"`
	SenderTg      int64         `gorm:"-" json:"-"`

	EntityType types.EntityLayer `gorm:"column:entity_type;type:smallint;index:idx_activity_workspace,priority:2;index:idx_activity_project,priority:2;index:idx_activity_issue,priority:2;index:idx_activity_doc,priority:2;index:idx_activity_form,priority:2;index:idx_activity_sprint,priority:2"`

	WorkspaceID uuid.NullUUID `gorm:"column:workspace_id;type:uuid;index:idx_activity_workspace,priority:1,where:workspace_id IS NOT NULL"`
	ProjectID   uuid.NullUUID `gorm:"column:project_id;type:uuid;index:idx_activity_project,priority:1,where:project_id IS NOT NULL"`
	IssueID     uuid.NullUUID `gorm:"column:issue_id;type:uuid;index:idx_activity_issue,priority:1,where:issue_id IS NOT NULL"`
	DocID       uuid.NullUUID `gorm:"column:doc_id;type:uuid;index:idx_activity_doc,priority:1,where:doc_id IS NOT NULL"`
	FormID      uuid.NullUUID `gorm:"column:form_id;type:uuid;index:idx_activity_form,priority:1,where:form_id IS NOT NULL"`
	SprintID    uuid.NullUUID `gorm:"column:sprint_id;type:uuid;index:idx_activity_sprint,priority:1,where:sprint_id IS NOT NULL"`

	Workspace *Workspace `gorm:"foreignKey:WorkspaceID"`
	Actor     *User      `gorm:"foreignKey:ActorID;references:ID"`
	Issue     *Issue     `gorm:"foreignKey:IssueID"`
	Project   *Project   `gorm:"foreignKey:ProjectID"`
	Form      *Form      `gorm:"foreignKey:FormID"`
	Doc       *Doc       `gorm:"foreignKey:DocID"`
	Sprint    *Sprint    `gorm:"foreignKey:SprintID"`

	IssueActivityExtendFields
	ProjectActivityExtendFields
	DocActivityExtendFields
	WorkspaceActivityExtendFields
	RootActivityExtendFields
	SprintActivityExtendFields
}

func (ActivityEvent) TableName() string {
	return "activity_events"
}

func (a *ActivityEvent) AfterFind(tx *gorm.DB) error {
	targetField := string(a.Field)

	switch targetField {
	case "target_date":
		if a.NewValue != "" {
			if formatted, err := utils.FormatDateStr(a.NewValue, "2006-01-02T15:04:05Z07:00", nil); err == nil {
				a.NewValue = formatted
			} else {
				slog.Error("date format", "newValue", a.NewValue, "id", a.ID, "error", err)
			}
		}

		if a.OldValue != nil && *a.OldValue != "" {
			if formatted, err := utils.FormatDateStr(*a.OldValue, "2006-01-02T15:04:05Z07:00", nil); err == nil {
				a.OldValue = &formatted
			} else {
				slog.Error("date format", "oldValue", *a.OldValue, "id", a.ID, "error", err)
			}
		}
	}

	if !a.NewIdentifier.Valid && !a.OldIdentifier.Valid {
		return nil
	}

	val := reflect.ValueOf(a).Elem()
	typ := val.Type()

	targetFieldExt := fmt.Sprintf("%s::%s", targetField, a.EntityType.String())

	var walkStruct func(reflect.Value, reflect.Type) error

	walkStruct = func(v reflect.Value, t reflect.Type) error {
		for i := 0; i < t.NumField(); i++ {
			structField := t.Field(i)
			fieldVal := v.Field(i)

			// Рекурсивно обходим встроенные структуры
			if structField.Anonymous && structField.Type.Kind() == reflect.Struct {
				if err := walkStruct(fieldVal, structField.Type); err != nil {
					return err
				}
				continue
			}

			// Проверяем наличие тега field
			fieldTag, ok := structField.Tag.Lookup("field")
			if !ok {
				continue
			}

			// Для составных полей (link_title, link_url и т.д.) берем только первую часть
			normalizedTarget := targetField
			switch targetField {
			case "link_title", "link_url", "status_color", "status_name",
				"status_description", "status_group", "label_name", "label_color",
				"status_default", "template_name", "template_template":
				normalizedTarget = strings.Split(targetField, "_")[0]
			}

			// Проверяем совпадение тега
			if fieldTag != normalizedTarget && fieldTag != targetFieldExt {
				continue
			}

			fieldName := structField.Name

			// Загружаем новую сущность
			if a.NewIdentifier.Valid && strings.HasPrefix(fieldName, "New") {
				ptr := reflect.New(structField.Type.Elem()) // *T
				err := tx.Where("id = ?", a.NewIdentifier.UUID).First(ptr.Interface()).Error
				if err == nil {
					fieldVal.Set(ptr)
				} else if err != gorm.ErrRecordNotFound {
					slog.Debug("failed to load new entity",
						"field", fieldName,
						"fieldTag", fieldTag,
						"id", a.NewIdentifier.UUID,
						"activityId", a.ID,
						"error", err.Error())
					continue
				} else {
					slog.Debug("entity not found",
						"field", fieldName,
						"fieldTag", fieldTag,
						"id", a.NewIdentifier.UUID,
						"activityId", a.ID)
					continue
				}
			}

			// Загружаем старую сущность
			if a.OldIdentifier.Valid && strings.HasPrefix(fieldName, "Old") {
				ptr := reflect.New(structField.Type.Elem()) // *T
				err := tx.Where("id = ?", a.OldIdentifier.UUID).First(ptr.Interface()).Error
				if err == nil {
					fieldVal.Set(ptr)
				} else if err != gorm.ErrRecordNotFound {
					slog.Debug("failed to load old entity",
						"field", fieldName,
						"fieldTag", fieldTag,
						"id", a.OldIdentifier.UUID,
						"activityId", a.ID,
						"error", err.Error())
					continue
				} else {
					slog.Debug("entity not found",
						"field", fieldName,
						"fieldTag", fieldTag,
						"id", a.OldIdentifier.UUID,
						"activityId", a.ID)
					continue
				}
			}
		}
		return nil
	}

	return walkStruct(val, typ)
}

// Создает легкий DTO из FullActivity.
func (e *ActivityEvent) ToLightDTO() *dto.ActivityEventLight {
	if e == nil {
		return nil
	}
	return &dto.ActivityEventLight{
		Id:         e.ID,
		Verb:       e.Verb,
		Field:      e.Field,
		OldValue:   e.OldValue,
		NewValue:   e.NewValue,
		EntityType: e.EntityType,
		//TargetUser: e.AffectedUser.ToLightDTO(),
		EntityUrl: e.GetUrl(),
		CreatedAt: e.CreatedAt,
		NewEntity: GetActionEntity2(*e, "New"),
		OldEntity: GetActionEntity2(*e, "Old"),
	}
}

// Создает полный DTO из структуры FullActivity.
func (e *ActivityEvent) ToDTO() *dto.ActivityEventFull {
	if e == nil {
		return nil
	}

	return &dto.ActivityEventFull{
		ActivityEventLight: *e.ToLightDTO(),
		Workspace:          e.Workspace.ToLightDTO(),
		Actor:              e.Actor.ToLightDTO(),
		Issue:              e.Issue.ToLightDTO(),
		Project:            e.Project.ToLightDTO(),
		Form:               e.Form.ToLightDTO(),
		Doc:                e.Doc.ToLightDTO(),
		Sprint:             e.Sprint.ToLightDTO(),
		NewIdentifier:      e.NewIdentifier,
		OldIdentifier:      e.OldIdentifier,
	}
}

// Проверяет, следует ли пропустить предварительную загрузку данных.  Возвращает true, если поле не определено или идентификаторы не установлены, что указывает на то, что предварительная загрузка не требуется.  В противном случае возвращает false.
func (e ActivityEvent) SkipPreload() bool {
	//if e.Field == nil {
	//  return true
	//}

	if !e.NewIdentifier.Valid && !e.OldIdentifier.Valid {
		return true
	}
	return false
}

func (e *ActivityEvent) GetUrl() *string {
	switch e.EntityType {
	case types.EntityIssue:
		if e.Issue != nil && e.Issue.URL != nil {
			urlStr := e.Issue.URL.String()
			return &urlStr
		}
	case types.EntityProject:
		if e.Project != nil && e.Project.URL != nil {
			urlStr := e.Project.URL.String()
			return &urlStr
		}
	case types.EntityWorkspace:
		if e.Workspace != nil && e.Workspace.URL != nil {
			urlStr := e.Workspace.URL.String()
			return &urlStr
		}
	case types.EntityForm:
		if e.Form != nil && e.Form.URL != nil {
			urlStr := e.Form.URL.String()
			return &urlStr
		}
	}
	return nil
}
