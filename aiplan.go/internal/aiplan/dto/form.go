// Содержит структуры данных (DTO) для представления форм и ответов форм в приложении.  Используется для сериализации/десериализации данных и передачи между слоями приложения.
//
// Основные возможности:
//   - Представление структуры формы, включая поля, описание, авторизацию и дату окончания.
//   - Представление структуры ответа формы, включая поля, автора, форму, ответственного и вложения.
//   - Представление информации об вложениях к форме.
package dto

import (
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/gofrs/uuid"
)

type FormLight struct {
	ID              uuid.UUID             `json:"id"`
	Slug            string                `json:"slug"`
	Title           string                `json:"title" validate:"required"`
	Description     types.RedactorHTML    `json:"description" swaggertype:"string"`
	AuthRequire     bool                  `json:"auth_require"`
	EndDate         *types.TargetDate     `json:"end_date" extensions:"x-nullable" swaggertype:"string"`
	TargetProjectId uuid.NullUUID         `json:"target_project_id,omitempty"  extensions:"x-nullable"`
	WorkspaceId     uuid.UUID             `json:"workspace" `
	Fields          types.FormFieldsSlice `json:"fields"`
	Active          bool                  `json:"active"`
	Url             types.JsonURL         `json:"url,omitempty"`
}

type Form struct {
	FormLight
	Author               *UserLight             `json:"author_detail" extensions:"x-nullable"`
	TargetProject        *ProjectLight          `json:"target_project_detail,omitempty" extensions:"x-nullable"`
	Workspace            *WorkspaceLight        `json:"workspace_detail" extensions:"x-nullable"`
	NotificationChannels types.FormAnswerNotify `json:"notification_channels" extensions:"x-nullable"`
}

type FormAnswer struct {
	ID        uuid.UUID `json:"id"`
	SeqId     int       `json:"seq_id"`
	CreatedAt time.Time `json:"created_at"`

	Responder *UserLight `json:"responder" extensions:"x-nullable"`
	Form      *Form      `json:"form" extensions:"x-nullable"`

	Fields types.FormFieldsSlice `json:"fields"`

	Attachment *Attachment `json:"attachment,omitempty" extensions:"x-nullable"`
}

type FormAttachmentLight struct {
	Id        uuid.UUID  `json:"id"`
	CreatedAt time.Time  `json:"created_at"`
	Asset     *FileAsset `json:"asset"`
}

//**REQUEST**

// RequestAnswer ответы на форму
type RequestAnswer struct {
	Val interface{} `json:"value,omitempty"`
}

//  **RESPONSE**

// ResponseAnswers ответы на поля формы
type ResponseAnswers struct {
	Form   FormLight             `json:"form"`
	Fields types.FormFieldsSlice `json:"fields"`
}
