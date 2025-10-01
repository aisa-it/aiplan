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

	"github.com/aisa-it/aiplan/internal/aiplan/dto"
	"github.com/gofrs/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

const (
	ACTIVITY_UPDATED = "updated"
	ACTIVITY_REMOVED = "removed"
	ACTIVITY_ADDED   = "added"
)

type Entity interface {
	User |
		Workspace | WorkspaceMember |
		Project | ProjectMember | Label | State | IssueTemplate |
		Issue | IssueLink | IssueComment | IssueAttachment |
		Doc | DocComment | DocAttachment |
		Form
}

type Activity interface {
	IssueActivity | ProjectActivity | FormActivity | WorkspaceActivity | DocActivity | EntityActivity | FullActivity | RootActivity
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
func NewTemplateActivity(verb string, field *string, oldVal *string, newVal string, newId, oldId *string, actor *User, valToComment string) TemplateActivity {
	var comment string
	switch verb {
	case ACTIVITY_UPDATED:
		comment = fmt.Sprintf("%s updated %s to %s", actor.Email, strings.Replace(*field, "_", " ", 1), valToComment)
	case ACTIVITY_REMOVED:
		comment = fmt.Sprintf("%s removed from %s - %s", actor.Email, strings.Replace(*field, "_", " ", 1), valToComment)
	case ACTIVITY_ADDED:
		comment = fmt.Sprintf("%s added to %s - %s", actor.Email, strings.Replace(*field, "_", " ", 1), valToComment)
	}

	return TemplateActivity{
		IdActivity:    GenID(),
		Verb:          verb,
		Field:         field,
		OldValue:      oldVal,
		NewValue:      newVal,
		Comment:       comment,
		NewIdentifier: newId,
		OldIdentifier: oldId,
		Actor:         actor,
	}
}

func (a *TemplateActivity) BuildWorkspaceActivity(entity WorkspaceEntityI) WorkspaceActivity {
	return WorkspaceActivity{
		Id:            a.IdActivity,
		Verb:          a.Verb,
		Field:         a.Field,
		OldValue:      a.OldValue,
		NewValue:      a.NewValue,
		Comment:       a.Comment,
		WorkspaceId:   entity.GetWorkspaceId(),
		ActorId:       &a.Actor.ID,
		NewIdentifier: a.NewIdentifier,
		OldIdentifier: a.OldIdentifier,
		Notified:      false,
		Actor:         a.Actor,
	}
}

func (a *TemplateActivity) BuildProjectActivity(entity ProjectEntityI) ProjectActivity {
	return ProjectActivity{
		Id:            a.IdActivity,
		Verb:          a.Verb,
		Field:         a.Field,
		OldValue:      a.OldValue,
		NewValue:      a.NewValue,
		Comment:       a.Comment,
		WorkspaceId:   entity.GetWorkspaceId(),
		ProjectId:     entity.GetProjectId(),
		ActorId:       &a.Actor.ID,
		NewIdentifier: a.NewIdentifier,
		OldIdentifier: a.OldIdentifier,
		Notified:      false,
		Actor:         a.Actor,
	}
}

func (a *TemplateActivity) BuildIssueActivity(entity IssueEntityI) IssueActivity {
	return IssueActivity{
		Id:            a.IdActivity,
		Verb:          a.Verb,
		Field:         a.Field,
		OldValue:      a.OldValue,
		NewValue:      a.NewValue,
		Comment:       a.Comment,
		WorkspaceId:   entity.GetWorkspaceId(),
		ProjectId:     entity.GetProjectId(),
		IssueId:       entity.GetIssueId(),
		ActorId:       &a.Actor.ID,
		NewIdentifier: a.NewIdentifier,
		OldIdentifier: a.OldIdentifier,
		Notified:      false,
		Actor:         a.Actor,
	}
}

func (a *TemplateActivity) BuildFormActivity(entity FormEntityI) FormActivity {
	return FormActivity{
		Id:            a.IdActivity,
		Verb:          a.Verb,
		Field:         a.Field,
		OldValue:      a.OldValue,
		NewValue:      a.NewValue,
		Comment:       a.Comment,
		WorkspaceId:   entity.GetWorkspaceId(),
		FormId:        entity.GetFormId(),
		ActorId:       &a.Actor.ID,
		NewIdentifier: a.NewIdentifier,
		OldIdentifier: a.OldIdentifier,
		Notified:      false,
		Actor:         a.Actor,
	}
}

func (a *TemplateActivity) BuildDocActivity(entity DocEntityI) DocActivity {
	return DocActivity{
		Id:            a.IdActivity,
		Verb:          a.Verb,
		Field:         a.Field,
		OldValue:      a.OldValue,
		NewValue:      a.NewValue,
		Comment:       a.Comment,
		WorkspaceId:   entity.GetWorkspaceId(),
		DocId:         entity.GetDocId(),
		ActorId:       &a.Actor.ID,
		NewIdentifier: a.NewIdentifier,
		OldIdentifier: a.OldIdentifier,
		Notified:      false,
		Actor:         a.Actor,
	}
}

func (a *TemplateActivity) BuildRootActivity(entity interface{}) RootActivity {
	return RootActivity{
		Id:            a.IdActivity,
		Verb:          a.Verb,
		Field:         a.Field,
		OldValue:      a.OldValue,
		NewValue:      a.NewValue,
		Comment:       a.Comment,
		ActorId:       &a.Actor.ID,
		NewIdentifier: a.NewIdentifier,
		OldIdentifier: a.OldIdentifier,
		Notified:      false,
		Actor:         a.Actor,
	}
}

// FullActivity
// -migration
type FullActivity struct {
	Id          string
	CreatedAt   time.Time
	Verb        string
	Field       *string
	OldValue    *string
	NewValue    string
	Comment     string `gorm:"-"`
	CreatedById *string
	IssueId     *string
	ProjectId   *string
	UpdatedById *string
	WorkspaceId string
	FormId      *string
	ActorId     *string
	DocId       uuid.NullUUID
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

	TelegramMsgId pq.Int64Array `json:"-" gorm:"column:telegram_msg_ids;index;type:integer[]"`

	//AffectedUser *User
	EntityType string

	UnionCustomFields string `json:"-" gorm:"-"`

	IssueActivityExtendFields
	ProjectActivityExtendFields
	DocActivityExtendFields
	WorkspaceActivityExtendFields
	RootActivityExtendFields

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
		"id",
		"created_at",
		"deleted_at",
		"verb",
		"field",
		"old_value",
		"new_value",
		"created_by_id",
		"issue_id",
		"project_id",
		"updated_by_id",
		"workspace_id",
		"form_id",
		"actor_id",
		"doc_id::uuid",
		"affected_user_id",
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
	return e.Id
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
