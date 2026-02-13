package dto

import (
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/gofrs/uuid"
)

type CommentHistory struct {
	CommentHtml     types.RedactorHTML `json:"comment_html"`
	CommentStripped string             `json:"comment_stripped"`
	UpdatedById     uuid.UUID          `json:"updated_by_id,omitempty"`
	ActorUpdate     *UserLight         `json:"actor_update,omitempty"`
	CommentId       uuid.NullUUID      `json:"comment_id"`
	CreatedAt       time.Time          `json:"created_at"`
	Attachments     []FileAsset        `json:"comment_attachments,omitempty" `
}
