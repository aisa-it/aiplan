package activities

import "fmt"

type ActivityField string

const (
	EntityUpdatedActivity = "entity.updated"
	EntityCreateActivity  = "entity.create"
	EntityDeleteActivity  = "entity.delete"
	EntityAddActivity     = "entity.add"
	EntityRemoveActivity  = "entity.remove"
	EntityMoveActivity    = "entity.move"

	VerbUpdated = "updated"
	VerbRemoved = "removed"
	VerbAdded   = "added"
	VerbDeleted = "deleted"
	VerbCreated = "created"
	VerbMove    = "move"

	ReqName             = "name"
	ReqTitle            = "title"
	ReqTemplate         = "template"
	ReqWatchers         = "watchers_list"
	ReqAssignees        = "assignees_list"
	ReqReaders          = "readers_list"
	ReqEditors          = "editors_list"
	ReqIssues           = "issue_list"
	ReqSprint           = "sprint"
	ReqEmoj             = "emoji"
	ReqPublic           = "public"
	ReqIdentifier       = "identifier"
	ReqProjectLead      = "project_lead"
	ReqPriority         = "priority"
	ReqRole             = "role"
	ReqDefaultAssignees = "default_assignees"
	ReqDefaultWatchers  = "default_watchers"
	ReqDescription      = "description"
	ReqDescriptionHtml  = "description_html"
	ReqColor            = "color"
	ReqTargetDate       = "target_date"
	ReqStartDate        = "start_date"
	ReqCompletedAt      = "completed_at"
	ReqEndDate          = "end_date"
	ReqLabel            = "labels_list"
	ReqAuthRequire      = "auth_require"
	ReqFields           = "fields"
	ReqGroup            = "group"
	ReqState            = "state"
	ReqParent           = "parent"
	ReqDefault          = "default"
	ReqEstimatePoint    = "estimate_point"
	ReqBlocksList       = "blocks_list"
	ReqBlockersList     = "blockers_list"
	ReqUrl              = "url"
	ReqCommentHtml      = "comment_html"
	ReqDocSort          = "doc_sort"
	ReqReaderRole       = "reader_role"
	ReqEditorRole       = "editor_role"
	ReqLinked           = "linked_issues_ids"
	ReqLogo             = "logo"
	ReqToken            = "integration_token"
	ReqOwner            = "owner_id"

	Readers          ActivityField = "readers"
	Editors          ActivityField = "editors"
	Watchers         ActivityField = "watchers"
	Assignees        ActivityField = "assignees"
	Linked           ActivityField = "linked"
	Issues           ActivityField = "issues"
	Issue            ActivityField = "issue"
	Blocks           ActivityField = "blocks"
	Blocking         ActivityField = "blocking"
	Attachment       ActivityField = "attachment"
	Comment          ActivityField = "comment"
	Doc              ActivityField = "doc"
	Description      ActivityField = "description"
	Title            ActivityField = "title"
	Emoj             ActivityField = "emoji"
	Public           ActivityField = "public"
	Identifier       ActivityField = "identifier"
	ProjectLead      ActivityField = "project_lead"
	Priority         ActivityField = "priority"
	Role             ActivityField = "role"
	ReaderRole       ActivityField = "reader_role"
	EditorRole       ActivityField = "editor_role"
	Status           ActivityField = "status"
	DefaultAssignees ActivityField = "default_assignees"
	DefaultWatchers  ActivityField = "default_watchers"
	SubIssue         ActivityField = "sub_issue"
	Token            ActivityField = "integration_token"
	Owner            ActivityField = "owner_id"
	Logo             ActivityField = "logo"
	Parent           ActivityField = "parent"
	Default          ActivityField = "default"
	EstimatePoint    ActivityField = "estimate_point"
	Url              ActivityField = "url"
	CommentHtml      ActivityField = "comment_html"
	DocSort          ActivityField = "doc_sort"
	Name             ActivityField = "name"
	Template         ActivityField = "template"
	Color            ActivityField = "color"
	DescriptionHtml  ActivityField = "description_html"
	TargetDate       ActivityField = "target_date"
	StartDate        ActivityField = "start_date"
	CompletedAt      ActivityField = "completed_at"
	EndDate          ActivityField = "end_date"
	Label            ActivityField = "label"
	AuthRequire      ActivityField = "auth_require"
	Fields           ActivityField = "fields"
	Group            ActivityField = "group"
	Member           ActivityField = "member"

	Link      ActivityField = "link"
	LinkTitle ActivityField = "link_title"
	LinkUrl   ActivityField = "link_url"

	LabelName  ActivityField = "label_name"
	LabelColor ActivityField = "label_color"

	StatusColor       ActivityField = "status_color"
	StatusGroup       ActivityField = "status_group"
	StatusDescription ActivityField = "status_description"
	StatusName        ActivityField = "status_name"
	StatusDefault     ActivityField = "status_default"

	TemplateName     ActivityField = "template_name"
	TemplateTemplate ActivityField = "template_template"

	Project  ActivityField = "project"
	Deadline ActivityField = "deadline"

	Form           ActivityField = "form"
	Integration    ActivityField = "integration"
	WorkspaceOwner ActivityField = "owner"
	IssueTransfer  ActivityField = "issue_transfer"
	Sprint         ActivityField = "sprint"
	Workspace      ActivityField = "workspace"
)

func (a ActivityField) String() string {
	return string(a)
}

func (a ActivityField) WithActivityValStr() string {
	return fmt.Sprintf("%s_%s", a.String(), "activity_val")
}

func (a ActivityField) WithFuncStr() string {
	return fmt.Sprintf("%s_%s", a.String(), "func")
}

func (a ActivityField) WithGetFieldStr() string {
	return fmt.Sprintf("%s_%s", a.String(), "get_field")
}
