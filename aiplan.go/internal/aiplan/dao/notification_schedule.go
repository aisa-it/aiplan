package dao

import (
	"encoding/json"
	"time"

	"github.com/gofrs/uuid"
)

type NotificationSchedule struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	CreatedAt time.Time `json:"created_at" gorm:"index:idx_ns_issue_type_created,priority:3,sort:desc"`
	UpdatedAt time.Time `json:"updated_at"`

	NotificationType string `gorm:"type:varchar(50);not null;index:idx_ns_poll_type,where:status = 'pending';index:idx_ns_poll_issue_pending,where:status = 'pending';index:idx_ns_issue_type_created,priority:2" json:"notification_type"`

	AuthorID    uuid.NullUUID `gorm:"type:uuid" json:"author_id"`
	Author      *User         `gorm:"foreignKey:AuthorID;references:ID;belongsTo" json:"author,omitempty" extensions:"x-nullable"`
	WorkspaceID uuid.NullUUID `gorm:"type:uuid;index;index:idx_ns_poll_workspace,where:status = 'pending'" json:"workspace_id"`
	Workspace   *Workspace    `gorm:"foreignKey:WorkspaceID" json:"workspace,omitempty" extensions:"x-nullable"`
	ProjectID   uuid.NullUUID `gorm:"type:uuid;index" json:"project_id"`
	Project     *Project      `gorm:"foreignKey:ProjectID" json:"project,omitempty" extensions:"x-nullable"`
	IssueID     uuid.NullUUID `gorm:"type:uuid;index;index:idx_ns_poll_issue_pending,where:status = 'pending';index:idx_ns_issue_type_created,priority:1" json:"issue_id"`
	Issue       *Issue        `gorm:"foreignKey:IssueID" json:"issue,omitempty" extensions:"x-nullable"`

	ScheduledAt time.Time `gorm:"type:timestamptz;not null;index:idx_ns_poll_scheduled,where:status = 'pending'" json:"scheduled_at"`

	SendWindowStart *time.Time `gorm:"type:timestamptz;index:idx_ns_poll_window,where:status = 'pending'" json:"send_window_start,omitempty"`

	Status      string          `gorm:"type:varchar(20);not null;default:pending" json:"status"`
	ProcessedAt *time.Time      `gorm:"type:timestamptz" json:"processed_at,omitempty"`
	Payload     json.RawMessage `gorm:"type:jsonb;not null;default:'{}'" json:"payload"`
}

// TableName sets the insert table name for this struct type
func (NotificationSchedule) TableName() string {
	return "notification_schedules"
}

type DeliveryStatus int

const (
	DeliveryNotAttempted DeliveryStatus = 0
	DeliverySuccess      DeliveryStatus = -1
)

const MaxDeliveryAttempts = 3
const MaxDeadlineAdvance = 72 * time.Hour

const (
	StatusPending   = "pending"
	StatusCompleted = "completed"
	StatusCancelled = "cancelled"
)

const (
	ChannelTG    = "tg"
	ChannelEmail = "email"
	ChannelApp   = "app"
)

const (
	NotificationTypeDeadline         = "deadline"
	NotificationTypeWorkspaceMessage = "workspace_message"
	NotificationTypeServiceMessage   = "service_message"
)

func (d DeliveryStatus) IsDelivered() bool {
	return d == DeliverySuccess
}

func (d DeliveryStatus) IsExhausted() bool {
	return int(d) >= MaxDeliveryAttempts
}
