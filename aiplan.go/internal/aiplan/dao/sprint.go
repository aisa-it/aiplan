package dao

import (
	"database/sql"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
	"sheff.online/aiplan/internal/aiplan/types"
	"time"
)

type Sprint struct {
	Id        uuid.UUID      `gorm:"primaryKey;type:uuid"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`

	CreatedById uuid.UUID     `json:"created_by"`
	UpdatedById uuid.NullUUID `json:"updated_by" extensions:"x-nullable"`
	WorkspaceId uuid.UUID     //`gorm:"index:issue_template,priority:1"`

	CreatedBy User
	UpdatedBy *User
	Workspace *Workspace

	Name        string             `json:"name"`
	NameTokens  types.TsVector     `gorm:"index:sprint_name_tokens,type:gin"` //TODO <<<
	SequenceId  int                `json:"sequence_id" gorm:"default:1;index:,where:deleted_at is not null"`
	Description types.RedactorHTML `json:"description"`

	StartDate sql.NullTime `gorm:"index"`
	EndDate   sql.NullTime `gorm:"index"`

	LabelName  string `gorm:"-"`
	LabelColor string `gorm:"default:#000000"`

	Issues []Issue `gorm:"many2many:sprint_issues"`
}

type SprintIssue struct {
	Id        uuid.UUID `gorm:"primaryKey;type:uuid"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	SprintId uuid.UUID `gorm:"type:uuid;not null;index:idx_sprint_issue,priority:1"`
	IssueId  uuid.UUID `gorm:"type:text;index:idx_sprint_issue,priority:2"`

	CreatedById uuid.UUID `json:"added_by"`

	Position int `json:"position" gorm:"default:0"`

	Sprint    Sprint `gorm:"foreignKey:SprintId;references:Id"`
	Issue     Issue  `gorm:"foreignKey:IssueId;references:ID"`
	CreatedBy User   `gorm:"foreignKey:CreatedById"`
}
