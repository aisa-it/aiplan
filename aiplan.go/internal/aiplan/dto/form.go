// Содержит структуры данных (DTO) для представления форм и ответов форм в приложении.  Используется для сериализации/десериализации данных и передачи между слоями приложения.
//
// Основные возможности:
//   - Представление структуры формы, включая поля, описание, авторизацию и дату окончания.
//   - Представление структуры ответа формы, включая поля, автора, форму, ответственного и вложения.
//   - Представление информации об вложениях к форме.
package dto

import (
	"time"

	"sheff.online/aiplan/internal/aiplan/types"
)

type FormLight struct {
	ID              string                `json:"id"`
	Slug            string                `json:"slug"`
	Title           string                `json:"title" validate:"required"`
	Description     types.RedactorHTML    `json:"description" swaggertype:"string"`
	AuthRequire     bool                  `json:"auth_require"`
	EndDate         *types.TargetDate     `json:"end_date" extensions:"x-nullable" swaggertype:"string"`
	TargetProjectId *string               `json:"target_project_id,omitempty"  extensions:"x-nullable"`
	WorkspaceId     string                `json:"workspace" `
	Fields          types.FormFieldsSlice `json:"fields"`
	Active          bool                  `json:"active"`
	Url             types.JsonURL         `json:"url,omitempty"`
}

type Form struct {
	FormLight
	Author        *UserLight      `json:"author_detail" extensions:"x-nullable"`
	TargetProject *ProjectLight   `json:"target_project_detail,omitempty" extensions:"x-nullable"`
	Workspace     *WorkspaceLight `json:"workspace_detail" extensions:"x-nullable"`
}

type FormAnswer struct {
	ID        string    `json:"id"`
	SeqId     int       `json:"seq_id"`
	CreatedAt time.Time `json:"created_at"`

	Responder *UserLight `json:"responder" extensions:"x-nullable"`
	Form      *Form      `json:"form" extensions:"x-nullable"`

	Fields types.FormFieldsSlice `json:"fields"`

	Attachment *Attachment `json:"attachment,omitempty" extensions:"x-nullable"`
}

type FormAttachmentLight struct {
	Id        string     `json:"id"`
	CreatedAt time.Time  `json:"created_at"`
	Asset     *FileAsset `json:"asset"`
}
