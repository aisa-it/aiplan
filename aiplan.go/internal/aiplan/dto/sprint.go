package dto

import (
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/gofrs/uuid"
)

type SprintLight struct {
	Id          uuid.UUID          `json:"id"`
	Name        string             `json:"name"`
	SequenceId  int                `json:"sequence_id"`
	Description types.RedactorHTML `json:"description"`

	Url      types.JsonURL `json:"url,omitempty"`
	ShortUrl types.JsonURL `json:"short_url,omitempty"`

	StartDate *time.Time `json:"start_date,omitempty"`
	EndDate   *time.Time `json:"end_date,omitempty"`

	SprintFolder *SprintFolder `json:"sprint_folder,omitempty"`

	Stats *types.SprintStats `json:"stats,omitempty"`
}

type Sprint struct {
	SprintLight

	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`

	CreatedBy *UserLight      `json:"created_by"`
	UpdatedBy *UserLight      `json:"updated_by,omitempty" extensions:"x-nullable"`
	Workspace *WorkspaceLight `json:"workspace,omitempty"`
	Issues    []IssueLight    `json:"issues,omitempty"`
	Watchers  []UserLight     `json:"watchers,omitempty"`
	View      types.ViewProps `json:"view_props,omitempty"`
}

type SprintFolder struct {
	Id      uuid.UUID     `json:"id"`
	Name    string        `json:"name"`
	Sprints []SprintLight `json:"sprints,omitempty"`
}

type RequestSprint struct {
	Name        string             `json:"name,omitempty"`
	Description types.RedactorHTML `json:"description,omitempty" swaggertype:"string"`
	StartDate   *types.TargetDate  `json:"start_date,omitempty" extensions:"x-nullable" swaggertype:"string"`
	EndDate     *types.TargetDate  `json:"end_date,omitempty" extensions:"x-nullable" swaggertype:"string"`
	Folder      uuid.NullUUID      `json:"sprint_folder_id,omitempty" swaggertype:"string"`
}

type RequestSprintFolder struct {
	Name string `json:"name"`
}

type RequestIssueIdList struct {
	IssuesAdd    []string `json:"issues_add,omitempty"`
	IssuesRemove []string `json:"issues_remove,omitempty"`
}

type RequestUserIdList struct {
	MembersAdd    []string `json:"members_add,omitempty"`
	MembersRemove []string `json:"members_remove,omitempty"`
}
