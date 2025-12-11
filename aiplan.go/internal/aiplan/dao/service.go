// Пакет dao содержит интерфейсы и структуры данных для доступа к данным приложения.  Он предоставляет абстракции для взаимодействия с базой данных, позволяя изолировать бизнес-логику от деталей реализации доступа к данным.
//
// Основные возможности:
//   - Определяет интерфейсы для работы с различными сущностями (User, Workspace, Project и т.д.).
//   - Предоставляет структуры данных для представления изменений сущностей (TemplateActivity, FullActivity).
//   - Содержит вспомогательные функции для преобразования данных в DTO (DTO).
//   - Обеспечивает гибкий механизм для работы с различными типами сущностей и активностей.
package dao

import (
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dto"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

type Entity interface {
	User |
		Workspace | WorkspaceMember |
		Project | ProjectMember | Label | State | IssueTemplate |
		Issue | IssueLink | IssueComment | IssueAttachment |
		Sprint |
		Doc | DocComment | DocAttachment |
		Form | FormAnswer
}

type Activity interface {
	IssueActivity | SprintActivity | ProjectActivity | FormActivity | WorkspaceActivity | DocActivity | EntityActivity | FullActivity | RootActivity
}

type IDaoAct interface {
	GetId() string
	GetString() string
}

type ActivityI interface {
	SkipPreload() bool
	GetField() string
	GetEntity() string
	GetNewIdentifier() string
	GetOldIdentifier() string
	GetVerb() string
	GetId() string
	//SetAffectedUser(user *User)
	//BuildActivity(entity E, tmpl TemplateActivity) (A, error)
}

type IEntity[A Activity] interface {
	GetEntityType() string
	IDaoAct
}

type UnionableTable interface {
	GetCustomFields() string
	TableName() string
	GetFields() []string
}

type FullTable interface {
	GetFields() []string
}

// TemplateActivity
// -migration
type TemplateActivity struct {
	IdActivity string
	Verb       string
	Field      *string
	OldValue   *string
	NewValue   string
	Comment    string

	NewIdentifier *string
	OldIdentifier *string
	Actor         *User
}

// Создает новую активность шаблона.
func NewTemplateActivity(verb string, field actField.ActivityField, oldVal *string, newVal string, newId, oldId *string, actor *User, valToComment string) TemplateActivity {
	var comment string
	switch verb {
	case actField.VerbUpdated:
		comment = fmt.Sprintf("%s updated %s to %s", actor.Email, strings.Replace(field.String(), "_", " ", 1), valToComment)
	case actField.VerbRemoved:
		comment = fmt.Sprintf("%s removed from %s - %s", actor.Email, strings.Replace(field.String(), "_", " ", 1), valToComment)
	case actField.VerbAdded:
		comment = fmt.Sprintf("%s added to %s - %s", actor.Email, strings.Replace(field.String(), "_", " ", 1), valToComment)
	}

	return TemplateActivity{
		IdActivity:    GenID(),
		Verb:          verb,
		Field:         utils.ToPtr(field.String()),
		OldValue:      oldVal,
		NewValue:      newVal,
		Comment:       comment,
		NewIdentifier: newId,
		OldIdentifier: oldId,
		Actor:         actor,
	}
}

func (a *TemplateActivity) BuildWorkspaceActivity(entity WorkspaceEntityI) WorkspaceActivity {
	actorId := uuid.NullUUID{UUID: a.Actor.ID, Valid: true}
	workspaceId := entity.GetWorkspaceId()
	return WorkspaceActivity{
		Id:            uuid.Must(uuid.FromString(a.IdActivity)),
		Verb:          a.Verb,
		Field:         a.Field,
		OldValue:      a.OldValue,
		NewValue:      a.NewValue,
		Comment:       a.Comment,
		WorkspaceId:   workspaceId,
		ActorId:       actorId,
		NewIdentifier: a.NewIdentifier,
		OldIdentifier: a.OldIdentifier,
		Notified:      false,
		Actor:         a.Actor,
	}
}

func (a *TemplateActivity) BuildProjectActivity(entity ProjectEntityI) ProjectActivity {
	actorId := uuid.NullUUID{UUID: a.Actor.ID, Valid: true}
	projectId := uuid.Must(uuid.FromString(entity.GetProjectId()))
	workspaceId := entity.GetWorkspaceId()
	return ProjectActivity{
		Id:            uuid.Must(uuid.FromString(a.IdActivity)),
		Verb:          a.Verb,
		Field:         a.Field,
		OldValue:      a.OldValue,
		NewValue:      a.NewValue,
		Comment:       a.Comment,
		WorkspaceId:   workspaceId,
		ProjectId:     projectId,
		ActorId:       actorId,
		NewIdentifier: a.NewIdentifier,
		OldIdentifier: a.OldIdentifier,
		Notified:      false,
		Actor:         a.Actor,
	}
}

func (a *TemplateActivity) BuildSprintActivity(entity SprintEntityI) SprintActivity {
	id := uuid.Must(uuid.FromString(a.IdActivity))
	actorId := uuid.NullUUID{UUID: a.Actor.ID, Valid: true}
	workspaceId := entity.GetWorkspaceId()
	sprintId := uuid.Must(uuid.FromString(entity.GetSprintId()))
	return SprintActivity{
		Id:            id,
		Verb:          a.Verb,
		Field:         a.Field,
		OldValue:      a.OldValue,
		NewValue:      a.NewValue,
		Comment:       a.Comment,
		WorkspaceId:   workspaceId,
		SprintId:      sprintId,
		ActorId:       actorId,
		NewIdentifier: a.NewIdentifier,
		OldIdentifier: a.OldIdentifier,
		Notified:      false,
		Actor:         a.Actor,
	}
}

func (a *TemplateActivity) BuildIssueActivity(entity IssueEntityI) IssueActivity {
	actorId := uuid.NullUUID{UUID: a.Actor.ID, Valid: true}
	projectId := uuid.Must(uuid.FromString(entity.GetProjectId()))
	workspaceId := entity.GetWorkspaceId()
	issueId := uuid.Must(uuid.FromString(entity.GetIssueId()))
	return IssueActivity{
		Id:            uuid.Must(uuid.FromString(a.IdActivity)),
		Verb:          a.Verb,
		Field:         a.Field,
		OldValue:      a.OldValue,
		NewValue:      a.NewValue,
		Comment:       a.Comment,
		WorkspaceId:   workspaceId,
		ProjectId:     projectId,
		IssueId:       issueId,
		ActorId:       actorId,
		NewIdentifier: a.NewIdentifier,
		OldIdentifier: a.OldIdentifier,
		Notified:      false,
		Actor:         a.Actor,
	}
}

func (a *TemplateActivity) BuildFormActivity(entity FormEntityI) FormActivity {
	actorId := uuid.NullUUID{UUID: a.Actor.ID, Valid: true}
	workspaceId := entity.GetWorkspaceId()
	formId := entity.GetFormId()
	return FormActivity{
		Id:            uuid.Must(uuid.FromString(a.IdActivity)),
		Verb:          a.Verb,
		Field:         a.Field,
		OldValue:      a.OldValue,
		NewValue:      a.NewValue,
		Comment:       a.Comment,
		WorkspaceId:   workspaceId,
		FormId:        formId,
		ActorId:       actorId,
		NewIdentifier: a.NewIdentifier,
		OldIdentifier: a.OldIdentifier,
		Notified:      false,
		Actor:         a.Actor,
	}
}

func (a *TemplateActivity) BuildDocActivity(entity DocEntityI) DocActivity {
	actorId := uuid.NullUUID{UUID: a.Actor.ID, Valid: true}
	workspaceId := entity.GetWorkspaceId()
	docId := uuid.Must(uuid.FromString(entity.GetDocId()))
	return DocActivity{
		Id:            uuid.Must(uuid.FromString(a.IdActivity)),
		Verb:          a.Verb,
		Field:         a.Field,
		OldValue:      a.OldValue,
		NewValue:      a.NewValue,
		Comment:       a.Comment,
		WorkspaceId:   workspaceId,
		DocId:         docId,
		ActorId:       actorId,
		NewIdentifier: a.NewIdentifier,
		OldIdentifier: a.OldIdentifier,
		Notified:      false,
		Actor:         a.Actor,
	}
}

func (a *TemplateActivity) BuildRootActivity(entity interface{}) RootActivity {
	actorId := uuid.NullUUID{UUID: a.Actor.ID, Valid: true}
	return RootActivity{
		Id:            uuid.Must(uuid.FromString(a.IdActivity)),
		Verb:          a.Verb,
		Field:         a.Field,
		OldValue:      a.OldValue,
		NewValue:      a.NewValue,
		Comment:       a.Comment,
		ActorId:       actorId,
		NewIdentifier: a.NewIdentifier,
		OldIdentifier: a.OldIdentifier,
		Notified:      false,
		Actor:         a.Actor,
	}
}

// FullActivity
// -migration
type FullActivity struct {
	Id          uuid.UUID
	CreatedAt   time.Time
	Verb        string
	Field       *string
	OldValue    *string
	NewValue    string
	Comment     string `gorm:"-"`
	CreatedById uuid.UUID
	IssueId     uuid.UUID
	ProjectId   uuid.UUID
	UpdatedById uuid.UUID
	WorkspaceId uuid.UUID
	FormId      uuid.UUID
	ActorId     uuid.UUID
	DocId       uuid.NullUUID
	SprintId    uuid.NullUUID
	//AffectedUserId *string

	NewIdentifier *string
	OldIdentifier *string
	Notified      bool `gorm:"-"`

	Workspace *Workspace `gorm:"foreignKey:WorkspaceId" `
	Actor     *User      `gorm:"foreignKey:ActorId;references:ID"`
	Issue     *Issue     `gorm:"foreignKey:IssueId"`
	Project   *Project   `gorm:"foreignKey:ProjectId"`
	Form      *Form      `gorm:"foreignKey:FormId"`
	Doc       *Doc       `gorm:"foreignKey:DocId"`
	Sprint    *Sprint    `gorm:"foreignKey:SprintId"`

	TelegramMsgId pq.Int64Array `json:"-" gorm:"column:telegram_msg_ids;index;type:integer[]"`

	//AffectedUser *User
	EntityType string

	UnionCustomFields string `json:"-" gorm:"-"`

	IssueActivityExtendFields
	ProjectActivityExtendFields
	DocActivityExtendFields
	WorkspaceActivityExtendFields
	RootActivityExtendFields
	SprintActivityExtendFields
	ActivitySender
}

func AddCustomFields(activity UnionableTable, fields []string) []string {
	if field := activity.GetCustomFields(); field == "" {
		return fields
	} else {
		return append(fields, field)

	}
}

func (activity FullActivity) GetCustomFields() string {
	return ""
}

func (fa FullActivity) GetEntity() string {
	return fa.EntityType
}

func (activity FullActivity) TableName() string {
	return "full_activity"
}

func (FullActivity) GetFields() []string {
	return []string{
		"id::uuid",
		"created_at",
		"deleted_at",
		"verb",
		"field",
		"old_value",
		"new_value",
		"created_by_id::uuid",
		"issue_id::uuid",
		"project_id::uuid",
		"updated_by_id::uuid",
		"workspace_id::uuid",
		"form_id::uuid",
		"actor_id::uuid",
		"doc_id::uuid",
		"sprint_id::uuid",
		"new_identifier",
		"old_identifier",
		"telegram_msg_ids",
	}
}

func (fa FullActivity) GetVerb() string {
	return fa.Verb
}

// Функция AfterFind выполняется после получения данных сущности из базы данных.  Она предназначена для выполнения дополнительных операций, связанных с данными, полученными из базы. В данном случае, она вызывает функцию entityActivityAfterFind для обработки данных активности.
func (activity *FullActivity) AfterFind(tx *gorm.DB) error {
	return EntityActivityAfterFind(activity, tx)
}

// Создает легкий DTO из FullActivity.
func (e *FullActivity) ToLightDTO() *dto.EntityActivityLight {
	if e == nil {
		return nil
	}
	return &dto.EntityActivityLight{
		Id:         e.Id,
		Verb:       e.Verb,
		Field:      e.Field,
		OldValue:   e.OldValue,
		NewValue:   e.NewValue,
		EntityType: e.EntityType,
		//TargetUser: e.AffectedUser.ToLightDTO(),
		EntityUrl: e.GetUrl(),
		CreatedAt: e.CreatedAt,
		NewEntity: GetActionEntity(*e, "New"),
		OldEntity: GetActionEntity(*e, "Old"),
	}
}

// Создает полный DTO из структуры FullActivity.
func (e *FullActivity) ToDTO() *dto.EntityActivityFull {
	if e == nil {
		return nil
	}

	return &dto.EntityActivityFull{
		EntityActivityLight: *e.ToLightDTO(),
		Workspace:           e.Workspace.ToLightDTO(),
		Actor:               e.Actor.ToLightDTO(),
		Issue:               e.Issue.ToLightDTO(),
		Project:             e.Project.ToLightDTO(),
		Form:                e.Form.ToLightDTO(),
		Doc:                 e.Doc.ToLightDTO(),
		Sprint:              e.Sprint.ToLightDTO(),
		NewIdentifier:       e.NewIdentifier,
		OldIdentifier:       e.OldIdentifier,
	}
}

// Проверяет, следует ли пропустить предварительную загрузку данных.  Возвращает true, если поле не определено или идентификаторы не установлены, что указывает на то, что предварительная загрузка не требуется.  В противном случае возвращает false.
func (e FullActivity) SkipPreload() bool {
	if e.Field == nil {
		return true
	}

	if e.NewIdentifier == nil && e.OldIdentifier == nil {
		return true
	}
	return false
}

// Возвращает имя поля, связанного с данной активностью.
func (e FullActivity) GetField() string {
	return pointerToStr(e.Field)
}

func (e FullActivity) GetNewIdentifier() string {
	return pointerToStr(e.NewIdentifier)
}

func (e FullActivity) GetOldIdentifier() string {
	return pointerToStr(e.OldIdentifier)
}

func (e FullActivity) GetId() string {
	return e.Id.String()
}

func (e *FullActivity) GetUrl() *string {
	switch e.EntityType {
	case "issue":
		if e.Issue != nil && e.Issue.URL != nil {
			urlStr := e.Issue.URL.String()
			return &urlStr
		}
	case "project":
		if e.Project != nil && e.Project.URL != nil {
			urlStr := e.Project.URL.String()
			return &urlStr
		}
	case "workspace":
		if e.Workspace != nil && e.Workspace.URL != nil {
			urlStr := e.Workspace.URL.String()
			return &urlStr
		}
	case "form":
		if e.Form != nil && e.Form.URL != nil {
			urlStr := e.Form.URL.String()
			return &urlStr
		}
	}
	return nil
}

func GetActionEntity[A Activity](a A, pref string) any {
	val := reflect.ValueOf(a)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	typ := val.Type()
	return getEntityRecursive(val, typ, pref)
}

func getEntityRecursive(val reflect.Value, typ reflect.Type, pref string) any {
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		fieldVal := val.Field(i)

		if !fieldVal.IsValid() || !fieldVal.CanInterface() {
			continue
		}

		if field.Anonymous && fieldVal.Kind() == reflect.Struct {
			if res := getEntityRecursive(fieldVal, fieldVal.Type(), pref); res != nil {
				return res
			}
			continue
		}

		tag := field.Tag.Get("field")
		if tag == "" || !strings.HasPrefix(field.Name, pref) {
			continue
		}

		if fieldVal.Kind() == reflect.Ptr && !fieldVal.IsNil() {
			method := fieldVal.MethodByName("ToLightDTO")
			if method.IsValid() && method.Type().NumIn() == 0 && method.Type().NumOut() == 1 {
				return method.Call(nil)[0].Interface()
			} else {
				errMsg := fmt.Sprintf("Err: %s not have method 'ToLightDTO'", fieldVal.Type().String())
				slog.Info(errMsg)
			}
		}
	}
	return nil
}

// ActivitySender
// -migration
type ActivitySender struct {
	SenderTg int64 `json:"-" gorm:"-"`
}
