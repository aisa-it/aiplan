// Содержит структуры данных (DTO) для представления объектов проекта в системе. Используется для обмена данными между слоями приложения и API.
//
// Основные возможности:
//   - Представление информации о проекте, включая общие сведения, участников, настройки и историю.
//   - Определение структуры данных для связанных объектов, таких как участники проекта, оценки, логи правил и шаблоны задач.
//   - Поддержка nullable полей с использованием `extensions` для упрощения сериализации и десериализации данных.
package dto

import (
	"time"

	"github.com/gofrs/uuid"
	"sheff.online/aiplan/internal/aiplan/types"
)

type ProjectLight struct {
	ID string `json:"id"`

	Name          string        `json:"name"`
	Public        bool          `json:"public"`
	Identifier    string        `json:"identifier"`
	ProjectLeadId string        `json:"project_lead"`
	WorkspaceId   string        `json:"workspace"`
	Emoji         int32         `json:"emoji,string"`
	CoverImage    *string       `json:"cover_image" extensions:"x-nullable"`
	Url           types.JsonURL `json:"url,omitempty"`
	IsFavorite    bool          `json:"is_favorite"`

	CurrentUserMembership *ProjectMemberLight `json:"current_user_membership,omitempty"  extensions:"x-nullable"`

	DefaultAssignees []string `json:"default_assignees"`
	DefaultWatchers  []string `json:"default_watchers"`

	DefaultAssigneesDetails []ProjectMemberLight `json:"default_assignees_details"`
	DefaultWatchersDetails  []ProjectMemberLight `json:"default_watchers_details"`

	TotalMembers    int    `json:"total_members,omitempty"`
	NameHighlighted string `json:"name_highlighted,omitempty"`
}

type Project struct {
	ProjectLight
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	ProjectLead *UserLight `json:"project_lead_detail" extensions:"x-nullable"`

	RulesScript *string `json:"rules_script,omitempty" extensions:"x-nullable"`

	Workspace *WorkspaceLight `json:"workspace_detail,omitempty" extensions:"x-nullable"`
}

type ProjectMemberLight struct {
	ID   string `json:"id"`
	Role int    `json:"role"`

	WorkspaceAdmin    bool `json:"workspace_admin"`
	IsDefaultAssignee bool `json:"is_default_assignee"`
	IsDefaultWatcher  bool `json:"is_default_watcher"`

	MemberId string     `json:"member_id"`
	Member   *UserLight `json:"member,omitempty" extensions:"x-nullable"`

	ProjectId string        `json:"project_id"`
	Project   *ProjectLight `json:"project,omitempty" extensions:"x-nullable"`
}

type ProjectMember struct {
	ProjectMemberLight

	ViewProps                       types.ViewProps       `json:"view_props"`
	NotificationSettingsApp         types.ProjectMemberNS `json:"notification_settings_app"`
	NotificationAuthorSettingsApp   types.ProjectMemberNS `json:"notification_author_settings_app"`
	NotificationSettingsTG          types.ProjectMemberNS `json:"notification_settings_tg" `
	NotificationAuthorSettingsTG    types.ProjectMemberNS `json:"notification_author_settings_tg" `
	NotificationSettingsEmail       types.ProjectMemberNS `json:"notification_settings_email" `
	NotificationAuthorSettingsEmail types.ProjectMemberNS `json:"notification_author_settings_email" `
}

type ProjectFavorites struct {
	ID        string        `json:"id"`
	ProjectId string        `json:"project_id"`
	Project   *ProjectLight `json:"project_detail,omitempty" extensions:"x-nullable"`
}

type Estimate struct {
	Id          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`

	ProjectId string          `json:"project_id"`
	Project   *ProjectLight   `json:"project_detail" extensions:"x-nullable"`
	Points    []EstimatePoint `json:"points"`
}

type EstimatePoint struct {
	Id          string `json:"id"`
	Key         int    `json:"key"`
	Description string `json:"description"`
	Value       string `json:"value"`

	EstimateId string    `json:"estimate"`
	Estimate   *Estimate `json:"estimate_detail" extensions:"x-nullable"`

	ProjectId string        `json:"project"`
	Project   *ProjectLight `json:"project_detail" extensions:"x-nullable"`
}

type RulesLog struct {
	Id        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`

	Project   *ProjectLight   `json:"project_detail"`
	Workspace *WorkspaceLight `json:"workspace_detail"`
	Issue     *IssueLight     `json:"issue_detail"`
	User      *UserLight      `json:"user_detail"`

	Time         time.Time `json:"time"`
	FunctionName *string   `json:"function_name,omitempty" extensions:"x-nullable"`
	Type         string    `json:"type"`
	Msg          string    `json:"msg"`
	LuaErr       *string   `json:"lua_err,omitempty" extensions:"x-nullable"`
}

type IssueTemplate struct {
	Id          uuid.UUID `json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	CreatedById uuid.UUID `json:"created_by_id"`
	UpdatedAt   time.Time `json:"updated_at"`
	UpdatedById uuid.UUID `json:"updated_by_id"`

	WorkspaceId uuid.UUID `json:"workspace_id"`
	ProjectId   uuid.UUID `json:"project_id"`

	Name     string             `json:"name"`
	Template types.RedactorHTML `json:"template" swaggertype:"string"`
}
