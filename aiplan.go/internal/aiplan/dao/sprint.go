package dao

import (
	"database/sql"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
	"sheff.online/aiplan/internal/aiplan/dto"
	"sheff.online/aiplan/internal/aiplan/types"
	"sheff.online/aiplan/internal/aiplan/utils"
	"time"
)

type Sprint struct {
	Id        uuid.UUID `gorm:"primaryKey;type:uuid"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`

	CreatedById uuid.UUID     `gorm:"type:uuid"`
	UpdatedById uuid.NullUUID `gorm:"type:uuid" extensions:"x-nullable"`
	WorkspaceId uuid.UUID     `gorm:"type:uuid; index:issue_template,priority:1"`

	CreatedBy User
	UpdatedBy *User
	Workspace *Workspace

	Name        string             `json:"name"`
	NameTokens  types.TsVector     `gorm:"index:sprint_name_tokens,type:gin"`
	SequenceId  int                `json:"sequence_id" gorm:"default:1;index:,where:deleted_at is not null"`
	Description types.RedactorHTML `json:"description"`

	StartDate sql.NullTime `gorm:"index"`
	EndDate   sql.NullTime `gorm:"index"`

	Issues   []Issue `gorm:"many2many:sprint_issues;joinForeignKey:SprintId;joinReferences:IssueId"`
	Watchers []User  `gorm:"many2many:sprint_watchers;foreignKey:Id;joinForeignKey:SprintId;references:ID;joinReferences:WatcherId"`

	Stats types.SprintStats `gorm:"-" json:"-"`
}

func (Sprint) TableName() string { return "sprints" }

func (s *Sprint) BeforeCreate(tx *gorm.DB) (err error) {
	// Calculate sequence id
	var lastId sql.NullInt64
	row := tx.Model(Sprint{}).
		Select("max(sequence_id)").
		Unscoped().
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
	if err := tx.Where("workspace_id = ?", s.WorkspaceId).
		Where("sprint_id = ?", s.Id).Delete(SprintWatcher{}).Error; err != nil {
		return err
	}

	if err := tx.Where("workspace_id = ?", s.WorkspaceId).
		Where("sprint_id = ?", s.Id).Delete(SprintIssue{}).Error; err != nil {
		return err
	}

	return nil
}

func (s *Sprint) ToLightDTO() *dto.SprintLight {
	if s == nil {
		return nil
	}
	return &dto.SprintLight{
		Id:         s.Id,
		Name:       s.Name,
		SequenceId: s.SequenceId,
		//Url:        types.JsonURL{},
		//ShortUrl:   types.JsonURL{},
		StartDate: utils.SqlNullTimeToPointerTime(s.StartDate),
		EndDate:   utils.SqlNullTimeToPointerTime(s.EndDate),
		Stats:     s.Stats,
	}
}

func (s *Sprint) ToDTO() *dto.Sprint {
	if s == nil {
		return nil
	}
	return &dto.Sprint{
		SprintLight: *s.ToLightDTO(),
		Description: s.Description,
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

	SprintId    uuid.UUID `gorm:"type:uuid;not null;index:idx_sprint_issue,priority:1"`
	IssueId     uuid.UUID `gorm:"type:text;index:idx_sprint_issue,priority:2"`
	ProjectId   uuid.UUID `gorm:"type:uuid;index:,type:hash"`
	WorkspaceId uuid.UUID `gorm:"type:uuid;index:,type:hash"`
	CreatedById uuid.UUID `gorm:"type:uuid"`

	Position int `json:"position" gorm:"default:0"`

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
