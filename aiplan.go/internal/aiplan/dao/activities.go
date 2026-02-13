package dao

import (
	"time"

	"github.com/gofrs/uuid"
	"github.com/lib/pq"
)

type ActivityEvent struct {
	Id uuid.UUID `gorm:"column:id;primaryKey;type:uuid" json:"id"`

	CreatedAt time.Time `json:"created_at" gorm:"index:workspace_activities_workspace_index,sort:desc,type:btree,priority:2;index:workspace_activities_actor_index,sort:desc,type:btree,priority:2;index:workspace_activities_mail_index,type:btree,where:notified = false"`

	Verb string `json:"verb"`
	// field character varying IS_NULL:YES
	Field *string `json:"field,omitempty" extensions:"x-nullable"`
	// old_value text IS_NULL:YES
	OldValue *string `json:"old_value" extensions:"x-nullable"`
	// new_value text IS_NULL:YES
	NewValue string `json:"new_value" `
	// comment text IS_NULL:NO
	Comment string `json:"comment"`

	ActorId uuid.UUID `json:"actor,omitempty" gorm:"type:uuid;index:workspace_activities_actor_index,priority:1" extensions:"x-nullable"`

	//CreatedById uuid.UUID
	WorkspaceId uuid.NullUUID `json:"workspace" gorm:"type:uuid;index:workspace_activities_workspace_index,priority:1" extensions:"x-nullable"`
	ProjectId   uuid.NullUUID `json:"project_id" gorm:"type:uuid;index:project_activities_project_index,priority:1" extensions:"x-nullable"`
	IssueId     uuid.NullUUID `json:"issue_id" gorm:"type:uuid;index:issue_activities_issue_index,priority:1" extensions:"x-nullable"`
	FormId      uuid.NullUUID `json:"form_id,omitempty" gorm:"type:uuid;index:form_activities_form_index,priority:1" extensions:"x-nullable"`
	DocId       uuid.NullUUID `json:"doc" gorm:"type:uuid;index:doc_activities_doc_index,priority:1" extensions:"x-nullable"`
	SprintId    uuid.NullUUID `json:"sprint_id" gorm:"type:uuid;index:sprint_activities_sprint_index,priority:1" extensions:"x-nullable"`

	UpdatedById uuid.UUID
	// new_identifier uuid IS_NULL:YES
	NewIdentifier uuid.NullUUID `json:"new_identifier" gorm:"type:uuid" extensions:"x-nullable"`
	// old_identifier uuid IS_NULL:YES
	OldIdentifier uuid.NullUUID `json:"old_identifier" gorm:"type:uuid" extensions:"x-nullable"`
	Notified      bool          `json:"-" gorm:"default:false"`
	TelegramMsgId pq.Int64Array `json:"-" gorm:"column:telegram_msg_ids;index;type:integer[]"`

	Actor *User `gorm:"foreignKey:ActorId;references:ID"`

	Workspace *Workspace `gorm:"foreignKey:WorkspaceId" `
	Issue     *Issue     `gorm:"foreignKey:IssueId"`
	Project   *Project   `gorm:"foreignKey:ProjectId"`
	Form      *Form      `gorm:"foreignKey:FormId"`
	Doc       *Doc       `gorm:"foreignKey:DocId"`
	Sprint    *Sprint    `gorm:"foreignKey:SprintId"`

	EntityType string

	UnionCustomFields string `json:"-" gorm:"-"`
	SenderTg          int64  `json:"-" gorm:"-"`

	IssueActivityExtendFields
	ProjectActivityExtendFields
	DocActivityExtendFields
	WorkspaceActivityExtendFields
	RootActivityExtendFields
	SprintActivityExtendFields
	//ActivitySender
}
