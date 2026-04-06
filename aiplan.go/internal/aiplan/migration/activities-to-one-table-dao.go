package migration

import (
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/gofrs/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

// old activity tables

type DocActivity struct {
	Id        uuid.UUID `json:"id" gorm:"primaryKey;type:uuid"`
	CreatedAt time.Time `json:"created_at" gorm:"index:doc_activities_doc_index,sort:desc,type:btree,priority:2;index:doc_activities_actor_index,sort:desc,type:btree,priority:2;index:doc_activities_mail_index,type:btree,where:notified = false"`
	// verb character varying IS_NULL:NO
	Verb string `json:"verb"`
	// field character varying IS_NULL:YES
	Field *string `json:"field,omitempty" extensions:"x-nullable"`
	// old_value text IS_NULL:YES
	OldValue *string `json:"old_value" extensions:"x-nullable"`
	// new_value text IS_NULL:YES
	NewValue string `json:"new_value" `
	// comment text IS_NULL:NO
	Comment string `json:"comment"`
	// doc_id uuid IS_NULL:YES
	DocId uuid.UUID `json:"doc" gorm:"type:uuid;index:doc_activities_doc_index,priority:1" `
	// workspace_id uuid IS_NULL:NO
	WorkspaceId uuid.UUID `json:"workspace" gorm:"type:uuid"`
	// actor_id uuid IS_NULL:YES
	ActorId uuid.NullUUID `json:"actor,omitempty" gorm:"type:uuid;index:doc_activities_actor_index,priority:1" extensions:"x-nullable"`

	// new_identifier uuid IS_NULL:YES
	NewIdentifier uuid.NullUUID `json:"new_identifier" gorm:"type:uuid" extensions:"x-nullable"`
	// old_identifier uuid IS_NULL:YES
	OldIdentifier uuid.NullUUID `json:"old_identifier" gorm:"type:uuid" extensions:"x-nullable"`
	Notified      bool          `json:"-" gorm:"default:false"`
	TelegramMsgId pq.Int64Array `json:"-" gorm:"column:telegram_msg_ids;index;type:integer[]"`

	Workspace *dao.Workspace `json:"workspace_detail" gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	Actor     *dao.User      `json:"actor_detail" gorm:"foreignKey:ActorId" extensions:"x-nullable"`
	Doc       *dao.Doc       `json:"doc_detail" gorm:"foreignKey:DocId" extensions:"x-nullable"`

	OldDoc *dao.Doc `json:"-" gorm:"-" field:"doc" extensions:"x-nullable"`
	NewDoc *dao.Doc `json:"-" gorm:"-" field:"doc" extensions:"x-nullable"`

	//AffectedUser      *User  `json:"affected_user,omitempty" gorm:"-" extensions:"x-nullable"`
	UnionCustomFields string `json:"-" gorm:"-"`
}

func (DocActivity) TableName() string {
	return "doc_activities"
}

type UserNotifications struct {
	ID        uuid.UUID      `gorm:"column:id;primaryKey;type:uuid" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`

	// Note: type:text используется потому что в существующей БД это поле имеет тип text, а не uuid
	UserId uuid.UUID `json:"user_id" gorm:"type:uuid;index"`
	User   *dao.User `json:"user_detail,omitempty" gorm:"foreignKey:UserId" extensions:"x-nullable"`

	Type             string          `json:"type"`
	EntityActivityId uuid.NullUUID   `json:"entity_activity,omitempty"`
	EntityActivity   *EntityActivity `json:"entity_activity_detail,omitempty" gorm:"foreignKey:EntityActivityId" extensions:"x-nullable"`

	CommentId uuid.NullUUID     `json:"comment_id,omitempty" gorm:"type:uuid"`
	Comment   *dao.IssueComment `json:"comment,omitempty" gorm:"foreignKey:CommentId" extensions:"x-nullable"`

	WorkspaceId uuid.NullUUID  `json:"workspace_id,omitempty" gorm:"type:uuid"`
	Workspace   *dao.Workspace `json:"workspace,omitempty" gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`

	IssueId uuid.NullUUID `json:"issue_id,omitempty" gorm:"type:uuid"`
	Issue   *dao.Issue    `json:"issue,omitempty" gorm:"foreignKey:IssueId" extensions:"x-nullable"`

	Title    string        `json:"title,omitempty"`
	Msg      string        `json:"msg,omitempty"`
	AuthorId uuid.NullUUID `json:"author_id" gorm:"type:uuid"`
	Author   *dao.User     `json:"author,omitempty" gorm:"foreignKey:AuthorId" extensions:"x-nullable"`
	Viewed   bool          `json:"viewed" gorm:"default:false"`

	TargetUser *dao.User `json:"target_user,omitempty" gorm:"-"`

	IssueActivityId uuid.NullUUID  `json:"issue_activity,omitempty"`
	IssueActivity   *IssueActivity `json:"issue_activity_detail,omitempty" gorm:"foreignKey:IssueActivityId" extensions:"x-nullable"`

	ProjectActivityId uuid.NullUUID    `json:"project_activity,omitempty"`
	ProjectActivity   *ProjectActivity `json:"project_activity_detail,omitempty" gorm:"foreignKey:ProjectActivityId" extensions:"x-nullable"`

	FormActivityId uuid.NullUUID `json:"form_activity,omitempty"`
	FormActivity   *FormActivity `json:"form_activity_detail,omitempty" gorm:"foreignKey:FormActivityId" extensions:"x-nullable"`

	DocActivityId uuid.NullUUID `json:"doc_activity,omitempty"`
	DocActivity   *DocActivity  `json:"doc_activity_detail,omitempty" gorm:"foreignKey:DocActivityId" extensions:"x-nullable"`

	SprintActivityId uuid.NullUUID   `json:"sprint_activity,omitempty"`
	SprintActivity   *SprintActivity `json:"sprint_activity_detail,omitempty" gorm:"foreignKey:SprintActivityId" extensions:"x-nullable"`

	WorkspaceActivityId uuid.NullUUID      `json:"workspace_activity,omitempty"`
	WorkspaceActivity   *WorkspaceActivity `json:"workspace_activity_detail,omitempty" gorm:"foreignKey:WorkspaceActivityId" extensions:"x-nullable"`

	RootActivityId uuid.NullUUID `json:"root_activity,omitempty"`
	RootActivity   *RootActivity `json:"root_activity_detail,omitempty" gorm:"foreignKey:RootActivityId" extensions:"x-nullable"`

	ActivityEventId uuid.NullUUID      `json:"activity,omitempty"`
	ActivityEvent   *dao.ActivityEvent `json:"activity_event,omitempty" gorm:"foreignKey:ActivityEventId" extensions:"x-nullable"`
}

type EntityActivity struct {
	Id        uuid.UUID `json:"id" gorm:"primaryKey;type:uuid"`
	CreatedAt time.Time `json:"created_at" gorm:"index:entity_activities_issue_index,sort:desc,type:btree,priority:2;index:entity_activities_actor_index,sort:desc,type:btree,priority:2;index:entity_activities_mail_index,type:btree,where:notified = false and issue_id is not null"`
	//DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`
	// verb character varying IS_NULL:NO
	Verb string `json:"verb"`
	// field character varying IS_NULL:YES
	Field *string `json:"field,omitempty" extensions:"x-nullable"`
	// old_value text IS_NULL:YES
	OldValue *string `json:"old_value" extensions:"x-nullable"`
	// new_value text IS_NULL:YES
	NewValue string `json:"new_value" `
	// comment text IS_NULL:NO
	Comment string `json:"comment"`
	// attachments ARRAY IS_NULL:NO
	Attachments string `json:"attachments"`
	// created_by_id uuid IS_NULL:YES
	// Note: type:text используется потому что в существующей БД это поле имеет тип text, а не uuid
	CreatedById uuid.NullUUID `json:"created_by_id,omitempty" gorm:"type:uuid" extensions:"x-nullable"`
	// issue_id uuid IS_NULL:YES
	IssueId uuid.NullUUID `json:"issue_id,omitempty" gorm:"type:uuid;index:entity_activities_issue_index,priority:1" extensions:"x-nullable"`
	// project_id uuid IS_NULL:YES
	ProjectId uuid.NullUUID `json:"project_id" gorm:"type:uuid" extensions:"x-nullable"`
	// updated_by_id uuid IS_NULL:YES
	// Note: type:text используется потому что в существующей БД это поле имеет тип text, а не uuid
	UpdatedById uuid.NullUUID `json:"updated_by_id,omitempty" gorm:"type:uuid" extensions:"x-nullable"`
	// workspace_id uuid IS_NULL:NO
	WorkspaceId uuid.UUID `json:"workspace" gorm:"type:uuid"`
	// form_id uuid IS_NULL:YES
	FormId uuid.NullUUID `json:"form_id" gorm:"type:uuid" extensions:"x-nullable"`
	// actor_id uuid IS_NULL:YES
	ActorId uuid.NullUUID `json:"actor,omitempty" gorm:"type:uuid;index:entity_activities_actor_index,priority:1" extensions:"x-nullable"`
	// doc_id uuid IS_NULL:YES
	DocId uuid.NullUUID `json:"doc_id" gorm:"type:uuid" extensions:"x-nullable"`

	// new_identifier uuid IS_NULL:YES
	NewIdentifier uuid.NullUUID `json:"new_identifier" gorm:"type:uuid" extensions:"x-nullable"`
	// old_identifier uuid IS_NULL:YES
	OldIdentifier uuid.NullUUID `json:"old_identifier" gorm:"type:uuid" extensions:"x-nullable"`
	Notified      bool          `json:"-" gorm:"default:false"`
	TelegramMsgId pq.Int64Array `json:"-" gorm:"column:telegram_msg_ids;index;type:integer[]"`

	Workspace *dao.Workspace `json:"workspace_detail" gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	Actor     *dao.User      `json:"actor_detail" gorm:"foreignKey:ActorId" extensions:"x-nullable"`
	Issue     *dao.Issue     `json:"issue_detail" gorm:"foreignKey:IssueId" extensions:"x-nullable"`
	Project   *dao.Project   `json:"project_detail" gorm:"foreignKey:ProjectId" extensions:"x-nullable"`
	Form      *dao.Form      `json:"form_detail" gorm:"foreignKey:FormId" extensions:"x-nullable"`
	CreatedBy *dao.User      `json:"created_by_detail" gorm:"foreignKey:CreatedById;references:ID" extensions:"x-nullable"`
	UpdatedBy *dao.User      `json:"updated_by_detail" gorm:"foreignKey:UpdatedById;references:ID" extensions:"x-nullable"`

	NewAttachment *dao.IssueAttachment `json:"-" gorm:"-" field:"attachment" extensions:"x-nullable"`
	NewLink       *dao.IssueLink       `json:"-" gorm:"-" field:"link" extensions:"x-nullable"`

	NewAssignee *dao.User `json:"-" gorm:"-" field:"assignees" extensions:"x-nullable"`
	OldAssignee *dao.User `json:"-" gorm:"-" field:"assignees" extensions:"x-nullable"`

	NewWatcher *dao.User `json:"-" gorm:"-" field:"watchers" extensions:"x-nullable"`
	OldWatcher *dao.User `json:"-" gorm:"-" field:"watchers" extensions:"x-nullable"`

	NewSubIssue *dao.Issue `json:"-" gorm:"-" field:"sub_issue" extensions:"x-nullable"`
	OldSubIssue *dao.Issue `json:"-" gorm:"-" field:"sub_issue" extensions:"x-nullable"`

	NewRole *dao.User `json:"-" gorm:"-" field:"role" extensions:"x-nullable"`
	OldRole *dao.User `json:"-" gorm:"-" field:"role" extensions:"x-nullable"`

	NewMember *dao.User `json:"-" gorm:"-" field:"member" extensions:"x-nullable"`
	OldMember *dao.User `json:"-" gorm:"-" field:"member" extensions:"x-nullable"`

	NewDefaultAssignee *dao.User `json:"-" gorm:"-" field:"default_assignees" extensions:"x-nullable"`
	OldDefaultAssignee *dao.User `json:"-" gorm:"-" field:"default_assignees" extensions:"x-nullable"`

	NewDefaultWatcher *dao.User `json:"-" gorm:"-" field:"default_watchers" extensions:"x-nullable"`
	OldDefaultWatcher *dao.User `json:"-" gorm:"-" field:"default_watchers" extensions:"x-nullable"`

	NewProjectLead *dao.User `json:"-" gorm:"-" field:"project_lead" extensions:"x-nullable"`
	OldProjectLead *dao.User `json:"-" gorm:"-" field:"project_lead" extensions:"x-nullable"`

	//AffectedUser *User `json:"affected_user,omitempty" gorm:"-" extensions:"x-nullable"`

	NewIssueComment *dao.IssueComment `json:"-" gorm:"-" field:"comment::issue" extensions:"x-nullable"`

	EntityType        string `json:"entity_type"`
	EntityId          string `json:"entity_id"`
	UnionCustomFields string `json:"-" gorm:"-"`
}

type IssueActivity struct {
	Id uuid.UUID `json:"id" gorm:"primaryKey;type:uuid"`

	CreatedAt time.Time `json:"created_at" gorm:"index:issue_activities_issue_index,sort:desc,type:btree,priority:2;index:issue_activities_actor_index,sort:desc,type:btree,priority:2;index:issue_activities_mail_index,type:btree,where:notified = false"`
	// verb character varying IS_NULL:NO
	Verb string `json:"verb"`
	//field character varying IS_NULL:YES
	Field *string `json:"field,omitempty" extensions:"x-nullable"`
	// old_value text IS_NULL:YES
	OldValue *string `json:"old_value" extensions:"x-nullable"`
	// new_value text IS_NULL:YES
	NewValue string `json:"new_value" `
	// comment text IS_NULL:NO
	Comment string `json:"comment"`
	// issue_id uuid IS_NULL:YES
	IssueId uuid.UUID `json:"issue_id" gorm:"type:uuid;index:issue_activities_issue_index,priority:1" extensions:"x-nullable"`
	// project_id uuid IS_NULL:YES
	ProjectId uuid.UUID `json:"project_id" gorm:"type:uuid"`
	// workspace_id uuid IS_NULL:NO
	WorkspaceId uuid.UUID `json:"workspace" gorm:"type:uuid"`
	// actor_id uuid IS_NULL:YES
	ActorId uuid.NullUUID `json:"actor,omitempty" gorm:"type:uuid;index:issue_activities_actor_index,priority:1" extensions:"x-nullable"`

	// new_identifier uuid IS_NULL:YES
	NewIdentifier uuid.NullUUID `json:"new_identifier" gorm:"type:uuid" extensions:"x-nullable"`
	// old_identifier uuid IS_NULL:YES
	OldIdentifier uuid.NullUUID `json:"old_identifier" gorm:"type:uuid" extensions:"x-nullable"`
	Notified      bool          `json:"-" gorm:"default:false"`
	TelegramMsgId pq.Int64Array `json:"-" gorm:"column:telegram_msg_ids;index;type:integer[]"`

	Workspace *dao.Workspace `json:"workspace_detail" gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	Actor     *dao.User      `json:"actor_detail" gorm:"foreignKey:ActorId" extensions:"x-nullable"`
	Issue     *dao.Issue     `json:"issue_detail" gorm:"foreignKey:IssueId" extensions:"x-nullable"`
	Project   *dao.Project   `json:"project_detail" gorm:"foreignKey:ProjectId" extensions:"x-nullable"`

	//AffectedUser *User `json:"affected_user,omitempty" gorm:"-" extensions:"x-nullable"`

	UnionCustomFields string `json:"-" gorm:"-"`

	//NewIssueComment *IssueComment `json:"-" gorm:"-" field:"comment" extensions:"x-nullable"`
}

func (i IssueActivity) GetId() uuid.UUID {
	return i.Id
}

func (IssueActivity) TableName() string {
	return "issue_activities"
}

type SprintActivity struct {
	Id        uuid.UUID `json:"id" gorm:"primaryKey;type:uuid"`
	CreatedAt time.Time `json:"created_at" gorm:"index:sprint_activities_sprint_index,sort:desc,priority:2;index:sprint_activities_actor_index,sort:desc,priority:2;index:sprint_activities_mail_index,where:notified = false"`
	// verb character varying IS_NULL:NO
	Verb string `json:"verb"`
	// field character varying IS_NULL:YES
	Field *string `json:"field,omitempty" extensions:"x-nullable"`
	// old_value text IS_NULL:YES
	OldValue *string `json:"old_value" extensions:"x-nullable"`
	// new_value text IS_NULL:YES
	NewValue string `json:"new_value" `
	// comment text IS_NULL:NO
	Comment string `json:"comment"`
	// sprint_id uuid IS_NULL:YES
	SprintId uuid.UUID `json:"sprint_id" gorm:"type:uuid;index:sprint_activities_sprint_index,priority:1" extensions:"x-nullable"`
	// workspace_id uuid IS_NULL:NO
	WorkspaceId uuid.UUID `json:"workspace" gorm:"type:uuid"`
	// actor_id uuid IS_NULL:YES
	ActorId uuid.NullUUID `json:"actor,omitempty" gorm:"type:uuid;index:project_activities_actor_index,priority:1" extensions:"x-nullable"`

	// new_identifier uuid IS_NULL:YES
	NewIdentifier uuid.NullUUID `json:"new_identifier" gorm:"type:uuid" extensions:"x-nullable"`
	// old_identifier uuid IS_NULL:YES
	OldIdentifier uuid.NullUUID `json:"old_identifier" gorm:"type:uuid" extensions:"x-nullable"`
	Notified      bool          `json:"-" gorm:"default:false"`
	TelegramMsgId pq.Int64Array `json:"-" gorm:"column:telegram_msg_ids;index;type:integer[]"`

	Workspace *dao.Workspace `json:"workspace_detail" gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	Actor     *dao.User      `json:"actor_detail" gorm:"foreignKey:ActorId" extensions:"x-nullable"`
	Sprint    *dao.Sprint    `json:"sprint_detail" gorm:"foreignKey:SprintId" extensions:"x-nullable"`

	UnionCustomFields string `json:"-" gorm:"-"`
	dao.SprintActivityExtendFields
	dao.ActivitySender
}

func (SprintActivity) TableName() string {
	return "sprint_activities"
}

type ProjectActivity struct {
	Id        uuid.UUID `json:"id" gorm:"primaryKey;type:uuid"`
	CreatedAt time.Time `json:"created_at" gorm:"index:project_activities_project_index,sort:desc,type:btree,priority:2;index:project_activities_actor_index,sort:desc,type:btree,priority:2;index:project_activities_mail_index,type:btree,where:notified = false"`
	// verb character varying IS_NULL:NO
	Verb string `json:"verb"`
	// field character varying IS_NULL:YES
	Field *string `json:"field,omitempty" extensions:"x-nullable"`
	// old_value text IS_NULL:YES
	OldValue *string `json:"old_value" extensions:"x-nullable"`
	// new_value text IS_NULL:YES
	NewValue string `json:"new_value" `
	// comment text IS_NULL:NO
	Comment string `json:"comment"`
	// project_id uuid IS_NULL:YES
	// Note: type:text используется потому что в существующей БД это поле имеет тип text, а не uuid
	ProjectId uuid.UUID `json:"project_id" gorm:"type:uuid;index:project_activities_project_index,priority:1" extensions:"x-nullable"`
	// workspace_id uuid IS_NULL:NO
	WorkspaceId uuid.UUID `json:"workspace" gorm:"type:uuid"`
	// actor_id uuid IS_NULL:YES
	ActorId uuid.NullUUID `json:"actor,omitempty" gorm:"type:uuid;index:project_activities_actor_index,priority:1" extensions:"x-nullable"`

	// new_identifier uuid IS_NULL:YES
	NewIdentifier uuid.NullUUID `json:"new_identifier" gorm:"type:uuid" extensions:"x-nullable"`
	// old_identifier uuid IS_NULL:YES
	OldIdentifier uuid.NullUUID `json:"old_identifier" gorm:"type:uuid" extensions:"x-nullable"`
	Notified      bool          `json:"-" gorm:"default:false"`
	TelegramMsgId pq.Int64Array `json:"-" gorm:"column:telegram_msg_ids;index;type:integer[]"`

	Workspace *dao.Workspace `json:"workspace_detail" gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	Actor     *dao.User      `json:"actor_detail" gorm:"foreignKey:ActorId" extensions:"x-nullable"`
	Project   *dao.Project   `json:"project_detail" gorm:"foreignKey:ProjectId" extensions:"x-nullable"`

	UnionCustomFields string `json:"-" gorm:"-"`
	dao.ProjectActivityExtendFields
	dao.ActivitySender
}

func (p ProjectActivity) GetId() uuid.UUID {
	return p.Id
}

func (ProjectActivity) TableName() string {
	return "project_activities"
}

type FormActivity struct {
	Id        uuid.UUID `json:"id" gorm:"primaryKey;type:uuid"`
	CreatedAt time.Time `json:"created_at" gorm:"index:form_activities_form_index,sort:desc,type:btree,priority:2;index:form_activities_actor_index,sort:desc,type:btree,priority:2;index:form_activities_mail_index,type:btree,where:notified = false"`
	// verb character varying IS_NULL:NO
	Verb string `json:"verb"`
	// field character varying IS_NULL:YES
	Field *string `json:"field,omitempty" extensions:"x-nullable"`
	// old_value text IS_NULL:YES
	OldValue *string `json:"old_value" extensions:"x-nullable"`
	// new_value text IS_NULL:YES
	NewValue string `json:"new_value" `
	// comment text IS_NULL:NO
	Comment string `json:"comment"`
	// form_id uuid IS_NULL:YES
	FormId uuid.UUID `json:"form_id,omitempty" gorm:"type:uuid;index:form_activities_form_index,priority:1" extensions:"x-nullable"`
	// workspace_id uuid IS_NULL:NO
	WorkspaceId uuid.UUID `json:"workspace" gorm:"type:uuid"`
	// actor_id uuid IS_NULL:YES
	ActorId uuid.NullUUID `json:"actor,omitempty" gorm:"type:uuid;index:form_activities_actor_index,priority:1" extensions:"x-nullable"`

	// new_identifier uuid IS_NULL:YES
	NewIdentifier uuid.NullUUID `json:"new_identifier" gorm:"type:uuid" extensions:"x-nullable"`
	// old_identifier uuid IS_NULL:YES
	OldIdentifier uuid.NullUUID `json:"old_identifier" gorm:"type:uuid" extensions:"x-nullable"`
	Notified      bool          `json:"-" gorm:"default:false"`
	TelegramMsgId pq.Int64Array `json:"-" gorm:"column:telegram_msg_ids;index;type:integer[]"`

	Workspace *dao.Workspace `json:"workspace_detail" gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	Actor     *dao.User      `json:"actor_detail" gorm:"foreignKey:ActorId" extensions:"x-nullable"`
	Form      *dao.Form      `json:"form_detail" gorm:"foreignKey:FormId" extensions:"x-nullable"`

	//AffectedUser      *User  `json:"affected_user,omitempty" gorm:"-" extensions:"x-nullable"`
	UnionCustomFields string `json:"-" gorm:"-"`
	dao.FormActivityExtendFields
	dao.ActivitySender
}

func (FormActivity) TableName() string {
	return "form_activities"
}

type WorkspaceActivity struct {
	Id        uuid.UUID `json:"id" gorm:"primaryKey;type:uuid"`
	CreatedAt time.Time `json:"created_at" gorm:"index:workspace_activities_workspace_index,sort:desc,type:btree,priority:2;index:workspace_activities_actor_index,sort:desc,type:btree,priority:2;index:workspace_activities_mail_index,type:btree,where:notified = false"`
	// verb character varying IS_NULL:NO
	Verb string `json:"verb"`
	// field character varying IS_NULL:YES
	Field *string `json:"field,omitempty" extensions:"x-nullable"`
	// old_value text IS_NULL:YES
	OldValue *string `json:"old_value" extensions:"x-nullable"`
	// new_value text IS_NULL:YES
	NewValue string `json:"new_value" `
	// comment text IS_NULL:NO
	Comment string `json:"comment"`
	// workspace_id uuid IS_NULL:NO
	WorkspaceId uuid.UUID `json:"workspace" gorm:"type:uuid;index:workspace_activities_workspace_index,priority:1"`
	// actor_id uuid IS_NULL:YES
	ActorId uuid.NullUUID `json:"actor,omitempty" gorm:"type:uuid;index:workspace_activities_actor_index,priority:1" extensions:"x-nullable"`

	// new_identifier uuid IS_NULL:YES
	NewIdentifier uuid.NullUUID `json:"new_identifier" gorm:"type:uuid" extensions:"x-nullable"`
	// old_identifier uuid IS_NULL:YES
	OldIdentifier uuid.NullUUID `json:"old_identifier" gorm:"type:uuid" extensions:"x-nullable"`
	Notified      bool          `json:"-" gorm:"default:false"`
	TelegramMsgId pq.Int64Array `json:"-" gorm:"column:telegram_msg_ids;index;type:integer[]"`

	Workspace *dao.Workspace `json:"workspace_detail" gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	Actor     *dao.User      `json:"actor_detail" gorm:"foreignKey:ActorId" extensions:"x-nullable"`

	NewProject *dao.Project `json:"-" gorm:"-" field:"project" extensions:"x-nullable"`

	//AffectedUser      *User  `json:"affected_user,omitempty" gorm:"-" extensions:"x-nullable"`
	UnionCustomFields string `json:"-" gorm:"-"`
	dao.WorkspaceActivityExtendFields
	dao.ActivitySender
}

func (WorkspaceActivity) TableName() string {
	return "workspace_activities"
}

type RootActivity struct {
	Id        uuid.UUID `json:"id" gorm:"primaryKey;type:uuid"`
	CreatedAt time.Time `json:"created_at" gorm:"index:activities_actor_index,sort:desc,type:btree,priority:2;index:activities_mail_index,type:btree,where:notified = false"`
	// verb character varying IS_NULL:NO
	Verb string `json:"verb"`
	// field character varying IS_NULL:YES
	Field *string `json:"field,omitempty" extensions:"x-nullable"`
	// old_value text IS_NULL:YES
	OldValue *string `json:"old_value" extensions:"x-nullable"`
	// new_value text IS_NULL:YES
	NewValue string `json:"new_value" `
	// comment text IS_NULL:NO
	Comment string `json:"comment"`
	// actor_id uuid IS_NULL:YES
	ActorId uuid.NullUUID `json:"actor,omitempty" gorm:"type:uuid;index:activities_actor_index,priority:1" extensions:"x-nullable"`

	// new_identifier uuid IS_NULL:YES
	NewIdentifier uuid.NullUUID `json:"new_identifier" gorm:"type:uuid" extensions:"x-nullable"`
	// old_identifier uuid IS_NULL:YES
	OldIdentifier uuid.NullUUID `json:"old_identifier" gorm:"type:uuid" extensions:"x-nullable"`
	Notified      bool          `json:"-" gorm:"default:false"`
	TelegramMsgId pq.Int64Array `json:"-" gorm:"column:telegram_msg_ids;index;type:integer[]"`

	Actor *dao.User `json:"actor_detail" gorm:"foreignKey:ActorId" extensions:"x-nullable"`

	NewWorkspace      *dao.Workspace `json:"-" gorm:"-" field:"workspace" extensions:"x-nullable"`
	NewDoc            *dao.Doc       `json:"-" gorm:"-" field:"doc" extensions:"x-nullable"`
	UnionCustomFields string         `json:"-" gorm:"-"`
	dao.RootActivityExtendFields
	dao.ActivitySender
}

func (RootActivity) TableName() string {
	return "activities"
}
