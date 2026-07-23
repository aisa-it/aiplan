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
	"encoding/json"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/config"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dto"
	filestorage "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/file-storage"
	"github.com/gofrs/uuid"
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
	Id uuid.UUID `json:"id" gorm:"type:uuid"`
	// user_id uuid IS_NULL:NO
	// Note: type:text используется потому что в существующей БД это поле имеет тип text, а не uuid
	UserId uuid.UUID `json:"user_id" gorm:"type:uuid;index"`
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
		Id:        GenUUID(),
		UserId:    user.ID,
		ResetedAt: time.Now(),
	}).Error
}

type FileAsset struct {
	Id        uuid.UUID `json:"id" gorm:"primaryKey;type:uuid"`
	CreatedAt time.Time `json:"created_at"`
	// Note: type:text используется потому что в существующей БД это поле имеет тип text, а не uuid
	CreatedById uuid.NullUUID `json:"created_by,omitempty" gorm:"type:uuid" extensions:"x-nullable"`

	WorkspaceId uuid.NullUUID `json:"workspace,omitempty" gorm:"type:uuid"`
	IssueId     uuid.NullUUID `json:"issue" gorm:"foreignKey:ID"`
	CommentId   uuid.NullUUID `json:"comment" gorm:"foreignKey:Id;type:uuid" extensions:"x-nullable"`

	DocId        uuid.NullUUID `json:"doc" gorm:"foreignKey:ID;type:uuid"`
	DocCommentId uuid.NullUUID `json:"doc_comment" gorm:"type:uuid"`

	FormId uuid.NullUUID `json:"form" gorm:"foreignKey:ID;type:uuid"`

	Name        string `json:"name" gorm:"index"`
	FileSize    int    `json:"size"`
	ContentType string `json:"content_type"`

	Workspace *Workspace    `json:"-" gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	Author    *User         `json:"-" gorm:"foreignKey:CreatedById;references:ID" extensions:"x-nullable"`
	Comment   *IssueComment `json:"-" gorm:"foreignKey:CommentId;references:Id" extensions:"x-nullable"`
}

// Удаляет запись о сбросе сессии пользователя в базе данных.
//
// Параметры:
//   - tx: экземпляр gorm.DB для взаимодействия с базой данных.
//
// Возвращает:
//   - error: ошибка, если при создании записи произошла ошибка.
func (asset *FileAsset) BeforeDelete(tx *gorm.DB) error {
	if asset == nil || asset.Id.IsNil() {
		return nil
	}
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

func (asset *FileAsset) GetWorkspaceId() uuid.UUID {
	if asset.WorkspaceId.Valid {
		return asset.WorkspaceId.UUID
	}
	return uuid.Nil
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
		Id:          asset.Id,
		Name:        asset.Name,
		FileSize:    asset.FileSize,
		ContentType: asset.ContentType,
	}
}

type ReleaseNote struct {
	ID          uuid.UUID          `gorm:"primaryKey;type:uuid" json:"id"`
	TagName     string             `json:"tag_name" gorm:"uniqueIndex"`
	PublishedAt time.Time          `json:"published_at"`
	Body        types.RedactorHTML `json:"body"`
	AuthorId    uuid.UUID          `json:"-" gorm:"type:uuid"`

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
		ID:          r.ID,
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

// DeferredNotifications corresponds to the notifications_log table
type DeferredNotifications struct {
	ID uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`

	UserID      uuid.UUID     `gorm:"type:uuid;not null;index" json:"user_id"`
	User        *User         `gorm:"foreignKey:UserID" json:"user,omitempty" extensions:"x-nullable"`
	IssueID     uuid.NullUUID `gorm:"type:uuid;index" json:"issue_id" extensions:"x-nullable"`
	Issue       *Issue        `gorm:"foreignKey:IssueID" json:"issue,omitempty" extensions:"x-nullable"`
	ProjectID   uuid.NullUUID `gorm:"type:uuid;index" json:"project_id" extensions:"x-nullable"`
	Project     *Project      `gorm:"foreignKey:ProjectID" json:"project,omitempty" extensions:"x-nullable"`
	WorkspaceID uuid.NullUUID `gorm:"type:uuid;index" json:"workspace_id" extensions:"x-nullable"`
	Workspace   *Workspace    `gorm:"foreignKey:WorkspaceID" json:"workspace,omitempty" extensions:"x-nullable"`

	NotificationType    string          `gorm:"type:varchar(50);not null" json:"notification_type"`
	DeliveryMethod      string          `gorm:"type:varchar(50);not null" json:"delivery_method"`
	TimeSend            *time.Time      `gorm:"type:timestamptz;index" json:"time_send"`
	AttemptCount        int             `gorm:"default:0;index:idx_deferred_notifications_attempt_count" json:"attempt_count"`
	LastAttemptAt       time.Time       `gorm:"type:timestamptz;autoUpdateTime" json:"last_attempt_at"`
	SentAt              *time.Time      `gorm:"type:timestamptz;index:idx_deferred_notifications_sent_at" json:"sent_at" extensions:"x-nullable"`
	NotificationPayload json.RawMessage `gorm:"type:jsonb" json:"notification_payload"`
}

// TableName sets the insert table name for this struct type
func (DeferredNotifications) TableName() string {
	return "deferred_notifications"
}

// WorkspaceActivityExtendFields
// -migration
type RootActivityExtendFields struct {
	WorkspaceExtendFields
}

type JitsiTokenLog struct {
	ID          uint64        `gorm:"unique;primaryKey;autoIncrement"`
	UserId      uuid.UUID     `gorm:"index"`
	WorkspaceId uuid.NullUUID `gorm:"index"`
	Room        string        `gorm:"index"`
	CreatedAt   time.Time

	IP     string
	UAgent string

	User      *User      `gorm:"foreignKey:UserId"`
	Workspace *Workspace `gorm:"foreignKey:WorkspaceId"`
}

func (stl JitsiTokenLog) ToDTO() dto.JitsiTokenLog {
	return dto.JitsiTokenLog{
		ID:          stl.ID,
		UserId:      stl.UserId,
		WorkspaceId: stl.WorkspaceId,
		Room:        stl.Room,
		CreatedAt:   stl.CreatedAt,
		IP:          stl.IP,
		UAgent:      stl.UAgent,
		User:        stl.User.ToLightDTO(),
		Workspace:   stl.Workspace.ToLightDTO(),
	}
}
