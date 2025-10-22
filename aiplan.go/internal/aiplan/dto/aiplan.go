// Содержит структуры данных (DTO) для представления различных сущностей и данных, используемых в приложении.  Предназначен для обеспечения структурированного обмена данными между компонентами приложения и внешними системами.
//
// Основные возможности:
//   - Представление сущностей активности (EntityActivityLight, EntityActivityFull).
//   - Работа с Release Notes (ReleaseNoteLight).
//   - Описание файлов (FileAsset).
//   - Определение состояний (StateLight).
//   - Определение тарифных планов (Tariffication).
//   - Хранение истории изменений (HistoryBodyLight, HistoryBody).
//   - Работа с вложениями (Attachment).
package dto

import (
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
)

type EntityActivityLight struct {
	Id        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	Verb      string    `json:"verb"`
	Field     *string   `json:"field,omitempty"  extensions:"x-nullable"`

	OldValue *string `json:"old_value,omitempty"  extensions:"x-nullable"`
	NewValue string  `json:"new_value"`

	EntityType string `json:"entity_type"`

	NewEntity any `json:"new_entity_detail,omitempty" extensions:"x-nullable"`
	OldEntity any `json:"old_entity_detail,omitempty" extensions:"x-nullable"`

	//TargetUser *UserLight `json:"target_user,omitempty"  extensions:"x-nullable"`

	EntityUrl *string `json:"entity_url,omitempty"`
}

type EntityActivityFull struct {
	EntityActivityLight

	Workspace *WorkspaceLight `json:"workspace_detail,omitempty"  extensions:"x-nullable"`
	Actor     *UserLight      `json:"actor_detail,omitempty"  extensions:"x-nullable"`
	Issue     *IssueLight     `json:"issue_detail,omitempty" extensions:"x-nullable"`
	Project   *ProjectLight   `json:"project_detail,omitempty" extensions:"x-nullable"`
	Form      *FormLight      `json:"form_detail,omitempty"  extensions:"x-nullable"`
	Doc       *DocLight       `json:"doc_detail,omitempty" extensions:"x-nullable"`
	Sprint    *SprintLight    `json:"sprint_detail,omitempty" extensions:"x-nullable"`

	NewIdentifier *string `json:"new_identifier,omitempty" extensions:"x-nullable"`
	OldIdentifier *string `json:"old_identifier,omitempty" extensions:"x-nullable"`

	StateLag int `json:"state_lag_ms,omitempty"`
}

type ReleaseNoteLight struct {
	ID          string             ` json:"id"`
	TagName     string             `json:"tag_name" `
	PublishedAt time.Time          `json:"published_at"`
	Body        types.RedactorHTML `json:"body" swaggertype:"string"`
}

type FileAsset struct {
	Id          string `json:"id"`
	Name        string `json:"name"`
	FileSize    int    `json:"size"`
	ContentType string `json:"content_type"`
}

type StateLight struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Color       string `json:"color"`
	ProjectId   string `json:"project"`
	WorkspaceId string `json:"workspace"`
	Sequence    uint64 `json:"sequence"`
	Group       string `json:"group"`
	Default     bool   `json:"default"`
}

type HistoryBodyLight struct {
	Id       string    `json:"Id"`
	CratedAt time.Time `json:"crated_at"`
	Author   *UserLight
}

func (hbl *HistoryBodyLight) ToFullHistory(oldBody, currentBody *string, oldAttach, currentAttach []FileAsset) *HistoryBody {
	if hbl == nil {
		return nil
	}
	return &HistoryBody{
		HistoryBodyLight:        *hbl,
		OldBody:                 oldBody,
		CurrentBody:             currentBody,
		OldInlineAttachment:     oldAttach,
		CurrentInlineAttachment: currentAttach,
	}
}

type HistoryBody struct {
	HistoryBodyLight
	OldBody     *string `json:"old_body"`
	CurrentBody *string `json:"current_body"`

	OldInlineAttachment     []FileAsset `json:"old_inline_attachment"`
	CurrentInlineAttachment []FileAsset `json:"current_inline_attachment"`
}

type Attachment struct {
	Id        string     `json:"id"`
	CreatedAt time.Time  `json:"created_at"`
	Asset     *FileAsset `json:"asset"`
}

type WorkspaceLimitsInfo struct {
	TariffName        string `json:"tariff_name"`
	ProjectsRemains   int    `json:"projects_remains,omitempty"`
	ProjcetsMax       int    `json:"projects_max,omitempty"`
	InvitesRemains    int    `json:"invites_remains,omitempty"`
	InvitesMax        int    `json:"invites_max,omitempty"`
	AttachmentsRemain int    `json:"attachments_remains,omitempty"`
	AttachmentsMax    int    `json:"attachments_max,omitempty"`
}
