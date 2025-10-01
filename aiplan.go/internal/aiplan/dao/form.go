// DAO (Data Access Object) для работы с данными форм.  Предоставляет методы для создания, чтения, обновления и удаления форм, а также связанных с ними сущностей (ответы, вложения).  Включает логику валидации и преобразования данных для взаимодействия с базой данных и DTO (Data Transfer Objects).
package dao

import (
	"database/sql"
	"fmt"

	"github.com/lib/pq"

	"html"
	"net/url"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dto"
	policy "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/redactor-policy"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

type Form struct {
	ID        uuid.UUID `gorm:"column:id;primaryKey;type:text" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	CreatedById string  `json:"created_by" gorm:"index"`
	UpdatedById *string `json:"updated_by" extensions:"x-nullable"`
	Author      *User   `json:"author_detail" gorm:"foreignKey:CreatedById" extensions:"x-nullable"`

	Slug        string             `json:"slug" gorm:"uniqueIndex;not null"`
	Title       string             `json:"title" validate:"required"`
	Description types.RedactorHTML `json:"description"`
	AuthRequire bool               `json:"auth_require"`

	TargetProjectId sql.NullString
	TargetProject   *Project `gorm:"foreignKey:TargetProjectId" extensions:"x-nullable"`

	EndDate     *types.TargetDate `json:"end_date" gorm:"index" extensions:"x-nullable"`
	WorkspaceId string            `json:"workspace" gorm:"index"`
	Workspace   *Workspace        `json:"workspace_detail" gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`

	Fields types.FormFieldsSlice `json:"fields" gorm:"type:jsonb"`
	Active bool                  `json:"active" gorm:"-"`

	URL                    *url.URL         `json:"-" gorm:"-" extensions:"x-nullable"`
	CurrentWorkspaceMember *WorkspaceMember `json:"current_workspace_member,omitempty" gorm:"-" extensions:"x-nullable"`
}

func (f Form) GetId() string {
	return f.ID.String()
}

func (f Form) GetString() string {
	return f.Slug
}

func (f Form) GetEntityType() string {
	return "form"
}

func (f Form) GetWorkspaceId() string {
	return f.WorkspaceId
}

func (f Form) GetFormId() string {
	return f.GetId()
}

// ToLightDTO преобразует Form в FormLight для упрощенной передачи данных. Используется для создания более легкой версии формы для отображения в интерфейсе.
func (f *Form) ToLightDTO() *dto.FormLight {
	if f == nil {
		return nil
	}
	f.SetUrl()
	ff := &dto.FormLight{
		ID:          f.ID.String(),
		Slug:        f.Slug,
		Title:       f.Title,
		Description: f.Description,
		AuthRequire: f.AuthRequire,
		EndDate:     f.EndDate,
		WorkspaceId: f.WorkspaceId,
		Fields:      f.Fields,
		Active:      f.Active,
		Url:         types.JsonURL{f.URL},
	}

	if f.TargetProjectId.Valid {
		ff.TargetProjectId = &f.TargetProjectId.String
	}

	return ff
}

// ToDTO преобразует Form в dto.Form для удобной передачи данных в интерфейс.
func (f *Form) ToDTO() *dto.Form {
	if f == nil {
		return nil
	}

	return &dto.Form{
		FormLight:     *f.ToLightDTO(),
		Author:        f.Author.ToLightDTO(),
		TargetProject: f.TargetProject.ToLightDTO(),
		Workspace:     f.Workspace.ToLightDTO(),
	}
}

// :
func (Form) TableName() string { return "forms" }

// AfterFind -  Выполняет дополнительные действия после поиска формы в базе данных.  Проверяет активен ли объект на основе даты окончания,  получает информацию о текущем workspace пользователя и устанавливает URL для отображения формы.
//
// Параметры:
//   - tx: объект базы данных GORM для выполнения запросов.
//
// Возвращает:
//   - error:  Возвращает ошибку, если произошла ошибка при выполнении каких-либо операций.
func (form *Form) AfterFind(tx *gorm.DB) error {
	if form.EndDate == nil {
		form.Active = true
	} else {
		if !form.EndDate.Time.After(time.Now().Truncate(24 * time.Hour).UTC().Add(-time.Millisecond)) {
			form.Active = false
		} else {
			form.Active = true
		}
	}

	var raw string
	if userId, ok := tx.Get("userId"); ok {
		if err := tx.Where("member_id = ?", userId).
			Where("workspace_id = ?", form.WorkspaceId).
			First(&form.CurrentWorkspaceMember).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				form.CurrentWorkspaceMember = nil
			} else {
				return err
			}
		}
	}

	if form.CurrentWorkspaceMember != nil && form.CurrentWorkspaceMember.Role == types.AdminRole {
		raw = fmt.Sprintf("/%s/forms/%s/", form.WorkspaceId, form.Slug)
		u, _ := url.Parse(raw)
		form.URL = Config.WebURL.ResolveReference(u)
	} else {
		form.SetUrl()
	}

	return nil
}

func (form *Form) SetUrl() {
	raw := fmt.Sprintf("/f/%s/", form.Slug)
	u, _ := url.Parse(raw)
	form.URL = Config.WebURL.ResolveReference(u)
}

// BeforeSave - Преобразует значения полей формы, чтобы предотвратить XSS-атаки и корректно отображать данные.  Применяет санитацию для полей типа textarea и input.
//
// Парамметры:
//   - tx: объект базы данных GORM для выполнения запросов.
//
// Возвращает:
//   - error: Возвращает ошибку, если произошла ошибка при преобразовании данных.
func (form *Form) BeforeSave(tx *gorm.DB) error {
	form.Title = policy.StripTagsPolicy.Sanitize(form.Title)
	for i, fields := range form.Fields {
		form.Fields[i].Label = policy.StripTagsPolicy.Sanitize(fields.Label)
	}
	return nil
}

// BeforeDelete Удаляет связанные записи (активности, ответы и вложения) перед удалением формы. Это необходимо для обеспечения целостности данных и предотвращения ошибок при удалении формы.
//
// Параметры:
//   - tx: объект базы данных GORM для выполнения запросов.
//
// Возвращает:
//   - error: Возвращает ошибку, если при выполнении каких-либо операций произошла ошибка.
func (form *Form) BeforeDelete(tx *gorm.DB) error {

	if err := tx.
		Where("form_activity_id in (?)", tx.Select("id").Where("form_id = ?", form.ID).
			Model(&FormActivity{})).
		Unscoped().Delete(&UserNotifications{}).Error; err != nil {
		return err
	}

	tx.Where("new_identifier = ? AND verb = ? AND field = ?", form.ID, "created", form.GetEntityType()).
		Model(&WorkspaceActivity{}).
		Updates(map[string]interface{}{"new_identifier": nil, "new_value": form.Title})

	tx.Where("new_identifier = ? ", form.ID).
		Model(&FormActivity{}).
		Update("new_identifier", nil)

	tx.Where("old_identifier = ?", form.ID).
		Model(&FormActivity{}).
		Update("old_identifier", nil)

	tx.Where("form_id = ? ", form.ID).Delete(&FormActivity{})

	//delete activity
	if err := tx.Unscoped().Where("form_id = ?", form.ID).Delete(&EntityActivity{}).Error; err != nil {
		return err
	}

	//delete answers
	if err := tx.Unscoped().Where("form_id = ?", form.ID).Delete(&FormAnswer{}).Error; err != nil {
		return err
	}
	// Remove attachments
	var attachments []FormAttachment
	if err := tx.Where("form_id = ?", form.ID).Find(&attachments).Error; err != nil {
		return err
	}
	for _, attachment := range attachments {
		if err := tx.Delete(&attachment).Error; err != nil {
			return err
		}
	}

	return nil
}

// FormExtendFields
// -migration
type FormExtendFields struct {
	NewForm *Form `json:"-" gorm:"-" field:"form" extensions:"x-nullable"`
	OldForm *Form `json:"-" gorm:"-" field:"form" extensions:"x-nullable"`
}

type FormAnswer struct {
	ID        uuid.UUID `gorm:"column:id;primaryKey;type:text" json:"id"`
	SeqId     int       `json:"seq_id" gorm:"uniqueIndex:idx_form_seq,priority:2"`
	CreatedAt time.Time `json:"created_at"`

	CreatedById uuid.NullUUID `json:"created_by_id" gorm:"index;type:text"`
	Responder   *User         `json:"responder" gorm:"foreignKey:CreatedById" extensions:"x-nullable"`

	WorkspaceId string     `json:"workspace" gorm:"index"`
	Workspace   *Workspace `json:"workspace_detail" gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`

	FormId   uuid.UUID `json:"form_id" gorm:"uniqueIndex:idx_form_seq,priority:1;type:text"`
	Form     *Form     `json:"form" gorm:"foreignKey:FormId" extensions:"x-nullable"`
	FormDate time.Time `json:"form_date"`

	Fields types.FormFieldsSlice `json:"fields" gorm:"type:jsonb"`

	AttachmentId *string         `json:"attachment_id" extensions:"x-nullable"`
	Attachment   *FormAttachment `json:"attachment_detail" gorm:"foreignKey:AttachmentId" extensions:"x-nullable"`
}

// TableName возвращает имя таблицы для сущности Form. Используется GORM для определения имени таблицы в базе данных.
//
// Возвращает:
//   - string: имя таблицы для сущности Form.
func (FormAnswer) TableName() string { return "form_answers" }

func (f FormAnswer) GetId() string {
	return f.ID.String()
}

func (f FormAnswer) GetString() string {
	return fmt.Sprintf("answer #%d", f.SeqId)
}

func (f FormAnswer) GetEntityType() string {
	return "form_answers"
}

func (f *FormAnswer) GetWorkspaceId() string {
	return f.WorkspaceId
}

func (f *FormAnswer) GetFormId() string {
	return f.FormId.String()
}

// ToDTO преобразует FormAnswer в dto.FormAnswer для удобной передачи данных в интерфейс.
//
// Парамметры:
//   - None
//
// Возвращает:
//   - *dto.FormAnswer: новая структура dto.FormAnswer, содержащая данные из FormAnswer.
func (fa *FormAnswer) ToDTO() *dto.FormAnswer {
	if fa == nil {
		return nil
	}
	return &dto.FormAnswer{
		ID:         fa.ID.String(),
		SeqId:      fa.SeqId,
		CreatedAt:  fa.CreatedAt,
		Responder:  fa.Responder.ToLightDTO(),
		Form:       fa.Form.ToDTO(),
		Fields:     fa.Fields,
		Attachment: fa.Attachment.ToDTO(),
	}
}

// BeforeSave Преобразует значения полей формы для предотвращения XSS-атак и корректного отображения данных. Применяет санитацию для полей типа textarea и input.
//
// Параметры:
//   - tx: объект базы данных GORM для выполнения запросов.
//
// Возвращает:
//   - error: Возвращает ошибку, если при преобразовании данных произошла ошибка.
func (answer *FormAnswer) BeforeSave(tx *gorm.DB) error {
	for i, fields := range answer.Fields {
		switch fields.Type {
		case "textarea", "input":
			if answer.Fields[i].Val != nil {
				answer.Fields[i].Val = policy.UgcPolicy.Sanitize(fields.Val.(string))
			}
		}
	}
	return nil
}

// AfterFind Выполняет дополнительные действия после поиска формы в базе данных. Проверяет активность формы на основе даты окончания, получает информацию о текущем workspace пользователя и устанавливает URL для отображения формы.
//
// Параметры:
//   - tx: объект базы данных GORM для выполнения запросов.
//
// Возвращает:
//   - error: Возвращает ошибку, если при выполнении каких-либо операций произошла ошибка.
func (answer *FormAnswer) AfterFind(tx *gorm.DB) error {
	for i, field := range answer.Fields {
		if field.Type == "input" || field.Type == "textarea" {
			if answer.Fields[i].Val != nil {
				answer.Fields[i].Val = html.UnescapeString(field.Val.(string))
			}
		}
	}
	return nil
}

type FormEntityI interface {
	WorkspaceEntityI
	GetFormId() string
}

type FormActivity struct {
	Id        string    `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time `json:"created_at" gorm:"index:form_activities_form_index,sort:desc,type:btree,priority:2;index:form_activities_actor_index,sort:desc,type:btree,priority:2;index:form_activities_mail_index,type:btree,where:notified = false"`
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
	// form_id uuid IS_NULL:YES
	FormId string `json:"form_id,omitempty" gorm:"index:form_activities_form_index,priority:1" extensions:"x-nullable"`
	// workspace_id uuid IS_NULL:NO
	WorkspaceId string `json:"workspace"`
	// actor_id uuid IS_NULL:YES
	ActorId *string `json:"actor,omitempty" gorm:"index:form_activities_actor_index,priority:1" extensions:"x-nullable"`

	// new_identifier uuid IS_NULL:YES
	NewIdentifier *string `json:"new_identifier" extensions:"x-nullable"`
	// old_identifier uuid IS_NULL:YES
	OldIdentifier *string       `json:"old_identifier" extensions:"x-nullable"`
	Notified      bool          `json:"-" gorm:"default:false"`
	TelegramMsgId pq.Int64Array `json:"-" gorm:"column:telegram_msg_ids;index;type:integer[]"`

	Workspace *Workspace `json:"workspace_detail" gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	Actor     *User      `json:"actor_detail" gorm:"foreignKey:ActorId" extensions:"x-nullable"`
	Form      *Form      `json:"form_detail" gorm:"foreignKey:FormId" extensions:"x-nullable"`

	//AffectedUser      *User  `json:"affected_user,omitempty" gorm:"-" extensions:"x-nullable"`
	UnionCustomFields string `json:"-" gorm:"-"`
	FormActivityExtendFields
	ActivitySender
}

func (FormActivity) TableName() string { return "form_activities" }

func (fa FormActivity) GetCustomFields() string {
	return fa.UnionCustomFields
}

func (FormActivity) GetEntity() string {
	return "form"
}

func (FormActivity) GetFields() []string {
	return []string{"id", "created_at", "verb", "field", "old_value", "new_value", "form_id", "workspace_id", "actor_id", "new_identifier", "old_identifier", "telegram_msg_ids"}
}

func (fa FormActivity) SkipPreload() bool {
	if fa.Field == nil {
		return true
	}

	if fa.NewIdentifier == nil && fa.OldIdentifier == nil {
		return true
	}
	return false
}

func (fa FormActivity) GetField() string {
	return pointerToStr(fa.Field)
}

func (fa FormActivity) GetVerb() string {
	return fa.Verb
}

func (fa FormActivity) GetNewIdentifier() string {
	return pointerToStr(fa.NewIdentifier)
}

func (fa FormActivity) GetOldIdentifier() string {
	return pointerToStr(fa.OldIdentifier)

}

func (fa FormActivity) GetId() string {
	return fa.Id
}

// FormActivityExtendFields
// -migration
type FormActivityExtendFields struct {
	//DocCommentExtendFields
	//DocExtendFields
	//DocAttachmentExtendFields
}

//func (fa FormActivity) SetAffectedUser(user *User) {
//	fa.AffectedUser = user
//}

type FormAttachment struct {
	Id string `json:"id" gorm:"primaryKey"`
	// created_at timestamp with time zone IS_NULL:NO
	CreatedAt time.Time `json:"created_at"`
	// updated_at timestamp with time zone IS_NULL:NO
	UpdatedAt time.Time `json:"updated_at"`
	// asset character varying IS_NULL:NO
	AssetId uuid.UUID `json:"asset" gorm:"type:uuid"`
	// created_by_id uuid IS_NULL:YES
	CreatedById *string `json:"created_by_id,omitempty" extensions:"x-nullable"`
	// form_id uuid IS_NULL:NO
	FormId string `json:"form" gorm:"index"`
	// updated_by_id uuid IS_NULL:YES
	UpdatedById *string `json:"updated_by_id,omitempty" extensions:"x-nullable"`
	// workspace_id uuid IS_NULL:NO
	WorkspaceId string `json:"workspace"`

	Workspace *Workspace `json:"-" gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	Asset     *FileAsset `json:"file_details" gorm:"foreignKey:AssetId" extensions:"x-nullable"`
}

// TableName возвращает имя таблицы для сущности Form. Используется GORM для определения имени таблицы в базе данных.
//
// Параметры:
//   - Нет
//
// Возвращает:
//   - string: имя таблицы для сущности Form.
func (FormAttachment) TableName() string { return "form_attachments" }

// GetId возвращает идентификатор аттачмента.
//
// Парамметры:
//   - Нет
//
// Возвращает:
//   - string: идентификатор аттачмента.
func (fa FormAttachment) GetId() string {
	return fa.Id
}

// GetString возвращает имя файла, если связанный объект asset существует, иначе возвращает тип объекта.
func (fa FormAttachment) GetString() string {
	if fa.Asset != nil {
		return fa.Asset.Name
	}
	return fa.GetEntityType()
}

// GetEntityType возвращает строку, представляющую тип объекта, связанного с аттачментом. Если объект asset существует, возвращается имя файла, иначе - тип объекта.
//
// Параметры:
//   - Нет
//
// Возвращает:
//   - string: имя файла или тип объекта.
func (fa FormAttachment) GetEntityType() string {
	return "attachment"
}

func (f *FormAttachment) GetWorkspaceId() string {
	return f.WorkspaceId
}

func (f *FormAttachment) GetFormId() string {
	return f.FormId
}

// ToDTO преобразует FormAttachment в dto.Attachment для удобной передачи данных в интерфейс.
//
// Парамметры:
//   - None
//
// Возвращает:
//   - *dto.Attachment: новая структура dto.Attachment, с содержащая данные из FormAttachment.
func (fa *FormAttachment) ToDTO() *dto.Attachment {
	if fa == nil {
		return nil
	}
	return &dto.Attachment{
		Id:        fa.Id,
		CreatedAt: fa.CreatedAt,
		Asset:     fa.Asset.ToDTO(),
	}
}

//func (attachment *FormAttachment) BeforeDelete(tx *gorm.DB) error {
//	tx.Where("new_identifier = ? AND verb = ? AND field = ?", attachment.Id, "created", "attachment").Model(&DocActivity{}).Update("new_identifier", nil)
//	return nil
//}

// AfterDelete Удаляет связанные с формой аттачменты перед удалением самой формы.  Это необходимо для обеспечения целостности данных и предотвращения ошибок при удалении формы.
//
// Парамметры:
//   - tx: объект базы данных GORM для выполнения запросов.
//
// Возвращает:
//   - error: Возвращает ошибку, если при выполнении каких-либо операций произошла ошибка.
func (attachment *FormAttachment) AfterDelete(tx *gorm.DB) error {
	if attachment.Asset == nil {
		if err := tx.Where("id = ?", &attachment.AssetId).First(&attachment.Asset).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return nil
			}
			return err
		}
	}

	// Check if this asset used in another attachment
	if attachment.Asset != nil {
		del, err := attachment.Asset.CanBeDeleted(tx)
		if err != nil {
			return err
		}

		if del {
			if err := tx.Delete(attachment.Asset).Error; err != nil {
				return err
			}
		}
	}
	return nil
}
