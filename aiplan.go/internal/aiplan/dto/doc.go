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

	"github.com/aisa-it/aiplan/internal/aiplan/types"
)

type Doc struct {
	DocLight

	Author    *UserLight `json:"author,omitempty"`
	UpdateBy  *UserLight `json:"update_by,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdateAt  time.Time  `json:"update_at,omitempty"`

	Content types.RedactorHTML `json:"content" swaggertype:"string"`

	ParentDoc *string `json:"parent_doc,omitempty"`

	InlineAttachments []FileAsset `json:"doc_inline_attachments"`

	Breadcrumbs []string `json:"breadcrumbs"`

	ReaderRole int `json:"reader_role"`
	EditorRole int `json:"editor_role"`

	ReaderIds []string    `json:"reader_ids"`
	Readers   []UserLight `json:"readers"`

	EditorIds []string    `json:"editor_ids"`
	Editors   []UserLight `json:"editors"`

	WatcherIds []string    `json:"watcher_ids"`
	Watchers   []UserLight `json:"watchers"`
}

type DocLight struct {
	Id           string `json:"id"`
	Title        string `json:"title"`
	HasChildDocs bool   `json:"has_child_docs"`
	Draft        *bool  `json:"draft,omitempty"`
	IsFavorite   bool   `json:"is_favorite"`

	Url      types.JsonURL `json:"url,omitempty"`
	ShortUrl types.JsonURL `json:"short_url,omitempty"`
}

type DocCommentLight struct {
	Id string `json:"id"`

	CommentStripped string             `json:"comment_stripped"`
	CommentHtml     types.RedactorHTML `json:"comment_html" swaggertype:"string"`
	URL             types.JsonURL      `json:"url"`
}

type DocComment struct {
	DocCommentLight

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	UpdatedById *string `json:"updated_by_id,omitempty"`

	CommentType     int         `json:"comment_type"`
	OriginalComment *DocComment `json:"original_comment,omitempty" extensions:"x-nullable"`

	Actor           *UserLight         `json:"actor_detail"`
	Attachments     []FileAsset        `json:"comment_attachments"`
	ReactionSummary map[string]int     `json:"reaction_summary,omitempty"`
	Reactions       []*CommentReaction `json:"reactions,omitempty"`
}

type CommentReaction struct {
	Id        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	CommentId string    `json:"comment_id"`
	UserId    string    `json:"user_id"`
	Reaction  string    `json:"reaction"`
}

type DocFavorites struct {
	Id    string    `json:"id"`
	DocId string    `json:"doc_id"`
	Doc   *DocLight `json:"doc"`
}
