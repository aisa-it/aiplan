// Содержит структуры данных для представления ответа поиска в упрощенном формате (lightweight). Используется для передачи данных, необходимых для отображения и базовой функциональности поиска, избегая избыточной информации из других сервисов.
//
// Основные возможности:
//   - Представление структуры данных ответа поиска с использованием упрощенных типов.
//   - Поддержка nullable полей через `*string` и `*types.TargetDate`.
//   - Использование `uuid.UUID` для идентификаторов.
//   - Интеграция с типами данных из пакета `github.com/aisa-it/aiplan/internal/aiplan/types`.
package dto

import (
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/gofrs/uuid"
)

type SearchLightweightResponse struct {
	ID uuid.UUID `json:"id"`

	WorkspaceId string          `json:"workspace"`
	Workspace   *WorkspaceLight `json:"workspace_detail"`

	ProjectId  string        `json:"project"`
	Project    *ProjectLight `json:"project_detail"`
	SequenceId int           `json:"sequence_id"`

	Name     string  `json:"name"`
	Priority *string `json:"priority" extensions:"x-nullable"`

	StartDate   *types.TargetDate      `json:"start_date" extensions:"x-nullable" swaggertype:"string"`
	TargetDate  *types.TargetDateTimeZ `json:"target_date" extensions:"x-nullable" swaggertype:"string"`
	CompletedAt *types.TargetDate      `json:"completed_at" extensions:"x-nullable" swaggertype:"string"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Author    *UserLight   `json:"author_detail" extensions:"x-nullable"`
	State     *StateLight  `json:"state_detail" extensions:"x-nullable"`
	Assignees []UserLight  `json:"assignee_details,omitempty"`
	Watchers  []UserLight  `json:"watcher_details,omitempty"`
	Labels    []LabelLight `json:"label_details" extensions:"x-nullable"`

	NameHighlighted string `json:"name_highlighted,omitempty"`
	DescHighlighted string `json:"desc_highlighted,omitempty"`
}
