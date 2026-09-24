package dto

import (
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/types"
	"github.com/gofrs/uuid"
)

type PersonalPage struct {
	ID uuid.UUID `json:"id"`

	CreatedAt time.Time  `json:"created_at"`
	OwnerId   uuid.UUID  `json:"created_by"`
	Owner     *UserLight `json:"owner_detail,omitempty"`

	UpdatedAt   time.Time     `json:"updated_at"`
	UpdatedById uuid.NullUUID `json:"updated_by"`
	UpdatedBy   *UserLight    `json:"updated_by_detail"`

	Title   string             `json:"title" validate:"required,max=150"`
	Content types.RedactorHTML `json:"description"`

	InlineAttachments []FileAsset `json:"doc_inline_attachments"`

	Editors []UserLight `json:"editor_details,omitempty"`
	Readers []UserLight `json:"reader_details,omitempty"`

	EditorsIDs []uuid.UUID `json:"editors"`
	ReaderIDs  []uuid.UUID `json:"readers"`
}
