// Содержит структуры данных (DTO) для представления пользователей, уведомлений, отзывов и фильтров поиска в приложении.
// Используется для обмена данными между слоями приложения и обеспечения структурированного представления информации.
//
// Основные возможности:
//   - Представление информации о пользователях (с возможностью хранения nullable полей).
//   - Обработка уведомлений пользователей с детальной информацией.
//   - Сбор и хранение отзывов пользователей.
//   - Фильтрация данных для поиска по различным критериям.
package dto

import (
	"fmt"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/gofrs/uuid"
)

type UserLight struct {
	ID            uuid.UUID      `json:"id"`
	Username      *string        `json:"username,omitempty" extensions:"x-nullable"`
	Email         string         `json:"email"`
	FirstName     string         `json:"first_name"`
	LastName      string         `json:"last_name"`
	Avatar        string         `json:"avatar"`
	AvatarId      uuid.NullUUID  `json:"avatar_id" extensions:"x-nullable" swaggertype:"string"`
	UserTimezone  types.TimeZone `json:"user_timezone" swaggertype:"string"`
	LastActive    *time.Time     `json:"last_active" extensions:"x-nullable"`
	TelegramId    *int64         `json:"telegram_id,omitempty" extensions:"x-nullable"`
	StatusEmoji   *string        `json:"status_emoji" extensions:"x-nullable"`
	Status        *string        `json:"status" extensions:"x-nullable"`
	StatusEndDate *time.Time     `json:"status_end_date" extensions:"x-nullable"`
	CreatedAt     time.Time      `json:"created_at"`

	IsSuperuser   bool       `json:"is_superuser"`
	IsActive      bool       `json:"is_active"`
	BlockedUntil  *time.Time `json:"blocked_until"`
	IsOnboarded   bool       `json:"is_onboarded"`
	IsBot         bool       `json:"is_bot"`
	IsIntegration bool       `json:"is_integration"`
}

func (u *UserLight) GetName() string {
	if u.FirstName != "" && u.LastName != "" {
		return fmt.Sprintf("%s %s", u.FirstName, u.LastName)
	}
	return u.Email
}

type User struct {
	UserLight

	Theme     types.Theme        `json:"theme"`
	ViewProps types.ViewProps    `json:"view_props"`
	Settings  types.UserSettings `json:"settings"`
	Tutorial  int                `json:"tutorial"`

	LastWorkspaceId   *string `json:"last_workspace_id"  extensions:"x-nullable"`
	LastWorkspaceSlug *string `json:"last_workspace_slug"  extensions:"x-nullable"`
	NotificationCount int     `json:"notification_count,omitempty"`
	AttachmentsAllow  *bool   `json:"attachments_allow,omitempty"  extensions:"x-nullable"`
}

type UserNotificationsLight struct {
	ID     string `json:"id"`
	UserId string `json:"user_id"`
	Type   string `json:"type"`
	Viewed bool   `json:"viewed"`

	Title    string  `json:"title,omitempty"`
	Msg      string  `json:"msg,omitempty"`
	AuthorId *string `json:"author_id"  extensions:"x-nullable"`

	EntityActivityId *string `json:"entity_activity,omitempty"  extensions:"x-nullable"`
	CommentId        *string `json:"comment_id,omitempty"  extensions:"x-nullable"`
	WorkspaceId      *string `json:"workspace_id,omitempty"  extensions:"x-nullable"`
	IssueId          *string `json:"issue_id,omitempty"  extensions:"x-nullable"`
}

type UserNotificationsFull struct {
	UserNotificationsLight
	User       *UserLight         `json:"user_detail,omitempty"  extensions:"x-nullable"`
	Comment    *IssueCommentLight `json:"comment,omitempty"  extensions:"x-nullable"`
	Workspace  *WorkspaceLight    `json:"workspace,omitempty"  extensions:"x-nullable"`
	Issue      *IssueLight        `json:"issue,omitempty"  extensions:"x-nullable"`
	Author     *UserLight         `json:"author,omitempty"  extensions:"x-nullable"`
	TargetUser *UserLight         `json:"target_user,omitempty"  extensions:"x-nullable"`
}

type UserFeedback struct {
	UserID string `json:"user_id"`

	Stars    int    `json:"stars"`
	Feedback string `json:"feedback"`

	User UserLight `json:"user_detail"`
}

// searchFilter

type SearchFilterLight struct {
	ID          string                  `json:"id"`
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	Public      bool                    `json:"public"`
	Filter      types.IssuesListFilters `json:"filter"`
	Url         types.JsonURL           `json:"url,omitempty"`
	ShortUrl    types.JsonURL           `json:"short_url,omitempty"`
}

type SearchFilterFull struct {
	SearchFilterLight
	AuthorID string     `json:"author_id"`
	Author   *UserLight `json:"author_detail"  extensions:"x-nullable"`
}
