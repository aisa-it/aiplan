// DAO (Data Access Object) для работы с данными пользователей, поисковыми фильтрами и уведомлениями.
//
// Основные возможности:
//   - CRUD операции с пользователями.
//   - Получение и фильтрация поисковых фильтров.
//   - Создание, чтение и обновление уведомлений для пользователей.
package dao

import (
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dto"
	policy "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/redactor-policy"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

// Пользователи
type User struct {
	ID uuid.UUID `gorm:"column:id;primaryKey;type:uuid" json:"id"`

	Password   string  `json:"-"`
	Username   *string `json:"username" gorm:"uniqueIndex:,where:deleted_at is NULL" validate:"omitempty,username"`
	Email      string  `json:"email" gorm:"uniqueIndex:,where:deleted_at is NULL and email <> ''"`
	TelegramId *int64  `json:"telegram_id,omitempty" gorm:"index" extensions:"x-nullable"`
	FirstName  string  `json:"first_name" validate:"fullName"`
	LastName   string  `json:"last_name" validate:"fullName"`

	Avatar   string        `json:"avatar" gorm:"-"`
	AvatarId uuid.NullUUID `json:"avatar_id" gorm:"type:uuid"`

	StatusEmoji   sql.NullString `gorm:"type:text" validate:"statusEmoji"`
	Status        sql.NullString `gorm:"type:varchar(20)"`
	StatusEndDate sql.NullTime

	CreatedAt   time.Time      `json:"created_at"`
	CreatedByID uuid.NullUUID  `json:"created_by_id,omitempty" gorm:"type:uuid" extensions:"x-nullable"`
	DeletedAt   gorm.DeletedAt `json:"-" gorm:"index"`

	UpdatedAt time.Time `json:"-"`
	// Note: type:text используется потому что в существующей БД это поле имеет тип text, а не uuid
	// Переименовано поле для избежания конфликта с UpdatedById в других моделях
	SelfUpdatedByUserId uuid.NullUUID `json:"-" gorm:"column:updated_by_id;type:uuid;index" extensions:"x-nullable"`

	IsSuperuser     bool `json:"is_superuser"`
	IsActive        bool `json:"is_active" gorm:"default:true"`
	IsEmailVerified bool `json:"-"`
	IsOnboarded     bool `json:"is_onboarded"`

	Tutorial int `json:"tutorial" gorm:"default:0"`

	Token     string  `json:"-" gorm:"index"`
	AuthToken *string `json:"-" gorm:"uniqueIndex" extensions:"x-nullable"`

	UserTimezone types.TimeZone `json:"user_timezone" gorm:"default:'Europe/Moscow'"`

	LastActive      *time.Time `json:"last_active" extensions:"x-nullable"`
	LastLoginTime   *time.Time `json:"-" extensions:"x-nullable"`
	LastLogoutTime  *time.Time `json:"-" extensions:"x-nullable"`
	LastLoginIp     string     `json:"-"`
	LastLogoutIp    string     `json:"-"`
	LastLoginUagent string     `json:"-"`
	LoginAttempts   int        `json:"-"`
	BlockedUntil    sql.NullTime
	TokenUpdatedAt  *time.Time `json:"-" extensions:"x-nullable"`

	AuthProvider string `json:"-" gorm:"default:'local'"`

	LastWorkspaceId uuid.NullUUID `json:"-" gorm:"type:uuid;index" extensions:"x-nullable"`

	Role *string `json:"role" extensions:"x-nullable"`

	IsBot         bool `json:"is_bot"`
	IsIntegration bool `json:"is_integration"`

	Theme types.Theme `json:"theme" gorm:"type:jsonb"`

	Domain types.NullDomain `json:"-" gorm:"text"`

	ViewProps types.ViewProps    `json:"view_props" gorm:"type:jsonb"`
	Settings  types.UserSettings `json:"settings" gorm:"type:jsonb"`

	AvatarAsset   *FileAsset `json:"avatar_details,omitempty" gorm:"foreignKey:AvatarId" extensions:"x-nullable"`
	CreatedBY     *User      `json:"created_by" gorm:"foreignKey:CreatedByID" extensions:"x-nullable"` // 'BY' NOT A MISTAKE, SOME SPECIAL SHIT FOR MIGRATOR
	SelfUpdatedBy *User      `json:"-" gorm:"foreignKey:SelfUpdatedByUserId;references:ID;constraint:-" extensions:"x-nullable"`
	LastWorkspace *Workspace `json:"-" gorm:"foreignKey:LastWorkspaceId" extensions:"x-nullable"`

	SearchFilters []SearchFilter `json:"-" gorm:"constraint:OnDelete:CASCADE;many2many:user_search_filters"`
}

func (u User) GetId() uuid.UUID {
	return u.ID
}

func (u User) GetString() string {
	return u.Email
}

func (u User) GetEntityType() actField.ActivityField {
	return "user"
}

func (u *User) ToLightDTO() *dto.UserLight {
	if u == nil {
		return nil
	}
	d := &dto.UserLight{
		ID:           u.ID,
		Username:     u.Username,
		Email:        u.Email,
		FirstName:    u.FirstName,
		LastName:     u.LastName,
		Avatar:       u.Avatar,
		AvatarId:     u.AvatarId,
		UserTimezone: u.UserTimezone,
		LastActive:   u.LastActive,
		TelegramId:   u.TelegramId,
		CreatedAt:    u.CreatedAt,

		IsSuperuser:   u.IsSuperuser,
		IsActive:      u.IsActive,
		IsOnboarded:   u.IsOnboarded,
		IsBot:         u.IsBot,
		IsIntegration: u.IsIntegration,
	}

	if u.BlockedUntil.Valid {
		d.BlockedUntil = &u.BlockedUntil.Time
	}

	if u.StatusEmoji.Valid && (!u.StatusEndDate.Valid || u.StatusEndDate.Time.After(time.Now())) {
		d.StatusEmoji = &u.StatusEmoji.String

		if u.Status.Valid && u.StatusEmoji.String == "💬" {
			d.Status = &u.Status.String
		} else {
			ss := utils.ValidStatusEmoji[u.StatusEmoji.String]
			d.Status = &ss
		}

		if u.StatusEndDate.Valid {
			st := u.StatusEndDate.Time
			d.StatusEndDate = &st
		}
	}

	return d
}

func (u *User) ToDTO() *dto.User {
	if u == nil {
		return nil
	}

	userDto := dto.User{
		UserLight: *u.ToLightDTO(),

		Theme:             u.Theme,
		ViewProps:         u.ViewProps,
		Settings:          u.Settings,
		Tutorial:          u.Tutorial,
		LastWorkspaceId:   u.LastWorkspaceId,
		NotificationCount: 0,
		AttachmentsAllow:  nil,
	}

	if u.LastWorkspace != nil {
		userDto.LastWorkspaceSlug = &u.LastWorkspace.Slug
	}

	return &userDto
}

func (u *User) BeforeCreate(tx *gorm.DB) (err error) {
	if u.ID == uuid.Nil {
		u.ID = GenUUID()
	}
	u.Settings = types.DefaultSettings
	u.ViewProps = types.DefaultViewProps
	u.CreatedAt = time.Now()

	return
}

func (u *User) AfterUpdate(tx *gorm.DB) (err error) {
	if u.TelegramId != nil && *u.TelegramId == 0 {
		if err := tx.Model(u).UpdateColumn("telegram_id", nil).Error; err != nil {
			return err
		}
	}

	return u.AfterFind(tx)
}

func (u *User) AfterFind(tx *gorm.DB) (err error) {
	if u.AvatarId.Valid {
		u.Avatar = Config.WebURL.URL.String() + filepath.Join("/", Config.AWSBucketName, u.AvatarId.UUID.String())
	} else {
		u.AvatarAsset = nil
	}

	if !u.Domain.Valid {
		u.Domain = types.NullDomain{URL: Config.WebURL.URL, Valid: true}
	}

	if u.Settings.IsEmpty() {
		u.Settings = types.DefaultSettings
	}

	return nil
}

func (u *User) String() string {
	return fmt.Sprintf("%s (%s)", u.ID, u.Email)
}

func (u *User) GetName() string {
	if u.FirstName != "" && u.LastName != "" {
		return fmt.Sprintf("%s %s", u.FirstName, u.LastName)
	}
	return u.Email
}

func (u *User) CanReceiveNotifications() bool {
	return u.IsActive && !u.IsIntegration && !u.IsBot
}

func (u *User) IsNotify(typeMsg string) bool {
	switch typeMsg {
	case "email":
		return !u.Settings.EmailNotificationMute
	case "app":
		return !u.Settings.AppNotificationMute
	case "telegram":
		return !u.Settings.TgNotificationMute
	}
	return false
}

func (User) TableName() string {
	return "users"
}

type UserFeedback struct {
	// Note: type:text используется потому что в существующей БД это поле имеет тип text, а не uuid
	UserID    uuid.UUID `json:"user_id" gorm:"primaryKey;type:uuid"`
	CreatedAt time.Time `json:"created_at" gorm:"<-:create"`
	UpdatedAt time.Time `json:"updated_at"`

	Stars    int    `json:"stars"`
	Feedback string `json:"feedback"`

	User User `json:"user_detail" gorm:"foreignKey:UserID;references:ID"`
}

func (uf *UserFeedback) ToDTO() *dto.UserFeedback {
	if uf == nil {
		return nil
	}
	return &dto.UserFeedback{
		UserID: uf.UserID,

		Stars:    uf.Stars,
		Feedback: uf.Feedback,
		User:     *uf.User.ToLightDTO(),
	}
}
func (UserFeedback) TableName() string { return "user_feedbacks" }

type SearchFilter struct {
	ID uuid.UUID `gorm:"column:id;primaryKey;type:uuid" json:"id"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	AuthorID  uuid.UUID `json:"author_id" gorm:"type:uuid"`

	Name        string         `json:"name"`
	NameTokens  types.TsVector `json:"-" gorm:"index:search_filter_name_tokens,type:gin"`
	Description string         `json:"description"`
	Public      bool           `json:"public"`

	Filter types.IssuesListFilters `json:"filter" gorm:"type:jsonb"`

	Author *User `json:"author_detail" gorm:"foreignKey:AuthorID" extensions:"x-nullable"`

	Users    []User   `json:"-" gorm:"constraint:OnDelete:CASCADE;many2many:user_search_filters"`
	URL      *url.URL `json:"-" gorm:"-" extensions:"x-nullable"`
	ShortURL *url.URL `json:"-" gorm:"-" extensions:"x-nullable"`
}

func (sf *SearchFilter) ToLightDTO() *dto.SearchFilterLight {
	if sf == nil {
		return nil
	}
	sf.SetUrl()
	return &dto.SearchFilterLight{
		ID:          sf.ID,
		Name:        sf.Name,
		Description: sf.Description,
		Public:      sf.Public,
		Filter:      sf.Filter,
		Url:         types.JsonURL{URL: sf.URL},
		ShortUrl:    types.JsonURL{URL: sf.ShortURL},
	}
}

func (sf *SearchFilter) ToFullDTO() *dto.SearchFilterFull {
	if sf == nil {
		return nil
	}
	return &dto.SearchFilterFull{
		SearchFilterLight: *sf.ToLightDTO(),
		AuthorID:          sf.AuthorID,
		Author:            sf.Author.ToLightDTO(),
	}
}

func (sf *SearchFilter) AfterFind(tx *gorm.DB) (err error) {
	sf.SetUrl()
	return nil
}

func (SearchFilter) TableName() string { return "search_filters" }

func (sf *SearchFilter) SetUrl() {
	if !sf.Public {
		return
	}
	urlFilter := fmt.Sprintf("/filters/%s/", sf.ID.String())
	shortUrl := fmt.Sprintf("/sf/%s/", sf.ID)

	u, _ := url.Parse(urlFilter)
	shortU, _ := url.Parse(shortUrl)

	sf.URL = Config.WebURL.URL.ResolveReference(u)
	sf.ShortURL = Config.WebURL.URL.ResolveReference(shortU)
}

type UserNotifications struct {
	ID        uuid.UUID      `gorm:"column:id;primaryKey;type:uuid" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`

	// Note: type:text используется потому что в существующей БД это поле имеет тип text, а не uuid
	UserId uuid.UUID `json:"user_id" gorm:"type:uuid;index"`
	User   *User     `json:"user_detail,omitempty" gorm:"foreignKey:UserId" extensions:"x-nullable"`

	Type             string          `json:"type"`
	EntityActivityId uuid.NullUUID   `json:"entity_activity,omitempty"`
	EntityActivity   *EntityActivity `json:"entity_activity_detail,omitempty" gorm:"foreignKey:EntityActivityId" extensions:"x-nullable"`

	CommentId uuid.NullUUID `json:"comment_id,omitempty" gorm:"type:uuid"`
	Comment   *IssueComment `json:"comment,omitempty" gorm:"foreignKey:CommentId" extensions:"x-nullable"`

	WorkspaceId uuid.NullUUID `json:"workspace_id,omitempty" gorm:"type:uuid"`
	Workspace   *Workspace    `json:"workspace,omitempty" gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`

	IssueId uuid.NullUUID `json:"issue_id,omitempty" gorm:"type:uuid"`
	Issue   *Issue        `json:"issue,omitempty" gorm:"foreignKey:IssueId" extensions:"x-nullable"`

	Title    string        `json:"title,omitempty"`
	Msg      string        `json:"msg,omitempty"`
	AuthorId uuid.NullUUID `json:"author_id" gorm:"type:uuid"`
	Author   *User         `json:"author,omitempty" gorm:"foreignKey:AuthorId" extensions:"x-nullable"`
	Viewed   bool          `json:"viewed" gorm:"default:false"`

	TargetUser *User `json:"target_user,omitempty" gorm:"-"`

	IssueActivityId uuid.NullUUID  `json:"issue_activity,omitempty"`
	IssueActivity   *IssueActivity `json:"issue_activity_detail,omitempty" gorm:"foreignKey:IssueActivityId" extensions:"x-nullable"`

	ProjectActivityId uuid.NullUUID    `json:"project_activity,omitempty"`
	ProjectActivity   *ProjectActivity `json:"project_activity_detail,omitempty" gorm:"foreignKey:ProjectActivityId" extensions:"x-nullable"`

	FormActivityId uuid.NullUUID `json:"form_activity,omitempty"`
	FormActivity   *FormActivity `json:"form_activity_detail,omitempty" gorm:"foreignKey:FormActivityId" extensions:"x-nullable"`

	DocActivityId uuid.NullUUID `json:"doc_activity,omitempty"`
	DocActivity   *DocActivity  `json:"doc_activity_detail,omitempty" gorm:"foreignKey:DocActivityId" extensions:"x-nullable"`

	SprintActivityId uuid.NullUUID   `json:"sprint_activity,omitempty"`
	SprintActivity   *SprintActivity `json:"sprint_activity_detail,omitempty" gorm:"foreignKey:SprintActivityId" extensions:"x-nullable"`

	WorkspaceActivityId uuid.NullUUID      `json:"workspace_activity,omitempty"`
	WorkspaceActivity   *WorkspaceActivity `json:"workspace_activity_detail,omitempty" gorm:"foreignKey:WorkspaceActivityId" extensions:"x-nullable"`

	RootActivityId uuid.NullUUID `json:"root_activity,omitempty"`
	RootActivity   *RootActivity `json:"root_activity_detail,omitempty" gorm:"foreignKey:RootActivityId" extensions:"x-nullable"`

	ActivityEventId uuid.NullUUID  `json:"activity,omitempty"`
	ActivityEvent   *ActivityEvent `json:"activity_event,omitempty" gorm:"foreignKey:ActivityEventId" extensions:"x-nullable"`

	FullActivity *FullActivity `json:"full_activity_detail,omitempty" gorm:"-" extensions:"x-nullable"`
}

func (un *UserNotifications) ToLightDTO() *dto.UserNotificationsLight {
	if un == nil {
		return nil
	}
	return &dto.UserNotificationsLight{
		ID:               un.ID,
		UserId:           un.UserId,
		Type:             un.Type,
		Viewed:           un.Viewed,
		Title:            un.Title,
		Msg:              un.Msg,
		AuthorId:         un.AuthorId,
		EntityActivityId: un.EntityActivityId,
		CommentId:        un.CommentId,
		WorkspaceId:      un.WorkspaceId,
		IssueId:          un.IssueId,
	}
}

func (un *UserNotifications) ToFullDTO() *dto.UserNotificationsFull {
	if un == nil {
		return nil
	}
	return &dto.UserNotificationsFull{
		UserNotificationsLight: *un.ToLightDTO(),
		User:                   un.User.ToLightDTO(),
		Comment:                un.Comment.ToLightDTO(),
		Workspace:              un.Workspace.ToLightDTO(),
		Issue:                  un.Issue.ToLightDTO(),
		Author:                 un.Author.ToLightDTO(),
		TargetUser:             un.TargetUser.ToLightDTO(),
	}
}

func (un *UserNotifications) AfterCreate(tx *gorm.DB) (err error) {
	if un.ID == uuid.Nil {
		un.ID = GenUUID()
	}
	if un.Title != "" {
		un.Title = policy.StripTagsPolicy.Sanitize(un.Title)
	}
	if un.Msg != "" {
		un.Msg = policy.StripTagsPolicy.Sanitize(un.Msg)
	}

	return
}

func (un *UserNotifications) GetWorkspaceId() uuid.UUID {
	if un.WorkspaceId.Valid {
		return un.WorkspaceId.UUID
	}
	return uuid.Nil
}

func (un *UserNotifications) AfterFind(tx *gorm.DB) (err error) {
	if un.EntityActivity != nil {
		if un.EntityActivity.Verb == "updated" && (*un.EntityActivity.Field == "assignees" || *un.EntityActivity.Field == "watchers") {
			if err := tx.Where("id = ? or id = ?", un.EntityActivity.OldIdentifier, un.EntityActivity.NewIdentifier).First(&un.TargetUser).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

/*type UserSearchFilter struct {
	UserId   string    `json:"user_id"`
	FilterId uuid.UUID `json:"filter_id"`

	Filter *SearchFilter `json:"filter" gorm:"foreignKey:FilterId"`
}*/

func GetUsers(db *gorm.DB) []User {
	var res []User
	db.Find(&res)
	return res
}

func UserExists(db *gorm.DB, id string) (bool, error) {
	var exists bool
	if err := db.Model(&User{}).
		Select("EXISTS(?)",
			db.Model(&User{}).
				Select("1").
				Where("id = ?", id),
		).
		Find(&exists).Error; err != nil {
		return false, err
	}
	return exists, nil
}
