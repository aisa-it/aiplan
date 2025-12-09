// Пакет dao содержит методы для взаимодействия с базой данных, отвечающей за хранение документов.
// Он предоставляет функции для создания, чтения, обновления и удаления документов, а также для выполнения операций с связанными данными, такими как редакторы, читатели и отслеживатели.
//
// Основные возможности:
//   - Создание новых документов.
//   - Получение документов по ID и другим параметрам.
//   - Обновление существующих документов.
//   - Удаление документов.
//   - Работа с связанными данными (редакторы, читатели, отслеживатели).
package dao

import (
	"fmt"
	"net/url"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dto"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

type Doc struct {
	ID uuid.UUID `gorm:"column:id;primaryKey;type:uuid" json:"id"`

	CreatedAt   time.Time `json:"created_at"`
	CreatedById string    `json:"created_by" gorm:"index"`
	Author      *User     `json:"author_detail" gorm:"foreignKey:CreatedById" extensions:"x-nullable"`

	UpdatedAt   time.Time `json:"updated_at"`
	UpdatedById *string   `json:"updated_by" extensions:"x-nullable"`
	Updater     *User     `json:"update_author,omitempty"  gorm:"-" extensions:"x-nullable"`

	Tokens types.TsVector `json:"-" gorm:"index:doc_tokens_gin,type:gin;->:false"`

	Title       string             `json:"title" validate:"required,max=150"`
	Content     types.RedactorHTML `json:"description"`
	EditorRole  int                `json:"editor_role" gorm:"default:10"`
	ReaderRole  int                `json:"reader_role" gorm:"default:5"`
	WorkspaceId string             `json:"workspace" gorm:"index"`
	Workspace   *Workspace         `json:"workspace_detail" gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	ParentDocID uuid.NullUUID      `json:"parent_doc_id"`
	SeqId       int                `json:"seq_id"`
	Draft       bool               `json:"draft"`
	ParentDoc   *Doc               `json:"parent_doc" gorm:"foreignKey:ParentDocID" extensions:"x-nullable"`

	InlineAttachments []FileAsset `json:"doc_inline_attachments" gorm:"foreignKey:DocId"`

	ChildDocs   []string `json:"-" gorm:"-"`
	Breadcrumbs []string `json:"-" gorm:"-"`

	Editors  *[]User `json:"editor_details,omitempty" gorm:"-"`
	Readers  *[]User `json:"reader_details,omitempty" gorm:"-"`
	Watchers *[]User `json:"watcher_details,omitempty" gorm:"-"`

	EditorsIDs []string `json:"editors" gorm:"-"`
	ReaderIDs  []string `json:"readers" gorm:"-"`
	WatcherIDs []string `json:"watchers" gorm:"-"`

	AccessRules []DocAccessRules `json:"-" gorm:"foreignKey:DocId"`

	URL      *url.URL `json:"-" gorm:"-"`
	ShortURL *url.URL `json:"-" gorm:"-"`

	IsFavorite           bool `json:"is_favorite" gorm:"-"`
	CurrentWorkspaceRole int  `json:"-" gorm:"-"`
}

func (d *Doc) PopulateAccessFields() {
	d.EditorsIDs = []string{}
	d.ReaderIDs = []string{}
	d.WatcherIDs = []string{}
	editorUsers := make([]User, 0)
	readerUsers := make([]User, 0)
	watcherUsers := make([]User, 0)

	d.Editors = &editorUsers
	d.Readers = &readerUsers
	d.Watchers = &watcherUsers

	if len(d.AccessRules) == 0 {
		return
	}

	editors := make([]string, 0)
	readers := make([]string, 0)
	watchers := make([]string, 0)

	for _, access := range d.AccessRules {
		if access.Edit {
			editors = append(editors, access.MemberId.String())
			if access.Member != nil {
				editorUsers = append(editorUsers, *access.Member)
			}
		} else {
			readers = append(readers, access.MemberId.String())
			if access.Member != nil {
				readerUsers = append(readerUsers, *access.Member)
			}
		}

		if access.Watch {
			watchers = append(watchers, access.MemberId.String())
			if access.Member != nil {
				watcherUsers = append(watcherUsers, *access.Member)
			}
		}
	}

	d.EditorsIDs = editors
	d.ReaderIDs = readers
	d.WatcherIDs = watchers

	if len(editorUsers) > 0 {
		d.Editors = &editorUsers
	}
	if len(readerUsers) > 0 {
		d.Readers = &readerUsers
	}
	if len(watcherUsers) > 0 {
		d.Watchers = &watcherUsers
	}
}

// Возвращает имя таблицы, соответствующей сущности Doc. Используется для определения имени таблицы при работе с базой данных.
func (Doc) TableName() string { return "docs" }

// DocExtendFields
// -migration
type DocExtendFields struct {
	NewDoc *Doc `json:"-" gorm:"-" field:"doc" extensions:"x-nullable"`
	OldDoc *Doc `json:"-" gorm:"-" field:"doc" extensions:"x-nullable"`
}

// Возвращает идентификатор документа в виде строки.
func (d Doc) GetId() string {
	return d.ID.String()
}

// Возвращает заголовок документа.
func (d Doc) GetString() string {
	return d.Title
}

// Возвращает тип сущности документа (doc). Используется для определения типа данных при работе с базой данных.
func (d Doc) GetEntityType() string {
	return actField.Doc.Field.String()
}

func (d Doc) GetWorkspaceId() string {
	return d.WorkspaceId
}

func (d Doc) GetDocId() string {
	return d.GetId()
}

// Функция AfterFind выполняется после успешного поиска записи в базе данных.  Она выполняет дополнительные операции, такие как обновление информации о URL,  получение итоговых данных о реакции на комментарии, и другие необходимые действия после извлечения данных из базы.
func (d *Doc) AfterFind(tx *gorm.DB) error {
	d.SetUrl()

	if userId, ok := tx.Get("member_id"); ok {
		if err := tx.Model(&DocFavorites{}).
			Select("EXISTS(?)",
				tx.Model(&DocFavorites{}).
					Select("1").
					Where("user_id = ?", userId).
					Where("doc_id = ?", d.ID),
			).
			Find(&d.IsFavorite).Error; err != nil {
			return err
		}
	}

	d.PopulateAccessFields()

	if d.UpdatedById != nil {
		if err := tx.Where("id = ?", d.UpdatedById).First(&d.Updater).Error; err != nil {
			return err
		}
	}

	memberRole, ok := tx.Get("member_role")
	if ok {
		d.CurrentWorkspaceRole = memberRole.(int)
	} else {
		d.CurrentWorkspaceRole = 0

	}

	memberId, ok := tx.Get("member_id")
	if ok {
		if err := tx.Model(&Doc{}).
			Joins("LEFT JOIN doc_access_rules dar ON dar.doc_id = docs.id").
			Select("docs.id").
			Where("docs.parent_doc_id = ?", d.ID).
			Where("docs.reader_role <= ? OR docs.editor_role <= ? OR dar.member_id = ? OR docs.created_by_id = ?",
				d.CurrentWorkspaceRole, d.CurrentWorkspaceRole, memberId, memberId).
			Group("docs.id").
			Scan(&d.ChildDocs).Error; err != nil {
			return err
		}
	}

	if _, ok := tx.Get("breadcrumbs"); ok {
		if d.ParentDocID.Valid {
			var breadcrumbs []string

			err := tx.Raw(`
    WITH RECURSIVE breadcrumbs AS (
        SELECT
            id, title, parent_doc_id,
            ARRAY[id] AS path
        FROM docs
        WHERE id = ?

        UNION ALL

        SELECT
            d.id, d.title, d.parent_doc_id,
            b.path || d.id
        FROM docs d
        INNER JOIN breadcrumbs b ON d.id = b.parent_doc_id
        WHERE NOT d.id = ANY(b.path)
    )
    SELECT id FROM breadcrumbs;
`, d.ID).Scan(&breadcrumbs).Error

			if err != nil {
				return err
			}

			d.Breadcrumbs = breadcrumbs
		}
		return nil
	}
	return nil
}

func (d *Doc) SetUrl() {
	raw := fmt.Sprintf("/%s/aidoc/%s", d.WorkspaceId, d.ID)
	u, _ := url.Parse(raw)
	d.URL = Config.WebURL.ResolveReference(u)

	if d.Workspace != nil {
		ref, _ := url.Parse(fmt.Sprintf("/d/%s/%s",
			d.Workspace.Slug,
			d.ID))
		d.ShortURL = Config.WebURL.ResolveReference(ref)
	}
}

// Удаляет активность, связанную с документом перед его удалением из базы данных.  Параметр tx - это объект базы данных GORM, используемый для выполнения операций с базой данных. Функция возвращает ошибку, если при выполнении каких-либо операций с базой данных возникает ошибка.
func (d *Doc) BeforeDelete(tx *gorm.DB) error {
	var childDocs []Doc
	if err := tx.Where("workspace_id = ?", d.WorkspaceId).Where("parent_doc_id = ?", d.ID).Find(&childDocs).Error; err != nil {
		return err
	}
	for _, doc := range childDocs {
		if err := tx.Delete(&doc).Error; err != nil {
			return err
		}
	}

	cleanId := map[string]interface{}{"new_identifier": nil, "old_identifier": nil}

	tx.
		Where("(new_identifier = ? OR old_identifier = ?) AND (verb = ? OR verb = ? OR verb = ?) AND field = ?", d.ID, d.ID, "created", "removed", "added", d.GetEntityType()).
		Model(&DocActivity{}).
		Updates(cleanId)

	tx.
		Where("(new_identifier = ? OR old_identifier = ?) AND (verb = ? OR verb = ? OR verb = ?) AND field = ?", d.ID, d.ID, "created", "removed", "added", d.GetEntityType()).
		Model(&WorkspaceActivity{}).
		Updates(cleanId)

	tx.Where("new_identifier = ? AND (verb = ? OR verb = ?) AND field = ?", d.ID, d.ID, "move_workspace_to_doc", "move_doc_to_doc", d.GetEntityType()).Update("new_identifier", nil)
	tx.Where("old_identifier = ? AND (verb = ? OR verb = ?) AND field = ?", d.ID, d.ID, "move_doc_to_workspace", "move_doc_to_doc", d.GetEntityType()).Update("old_identifier", nil)

	if err := tx.Where("workspace_id = ?", d.WorkspaceId).
		Where("doc_activity_id IN (?)",
			tx.Select("id").
				Where("doc_id = ?", d.ID).
				Model(&DocActivity{})).
		Unscoped().
		Delete(&UserNotifications{}).Error; err != nil {
		return err
	}

	tx.Where("doc_id = ?", d.ID).Unscoped().
		Delete(&DocActivity{})

	if err := tx.Unscoped().Where("doc_id = ?", d.ID).Delete(&DocFavorites{}).Error; err != nil {
		return err
	}

	if err := tx.Where("doc_id = ?", d.ID).Delete(&DocAccessRules{}).Error; err != nil {
		return err
	}

	// Delete comments, reaction
	var comments []DocComment
	if err := tx.Where("doc_id = ?", d.ID).Preload("Attachments").Find(&comments).Error; err != nil {
		return err
	}

	var commentId []string

	for _, comment := range comments {
		commentId = append(commentId, comment.Id.String())
	}

	if err := tx.Where("comment_id in ?", commentId).Delete(&DocCommentReaction{}).Error; err != nil {
		return err
	}

	if err := tx.Model(&DocComment{}).
		Where("doc_id = ?", d.ID).
		Update("reply_to_comment_id", nil).Error; err != nil {
		return err
	}

	if err := tx.Where("doc_id = ?", d.ID).Delete(comments).Error; err != nil {
		return err
	}

	// Remove attachments
	var attachments []DocAttachment
	if err := tx.Where("doc_id = ?", d.ID).Find(&attachments).Error; err != nil {
		return err
	}
	for _, attachment := range attachments {
		if err := tx.Delete(&attachment).Error; err != nil {
			return err
		}
	}

	// Remove inline attachments
	if len(d.InlineAttachments) == 0 {
		if err := tx.Where("doc_id = ?", d.ID).Find(&d.InlineAttachments).Error; err != nil {
			return err
		}
	}
	for _, attach := range d.InlineAttachments {
		if err := tx.Delete(&attach).Error; err != nil {
			return err
		}
	}
	return nil
}

// Преобразует структуру Doc в структуру dto.Doc для удобства использования в API.
func (d *Doc) ToDTO() *dto.Doc {
	if d == nil {
		return nil
	}

	var parentId *string
	if d.ParentDocID.Valid {
		id := d.ParentDocID.UUID.String()
		parentId = &id
	}

	docDTO := dto.Doc{
		DocLight:          *d.ToLightDTO(),
		CreatedAt:         d.CreatedAt,
		UpdateAt:          d.UpdatedAt,
		Content:           d.Content,
		ParentDoc:         parentId,
		InlineAttachments: utils.SliceToSlice(&d.InlineAttachments, func(f *FileAsset) dto.FileAsset { return *f.ToDTO() }),
		Breadcrumbs:       d.Breadcrumbs,
		Author:            d.Author.ToLightDTO(),
		UpdateBy:          d.Updater.ToLightDTO(),
		ReaderIds:         d.ReaderIDs,
		ReaderRole:        d.ReaderRole,
		EditorRole:        d.EditorRole,
		Readers:           utils.SliceToSlice(d.Readers, func(u *User) dto.UserLight { return *u.ToLightDTO() }),
		EditorIds:         d.EditorsIDs,
		Editors:           utils.SliceToSlice(d.Editors, func(u *User) dto.UserLight { return *u.ToLightDTO() }),
		WatcherIds:        d.WatcherIDs,
		Watchers:          utils.SliceToSlice(d.Watchers, func(u *User) dto.UserLight { return *u.ToLightDTO() }),
	}

	docDTO.Url = types.JsonURL{d.URL}
	docDTO.ShortUrl = types.JsonURL{d.ShortURL}

	return &docDTO
}

// Преобразует Doc в структуру DtoDocLight для удобства использования в API.  Возвращает копию Doc, содержащую только необходимые поля для представления в API.
func (d *Doc) ToLightDTO() *dto.DocLight {
	if d == nil {
		return nil
	}
	d.SetUrl()
	return &dto.DocLight{
		Id:           d.ID.String(),
		Title:        d.Title,
		HasChildDocs: len(d.ChildDocs) > 0,
		Draft:        &d.Draft,
		IsFavorite:   d.IsFavorite,
		Url:          types.JsonURL{d.URL},
		ShortUrl:     types.JsonURL{d.ShortURL},
	}
}

type DocComment struct {
	Id        uuid.UUID `json:"id" gorm:"column:id;primaryKey;type:uuid"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	// comment_stripped text IS_NULL:NO
	CommentStripped string `json:"comment_stripped"`
	// created_by_id uuid IS_NULL:YES
	CreatedById *string `json:"created_by_id,omitempty" extensions:"x-nullable"`
	// updated_by_id uuid IS_NULL:YES
	UpdatedById *string `json:"updated_by_id,omitempty" extensions:"x-nullable"`
	// workspace_id uuid IS_NULL:NO
	WorkspaceId string `json:"workspace_id"`
	// doc_id uuid IS_NULL:NO
	DocId string `json:"doc_id"`
	// actor_id uuid IS_NULL:YES
	ActorId *string `json:"actor_id,omitempty" gorm:"index;index:integration_doc,priority:1" extensions:"x-nullable"`
	// comment_html text IS_NULL:NO
	CommentHtml types.RedactorHTML `json:"comment_html"`
	// comment_json jsonb IS_NULL:NO
	CommentType      int           `json:"comment_type" gorm:"default:1"`
	IntegrationMeta  string        `json:"-" gorm:"index:integration_doc,priority:2"`
	ReplyToCommentId uuid.NullUUID `json:"reply_to_comment_id"`
	OriginalComment  *DocComment   `json:"original_comment,omitempty" gorm:"foreignKey:ReplyToCommentId" extensions:"x-nullable"`

	Workspace *Workspace `json:"-" gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	Doc       *Doc       `json:"-" gorm:"foreignKey:DocId" extensions:"x-nullable"`
	Actor     *User      `json:"actor_detail" gorm:"foreignKey:ActorId" extensions:"x-nullable"`

	Attachments []FileAsset `json:"comment_attachments" gorm:"foreignKey:DocCommentId"`

	Reactions       []DocCommentReaction `json:"reactions" gorm:"foreignKey:CommentId"`
	ReactionSummary map[string]int       `json:"reaction_summary,omitempty" gorm:"-"`
	URL             *url.URL             `json:"-" gorm:"-"`
}

// Возвращает имя таблицы, соответствующей сущности Doc. Используется для определения имени таблицы при работе с базой данных.
func (DocComment) TableName() string { return "doc_comments" }

// DocCommentExtendFields
// -migration
type DocCommentExtendFields struct {
	NewDocComment *DocComment `json:"-" gorm:"-" field:"comment::doc" extensions:"x-nullable"`
}

// Возвращает идентификатор документа в виде строки.
func (dc DocComment) GetId() string {
	return dc.Id.String()
}

// Возвращает заголовок документа.
func (dc DocComment) GetString() string {
	return dc.CommentHtml.String()
}

// Возвращает тип сущности Doc (doc). Используется для представления сущности Doc в API.
func (dс DocComment) GetEntityType() string {
	return actField.Comment.Field.String()
}

func (dc DocComment) GetWorkspaceId() string {
	return dc.WorkspaceId
}

func (dc DocComment) GetDocId() string {
	return dc.DocId
}

// Выполняет дополнительные операции после успешного поиска записи в базе данных. В частности, обновляет информацию об URL, получает итоги реакции на комментарии и другие необходимые действия после извлечения данных из базы.
func (dc *DocComment) AfterFind(tx *gorm.DB) error {
	raw := fmt.Sprintf("/api/auth/workspaces/%s/doc/%s/comments/%s/", dc.WorkspaceId, dc.DocId, dc.Id)
	u, _ := url.Parse(raw)
	dc.URL = Config.WebURL.ResolveReference(u)

	reactionCounts := make(map[string]int)
	for _, reaction := range dc.Reactions {
		reactionCounts[reaction.Reaction]++
	}
	dc.ReactionSummary = reactionCounts
	return nil
}

// Удаляет активность, связанную с документом перед его удалением из базы данных.
//
// Параметры:
//   - tx: объект базы данных GORM, используемый для выполнения операций с базой данных.
//
// Возвращает:
//   - error: ошибка, если при выполнении каких-либо операций с базой данных возникает ошибка.
func (dc *DocComment) BeforeDelete(tx *gorm.DB) error {
	if err := tx.Where("workspace_id = ?", dc.WorkspaceId).
		Where("doc_activity_id IN (?)",
			tx.Select("id").
				Where("doc_id = ?", dc.DocId).
				Where("new_identifier = ? or old_identifier = ? ", dc.Id, dc.Id).
				Model(&DocActivity{})).
		Unscoped().
		Delete(&UserNotifications{}).Error; err != nil {
		return err
	}

	// DocActivity update create to nil
	tx.Where("new_identifier = ? AND verb = ? AND field = ?", dc.Id, "created", "comment").Model(&DocActivity{}).Update("new_identifier", nil)

	//DocActivity delete other activity
	tx.Where("new_identifier = ? or old_identifier = ? ", dc.Id, dc.Id).Delete(&DocActivity{})

	for _, attach := range dc.Attachments {
		if err := tx.Delete(&attach).Error; err != nil {
			return err
		}
	}

	if err := tx.Where("comment_id = ?", dc.Id).Delete(&DocCommentReaction{}).Error; err != nil {
		return err
	}

	if err := tx.Model(&DocComment{}).Where("reply_to_comment_id = ?", dc.Id).Update("reply_to_comment_id", nil).Error; err != nil {
		return err
	}
	return nil
}
func (dc *DocComment) ToLightDTO() *dto.DocCommentLight {
	if dc == nil {
		return nil
	}

	return &dto.DocCommentLight{
		Id:              dc.Id.String(),
		CommentStripped: dc.CommentStripped,
		CommentHtml:     dc.CommentHtml,
		URL:             types.JsonURL{dc.URL},
	}
}

// Преобразует структуру Doc в структуру dto.DocComment для удобства использования в API.  Принимает структуру Doc в качестве аргумента и возвращает указатель на структуру dto.DocComment, содержащую преобразованные данные.
func (dc *DocComment) ToDTO() *dto.DocComment {
	if dc == nil {
		return nil
	}

	comment := dto.DocComment{
		DocCommentLight: *dc.ToLightDTO(),
		CreatedAt:       dc.CreatedAt,
		UpdatedAt:       dc.UpdatedAt,

		UpdatedById: dc.UpdatedById,
		Actor:       dc.Actor.ToLightDTO(),

		CommentType:     dc.CommentType,
		Attachments:     utils.SliceToSlice(&dc.Attachments, func(fa *FileAsset) dto.FileAsset { return *fa.ToDTO() }),
		Reactions:       utils.SliceToSlice(&dc.Reactions, func(cr *DocCommentReaction) *dto.CommentReaction { return cr.ToDTO() }),
		ReactionSummary: dc.ReactionSummary,
	}

	if dc.ReplyToCommentId.Valid {
		if dc.OriginalComment != nil {
			dc.OriginalComment.ReplyToCommentId = uuid.NullUUID{
				UUID:  uuid.UUID{},
				Valid: false,
			}
			dc.OriginalComment.OriginalComment = nil
			comment.OriginalComment = dc.OriginalComment.ToDTO()
		}
	}

	return &comment
}

type DocCommentReaction struct {
	Id        uuid.UUID `json:"id" gorm:"column:id;primaryKey;type:uuid"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	UserId    string    `json:"user_id"`
	CommentId uuid.UUID `json:"comment_id" gorm:"type:uuid"`
	Reaction  string    `json:"reaction"`

	User    *User       `json:"-" gorm:"foreignKey:UserId" extensions:"x-nullable"`
	Comment *DocComment `json:"-" gorm:"foreignKey:CommentId" extensions:"x-nullable"`
}

// Преобразует структуру Doc в структуру dto.CommeantReacation для удобства использования в API.
//
// Параметры:
//   - None
//
// Возвращает:
//   - dto.CommeantReacation: структура, содержащая преобразованные данные.
func (dcr *DocCommentReaction) ToDTO() *dto.CommentReaction {
	if dcr == nil {
		return nil
	}
	return &dto.CommentReaction{
		Id:        dcr.Id,
		CreatedAt: dcr.CreatedAt,
		UpdatedAt: dcr.UpdatedAt,
		CommentId: dcr.CommentId,
		UserId:    dcr.UserId,
		Reaction:  dcr.Reaction,
	}
}

// Возвращает имя таблицы, соответствующей сущности Doc. Используется для определения имени таблицы при работе с базой данных.
func (DocCommentReaction) TableName() string { return "doc_comment_reactions" }

type DocEntityI interface {
	WorkspaceEntityI
	GetDocId() string
}

type DocActivity struct {
	Id        string    `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time `json:"created_at" gorm:"index:doc_activities_doc_index,sort:desc,type:btree,priority:2;index:doc_activities_actor_index,sort:desc,type:btree,priority:2;index:doc_activities_mail_index,type:btree,where:notified = false"`
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
	// doc_id uuid IS_NULL:YES
	DocId string `json:"doc" gorm:"index:doc_activities_doc_index,priority:1" `
	// workspace_id uuid IS_NULL:NO
	WorkspaceId string `json:"workspace"`
	// actor_id uuid IS_NULL:YES
	ActorId *string `json:"actor,omitempty" gorm:"index:doc_activities_actor_index,priority:1" extensions:"x-nullable"`

	// new_identifier uuid IS_NULL:YES
	NewIdentifier *string `json:"new_identifier" extensions:"x-nullable"`
	// old_identifier uuid IS_NULL:YES
	OldIdentifier *string       `json:"old_identifier" extensions:"x-nullable"`
	Notified      bool          `json:"-" gorm:"default:false"`
	TelegramMsgId pq.Int64Array `json:"-" gorm:"column:telegram_msg_ids;index;type:integer[]"`

	Workspace *Workspace `json:"workspace_detail" gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	Actor     *User      `json:"actor_detail" gorm:"foreignKey:ActorId" extensions:"x-nullable"`
	Doc       *Doc       `json:"doc_detail" gorm:"foreignKey:DocId" extensions:"x-nullable"`

	OldDoc *Doc `json:"-" gorm:"-" field:"doc" extensions:"x-nullable"`
	NewDoc *Doc `json:"-" gorm:"-" field:"doc" extensions:"x-nullable"`

	//AffectedUser      *User  `json:"affected_user,omitempty" gorm:"-" extensions:"x-nullable"`
	UnionCustomFields string `json:"-" gorm:"-"`

	DocActivityExtendFields
	ActivitySender
}

func (da DocActivity) GetCustomFields() string {
	return da.UnionCustomFields
}

func (da DocActivity) GetFields() []string {
	return []string{"id", "created_at", "verb", "field", "old_value", "doc_id", "new_value", "workspace_id", "actor_id", "new_identifier", "old_identifier", "telegram_msg_ids"}

}

func (DocActivity) GetEntity() string {
	return "doc"
}

// Возвращает имя таблицы, соответствующей сущности Doc. Используется для определения имени таблицы при работе с базой данных.
func (DocActivity) TableName() string { return "doc_activities" }

// DocActivityExtendFields
// -migration
type DocActivityExtendFields struct {
	DocCommentExtendFields
	DocExtendFields
	DocAttachmentExtendFields
	DocMemberExtendFields
}

// Выполняет дополнительные операции после успешного поиска записи в базе данных. В частности, обновляет информацию об URL, получает итоги реакции на комментарии и другие необходимые действия после извлечения данных из базы.
func (da *DocActivity) AfterFind(tx *gorm.DB) error {
	return EntityActivityAfterFind(da, tx)
}

// Пропускает презагрузку связанных данных. Возвращает true, если презагрузка не нужна, false - если нужна.
func (da DocActivity) SkipPreload() bool {
	if da.Field == nil {
		return true
	}

	if da.NewIdentifier == nil && da.OldIdentifier == nil {
		return true
	}
	return false
}

// Возвращает поле, соответствующее указанному полю в структуре Doc.  Параметр - имя поля, которое необходимо вернуть. Возвращает строку, представляющую значение указанного поля.
func (da DocActivity) GetField() string {
	return pointerToStr(da.Field)
}

func (da DocActivity) GetVerb() string {
	return da.Verb
}

// Добавляет новый идентификатор к объекту Doc. Используется для уникальной идентификации документа в базе данных.
func (da DocActivity) GetNewIdentifier() string {
	return pointerToStr(da.NewIdentifier)
}

// Возвращает старый идентификатор Doc, если он существует.  Используется для отслеживания изменений идентификаторов Doc при репликации или других операциях.
func (da DocActivity) GetOldIdentifier() string {
	return pointerToStr(da.OldIdentifier)

}

func (da DocActivity) GetId() string {
	return da.Id
}

// Преобразует Doc в структуру dto.EntityActivityLight для упрощения передачи данных в API.
//
// Параметры:
//   - Нет
//
// Возвращает:
//   - dto.EntityActivityLight: структура, содержащая упрощенные данные Doc.
//func (da *DocActivity) ToLightDTO() *dto.EntityActivityLight {
//	if da == nil {
//		return nil
//	}

//func (da *DocActivity) ToLightDTO() *dto.EntityActivityLight {
//	if da == nil {
//		return nil
//	}
//
//	return &dto.EntityActivityLight{
//		Id:         da.Id,
//		CreatedAt:  da.CreatedAt,
//		Verb:       da.Verb,
//		Field:      da.Field,
//		OldValue:   da.OldValue,
//		NewValue:   da.NewValue,
//		EntityType: "doc",
//		//TargetUser: da.AffectedUser.ToLightDTO(),
//		EntityUrl: nil,
//	}
//}

func (da *DocActivity) ToLightDTO() *dto.EntityActivityLight {
	if da == nil {
		return nil
	}

	res := dto.EntityActivityLight{
		Id:         da.Id,
		CreatedAt:  da.CreatedAt,
		Verb:       da.Verb,
		Field:      da.Field,
		OldValue:   da.OldValue,
		NewValue:   da.NewValue,
		EntityType: "doc",

		NewEntity: GetActionEntity(*da, "New"),
		OldEntity: GetActionEntity(*da, "Old"),

		//TargetUser: activity.AffectedUser.ToLightDTO(),

		//EntityUrl: da.GetUrl(),
	}

	return &res
}

// Преобразует структуру Doc в структуру dto.EntityActivityFull для удобства использования в API.
//
// Параметры:
//   - Нет
//
// Возвращает:
//   - dto.EntityActivityFull: структура, содержащая преобразованные данные Doc.
//func (da *DocActivity) ToDTO() *dto.EntityActivityFull {
//	if da == nil {
//		return nil
//	}
//
//	res := dto.EntityActivityFull{
//		EntityActivityLight: *da.ToLightDTO(),
//		Actor:               da.Actor.ToLightDTO(),
//		Workspace:           da.Workspace.ToLightDTO(),
//		Doc:                 da.Doc.ToLightDTO(),
//		NewIdentifier:       nil,
//		OldIdentifier:       nil,
//	}
//
//	if da.Field != nil {
//		switch *da.Field {
//		case "doc":
//			if da.OldIdentifier != nil {
//				res.OldEntity = da.OldDoc.ToLightDTO()
//			}
//			if da.NewIdentifier != nil {
//				res.NewEntity = da.NewDoc.ToLightDTO()
//			}
//		}
//	}
//
//	return &res
//}

// Преобразует Doc в структуру dto.HistoryBodyLight для упрощенной передачи данных в API.
//
// Параметры:
//   - Нет
//
// Возвращает:
//   - dto.HistoryBodyLight: структура, содержащая упрощенные данные Doc.
func (da *DocActivity) ToHistoryLightDTO() *dto.HistoryBodyLight {
	if da == nil {
		return nil
	}

	return &dto.HistoryBodyLight{
		Id:       da.Id,
		CratedAt: da.CreatedAt,
		Author:   da.Actor.ToLightDTO(),
	}
}

type DocAttachment struct {
	// created_at timestamp with time zone IS_NULL:NO
	CreatedAt time.Time `json:"created_at"`
	// updated_at timestamp with time zone IS_NULL:NO
	UpdatedAt time.Time `json:"updated_at"`
	// id uuid IS_NULL:NO
	Id string `json:"id" gorm:"primaryKey"`
	// asset character varying IS_NULL:NO
	AssetId uuid.UUID `json:"asset" gorm:"type:uuid"`
	// created_by_id uuid IS_NULL:YES
	CreatedById *string `json:"created_by_id,omitempty" extensions:"x-nullable"`
	// doc_id uuid IS_NULL:NO
	DocId string `json:"doc" gorm:"index"`
	// updated_by_id uuid IS_NULL:YES
	UpdatedById *string `json:"updated_by_id,omitempty" extensions:"x-nullable"`
	// workspace_id uuid IS_NULL:NO
	WorkspaceId string `json:"workspace"`

	Workspace *Workspace `json:"-" gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	Asset     *FileAsset `json:"file_details" gorm:"foreignKey:AssetId" extensions:"x-nullable"`
}

// Возвращает имя таблицы, соответствующей сущности Doc. Используется для определения имени таблицы при работе с базой данных.
func (DocAttachment) TableName() string { return "doc_attachments" }

// DocAttachmentExtendFields
// -migration
type DocAttachmentExtendFields struct {
	NewDocAttachment *DocAttachment `json:"-" gorm:"-" field:"attachment::doc" extensions:"x-nullable"`
	OldDocAttachment *DocAttachment `json:"-" gorm:"-" field:"attachment::doc" extensions:"x-nullable"`
}

// GetId возвращает идентификатор документа в виде строки.
//
// Парамметры:
//   - Нет
//
// Возвращает:
//   - string: идентификатор документа в виде строки.
func (da DocAttachment) GetId() string {
	return da.Id
}

// Возвращает заголовок документа.
func (da DocAttachment) GetString() string {
	if da.Asset != nil {
		return da.Asset.Name
	}
	return da.GetEntityType()
}

// Возвращает тип сущности Doc (doc). Используется для представления сущности Doc в API.
func (da DocAttachment) GetEntityType() string {
	return actField.Attachment.Field.String()
}

func (da DocAttachment) GetWorkspaceId() string {
	return da.WorkspaceId
}

func (da DocAttachment) GetDocId() string {
	return da.DocId
}

// Преобразует структуру Doc в структуру dto.Attachment для удобства передачи данных в API.
//
// Параметры:
//   - Нет
//
// Возвращает:
//   - dto.Attachment: структура, содержащая преобразованные данные Doc.
func (da *DocAttachment) ToLightDTO() *dto.Attachment {
	if da == nil {
		return nil
	}
	return &dto.Attachment{
		Id:        uuid.Must(uuid.FromString(da.Id)),
		CreatedAt: da.CreatedAt,
		Asset:     da.Asset.ToDTO(),
	}
}

// Удаляет активность, связанную с документом перед его удалением из базы данных.
//
// Параметры:
//   - tx: объект базы данных GORM, используемый для выполнения операций с базой данных.
//
// Возвращает:
//   - error: ошибка, если при выполнении каких-либо операций с базой данных возникает ошибка.
func (attachment *DocAttachment) BeforeDelete(tx *gorm.DB) error {
	tx.Where("new_identifier = ? AND verb = ? AND field = ?", attachment.Id, "created", "attachment").Model(&DocActivity{}).Update("new_identifier", nil)
	return nil
}

func (a *DocAttachment) AfterFind(tx *gorm.DB) error {
	if err := tx.Where("id = ?", a.AssetId).First(&a.Asset).Error; err != nil {
		return err
	}
	return nil
}

// Удаляет активность, связанную с документом перед его удалением из базы данных.
//
// Параметры:
//   - tx: объект базы данных GORM, используемый для выполнения операций с базой данных.
//
// Возвращает:
//   - error: ошибка, если при выполнении каких-либо операций с базой данных возникает ошибка.
func (attachment *DocAttachment) AfterDelete(tx *gorm.DB) error {
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

type DocAccessRules struct {
	Id        uuid.UUID `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	MemberId    uuid.UUID     `json:"editor_id" gorm:"uniqueIndex:doc_member_idx,priority:2"`
	CreatedById uuid.UUID     `json:"created_by_id,omitempty" extensions:"x-nullable"`
	DocId       uuid.UUID     `json:"doc_id" gorm:"index;uniqueIndex:doc_member_idx,priority:1"`
	UpdatedById uuid.NullUUID `json:"updated_by_id,omitempty" extensions:"x-nullable"`
	WorkspaceId uuid.UUID     `json:"workspace_id"`

	Edit  bool `json:"edit"`
	Watch bool `json:"watch"`

	Workspace *Workspace `gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	Doc       *Doc       `gorm:"foreignKey:DocId" extensions:"x-nullable"`
	Member    *User      `gorm:"foreignKey:MemberId" extensions:"x-nullable"`
}

// Возвращает имя таблицы, соответствующей сущности Doc. Используется для определения имени таблицы при работе с базой данных.
func (DocAccessRules) TableName() string { return "doc_access_rules" }

// DocMemberExtendFields
// -migration
type DocMemberExtendFields struct {
	NewDocWatcher *User `json:"-" gorm:"-" field:"watchers::doc" extensions:"x-nullable"`
	OldDocWatcher *User `json:"-" gorm:"-" field:"watchers::doc" extensions:"x-nullable"`

	NewDocReader *User `json:"-" gorm:"-" field:"readers" extensions:"x-nullable"`
	OldDocReader *User `json:"-" gorm:"-" field:"readers" extensions:"x-nullable"`

	NewDocEditor *User `json:"-" gorm:"-" field:"editors" extensions:"x-nullable"`
	OldDocEditor *User `json:"-" gorm:"-" field:"editors" extensions:"x-nullable"`
}

type DocFavorites struct {
	// id uuid IS_NULL:NO
	Id string `json:"id" gorm:"primaryKey"`
	// created_at timestamp with time zone IS_NULL:NO
	CreatedAt time.Time `json:"created_at"`
	// created_by_id uuid IS_NULL:YES
	CreatedById *string `json:"created_by_id,omitempty" extensions:"x-nullable"`
	// project_id uuid IS_NULL:NO
	DocId string `json:"doc_id" gorm:"index;uniqueIndex:doc_favorites_idx,priority:1"`
	// user_id uuid IS_NULL:NO
	UserId string `json:"user_id" gorm:"uniqueIndex:doc_favorites_idx,priority:2"`
	// workspace_id uuid IS_NULL:NO
	WorkspaceId string `json:"workspace_id"`

	Workspace *Workspace `json:"workspace" gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	Doc       *Doc       `json:"Doc" gorm:"foreignKey:DocId" extensions:"x-nullable"`
}

// Возвращает имя таблицы, соответствующей сущности Doc. Используется для определения имени таблицы при работе с базой данных.
func (DocFavorites) TableName() string {
	return "doc_favorites"
}

// Преобразует структуру Doc в структуру dto.DocFavorites для удобства передачи данных в API.
//
// Параметры:
//   - Нет
//
// Возвращает:
//   - dto.DocFavorites: структура, содержащая преобразованные данные Doc.
func (df *DocFavorites) ToDTO() *dto.DocFavorites {
	if df == nil {
		return nil
	}

	return &dto.DocFavorites{
		Id:    df.Id,
		DocId: df.DocId,
		Doc:   df.Doc.ToLightDTO(),
	}
}

// GetDoc получает документ по ID и другим параметрам, используя базу данных GORM.
//
// Параметры:
//   - db: объект базы данных GORM для выполнения операций с базой данных.
//   - workspaceId: ID рабочего пространства, к которому относится документ.
//   - docId: ID документа, который необходимо получить.
//   - workspaceMember: информация о членстве пользователя в рабочем пространстве.
//
// Возвращает:
//   - Doc: документ, соответствующий указанному ID, или ошибка, если документ не найден или произошла другая ошибка при работе с базой данных.
func GetDoc(db *gorm.DB, workspaceId, docId string, workspaceMember WorkspaceMember) (Doc, error) {
	d := Doc{}
	return d, db.Set("member_id", workspaceMember.MemberId).
		Set("member_role", workspaceMember.Role).
		Where("docs.workspace_id = ?", workspaceId).
		Where("docs.reader_role <= ? OR docs.editor_role <= ? OR EXISTS (SELECT 1 FROM doc_access_rules dar WHERE dar.doc_id = docs.id AND dar.member_id = ?) OR docs.created_by_id = ?",
			workspaceMember.Role, workspaceMember.Role, workspaceMember.MemberId, workspaceMember.MemberId).
		Where("docs.id = ?", docId).
		First(&d).Error
}

// CreateDoc создает новый документ в базе данных.
//
// Параметры:
//   - db: объект базы данных GORM для выполнения операций с базой данных.
//   - doc: структура Doc, представляющая создаваемый документ.
//   - user: структура User, представляющая пользователя, создающего документ.
//
// Возвращает:
//   - error: ошибка, если при создании документа произошла ошибка, в противном случае nil.
func CreateDoc(db *gorm.DB, doc *Doc, user *User) error {
	if err := db.Create(&doc).Error; err != nil {
		return err
	}

	ids := append(doc.EditorsIDs, append(doc.WatcherIDs, doc.ReaderIDs...)...)
	var users []User
	if len(ids) > 0 {
		if err := db.Where("id IN (?)", ids).Find(&users).Error; err != nil {
			return err
		}
	}

	doc.ReaderIDs = getUniqueDocMemberIDs(doc.WatcherIDs, doc.ReaderIDs, doc.EditorsIDs)

	userMap := utils.SliceToMap(&users, func(u *User) string { return u.ID })
	var newAccessRules []DocAccessRules
	for id, u := range userMap {
		newAccessRules = append(newAccessRules, DocAccessRules{
			Id: GenUUID(),

			MemberId:    uuid.Must(uuid.FromString(u.ID)),
			CreatedById: uuid.Must(uuid.FromString(user.ID)),
			DocId:       doc.ID,
			UpdatedById: uuid.NullUUID{},
			WorkspaceId: uuid.Must(uuid.FromString(doc.WorkspaceId)),
			Edit:        utils.CheckInSlice(doc.EditorsIDs, id),
			Watch:       utils.CheckInSlice(doc.WatcherIDs, id),
		})
	}

	if err := db.CreateInBatches(&newAccessRules, 10).Error; err != nil {
		return err
	}

	doc.Editors = utils.ToPtr(utils.SliceToSlice(&doc.EditorsIDs, func(s *string) User { return userMap[*s] }))
	doc.Watchers = utils.ToPtr(utils.SliceToSlice(&doc.WatcherIDs, func(s *string) User { return userMap[*s] }))
	doc.Readers = utils.ToPtr(utils.SliceToSlice(&doc.ReaderIDs, func(s *string) User { return userMap[*s] }))

	return nil
}

func getUniqueDocMemberIDs(watcherIDs, readerIDs, editorIDs []string) []string {
	editorSet := make(map[string]bool)
	for _, id := range editorIDs {
		editorSet[id] = true
	}

	filteredWatchers := make([]string, 0)
	for _, id := range watcherIDs {
		if !editorSet[id] {
			filteredWatchers = append(filteredWatchers, id)
		}
	}

	return utils.MergeUniqueSlices(filteredWatchers, readerIDs)
}
