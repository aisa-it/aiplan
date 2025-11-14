// DAO (Data Access Object) - предоставляет интерфейс для взаимодействия с базой данных.  Содержит функции для работы с сущностями, такими как сессии пользователей, состояния задач, файлы, заметки и другие.  Обеспечивает абстракцию от конкретной реализации базы данных и упрощает доступ к данным приложения.
//
// Основные возможности:
//   - Работа с сессиями пользователей (создание, обновление).
//   - Управление состояниями задач (создание, обновление, получение).
//   - Доступ к файлам (сохранение, удаление, получение).
//   - Работа с заметками (создание, обновление, получение).
//   - Работа с тарификацией (получение информации о лимитах).
//   - Поддержка различных типов сущностей и операций CRUD.
package dao

import (
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/config"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dto"
	filestorage "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/file-storage"
	"github.com/gofrs/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

// GenID генерирует уникальный идентификатор в формате UUID.
// Не принимает параметров и возвращает строку, представляющую собой UUID.
func GenID() string {
	u2, _ := uuid.NewV4()
	return u2.String()
}

// GenUUID генерирует уникальный идентификатор в формате UUID. Не принимает параметров и возвращает UUID.
//
// Возвращает:
//   - uuid.UUID: UUID, представляющий собой уникальный идентификатор.
func GenUUID() uuid.UUID {
	u2, _ := uuid.NewV4()
	return u2
}

var Config *config.Config
var FileStorage filestorage.FileStorage

type SessionsReset struct {
	// id uuid IS_NULL:NO
	Id string `json:"id"`
	// user_id uuid IS_NULL:NO
	UserId string `json:"user_id" gorm:"index"`
	// reseted_at timestamp without time zone IS_NULL:NO
	ResetedAt time.Time `json:"reseted_at"`

	User *User `gorm:"foreignKey:UserId"`
}

// Возвращает имя таблицы для данного типа структуры.
func (SessionsReset) TableName() string { return "sessions_resets" }

// Сбрасывает сессии пользователя, создавая запись о сбросе в базе данных.
//
// Параметры:
//   - db: экземпляр gorm.DB для взаимодействия с базой данных.
//   - user: пользователь, сессии которого необходимо сбросить.
//
// Возвращает:
//   - error: ошибка, если при создании записи произошла ошибка.
func ResetUserSessions(db *gorm.DB, user *User) error {
	return db.Create(&SessionsReset{
		Id:        GenID(),
		UserId:    user.ID,
		ResetedAt: time.Now(),
	}).Error
}

type FileAsset struct {
	Id          uuid.UUID `json:"id" gorm:"primaryKey;type:uuid"`
	CreatedAt   time.Time `json:"created_at"`
	CreatedById *string   `json:"created_by,omitempty" extensions:"x-nullable"`

	WorkspaceId *string       `json:"workspace,omitempty"`
	IssueId     uuid.NullUUID `json:"issue" gorm:"foreignKey:ID"`
	CommentId   *uuid.UUID    `json:"comment,omitempty" gorm:"foreignKey:IdActivity" extensions:"x-nullable"`

	DocId        uuid.NullUUID `json:"doc" gorm:"foreignKey:ID;type:uuid"`
	DocCommentId uuid.NullUUID `json:"doc_comment" gorm:"type:uuid"`

	FormId uuid.NullUUID `json:"form" gorm:"foreignKey:ID;type:uuid"`

	Name        string `json:"name" gorm:"index"`
	FileSize    int    `json:"size"`
	ContentType string `json:"content_type"`

	Workspace *Workspace `json:"-" gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	Author    *User      `json:"-" gorm:"foreignKey:CreatedById" extensions:"x-nullable"`
}

// Удаляет запись о сбросе сессии пользователя в базе данных.
//
// Параметры:
//   - tx: экземпляр gorm.DB для взаимодействия с базой данных.
//
// Возвращает:
//   - error: ошибка, если при создании записи произошла ошибка.
func (asset *FileAsset) BeforeDelete(tx *gorm.DB) error {
	exist, err := FileStorage.Exist(asset.Id)
	if err != nil {
		return err
	}

	if exist {
		if err := FileStorage.Delete(asset.Id); err != nil {
			return err
		}
	}
	return nil
}

// CanBeDeleted проверяет, можно ли удалить запись, основываясь на количестве связанных сущностей (issue, project, workspace, form). Если связанных сущностей нет, то запись может быть удалена.
//
// Парамметры:
//   - tx: экземпляр gorm.DB для выполнения запросов к базе данных.
//
// Возвращает:
//   - bool: true, если запись может быть удалена, false в противном случае.
func (asset *FileAsset) CanBeDeleted(tx *gorm.DB) (bool, error) {
	var exists bool
	if err := tx.Raw(`
        SELECT EXISTS(SELECT 1 FROM issue_attachments WHERE asset_id = ?)
           OR EXISTS(SELECT 1 FROM doc_attachments WHERE asset_id = ?)
           OR EXISTS(SELECT 1 FROM form_attachments WHERE asset_id = ?)
           OR EXISTS(SELECT 1 FROM users WHERE avatar_id = ?)
           OR EXISTS(SELECT 1 FROM workspaces WHERE logo_id = ?)`,
		asset.Id, asset.Id, asset.Id, asset.Id, asset.Id).Scan(&exists).Error; err != nil {
		return false, err
	}
	return !exists, nil
}

// Преобразует объект FileAsset в его DTO-представление для упрощенной передачи данных в интерфейс.
func (asset *FileAsset) ToDTO() *dto.FileAsset {
	if asset == nil {
		return nil
	}
	return &dto.FileAsset{
		Id:          asset.Id.String(),
		Name:        asset.Name,
		FileSize:    asset.FileSize,
		ContentType: asset.ContentType,
	}
}

type ReleaseNote struct {
	ID          uuid.UUID          `gorm:"primaryKey" json:"id"`
	TagName     string             `json:"tag_name" gorm:"uniqueIndex"`
	PublishedAt time.Time          `json:"published_at"`
	Body        types.RedactorHTML `json:"body"`
	AuthorId    string             `json:"-"`

	Author *User `gorm:"foreignKey:AuthorId" json:"-" extensions:"x-nullable"`
}

// ToLightDTO преобразует объект ReleaseNote в его облегченное DTO представление. Используется для упрощения передачи данных в интерфейс.
//
// Параметры:
//   - self: объект ReleaseNote, который нужно преобразовать.
//
// Возвращает:
//   - *dto.ReleaseNoteLight: DTO представление объекта ReleaseNote.
func (r *ReleaseNote) ToLightDTO() *dto.ReleaseNoteLight {
	if r == nil {
		return nil
	}

	return &dto.ReleaseNoteLight{
		ID:          r.ID.String(),
		TagName:     r.TagName,
		PublishedAt: r.PublishedAt,
		Body:        r.Body,
	}
}

// BeforeCreate создает запись о сбросе сессии пользователя в базе данных.
//
// Параметры:
//   - tx: экземпляр gorm.DB для взаимодействия с базой данных.
//   - user: пользователь, сессии которого необходимо сбросить.
//
// Возвращает:
//   - error: ошибка, если при создании записи произошла ошибка.
func (r *ReleaseNote) BeforeCreate(tx *gorm.DB) (err error) {
	r.ID, err = uuid.NewV4()
	r.PublishedAt = time.Now()
	return
}

type EntityActivity struct {
	Id        string    `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time `json:"created_at" gorm:"index:entity_activities_issue_index,sort:desc,type:btree,priority:2;index:entity_activities_actor_index,sort:desc,type:btree,priority:2;index:entity_activities_mail_index,type:btree,where:notified = false and issue_id is not null"`
	//DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`
	// verb character varying IS_NULL:NO
	Verb string `json:"verb"`
	// field character varying IS_NULL:YES
	Field *string `json:"field,omitempty" extensions:"x-nullable"`
	// old_value text IS_NULL:YES
	OldValue *string `json:"old_value" extensions:"x-nullable"`
	// new_value text IS_NULL:YES
	NewValue string `json:"new_value" `
	// comment text IS_NULL:NO
	Comment string `json:"comment"`
	// attachments ARRAY IS_NULL:NO
	Attachments string `json:"attachments"`
	// created_by_id uuid IS_NULL:YES
	CreatedById *string `json:"created_by_id,omitempty" extensions:"x-nullable"`
	// issue_id uuid IS_NULL:YES
	IssueId *string `json:"issue_id,omitempty" gorm:"index:entity_activities_issue_index,priority:1" extensions:"x-nullable"`
	// project_id uuid IS_NULL:YES
	ProjectId *string `json:"project_id"`
	// updated_by_id uuid IS_NULL:YES
	UpdatedById *string `json:"updated_by_id,omitempty" extensions:"x-nullable"`
	// workspace_id uuid IS_NULL:NO
	WorkspaceId string `json:"workspace"`
	// form_id uuid IS_NULL:YES
	FormId *string `json:"form_id"`
	// actor_id uuid IS_NULL:YES
	ActorId *string `json:"actor,omitempty" gorm:"index:entity_activities_actor_index,priority:1" extensions:"x-nullable"`
	// doc_id uuid IS_NULL:YES
	DocId *string `json:"doc_id"`

	// new_identifier uuid IS_NULL:YES
	NewIdentifier *string `json:"new_identifier" extensions:"x-nullable"`
	// old_identifier uuid IS_NULL:YES
	OldIdentifier *string       `json:"old_identifier" extensions:"x-nullable"`
	Notified      bool          `json:"-" gorm:"default:false"`
	TelegramMsgId pq.Int64Array `json:"-" gorm:"column:telegram_msg_ids;index;type:integer[]"`

	Workspace *Workspace `json:"workspace_detail" gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	Actor     *User      `json:"actor_detail" gorm:"foreignKey:ActorId" extensions:"x-nullable"`
	Issue     *Issue     `json:"issue_detail" gorm:"foreignKey:IssueId" extensions:"x-nullable"`
	Project   *Project   `json:"project_detail" gorm:"foreignKey:ProjectId" extensions:"x-nullable"`
	Form      *Form      `json:"form_detail" gorm:"foreignKey:FormId" extensions:"x-nullable"`

	NewAttachment *IssueAttachment `json:"-" gorm:"-" field:"attachment" extensions:"x-nullable"`
	NewLink       *IssueLink       `json:"-" gorm:"-" field:"link" extensions:"x-nullable"`

	NewAssignee *User `json:"-" gorm:"-" field:"assignees" extensions:"x-nullable"`
	OldAssignee *User `json:"-" gorm:"-" field:"assignees" extensions:"x-nullable"`

	NewWatcher *User `json:"-" gorm:"-" field:"watchers" extensions:"x-nullable"`
	OldWatcher *User `json:"-" gorm:"-" field:"watchers" extensions:"x-nullable"`

	NewSubIssue *Issue `json:"-" gorm:"-" field:"sub_issue" extensions:"x-nullable"`
	OldSubIssue *Issue `json:"-" gorm:"-" field:"sub_issue" extensions:"x-nullable"`

	NewRole *User `json:"-" gorm:"-" field:"role" extensions:"x-nullable"`
	OldRole *User `json:"-" gorm:"-" field:"role" extensions:"x-nullable"`

	NewMember *User `json:"-" gorm:"-" field:"member" extensions:"x-nullable"`
	OldMember *User `json:"-" gorm:"-" field:"member" extensions:"x-nullable"`

	NewDefaultAssignee *User `json:"-" gorm:"-" field:"default_assignees" extensions:"x-nullable"`
	OldDefaultAssignee *User `json:"-" gorm:"-" field:"default_assignees" extensions:"x-nullable"`

	NewDefaultWatcher *User `json:"-" gorm:"-" field:"default_watchers" extensions:"x-nullable"`
	OldDefaultWatcher *User `json:"-" gorm:"-" field:"default_watchers" extensions:"x-nullable"`

	NewProjectLead *User `json:"-" gorm:"-" field:"project_lead" extensions:"x-nullable"`
	OldProjectLead *User `json:"-" gorm:"-" field:"project_lead" extensions:"x-nullable"`

	//AffectedUser *User `json:"affected_user,omitempty" gorm:"-" extensions:"x-nullable"`

	NewIssueComment *IssueComment `json:"-" gorm:"-" field:"comment::issue" extensions:"x-nullable"`

	EntityType        string `json:"entity_type"`
	EntityId          string `json:"entity_id"`
	UnionCustomFields string `json:"-" gorm:"-"`
	ActivitySender
}

// ToLightDTO преобразует объект EntityActivity в его облегченное DTO представление. Используется для упрощения передачи данных в интерфейс.
//
// Параметры:
//   - self: объект EntityActivity, который нужно преобразовать.
//
// Возвращает:
//   - *dto.EntityActivityLight: облегченное представление объекта EntityActivity.
func (e *EntityActivity) ToLightDTO() *dto.EntityActivityLight {
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
		NewEntity: GetActionEntity(*e, "New"),
		OldEntity: GetActionEntity(*e, "Old"),

		EntityUrl: e.GetUrl(),
		CreatedAt: e.CreatedAt,
	}
}

// Преобразует объект EntityActivity в его полное DTO представление, включая все связанные данные и информацию о сущностях.
func (e *EntityActivity) ToFullDTO() *dto.EntityActivityFull {
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

		NewIdentifier: e.NewIdentifier,
		OldIdentifier: e.OldIdentifier,
	}
}

// Преобразует объект EntityActivity в его полное DTO представление, включая все связанные данные и информацию о сущностях.
func (eaWithLag *EntityActivityWithLag) ToDTO() *dto.EntityActivityFull {
	if eaWithLag == nil {
		return nil
	}
	res := eaWithLag.ToFullDTO()
	res.StateLag = eaWithLag.StateLag
	return res
}

// Возвращает URL для данного типа сущности, формируя его на основе имени поля 'url'. Если поле 'url' не найдено, возвращает пустую строку.
func (e *EntityActivity) GetUrl() *string {
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

func (ea EntityActivity) SetTgSender(id int64) {
	ea.ActivitySender.SenderTg = id
}

func (EntityActivity) GetFields() []string {
	return []string{
		"id",
		"created_at",
		"deleted_at",
		"verb",
		"field",
		"old_value",
		"new_value",
		"comment",
		"created_by_id",
		"issue_id",
		"project_id",
		"updated_by_id",
		"workspace_id",
		"form_id",
		"actor_id",
		"doc_id",
		"affected_user_id",
		"new_identifier",
		"old_identifier",
		"notified",
		"telegram_msg_ids",
	}
}

// EntityActivityWithLag
// -migration
type EntityActivityWithLag struct {
	EntityActivity
	StateLag int `json:"state_lag_ms,omitempty" gorm:"->;-:migration"`
}

// AfterFind выполняет дополнительные действия после поиска записи в базе данных.  В частности, она проверяет и обновляет поля, которые могут быть изменены в процессе поиска, такие как идентификаторы пользователей и другие связанные данные.  Функция принимает экземпляр базы данных (tx) для выполнения запросов и возвращает ошибку, если какие-либо проблемы возникают.
func (activity *EntityActivity) AfterFind(tx *gorm.DB) error {
	return EntityActivityAfterFind(activity, tx)
}

// Проверяет, нужно ли пропустить предварительную загрузку связанных данных. Возвращает true, если предварительная загрузка не требуется, иначе false.
func (ea EntityActivity) SkipPreload() bool {

	if ea.NewIdentifier == nil && ea.OldIdentifier == nil {
		return true
	}
	return false
}

// Возвращает значение поля, указанного в метоке поля.  Параметр `field` определяет имя поля, значение которого необходимо получить.  Возвращает строку, содержащую значение поля, или пустую строку, если поле не найдено или не определено.
func (ea EntityActivity) GetField() string {
	if ea.Field == nil {
		return ""
	}

	return pointerToStr(ea.Field)
}

func (ea EntityActivity) GetVerb() string {
	return ea.Verb
}

// Возвращает новый идентификатор, связанный с полем 'NewIdentifier' объекта EntityActivity.  Используется для получения уникального идентификатора, сгенерированного при создании или обновлении сущности.
func (ea EntityActivity) GetNewIdentifier() string {
	return pointerToStr(ea.NewIdentifier)
}

// GetOldIdentifier возвращает старый идентификатор сущности, используя префикс 'Old' для имени поля.
//
// Параметры:
//   - activity: объект EntityActivity, из которого нужно извлечь старый идентификатор.
//
// Возвращает:
//   - string: старый идентификатор сущности, или пустая строка, если он не найден или не может быть получен.
func (ea EntityActivity) GetOldIdentifier() string {
	return pointerToStr(ea.OldIdentifier)
}

// entityActivityAfterFind выполняет дополнительные действия после поиска записи в базе данных.  Она обновляет поля, которые могут быть изменены в процессе поиска, такие как идентификаторы пользователей.
//
// Параметры:
//   - activity: объект EntityActivity, после поиска которого нужно выполнить дополнительные действия.
//   - tx: экземпляр gorm.DB для выполнения запросов к базе данных.
//
// Возвращает:
//   - error: ошибка, если при выполнении каких-либо операций произошла ошибка.
func EntityActivityAfterFind[A Activity](activity *A, tx *gorm.DB) error {
	aI, ok := any(*activity).(ActivityI)

	targetField := aI.GetField()
	switch targetField {
	case "target_date":
		if v, ok := any(*activity).(FullActivity); ok {
			if v.NewValue != "" {
				if formatted, err := utils.FormatDateStr(v.NewValue, "2006-01-02T15:04:05Z07:00", nil); err == nil {
					v.NewValue = formatted
				} else {
					slog.Error("format error", "error", err)
				}
			}

			if v.OldValue != nil && *v.OldValue != "" {
				if formatted, err := utils.FormatDateStr(*v.OldValue, "2006-01-02T15:04:05Z07:00", nil); err == nil {
					v.OldValue = &formatted
				} else {
					slog.Error("format error", "error", err)
				}
			}
			//date, err := utils.FormatDateStr(v.NewValue, "2006-01-02T15:04:05Z07:00", nil)
			//if err != nil {
			//  return err
			//}
			//v.NewValue = date
			*activity = any(v).(A)
		}
	}

	if !ok || aI.SkipPreload() {
		return nil
	}

	newID := aI.GetNewIdentifier()
	oldID := aI.GetOldIdentifier()
	verb := aI.GetVerb()
	if newID == "" && oldID == "" {
		return nil
	}

	val := reflect.ValueOf(activity).Elem()
	typ := val.Type()

	targetFieldExt := fmt.Sprintf("%s::%s", targetField, aI.GetEntity())

	//var affectedUser *User

	var walkStruct func(reflect.Value, reflect.Type) error

	walkStruct = func(v reflect.Value, t reflect.Type) error {
		for i := 0; i < t.NumField(); i++ {
			structField := t.Field(i)
			fieldVal := v.Field(i)

			if structField.Anonymous && structField.Type.Kind() == reflect.Struct {
				if err := walkStruct(fieldVal, structField.Type); err != nil {
					return err
				}
				continue
			}

			fieldTag, ok := structField.Tag.Lookup("field")
			if ok {
				switch targetField {
				case "link_title", "link_url", "status_color", "status_name", "status_description", "status_group", "label_name", "label_color", "status_default", "template_name", "template_template":
					targetField = strings.Split(targetField, "_")[0]
					if targetField == "status" {
						targetField = "state"
					}
				}
				if fieldTag != targetField && fieldTag != targetFieldExt {
					continue
				}
				if verb == "move" {

				}
			} else {
				continue
			}

			fieldName := structField.Name

			if newID != "" && strings.HasPrefix(fieldName, "New") {
				ptr := reflect.New(structField.Type.Elem()) // *T
				err := tx.Where("id = ?", newID).First(ptr.Interface()).Error
				if err == nil {
					fieldVal.Set(ptr)
					//if structField.Type == reflect.TypeOf(&User{}) {
					//	affectedUser = ptr.Interface().(*User)
					//}
				} else if err != gorm.ErrRecordNotFound {
					continue
				} else {
					slog.Info(fmt.Sprintf("ERR EntityActivityAfterFind: field: \"%s\", fieldTag: \"%s\", fieldType: \"%T\", id: \"%s\", activityId: \"%s\" error: \"%s\"", fieldName, fieldTag, ptr.Interface(), newID, aI.GetId(), err.Error()))
					continue
				}
			}

			if oldID != "" && strings.HasPrefix(fieldName, "Old") {
				ptr := reflect.New(structField.Type.Elem()) // *T
				err := tx.Where("id = ?", oldID).First(ptr.Interface()).Error
				if err == nil {
					fieldVal.Set(ptr)

					//if structField.Type == reflect.TypeOf(&User{}) {
					//	affectedUser = ptr.Interface().(*User)
					//}
				} else if err != gorm.ErrRecordNotFound {
					continue
				} else {
					slog.Info(fmt.Sprintf("ERR EntityActivityAfterFind: field: \"%s\", fieldTag: \"%s\", fieldType: \"%T\", id: \"%s\", activityId: \"%s\" error: \"%s\"", fieldName, fieldTag, ptr.Interface(), oldID, aI.GetId(), err.Error()))
					continue
				}
			}
		}
		return nil
	}

	if err := walkStruct(val, typ); err != nil {
		return err
	}

	//if affectedUser != nil { // TODO remove affected user
	//	val.FieldByName("AffectedUser").Set(reflect.ValueOf(affectedUser))
	//}

	return nil
}

// BeforeSave обходит объект перед сохранением в базе данных.  В частности, проверяет и устанавливает значения полей, которые могут быть изменены в процессе сохранения, такие как 'NewValue'.
//
// Парамметры:
//   - tx: экземпляр gorm.DB для выполнения операций с базой данных.
//
// Возвращает:
//   - error: ошибка, если при выполнении операций произошла ошибка.
func (activity *EntityActivity) BeforeSave(tx *gorm.DB) error {
	if activity.Attachments == "" {
		activity.Attachments = "{}"
	}

	if activity.NewValue == "<nil>" {
		activity.NewValue = ""
	}

	return nil
}

// BeforeDelete удаляет запись из базы данных перед фактическим удалением.  Проверяет наличие связанных записей в таблице UserNotifications и удаляет их, чтобы избежать проблем с целостностью данных.
//
// Параметры:
//   - tx: экземпляр gorm.DB для выполнения операций с базой данных.
//
// Возвращает:
//   - error: ошибка, если при выполнении операций произошла ошибка.
func (activity *EntityActivity) BeforeDelete(tx *gorm.DB) error {
	if err := tx.Unscoped().Where("entity_activity_id = ?", activity.Id).Delete(&UserNotifications{}).Error; err != nil {
		return err
	}
	return nil
}

// Возвращает имя таблицы для данного типа структуры.
func (EntityActivity) TableName() string { return "entity_activities" }

// DeferredNotifications corresponds to the notifications_log table
type DeferredNotifications struct {
	ID string `gorm:"type:text;primaryKey"`

	UserID      string     `gorm:"type:text;not null;index"`
	User        *User      `gorm:"foreignKey:UserID" extensions:"x-nullable"`
	IssueID     *string    `gorm:"type:text;index" extensions:"x-nullable"`
	Issue       *Issue     `gorm:"foreignKey:IssueID" extensions:"x-nullable"`
	ProjectID   *string    `gorm:"type:text;index" extensions:"x-nullable"`
	Project     *Project   `gorm:"foreignKey:ProjectID" extensions:"x-nullable"`
	WorkspaceID *string    `gorm:"type:text;index" extensions:"x-nullable"`
	Workspace   *Workspace `gorm:"foreignKey:WorkspaceID" extensions:"x-nullable"`

	NotificationType    string     `gorm:"type:varchar(50);not null"`
	DeliveryMethod      string     `gorm:"type:varchar(50);not null"`
	TimeSend            *time.Time `gorm:"type:timestamptz;index"`
	AttemptCount        int        `gorm:"default:0;index:idx_deferred_notifications_attempt_count"`
	LastAttemptAt       time.Time  `gorm:"type:timestamptz;autoUpdateTime"`
	SentAt              *time.Time `gorm:"type:timestamptz;index:idx_deferred_notifications_sent_at" extensions:"x-nullable"`
	NotificationPayload []byte     `gorm:"type:jsonb"`
}

// TableName sets the insert table name for this struct type
func (DeferredNotifications) TableName() string {
	return "deferred_notifications"
}

type RootActivity struct {
	Id        string    `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time `json:"created_at" gorm:"index:activities_actor_index,sort:desc,type:btree,priority:2;index:activities_mail_index,type:btree,where:notified = false"`
	// verb character varying IS_NULL:NO
	Verb string `json:"verb"`
	// field character varying IS_NULL:YES
	Field *string `json:"field,omitempty" extensions:"x-nullable"`
	// old_value text IS_NULL:YES
	OldValue *string `json:"old_value" extensions:"x-nullable"`
	// new_value text IS_NULL:YES
	NewValue string `json:"new_value" `
	// comment text IS_NULL:NO
	Comment string `json:"comment"`
	// actor_id uuid IS_NULL:YES
	ActorId *string `json:"actor,omitempty" gorm:"index:activities_actor_index,priority:1" extensions:"x-nullable"`

	// new_identifier uuid IS_NULL:YES
	NewIdentifier *string `json:"new_identifier" extensions:"x-nullable"`
	// old_identifier uuid IS_NULL:YES
	OldIdentifier *string       `json:"old_identifier" extensions:"x-nullable"`
	Notified      bool          `json:"-" gorm:"default:false"`
	TelegramMsgId pq.Int64Array `json:"-" gorm:"column:telegram_msg_ids;index;type:integer[]"`

	Actor *User `json:"actor_detail" gorm:"foreignKey:ActorId" extensions:"x-nullable"`

	NewWorkspace      *Workspace `json:"-" gorm:"-" field:"workspace" extensions:"x-nullable"`
	NewDoc            *Doc       `json:"-" gorm:"-" field:"doc" extensions:"x-nullable"`
	UnionCustomFields string     `json:"-" gorm:"-"`
	RootActivityExtendFields
	ActivitySender
}

// WorkspaceActivityExtendFields
// -migration
type RootActivityExtendFields struct {
	WorkspaceExtendFields
}

func (ra RootActivity) GetCustomFields() string {
	return ra.UnionCustomFields
}

func (RootActivity) GetEntity() string {
	return "root"
}

func (ra RootActivity) GetFields() []string {
	return []string{"id", "created_at", "verb", "field", "old_value", "new_value", "actor_id", "new_identifier", "old_identifier"}
}

func (ra RootActivity) SetTgSender(id int64) {
	ra.ActivitySender.SenderTg = id
}

func (RootActivity) TableName() string { return "activities" }

func (ra RootActivity) SkipPreload() bool {
	if ra.Field == nil {
		return true
	}

	if ra.NewIdentifier == nil && ra.OldIdentifier == nil {
		return true
	}
	return false
}

func (ra RootActivity) GetField() string {
	return pointerToStr(ra.Field)
}

func (ra RootActivity) GetVerb() string {
	return ra.Verb
}

func (ra RootActivity) GetNewIdentifier() string {
	return pointerToStr(ra.NewIdentifier)
}

func (ra RootActivity) GetOldIdentifier() string {
	return pointerToStr(ra.OldIdentifier)
}

func (ra RootActivity) GetId() string {
	return ra.Id
}
