// Содержит структуры данных (DTO) для работы с документами в системе.
// Используется для передачи данных между слоями приложения.
//
// Основные возможности:
//   - Представление структуры документа с метаданными и контентом.
//   - Описание структуры комментариев к документам, включая реакции пользователей.
//   - Определение структуры избранных документов пользователя.
package dto

import (
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/gofrs/uuid"
)

type Doc struct {
	DocLight

	Author    *UserLight `json:"author,omitempty"`
	UpdateBy  *UserLight `json:"update_by,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdateAt  time.Time  `json:"update_at"`

	Content    types.RedactorHTML `json:"content" swaggertype:"string"`
	LLMContent bool               `json:"llm_content"`

	ParentDoc uuid.NullUUID `json:"parent_doc,omitempty"`

	InlineAttachments []FileAsset `json:"doc_inline_attachments"`

	Breadcrumbs []uuid.UUID `json:"breadcrumbs"`

	ReaderRole int `json:"reader_role"`
	EditorRole int `json:"editor_role"`

	ReaderIds []uuid.UUID `json:"reader_ids"`
	Readers   []UserLight `json:"readers"`

	EditorIds []uuid.UUID `json:"editor_ids"`
	Editors   []UserLight `json:"editors"`

	WatcherIds []uuid.UUID `json:"watcher_ids"`
	Watchers   []UserLight `json:"watchers"`
}

type DocLight struct {
	Id           uuid.UUID `json:"id"`
	Title        string    `json:"title"`
	HasChildDocs bool      `json:"has_child_docs"`
	Draft        *bool     `json:"draft,omitempty"`
	IsFavorite   bool      `json:"is_favorite"`

	Url      types.JsonURL `json:"url,omitempty"`
	ShortUrl types.JsonURL `json:"short_url,omitempty"`
}

type DocCommentLight struct {
	Id uuid.UUID `json:"id"`

	CommentStripped string             `json:"comment_stripped"`
	CommentHtml     types.RedactorHTML `json:"comment_html" swaggertype:"string"`
	URL             types.JsonURL      `json:"url"`
}

type DocComment struct {
	DocCommentLight

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	UpdatedById uuid.NullUUID `json:"updated_by_id,omitempty"`

	CommentType     int         `json:"comment_type"`
	OriginalComment *DocComment `json:"original_comment,omitempty" extensions:"x-nullable"`

	Actor           *UserLight         `json:"actor_detail"`
	Attachments     []FileAsset        `json:"comment_attachments"`
	ReactionSummary map[string]int     `json:"reaction_summary,omitempty"`
	Reactions       []*CommentReaction `json:"reactions,omitempty"`
}

type CommentReaction struct {
	Id        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	CommentId uuid.UUID `json:"comment_id"`
	UserId    uuid.UUID `json:"user_id"`
	Reaction  string    `json:"reaction"`
}

type DocFavorites struct {
	Id    uuid.UUID `json:"id"`
	DocId uuid.UUID `json:"doc_id"`
	Doc   *DocLight `json:"doc"`
}
