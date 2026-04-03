package dao

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dto"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

type SprintEntityI interface {
	WorkspaceEntityI
	GetSprintId() uuid.UUID
}

type Sprint struct {
	Id        uuid.UUID `gorm:"primaryKey;type:uuid"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`

	// Note: type:text используется потому что в существующей БД это поле имеет тип text, а не uuid
	CreatedById uuid.UUID `gorm:"type:uuid"`
	// Note: type:text используется потому что в существующей БД это поле имеет тип text, а не uuid
	UpdatedById    uuid.NullUUID `gorm:"type:uuid" extensions:"x-nullable"`
	WorkspaceId    uuid.UUID     `gorm:"type:uuid; uniqueIndex:sprint_uniq_idx,priority:1,where:deleted_at is NULL"`
	SprintFolderId uuid.NullUUID `gorm:"type:uuid"`

	CreatedBy    User
	UpdatedBy    *User
	Workspace    *Workspace    `gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	SprintFolder *SprintFolder `json:"sprint_folder_detail" gorm:"foreignKey:SprintFolderId" extensions:"x-nullable"`

	Name        string             `json:"name"`
	NameTokens  types.TsVector     `gorm:"index:sprint_name_tokens,type:gin"`
	SequenceId  int                `json:"sequence_id" gorm:"default:1;uniqueIndex:sprint_uniq_idx,priority:2,where:deleted_at is NULL"`
	Description types.RedactorHTML `json:"description"`

	StartDate sql.NullTime `json:"start_date" gorm:"index"`
	EndDate   sql.NullTime `json:"end_date" gorm:"index"`

	Issues   []Issue `gorm:"many2many:sprint_issues;joinForeignKey:SprintId;joinReferences:IssueId"`
	Watchers []User  `gorm:"many2many:sprint_watchers;foreignKey:Id;joinForeignKey:SprintId;references:ID;joinReferences:WatcherId"`

	Stats types.SprintStats `gorm:"-" json:"-"`
	View  types.ViewProps   `gorm:"-" json:"-"`
	URL   *url.URL          `json:"-" gorm:"-" extensions:"x-nullable"`
}

// SprintExtendFields
// -migration
type SprintExtendFields struct {
	NewSprint *Sprint `json:"-" gorm:"-" field:"sprint::workspace" extensions:"x-nullable"`
	OldSprint *Sprint `json:"-" gorm:"-" field:"sprint::workspace" extensions:"x-nullable"`
}

// IssueSprintExtendFields
// -migration
type IssueSprintExtendFields struct {
	NewIssueSprint *Sprint `json:"-" gorm:"-" field:"sprint::issue" extensions:"x-nullable"`
	OldIssueSprint *Sprint `json:"-" gorm:"-" field:"sprint::issue" extensions:"x-nullable"`
}

func (Sprint) TableName() string { return "sprints" }

// GetId Возвращает идентификатор спринта в виде строки.
func (s Sprint) GetId() uuid.UUID {
	return s.Id
}

// GetString Возвращает заголовок спринта.
func (s Sprint) GetString() string {
	return s.Name
}

// GetEntityType Возвращает тип сущности спринта (sprint). Используется для определения типа данных при работе с активностями.
func (s Sprint) GetEntityType() actField.ActivityField {
	return actField.Sprint.Field
}

func (s Sprint) GetWorkspaceId() uuid.UUID {
	return s.WorkspaceId
}

func (s Sprint) GetSprintId() uuid.UUID {
	return s.Id
}

func (s *Sprint) AfterFind(tx *gorm.DB) error {
	if userId, ok := tx.Get("userId"); ok {
		var sprintView *SprintViews
		if err := tx.Where("member_id = ?", userId).
			Where("sprint_id = ?", s.Id).
			First(&sprintView).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				s.View = types.DefaultViewProps
			} else {
				return err
			}
		} else {
			s.View = sprintView.ViewProps
		}
	}
	s.SetUrl()

	return nil
}

func (s *Sprint) SetUrl() {
	raw := fmt.Sprintf("/%s/sprints/%d", s.WorkspaceId.String(), s.SequenceId)
	u, _ := url.Parse(raw)
	s.URL = Config.WebURL.URL.ResolveReference(u)
}

func (s *Sprint) GetFullName() string {
	s.StartDate.Time.Format("02.01")

	return fmt.Sprintf("%s ( %s-%s )", s.Name, s.StartDate.Time.Format("02.01"), s.EndDate.Time.Format("02.01"))
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

func CleanupActivityData(tx, q *gorm.DB, id uuid.UUID, layers ...types.EntityLayer) error {
	subQuery := q.Model(&ActivityEvent{}).Select("id")

	if err := tx.Where("activity_event_id IN (?)", subQuery).
		Unscoped().
		Delete(&UserAppNotify{}).Error; err != nil {
		return err
	}

	if err := q.Unscoped().Delete(&ActivityEvent{}).Error; err != nil {
		return err
	}

	if len(layers) > 0 {
		cleanId := map[string]interface{}{"new_identifier": nil, "old_identifier": nil}
		if err := tx.Where("new_identifier = ? OR old_identifier = ?",
			id, id).
			Where("entity_type IN (?)", layers).
			Model(&ActivityEvent{}).
			Updates(cleanId).Error; err != nil {
			return err
		}
	}

	return nil
}

func (s *Sprint) BeforeDelete(tx *gorm.DB) error {

	query := tx.Where("entity_type = ?", types.LayerSprint).Where("sprint_id = ?", s.Id)
	if err := CleanupActivityData(tx, query, s.Id, types.LayerWorkspace, types.LayerIssue); err != nil {
		return err
	}

	if err := tx.Where("sprint_id = ?", s.Id).Delete(&SprintViews{}).Error; err != nil {
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

func (s *Sprint) GetIssuesIDs() []uuid.UUID {
	ids := make([]uuid.UUID, len(s.Issues))
	for i, issue := range s.Issues {
		ids[i] = issue.ID
	}
	return ids
}

func (s *Sprint) ToLightDTO() *dto.SprintLight {
	if s == nil {
		return nil
	}

	s.SetUrl()

	return &dto.SprintLight{
		Id:           s.Id,
		Name:         s.Name,
		SequenceId:   s.SequenceId,
		Description:  s.Description,
		Url:          types.JsonURL{URL: s.URL},
		SprintFolder: s.SprintFolder.ToDTO(),
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
		View: s.View,
	}
}

type SprintIssue struct {
	Id        uuid.UUID `gorm:"primaryKey;type:uuid"`
	CreatedAt time.Time
	UpdatedAt time.Time

	SprintId    uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:sprint_issue_uniq_idx,priority:3"`
	IssueId     uuid.UUID `gorm:"type:uuid;uniqueIndex:sprint_issue_uniq_idx,priority:4"`
	ProjectId   uuid.UUID `gorm:"type:uuid;uniqueIndex:sprint_issue_uniq_idx,priority:2"`
	WorkspaceId uuid.UUID `gorm:"type:uuid;uniqueIndex:sprint_issue_uniq_idx,priority:1"`
	// Note: type:text используется потому что в существующей БД это поле имеет тип text, а не uuid
	CreatedById uuid.UUID `gorm:"type:uuid"`

	Position int `json:"position" gorm:"default:0;index"`

	Sprint    *Sprint    `gorm:"foreignKey:SprintId;references:Id"`
	Issue     *Issue     `gorm:"foreignKey:IssueId;references:ID"`
	Project   *Project   `gorm:"foreignKey:ProjectId;references:ID"`
	Workspace *Workspace `gorm:"foreignKey:WorkspaceId;references:ID"`

	CreatedBy *User `gorm:"foreignKey:CreatedById"`
}

func (SprintIssue) TableName() string { return "sprint_issues" }

// SprintIssuesExtendFields
// -migration
type SprintIssuesExtendFields struct {
	NewSprintIssue *Issue `json:"-" gorm:"-" field:"issue::sprint" extensions:"x-nullable"`
	OldSprintIssue *Issue `json:"-" gorm:"-" field:"issue::sprint" extensions:"x-nullable"`
}

type SprintWatcher struct {
	Id        uuid.UUID `gorm:"primaryKey;type:uuid"`
	CreatedAt time.Time
	// Note: type:text используется потому что в существующей БД это поле имеет тип text, а не uuid
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

// SprintActivityExtendFields
// -migration
type SprintActivityExtendFields struct {
	SprintWatcherExtendFields
	SprintIssuesExtendFields
}

type SprintViews struct {
	Id        uuid.UUID `gorm:"primaryKey;type:uuid"`
	CreatedAt time.Time
	UpdatedAt time.Time

	SprintId uuid.UUID `gorm:"type:uuid;uniqueIndex:idx_sprint_user_unique,priority:1"`
	MemberId uuid.UUID `gorm:"type:uuid;uniqueIndex:idx_sprint_user_unique,priority:2"`

	Sprint *Sprint `gorm:"foreignKey:SprintId"`
	Member *User   `gorm:"foreignKey:MemberId"`

	ViewProps types.ViewProps `json:"view_props" gorm:"type:jsonb"`
}

func (SprintViews) TableName() string {
	return "sprint_views"
}

type SprintFolder struct {
	Id        uuid.UUID `gorm:"primaryKey;type:uuid"`
	CreatedAt time.Time
	UpdatedAt time.Time

	CreatedById uuid.UUID     `gorm:"type:uuid"`
	UpdatedById uuid.NullUUID `gorm:"type:uuid" extensions:"x-nullable"`

	WorkspaceId uuid.UUID `gorm:"type:uuid; index:sprint_folders_idx"`

	CreatedBy User
	UpdatedBy *User
	Workspace *Workspace

	Sprints []Sprint `gorm:"-"`

	Name string `json:"name"`
}

func (SprintFolder) TableName() string {
	return "sprint_folders"
}

func (s *SprintFolder) ToDTO() *dto.SprintFolder {
	if s == nil {
		return nil
	}
	return &dto.SprintFolder{
		Id:   s.Id,
		Name: s.Name,
		Sprints: utils.SliceToSlice(&s.Sprints, func(i *Sprint) dto.SprintLight {
			return *i.ToLightDTO()
		}),
	}
}
