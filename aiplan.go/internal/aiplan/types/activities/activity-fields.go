package activities

import "fmt"

type ActivityField string

type FieldMapping struct {
	Req   string
	Field ActivityField
}

func makeFieldMap(actFs ...FieldMapping) map[string]string {
	m := make(map[string]string)
	for _, act := range actFs {
		if act.Req != "" {
			m[act.Req] = act.Field.String()
		}
	}
	return m
}

var (
	fieldChange = makeFieldMap(
		Comment, DescriptionHtml, Status,
		Readers, Editors, Watchers, Assignees,
		Linked, Issues, Blocks, Blocking)

	Issue = FieldMapping{"", "issue"}
	Doc   = FieldMapping{"", "doc"}

	Comment     = FieldMapping{"comment_html", "comment"}
	CommentHtml = FieldMapping{"comment_html", "comment_html"}

	Readers   = FieldMapping{"readers_list", "readers"}
	Editors   = FieldMapping{"editors_list", "editors"}
	Watchers  = FieldMapping{"watchers_list", "watchers"}
	Assignees = FieldMapping{"assignees_list", "assignees"}
	Linked    = FieldMapping{"linked_issues_ids", "linked"}
	Issues    = FieldMapping{"issue_list", "issues"}

	Blocks   = FieldMapping{"blocks_list", "blocks"}
	Blocking = FieldMapping{"blockers_list", "blocking"}

	Attachment = FieldMapping{"", "attachment"}

	Description     = FieldMapping{"description", "description"}
	DescriptionHtml = FieldMapping{"description_html", "description"}

	Title            = FieldMapping{"title", "title"}
	Emoj             = FieldMapping{"emoji", "emoji"}
	Public           = FieldMapping{"public", "public"}
	Identifier       = FieldMapping{"identifier", "identifier"}
	ProjectLead      = FieldMapping{"project_lead", "project_lead"}
	Priority         = FieldMapping{"priority", "priority"}
	Role             = FieldMapping{"role", "role"}
	ReaderRole       = FieldMapping{"reader_role", "reader_role"}
	EditorRole       = FieldMapping{"editor_role", "editor_role"}
	Status           = FieldMapping{"state", "status"}
	DefaultAssignees = FieldMapping{"default_assignees", "default_assignees"}
	DefaultWatchers  = FieldMapping{"default_watchers", "default_watchers"}

	SubIssue = FieldMapping{"", "sub_issue"}

	Token         = FieldMapping{"integration_token", "integration_token"}
	Owner         = FieldMapping{"owner_id", "owner_id"}
	Logo          = FieldMapping{"logo", "logo"}
	Parent        = FieldMapping{"parent", "parent"}
	Default       = FieldMapping{"default", "default"}
	EstimatePoint = FieldMapping{"estimate_point", "estimate_point"}
	Url           = FieldMapping{"url", "url"}
	DocSort       = FieldMapping{"doc_sort", "doc_sort"}
	Name          = FieldMapping{"name", "name"}
	Template      = FieldMapping{"template", "template"}
	Color         = FieldMapping{"color", "color"}
	TargetDate    = FieldMapping{"target_date", "target_date"}
	StartDate     = FieldMapping{"start_date", "start_date"}
	CompletedAt   = FieldMapping{"completed_at", "completed_at"}
	EndDate       = FieldMapping{"end_date", "end_date"}
	Label         = FieldMapping{"labels_list", "label"}
	AuthRequire   = FieldMapping{"auth_require", "auth_require"}
	Fields        = FieldMapping{"fields", "fields"}
	Group         = FieldMapping{"group", "group"}
	Sprint        = FieldMapping{"sprint", "sprint"}

	Member = FieldMapping{"", "member"}

	Link      = FieldMapping{"", "link"} //
	LinkTitle = FieldMapping{"", "link_title"}
	LinkUrl   = FieldMapping{"", "link_url"}

	LabelName  = FieldMapping{"", "label_name"} // todo <<<
	LabelColor = FieldMapping{"", "label_color"}

	StatusColor       = FieldMapping{"", "status_color"} // todo <<<
	StatusGroup       = FieldMapping{"", "status_group"}
	StatusDescription = FieldMapping{"", "status_description"}
	StatusName        = FieldMapping{"", "status_name"}
	StatusDefault     = FieldMapping{"", "status_default"}

	TemplateName     = FieldMapping{"", "template_name"} // todo <<<
	TemplateTemplate = FieldMapping{"", "template_template"}

	Project  = FieldMapping{"", "project"}
	Deadline = FieldMapping{"", "deadline"}

	Form           = FieldMapping{"", "form"}
	Integration    = FieldMapping{"", "integration"}
	WorkspaceOwner = FieldMapping{"", "owner"}
	IssueTransfer  = FieldMapping{"", "issue_transfer"}
	Workspace      = FieldMapping{"", "workspace"}
)

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
	VerbCopied  = "copied"
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

func ReqFieldMapping(in string) string {
	if v, ok := fieldChange[in]; ok {
		return v
	}
	return in
}
