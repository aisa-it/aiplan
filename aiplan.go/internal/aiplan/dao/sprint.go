package dao

import (
	"database/sql"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dto"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

type SprintEntityI interface {
	WorkspaceEntityI
	GetSprintId() string
}

type Sprint struct {
	Id        uuid.UUID `gorm:"primaryKey;type:uuid"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`

	CreatedById uuid.UUID     `gorm:"type:uuid"`
	UpdatedById uuid.NullUUID `gorm:"type:uuid" extensions:"x-nullable"`
	WorkspaceId uuid.UUID     `gorm:"type:uuid; uniqueIndex:sprint_uniq_idx,priority:1,where:deleted_at is NULL"`

	CreatedBy User
	UpdatedBy *User
	Workspace *Workspace

	Name        string             `json:"name"`
	NameTokens  types.TsVector     `gorm:"index:sprint_name_tokens,type:gin"`
	SequenceId  int                `json:"sequence_id" gorm:"default:1;uniqueIndex:sprint_uniq_idx,priority:2,where:deleted_at is NULL"`
	Description types.RedactorHTML `json:"description"`

	StartDate sql.NullTime `json:"start_date" gorm:"index"`
	EndDate   sql.NullTime `json:"end_date" gorm:"index"`

	Issues   []Issue `gorm:"many2many:sprint_issues;joinForeignKey:SprintId;joinReferences:IssueId"`
	Watchers []User  `gorm:"many2many:sprint_watchers;foreignKey:Id;joinForeignKey:SprintId;references:ID;joinReferences:WatcherId"`

	Stats types.SprintStats `gorm:"-" json:"-"`
}

// SprintExtendFields
// -migration
type SprintExtendFields struct {
	NewSprint *Sprint `json:"-" gorm:"-" field:"sprint::workspace" extensions:"x-nullable"`
	OldSprint *Sprint `json:"-" gorm:"-" field:"sprint::workspace" extensions:"x-nullable"`

	NewSprintIssue *Issue `json:"-" gorm:"-" field:"issue::sprint" extensions:"x-nullable"`
	OldSprintIssue *Issue `json:"-" gorm:"-" field:"issue::sprint" extensions:"x-nullable"`
}

// IssueSprintExtendFields
// -migration
type IssueSprintExtendFields struct {
	NewIssueSprint *Sprint `json:"-" gorm:"-" field:"sprint::issue" extensions:"x-nullable"`
	OldIssueSprint *Sprint `json:"-" gorm:"-" field:"sprint::issue" extensions:"x-nullable"`
}

func (Sprint) TableName() string { return "sprints" }

// GetId Возвращает идентификатор спринта в виде строки.
func (s Sprint) GetId() string {
	return s.Id.String()
}

// GetString Возвращает заголовок спринта.
func (s Sprint) GetString() string {
	return s.Name
}

// GetEntityType Возвращает тип сущности спринта (sprint). Используется для определения типа данных при работе с активностями.
func (s Sprint) GetEntityType() string {
	return actField.FieldSprint.String()
}

func (s Sprint) GetWorkspaceId() string {
	return s.WorkspaceId.String()
}

func (s Sprint) GetSprintId() string {
	return s.GetId()
}

func (s *Sprint) BeforeCreate(tx *gorm.DB) (err error) {
	// Calculate sequence id
	var lastId sql.NullInt64
	row := tx.Model(Sprint{}).
		Select("max(sequence_id)").
		Where("workspace_id = ?", s.WorkspaceId).
		Row()
	if err := row.Scan(&lastId); err != nil {
		return err
	}

	// Just use the last ID specified (which should be the greatest) and add one to it
	if lastId.Valid {
		s.SequenceId = int(lastId.Int64 + 1)
	} else {
		s.SequenceId = 1
	}

	return nil
}

func (s *Sprint) BeforeDelete(tx *gorm.DB) (err error) {
	if err := tx.
		Where("sprint_activity_id in (?)", tx.Select("id").Where("sprint_id = ?", s.Id).
			Model(&SprintActivity{})).
		Unscoped().Delete(&UserNotifications{}).Error; err != nil {
		return err
	}

	cleanId := map[string]interface{}{"new_identifier": nil, "old_identifier": nil}
	tx.Where("(new_identifier = ? OR old_identifier =?) AND field = ?", s.Id, s.Id, s.GetEntityType()).
		Model(&WorkspaceActivity{}).
		Updates(cleanId)

	tx.Where("new_identifier = ? OR old_identifier =?", s.Id, s.Id).
		Model(&IssueActivity{}).
		Updates(cleanId)

	if err := tx.Where("workspace_id = ?", s.WorkspaceId).
		Where("sprint_id = ?", s.Id).Unscoped().Delete(&SprintActivity{}).Error; err != nil {
		return err
	}

	if err := tx.Where("workspace_id = ?", s.WorkspaceId).
		Where("sprint_id = ?", s.Id).Delete(&SprintWatcher{}).Error; err != nil {
		return err
	}

	if err := tx.Where("workspace_id = ?", s.WorkspaceId).
		Where("sprint_id = ?", s.Id).Delete(&SprintIssue{}).Error; err != nil {
		return err
	}

	return nil
}

func (s *Sprint) ToLightDTO() *dto.SprintLight {
	if s == nil {
		return nil
	}
	return &dto.SprintLight{
		Id:          s.Id,
		Name:        s.Name,
		SequenceId:  s.SequenceId,
		Description: s.Description,
		//Url:        types.JsonURL{},
		//ShortUrl:   types.JsonURL{},
		StartDate: utils.SqlNullTimeToPointerTime(s.StartDate),
		EndDate:   utils.SqlNullTimeToPointerTime(s.EndDate),
		Stats:     &s.Stats,
	}
}

func (s *Sprint) ToDTO() *dto.Sprint {
	if s == nil {
		return nil
	}
	return &dto.Sprint{
		SprintLight: *s.ToLightDTO(),
		CreatedAt:   s.CreatedAt,
		UpdatedAt:   &s.UpdatedAt,
		CreatedBy:   s.CreatedBy.ToLightDTO(),
		UpdatedBy:   s.UpdatedBy.ToLightDTO(),
		Workspace:   s.Workspace.ToLightDTO(),
		Issues: utils.SliceToSlice(&s.Issues, func(i *Issue) dto.IssueLight {
			return *i.ToLightDTO()
		}),
		Watchers: utils.SliceToSlice(&s.Watchers, func(i *User) dto.UserLight {
			return *i.ToLightDTO()
		}),
	}
}

type SprintIssue struct {
	Id        uuid.UUID `gorm:"primaryKey;type:uuid"`
	CreatedAt time.Time
	UpdatedAt time.Time

	SprintId    uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:sprint_issue_uniq_idx,priority:3"`
	IssueId     uuid.UUID `gorm:"type:text;uniqueIndex:sprint_issue_uniq_idx,priority:4"`
	ProjectId   uuid.UUID `gorm:"type:uuid;uniqueIndex:sprint_issue_uniq_idx,priority:2"`
	WorkspaceId uuid.UUID `gorm:"type:uuid;uniqueIndex:sprint_issue_uniq_idx,priority:1"`
	CreatedById uuid.UUID `gorm:"type:uuid"`

	Position int `json:"position" gorm:"default:0;index"`

	Sprint    *Sprint    `gorm:"foreignKey:SprintId;references:Id"`
	Issue     *Issue     `gorm:"foreignKey:IssueId;references:ID"`
	Project   *Project   `gorm:"foreignKey:ProjectId;references:ID"`
	Workspace *Workspace `gorm:"foreignKey:WorkspaceId;references:ID"`

	CreatedBy *User `gorm:"foreignKey:CreatedById"`
}

func (SprintIssue) TableName() string { return "sprint_issues" }

type SprintWatcher struct {
	Id          uuid.UUID `gorm:"primaryKey;type:uuid"`
	CreatedAt   time.Time
	CreatedById uuid.UUID `json:"created_by_id" gorm:"type:uuid" extensions:"x-nullable"`

	WatcherId   uuid.UUID `gorm:"uniqueIndex:sprint_watchers_idx,priority:2"`
	SprintId    uuid.UUID `gorm:"index;uniqueIndex:sprint_watchers_idx,priority:1"`
	WorkspaceId uuid.UUID `gorm:"type:uuid;index" json:"workspace_id"`

	Workspace *Workspace `gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	Sprint    *Sprint    `gorm:"foreignKey:SprintId" extensions:"x-nullable"`
	Watcher   *User      `gorm:"foreignKey:WatcherId" extensions:"x-nullable"`
}

func (SprintWatcher) TableName() string { return "sprint_watchers" }

// SprintWatcherExtendFields
// -migration
type SprintWatcherExtendFields struct {
	NewSprintWatcher *User `json:"-" gorm:"-" field:"watchers::sprint" extensions:"x-nullable"`
	OldSprintWatcher *User `json:"-" gorm:"-" field:"watchers::sprint" extensions:"x-nullable"`
}

type SprintActivity struct {
	Id        uuid.UUID `json:"id" gorm:"primaryKey;type:uuid"`
	CreatedAt time.Time `json:"created_at" gorm:"index:sprint_activities_sprint_index,sort:desc,priority:2;index:sprint_activities_actor_index,sort:desc,priority:2;index:sprint_activities_mail_index,where:notified = false"`
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
	// sprint_id uuid IS_NULL:YES
	SprintId string `json:"sprint_id" gorm:"index:sprint_activities_sprint_index,priority:1" extensions:"x-nullable"`
	// workspace_id uuid IS_NULL:NO
	WorkspaceId string `json:"workspace"`
	// actor_id uuid IS_NULL:YES
	ActorId *string `json:"actor,omitempty" gorm:"index:project_activities_actor_index,priority:1" extensions:"x-nullable"`

	// new_identifier uuid IS_NULL:YES
	NewIdentifier *string `json:"new_identifier" extensions:"x-nullable"`
	// old_identifier uuid IS_NULL:YES
	OldIdentifier *string       `json:"old_identifier" extensions:"x-nullable"`
	Notified      bool          `json:"-" gorm:"default:false"`
	TelegramMsgId pq.Int64Array `json:"-" gorm:"column:telegram_msg_ids;index;type:integer[]"`

	Workspace *Workspace `json:"workspace_detail" gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	Actor     *User      `json:"actor_detail" gorm:"foreignKey:ActorId" extensions:"x-nullable"`
	Sprint    *Sprint    `json:"sprint_detail" gorm:"foreignKey:SprintId" extensions:"x-nullable"`

	UnionCustomFields string `json:"-" gorm:"-"`
	SprintActivityExtendFields
	ActivitySender
}

// SprintActivityExtendFields
// -migration
type SprintActivityExtendFields struct {
	SprintWatcherExtendFields
}

func (SprintActivity) TableName() string { return "sprint_activities" }

func (sa SprintActivity) GetCustomFields() string {
	return sa.UnionCustomFields
}

func (SprintActivity) GetFields() []string {
	return []string{"id", "created_at", "verb", "field", "old_value", "new_value", "comment", "sprint_id", "workspace_id", "actor_id", "new_identifier", "old_identifier", "notified", "telegram_msg_ids"}
}

func (SprintActivity) GetEntity() string {
	return "sprint"
}

func (activity *SprintActivity) AfterFind(tx *gorm.DB) error {
	return EntityActivityAfterFind(activity, tx)
}

func (activity *SprintActivity) BeforeDelete(tx *gorm.DB) error {
	return tx.Where("sprint_activity_id = ?", activity.Id).Unscoped().Delete(&UserNotifications{}).Error
}

func (sa SprintActivity) GetUrl() *string {
	//if pa.Project.URL != nil {
	//	urlStr := pa.Project.URL.String()
	//	return &urlStr
	//}
	return nil
}

func (sa SprintActivity) SkipPreload() bool {
	if sa.Field == nil {
		return true
	}

	if sa.NewIdentifier == nil && sa.OldIdentifier == nil {
		return true
	}
	return false
}

func (sa SprintActivity) GetField() string {
	return pointerToStr(sa.Field)
}

func (sa SprintActivity) GetVerb() string {
	return sa.Verb
}

func (sa SprintActivity) GetNewIdentifier() string {
	return pointerToStr(sa.NewIdentifier)
}

func (sa SprintActivity) GetOldIdentifier() string {
	return pointerToStr(sa.OldIdentifier)
}

func (sa SprintActivity) GetId() string {
	return sa.Id.String()
}

func (activity *SprintActivity) ToLightDTO() *dto.EntityActivityLight {
	if activity == nil {
		return nil
	}

	return &dto.EntityActivityLight{
		Id:         activity.Id.String(),
		CreatedAt:  activity.CreatedAt,
		Verb:       activity.Verb,
		Field:      activity.Field,
		OldValue:   activity.OldValue,
		NewValue:   activity.NewValue,
		EntityType: "sprint",

		NewEntity: GetActionEntity(*activity, "New"),
		OldEntity: GetActionEntity(*activity, "Old"),

		EntityUrl: activity.GetUrl(),
	}
}
