package dao

import (
	"fmt"
	"log/slog"
	"reflect"
	"slices"
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
	ID uuid.UUID `gorm:"primaryKey;type:uuid"`

	CreatedAt time.Time `gorm:"index:idx_activity_issue_time,priority:2,sort:desc;index:idx_activity_project_time,priority:2,sort:desc;index:idx_activity_workspace_time,priority:2,sort:desc;index:idx_activity_actor_time,priority:2,sort:desc;index:idx_activity_notified_time,priority:1,sort:asc,where:notified = false"`
	ActorID   uuid.UUID `gorm:"type:uuid;not null;index:idx_activity_actor_time,priority:1"`

	Notified      bool `gorm:"default:false"`
	Verb          string
	Field         actField.ActivityField
	OldValue      string
	NewValue      string
	NewIdentifier uuid.NullUUID     `gorm:"type:uuid"`
	OldIdentifier uuid.NullUUID     `gorm:"type:uuid"`
	SenderTg      int64             `gorm:"-" json:"-"`
	EntityType    types.EntityLayer `gorm:"column:entity_type;type:smallint"`

	WorkspaceID uuid.NullUUID `gorm:"type:uuid;index:idx_activity_workspace_time,priority:1,where:workspace_id IS NOT NULL"`
	ProjectID   uuid.NullUUID `gorm:"type:uuid;index:idx_activity_project_time,priority:1,where:project_id IS NOT NULL"`
	IssueID     uuid.NullUUID `gorm:"type:uuid;index:idx_activity_issue_time,priority:1,where:issue_id IS NOT NULL"`
	DocID       uuid.NullUUID `gorm:"type:uuid"`
	FormID      uuid.NullUUID `gorm:"type:uuid"`
	SprintID    uuid.NullUUID `gorm:"type:uuid"`

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
	FormActivityExtendFields
}

func (e *ActivityEvent) SetEntityRefs(layer types.EntityLayer, entity IDaoAct) {
	if de, ok := any(entity).(WorkspaceEntityI); ok && layer != types.LayerRoot {
		e.WorkspaceID = uuid.NullUUID{UUID: de.GetWorkspaceId(), Valid: true}
	}
	if de, ok := any(entity).(DocEntityI); ok && layer != types.LayerWorkspace {
		e.DocID = uuid.NullUUID{UUID: de.GetDocId(), Valid: true}
	}
	if fe, ok := any(entity).(FormEntityI); ok && layer != types.LayerWorkspace {
		e.FormID = uuid.NullUUID{UUID: fe.GetFormId(), Valid: true}
	}
	if se, ok := any(entity).(SprintEntityI); ok && layer != types.LayerWorkspace {
		e.SprintID = uuid.NullUUID{UUID: se.GetSprintId(), Valid: true}
	}
	if pe, ok := any(entity).(ProjectEntityI); ok && layer != types.LayerWorkspace {
		e.ProjectID = uuid.NullUUID{UUID: pe.GetProjectId(), Valid: true}
	}
	if ie, ok := any(entity).(IssueEntityI); ok {
		e.IssueID = uuid.NullUUID{UUID: ie.GetIssueId(), Valid: true}
	}
	e.EntityType = layer
}

var (
	actEventRules = map[types.EntityLayer][]string{
		types.LayerRoot:      {},
		types.LayerWorkspace: {"WorkspaceID"},
		types.LayerProject:   {"WorkspaceID", "ProjectID"},
		types.LayerIssue:     {"WorkspaceID", "ProjectID", "IssueID"},
		types.LayerDoc:       {"WorkspaceID", "DocID"},
		types.LayerForm:      {"WorkspaceID", "FormID"},
		types.LayerSprint:    {"WorkspaceID", "SprintID"},
	}
	actEventFields = []string{"WorkspaceID", "ProjectID", "IssueID", "DocID", "FormID", "SprintID"}
)

func (e *ActivityEvent) ValidateAndSet(tx *gorm.DB) error {

	allowed, ok := actEventRules[e.EntityType]
	if !ok {
		return fmt.Errorf("unknown entity_type: %d", e.EntityType)
	}

	for _, field := range actEventFields {
		isAllowed := slices.Contains(allowed, field)
		isSet := e.isSet(field)

		if isAllowed && !isSet {
			var entity IDaoAct
			switch e.EntityType {
			case types.LayerWorkspace:
				entity = &Workspace{}
				if err := tx.Where("id = ?", e.WorkspaceID).First(entity).Error; err != nil {
					return err
				}
			case types.LayerProject:
				entity = &Project{}
				if err := tx.Where("id = ?", e.ProjectID).First(entity).Error; err != nil {
					return err
				}
			case types.LayerIssue:
				entity = &Issue{}
				if err := tx.Joins("Project").
					Where("issues.id = ?", e.IssueID).First(entity).Error; err != nil {
					return err
				}
			case types.LayerDoc:
				entity = &Doc{}
				if err := tx.Where("id = ?", e.DocID).First(entity).Error; err != nil {
					return err
				}
			case types.LayerForm:
				entity = &Form{}
				if err := tx.Where("id = ?", e.FormID).First(entity).Error; err != nil {
					return err
				}
			case types.LayerSprint:
				entity = &Sprint{}
				if err := tx.Where("id = ?", e.SprintID).First(entity).Error; err != nil {
					return err
				}
			}
			e.SetEntityRefs(e.EntityType, entity)
		}
		if !isAllowed && isSet {
			return fmt.Errorf("%s must be NULL for entity_type=%s", field, e.EntityType.String())
		}
	}

	return nil
}

func (e *ActivityEvent) isSet(field string) bool {
	switch field {
	case "WorkspaceID":
		return e.WorkspaceID.Valid
	case "ProjectID":
		return e.ProjectID.Valid
	case "IssueID":
		return e.IssueID.Valid
	case "DocID":
		return e.DocID.Valid
	case "FormID":
		return e.FormID.Valid
	case "SprintID":
		return e.SprintID.Valid
	}
	return false
}

func (ActivityEvent) TableName() string {
	return "activity_events"
}

// entityResolver заполняет ptr (*T) сущностью с данным id; true — если найдена.
type entityResolver func(id uuid.UUID, ptr reflect.Value) bool

// idGetter — общий аксессор первичного ключа целевых сущностей.
type idGetter interface {
	GetId() uuid.UUID
}

// resolveEntities обходит поля New*/Old*, подходящие под a.Field, и вызывает для них
// resolver. Обход общий для хука AfterFind и батча BatchPreloadActivityEntities.
func (a *ActivityEvent) resolveEntities(resolveNew, resolveOld entityResolver) {
	if !a.NewIdentifier.Valid && !a.OldIdentifier.Valid {
		return
	}

	targetField := string(a.Field)
	targetFieldExt := fmt.Sprintf("%s::%s", targetField, a.EntityType.String())

	val := reflect.ValueOf(a).Elem()

	var walk func(reflect.Value, reflect.Type)
	walk = func(v reflect.Value, t reflect.Type) {
		for i := 0; i < t.NumField(); i++ {
			structField := t.Field(i)
			fieldVal := v.Field(i)

			// Рекурсивно обходим встроенные структуры
			if structField.Anonymous && structField.Type.Kind() == reflect.Struct {
				walk(fieldVal, structField.Type)
				continue
			}

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

			if fieldTag != normalizedTarget && fieldTag != targetFieldExt {
				continue
			}

			fieldName := structField.Name

			if a.NewIdentifier.Valid && strings.HasPrefix(fieldName, "New") {
				ptr := reflect.New(structField.Type.Elem()) // *T
				if resolveNew(a.NewIdentifier.UUID, ptr) {
					fieldVal.Set(ptr)
				}
			}

			if a.OldIdentifier.Valid && strings.HasPrefix(fieldName, "Old") {
				ptr := reflect.New(structField.Type.Elem()) // *T
				if resolveOld(a.OldIdentifier.UUID, ptr) {
					fieldVal.Set(ptr)
				}
			}
		}
	}

	walk(val, val.Type())
}

// formatDateFields нормализует строковые даты в значениях; нужен и батчу (SkipHooks).
func (a *ActivityEvent) formatDateFields() {
	switch string(a.Field) {
	case "target_date", "end_date":
		if a.NewValue != "" {
			if formatted, err := utils.FormatDateStr(a.NewValue, "2006-01-02T15:04:05Z07:00", nil); err == nil {
				a.NewValue = formatted
			} else {
				slog.Error("date format", "newValue", a.NewValue, "id", a.ID, "error", err)
			}
		}

		if a.OldValue != "" {
			if formatted, err := utils.FormatDateStr(a.OldValue, "2006-01-02T15:04:05Z07:00", nil); err == nil {
				a.OldValue = formatted
			} else {
				slog.Error("date format", "oldValue", a.OldValue, "id", a.ID, "error", err)
			}
		}
	}
}

func (a *ActivityEvent) AfterFind(tx *gorm.DB) error {
	a.formatDateFields()

	// Per-row подгрузка. На списках это N+1 — там используйте LoadActivitiesBatched.
	dbResolver := func(id uuid.UUID, ptr reflect.Value) bool {
		err := tx.Where("id = ?", id).First(ptr.Interface()).Error
		if err == nil {
			return true
		}
		if err != gorm.ErrRecordNotFound {
			slog.Debug("failed to load activity entity", "id", id, "activityId", a.ID, "error", err.Error())
		}
		return false
	}

	a.resolveEntities(dbResolver, dbResolver)
	return nil
}

// BatchPreloadActivityEntities подгружает New*/Old*-сущности для среза активностей
// одним запросом на тип (анти-N+1). Активности должны грузиться с SkipHooks.
func BatchPreloadActivityEntities(tx *gorm.DB, events []*ActivityEvent) error {
	if len(events) == 0 {
		return nil
	}

	// Проход 1: собрать id по типам + отформатировать даты (замена работы хука).
	needed := map[reflect.Type]map[uuid.UUID]struct{}{}
	collect := func(id uuid.UUID, ptr reflect.Value) bool {
		et := ptr.Type().Elem()
		if needed[et] == nil {
			needed[et] = map[uuid.UUID]struct{}{}
		}
		needed[et][id] = struct{}{}
		return false // проход 1 ничего не присваивает
	}
	for _, ev := range events {
		ev.formatDateFields()
		ev.resolveEntities(collect, collect)
	}

	// По одному IN-запросу на тип, результат — в кэш по id.
	cache := map[reflect.Type]map[uuid.UUID]reflect.Value{}
	for et, idset := range needed {
		ids := make([]uuid.UUID, 0, len(idset))
		for id := range idset {
			ids = append(ids, id)
		}

		slicePtr := reflect.New(reflect.SliceOf(et)) // *[]T
		if err := tx.Where("id IN ?", ids).Find(slicePtr.Interface()).Error; err != nil {
			return err
		}

		slice := slicePtr.Elem()
		m := make(map[uuid.UUID]reflect.Value, slice.Len())
		for i := 0; i < slice.Len(); i++ {
			elem := slice.Index(i)
			if getter, ok := elem.Interface().(idGetter); ok {
				m[getter.GetId()] = elem
			}
		}
		cache[et] = m
	}

	// Проход 2: разложить сущности из кэша по полям.
	fromCache := func(id uuid.UUID, ptr reflect.Value) bool {
		m, ok := cache[ptr.Type().Elem()]
		if !ok {
			return false
		}
		v, ok := m[id]
		if !ok {
			return false
		}
		ptr.Elem().Set(v)
		return true
	}
	for _, ev := range events {
		ev.resolveEntities(fromCache, fromCache)
	}

	return nil
}

func (a *ActivityEvent) Comment() string {
	oldV := "nil"
	if a.OldValue != "" {
		oldV = a.OldValue
	}
	return fmt.Sprintf("layer: %s,  %s %s (%s-%s)", a.EntityType.String(), a.Verb, a.Field.String(), a.NewValue, oldV)
}

func (e *ActivityEvent) ToLightDTO() *dto.ActivityEventLight {
	if e == nil {
		return nil
	}
	oldValue := e.OldValue
	return &dto.ActivityEventLight{
		Id:         e.ID,
		Verb:       e.Verb,
		Field:      e.Field,
		OldValue:   oldValue,
		NewValue:   e.NewValue,
		EntityType: e.EntityType.String(),
		EntityUrl:  e.GetUrl(),
		CreatedAt:  e.CreatedAt,
		NewEntity:  GetActionEntity(*e, "New"),
		OldEntity:  GetActionEntity(*e, "Old"),
	}
}

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

func (e ActivityEvent) SkipPreload() bool {
	if !e.NewIdentifier.Valid && !e.OldIdentifier.Valid {
		return true
	}
	return false
}

func (e *ActivityEvent) GetUrl() *string {
	switch e.EntityType {
	case types.LayerIssue:
		if e.Issue != nil && e.Issue.URL != nil {
			urlStr := e.Issue.URL.String()
			return &urlStr
		}
	case types.LayerProject:
		if e.Project != nil && e.Project.URL != nil {
			urlStr := e.Project.URL.String()
			return &urlStr
		}
	case types.LayerWorkspace:
		if e.Workspace != nil && e.Workspace.URL != nil {
			urlStr := e.Workspace.URL.String()
			return &urlStr
		}
	case types.LayerForm:
		if e.Form != nil && e.Form.URL != nil {
			urlStr := e.Form.URL.String()
			return &urlStr
		}
	case types.LayerSprint:
		if e.Sprint != nil && e.Sprint.URL != nil {
			urlStr := e.Sprint.URL.String()
			return &urlStr
		}
	case types.LayerDoc:
		if e.Doc != nil && e.Doc.URL != nil {
			urlStr := e.Doc.URL.String()
			return &urlStr
		}
	}

	return nil
}

func (da *ActivityEvent) ToHistoryLightDTO() *dto.HistoryBodyLight {
	if da == nil {
		return nil
	}

	return &dto.HistoryBodyLight{
		Id:       da.ID,
		CratedAt: da.CreatedAt,
		Author:   da.Actor.ToLightDTO(),
	}
}

type ActivityTelegramMessage struct {
	MessageID  int64          `gorm:"primaryKey;autoIncrement:false"`
	ActivityID uuid.UUID      `gorm:"type:uuid;not null;index"`
	Activity   *ActivityEvent `gorm:"foreignKey:ActivityID;references:ID;constraint:OnDelete:CASCADE"`
}
