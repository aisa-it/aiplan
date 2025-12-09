// Определяет структуры данных для работы с рабочими пространствами (Workspaces). Сюда входят основные сведения о workspace, его владельце, членах, настройках и статистике.  Используется для сериализации/десериализации данных в формате JSON и для представления данных в приложении.
//
// Основные возможности:
//   - Представление информации о workspace, включая его свойства, владельца, членов и описание.
//   - Обработка взаимосвязей между workspace и его членами.
//   - Поддержка информации о избранных workspace и их статистике (количество членов, проектов).
package dto

import (
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/gofrs/uuid"
)

type WorkspaceLight struct {
	ID      uuid.UUID     `json:"id"`
	Name    string        `json:"name"`
	LogoId  uuid.NullUUID `json:"logo"  extensions:"x-nullable" swaggertype:"string"`
	Slug    string        `json:"slug"`
	OwnerId string        `json:"owner_id"`
	Url     types.JsonURL `json:"url,omitempty"`
}

type Workspace struct {
	WorkspaceLight

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Owner *UserLight `json:"owner,omitempty" extensions:"x-nullable"`

	Description           types.RedactorHTML `json:"description"`
	CurrentUserMembership *WorkspaceMember   `json:"current_user_membership,omitempty" extensions:"x-nullable"`
	IsFavorite            bool               `json:"is_favorite"`
}

type WorkspaceWithCount struct {
	Workspace
	TotalMembers  int  `json:"total_members"`
	TotalProjects int  `json:"total_projects"`
	IsFavorite    bool `json:"is_favorite" `

	NameHighlighted string `json:"name_highlighted,omitempty"`
}

type WorkspaceMemberLight struct {
	ID              string     `json:"id"`
	Role            int        `json:"role"`
	EditableByAdmin bool       `json:"editable_by_admin"`
	MemberId        uuid.UUID  `json:"member_id"`
	Member          *UserLight `json:"member"`
}

type WorkspaceMember struct {
	WorkspaceMemberLight
	NotificationSettingsApp         types.WorkspaceMemberNS `json:"notification_settings_app"`
	NotificationAuthorSettingsApp   types.WorkspaceMemberNS `json:"notification_author_settings_app"`
	NotificationSettingsTG          types.WorkspaceMemberNS `json:"notification_settings_tg" `
	NotificationAuthorSettingsTG    types.WorkspaceMemberNS `json:"notification_author_settings_tg" `
	NotificationSettingsEmail       types.WorkspaceMemberNS `json:"notification_settings_email" `
	NotificationAuthorSettingsEmail types.WorkspaceMemberNS `json:"notification_author_settings_email" `
}

type WorkspaceFavorites struct {
	ID          uuid.UUID  `json:"id"`
	WorkspaceId uuid.UUID  `json:"workspace_id"`
	Workspace   *Workspace `json:"workspace_detail,omitempty" extensions:"x-nullable"`
}
