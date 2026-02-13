// Содержит структуры данных (DTO) для представления информации об issue (задачах) в системе.
// Используется для сериализации/десериализации данных, передачи между слоями приложения и хранения в базе данных.
//
// Основные возможности:
//   - Представление комментариев к issue.
//   - Описание issue с детальной информацией, включая связанные данные (ссылки, метки, файлы).
//   - Поддержка различных типов данных, таких как даты, URL, и пользовательские объекты.
//   - Представление результатов поиска issue с возможностью пагинации.
package dto

import (
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/editor"
	_ "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/editor/tiptap" // Регистрация TipTap парсера и сериализатора
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/gofrs/uuid"
)

type LabelLight struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	ProjectId   uuid.UUID `json:"project"`
	Color       string    `json:"color" `
}
type IssueLight struct {
	Id         uuid.UUID     `json:"id"`
	Name       string        `json:"name"`
	SequenceId int           `json:"sequence_id"`
	Url        types.JsonURL `json:"url,omitempty"`
	ShortUrl   types.JsonURL `json:"short_url,omitempty"`

	StateId  uuid.UUID   `json:"state"`
	State    *StateLight `json:"state_detail" extensions:"x-nullable"`
	Priority *string     `json:"priority" extensions:"x-nullable"`
}

type IssueLinkLight struct {
	Id    uuid.UUID `json:"id"`
	Title string    `json:"title"`
	Url   string    `json:"url"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Issue struct {
	IssueLight

	SequenceId int `json:"sequence_id"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Priority *string `json:"priority" extensions:"x-nullable"`

	StartDate   *types.TargetDateTimeZ `json:"start_date" extensions:"x-nullable"`
	TargetDate  *types.TargetDateTimeZ `json:"target_date" extensions:"x-nullable"`
	CompletedAt *types.TargetDateTimeZ `json:"completed_at" extensions:"x-nullable"`

	ProjectId   uuid.UUID `json:"project"`
	WorkspaceId uuid.UUID `json:"workspace"`

	ParentId    uuid.NullUUID `json:"parent,omitempty"`
	UpdatedById uuid.NullUUID `json:"updated_by" extensions:"x-nullable"`

	DescriptionHtml     string          `json:"description_html"`
	DescriptionStripped *string         `json:"description_stripped" extensions:"x-nullable"`
	DescriptionType     int             `json:"description_type"`
	DescriptionJSON     editor.Document `json:"description_json"`
	EstimatePoint       int             `json:"estimate_point"`
	Draft               bool            `json:"draft"`
	Pinned              bool            `json:"pinned"`
	LLMContent          bool            `json:"llm_content"`

	Parent    *IssueLight      `json:"parent_detail"  extensions:"x-nullable"`
	Workspace *WorkspaceLight  `json:"workspace_detail"  extensions:"x-nullable"`
	Project   *ProjectLight    `json:"project_detail"  extensions:"x-nullable"`
	Assignees []UserLight      `json:"assignee_details" extensions:"x-nullable"`
	Watchers  []UserLight      `json:"watcher_details" extensions:"x-nullable"`
	Labels    []LabelLight     `json:"label_details" extensions:"x-nullable"`
	Links     []IssueLinkLight `json:"issue_link"  extensions:"x-nullable"`
	Author    *UserLight       `json:"author_detail"  extensions:"x-nullable"`

	InlineAttachments []FileAsset `json:"issue_inline_attachments"`

	BlockerIssuesIDs []IssueBlockerLight `json:"blocker_issues,omitempty"`
	BlockedIssuesIDs []IssueBlockerLight `json:"blocked_issues,omitempty" `
	Sprints          []SprintLight       `json:"sprints"`
}

type IssueCommentLight struct {
	Id              uuid.UUID     `json:"id"`
	CommentStripped string        `json:"comment_stripped"`
	CommentHtml     string        `json:"comment_html"`
	URL             types.JsonURL `json:"url"`
}
type IssueComment struct {
	IssueCommentLight

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	UpdatedById uuid.NullUUID `json:"updated_by_id,omitempty"`

	ActorId uuid.NullUUID `json:"actor_id,omitempty" extensions:"x-nullable"`

	ProjectId   uuid.UUID `json:"project_id"`
	WorkspaceId uuid.UUID `json:"workspace_id"`
	IssueId     uuid.UUID `json:"issue_id"`

	ReplyToCommentId uuid.NullUUID `json:"reply_to_comment_id" extensions:"x-nullable"`
	OriginalComment  *IssueComment `json:"original_comment,omitempty" extensions:"x-nullable"`

	Actor *UserLight `json:"actor_detail" extensions:"x-nullable"`

	Attachments []*FileAsset `json:"comment_attachments"`

	Reactions       []*CommentReaction `json:"reactions"`
	ReactionSummary map[string]int     `json:"reaction_summary,omitempty"`
}

type IssueWithCount struct {
	Issue
	SubIssuesCount    int `json:"sub_issues_count"`
	LinkCount         int `json:"link_count"`
	AttachmentCount   int `json:"attachment_count"`
	LinkedIssuesCount int `json:"linked_issues_count"`
	CommentsCount     int `json:"comments_count"`

	NameHighlighted string `json:"name_highlighted,omitempty"`
	DescHighlighted string `json:"desc_highlighted,omitempty"`
}

type IssueBlockerLight struct {
	Id          uuid.UUID   `json:"id"`
	BlockId     uuid.UUID   `json:"block" `
	BlockedById uuid.UUID   `json:"blocked_by" `
	Block       *IssueLight `json:"blocked_issue_detail" extensions:"x-nullable"`
	BlockedBy   *IssueLight `json:"blocker_issue_detail"  extensions:"x-nullable"`
}

type IssueSearchResult struct {
	Count  int              `json:"count"`
	Offset int              `json:"offset"`
	Limit  int              `json:"limit"`
	Issues []IssueWithCount `json:"issues"`
}

type ResponseSubIssueList struct {
	SubIssues         []Issue        `json:"sub_issues"`
	StateDistribution map[string]int `json:"state_distribution"`
}

type IssueLockResponse struct {
	Locked      bool       `json:"ok"`
	LockedBy    *UserLight `json:"locked_by,omitempty"`
	LockedUntil time.Time  `json:"locked_until"`
}

type NewIssueID struct {
	Id uuid.UUID `json:"id"`
}

// IssueProperty - значение кастомного поля для задачи
type IssueProperty struct {
	Id          uuid.UUID `json:"id,omitempty"`
	IssueId     uuid.UUID `json:"issue_id"`
	TemplateId  uuid.UUID `json:"template_id"`
	ProjectId   uuid.UUID `json:"project_id"`
	WorkspaceId uuid.UUID `json:"workspace_id"`
	Name        string    `json:"name"`
	Type        string    `json:"type"`
	Options     []string  `json:"options,omitempty"`
	Value       any       `json:"value"`
}

// ProjectPropertyTemplate - шаблон кастомного поля проекта
type ProjectPropertyTemplate struct {
	Id          uuid.UUID `json:"id"`
	ProjectId   uuid.UUID `json:"project_id"`
	WorkspaceId uuid.UUID `json:"workspace_id"`
	Name        string    `json:"name"`
	Type        string    `json:"type"`
	Options     []string  `json:"options,omitempty"`
	OnlyAdmin   bool      `json:"only_admin"`
	SortOrder   int       `json:"sort_order"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// CreatePropertyTemplateRequest - запрос на создание шаблона поля
type CreatePropertyTemplateRequest struct {
	Name      string   `json:"name" validate:"required,min=1,max=255"`
	Type      string   `json:"type" validate:"required,oneof=string boolean select"`
	Options   []string `json:"options,omitempty"`
	OnlyAdmin bool     `json:"only_admin"`
	SortOrder int      `json:"sort_order"`
}

// UpdatePropertyTemplateRequest - запрос на обновление шаблона поля
type UpdatePropertyTemplateRequest struct {
	Name      *string   `json:"name,omitempty"`
	Type      *string   `json:"type,omitempty"`
	Options   *[]string `json:"options,omitempty"`
	OnlyAdmin *bool     `json:"only_admin,omitempty"`
	SortOrder *int      `json:"sort_order,omitempty"`
}

// SetIssuePropertyRequest - запрос на установку значения поля задачи
type SetIssuePropertyRequest struct {
	Value any `json:"value"`
}
