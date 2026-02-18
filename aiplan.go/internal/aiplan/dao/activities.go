package dao

import (
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/gofrs/uuid"
)

type ActivityEvent struct {
	ID uuid.UUID `gorm:"column:id;primaryKey;type:uuid"`

	CreatedAt time.Time `gorm:"column:created_at;not null;index:,type:brin"`

	ActorID  uuid.UUID `gorm:"column:actor_id;type:uuid;not null;index:idx_activity_actor_entity_created,priority:1"`
	Notified bool      `gorm:"column:notified;default:false;index:idx_activity_notified,priority:1,where:notified=false"`

	Verb          string
	Field         actField.ActivityField
	OldValue      *string
	NewValue      string
	NewIdentifier uuid.NullUUID `gorm:"type:uuid"`
	OldIdentifier uuid.NullUUID `gorm:"type:uuid"`
	SenderTg      int64         `gorm:"-" json:"-"`

	EntityType types.EntityLayer `gorm:"column:entity_type;type:smallint;index:idx_activity_workspace,priority:2;index:idx_activity_project,priority:2;index:idx_activity_issue,priority:2;index:idx_activity_doc,priority:2;index:idx_activity_form,priority:2;index:idx_activity_sprint,priority:2"`

	WorkspaceID uuid.NullUUID `gorm:"column:workspace_id;type:uuid;index:idx_activity_workspace,priority:1,where:workspace_id IS NOT NULL"`
	ProjectID   uuid.NullUUID `gorm:"column:project_id;type:uuid;index:idx_activity_project,priority:1,where:project_id IS NOT NULL"`
	IssueID     uuid.NullUUID `gorm:"column:issue_id;type:uuid;index:idx_activity_issue,priority:1,where:issue_id IS NOT NULL"`
	DocID       uuid.NullUUID `gorm:"column:doc_id;type:uuid;index:idx_activity_doc,priority:1,where:doc_id IS NOT NULL"`
	FormID      uuid.NullUUID `gorm:"column:form_id;type:uuid;index:idx_activity_form,priority:1,where:form_id IS NOT NULL"`
	SprintID    uuid.NullUUID `gorm:"column:sprint_id;type:uuid;index:idx_activity_sprint,priority:1,where:sprint_id IS NOT NULL"`

	Workspace *Workspace `gorm:"foreignKey:WorkspaceID"`
	Actor     *User      `gorm:"foreignKey:ActorID;references:ID"`
	Issue     *Issue     `gorm:"foreignKey:IssueID"`
	Project   *Project   `gorm:"foreignKey:ProjectID"`
	Form      *Form      `gorm:"foreignKey:FormID"`
	Doc       *Doc       `gorm:"foreignKey:DocID"`
	Sprint    *Sprint    `gorm:"foreignKey:SprintID"`

	IssueActivityExtendFields
	ProjectActivityExtendFields
	DocActivityExtendFields
	WorkspaceActivityExtendFields
	RootActivityExtendFields
	SprintActivityExtendFields
}

func (ActivityEvent) TableName() string {
	return "activity_events"
}
