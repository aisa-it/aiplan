package dto

import (
	"github.com/gofrs/uuid"
	"sheff.online/aiplan/internal/aiplan/types"
	"time"
)

type SprintLight struct {
	Id         uuid.UUID `json:"id"`
	Name       string    `json:"name"`
	SequenceId int       `json:"sequence_id"`

	Url      types.JsonURL `json:"url,omitempty"`
	ShortUrl types.JsonURL `json:"short_url,omitempty"`

	StartDate *time.Time `json:"start_date,omitempty"`
	EndDate   *time.Time `json:"end_date,omitempty"`

	Stats types.SprintStats `json:"stats"`
}

type Sprint struct {
	SprintLight

	Description types.RedactorHTML `json:"description"`

	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`

	CreatedBy *UserLight      `json:"created_by"`
	UpdatedBy *UserLight      `json:"updated_by,omitempty" extensions:"x-nullable"`
	Workspace *WorkspaceLight `json:"workspace,omitempty"`
	Issues    []IssueLight    `json:"issues,omitempty"`
	Watchers  []UserLight     `json:"watchers,omitempty"`
}
