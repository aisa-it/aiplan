package activities

import (
	"database/sql/driver"
	"fmt"
)

const (
	activityVal   = "activity_val"
	updateScopeId = "updateScopeId"
	updateScope   = "updateScope"
	fieldLog      = "field_log"
	funcName      = "func_name"
	key           = "key"
	getField      = "get_field"
	entityParent  = "entityParent"
	entity        = "entity"
	customVerb    = "custom_verb"
	oldTitle      = "old_title"
)

type FieldKey struct {
	Field ActivityField
	Kind  string
}

func (fk FieldKey) String() string {
	if fk.Kind == "" {
		return fk.Field.String()
	}
	return fmt.Sprintf("%s_%s", fk.Field.String(), fk.Kind)
}

var (
	UpdateScopeIdKey = FieldKey{Field: updateScopeId, Kind: ""}
	UpdateScopeKey   = FieldKey{Field: updateScope, Kind: ""}
	FieldLogKey      = FieldKey{Field: fieldLog, Kind: ""}
	EntityParentKey  = FieldKey{Field: entityParent, Kind: ""}
	EntityKey        = FieldKey{Field: entity, Kind: ""}
	CustomVerbKey    = FieldKey{Field: customVerb, Kind: ""}
	OldTitleKey      = FieldKey{Field: oldTitle, Kind: ""}
)

type ActivityField string

func (f ActivityField) Value() (driver.Value, error) {
	return string(f), nil
}

func (f *ActivityField) Scan(value interface{}) error {
	if value == nil {
		*f = ActivityField("")
		return nil
	}

	switch v := value.(type) {
	case string:
		*f = ActivityField(v)
	case []byte:
		*f = ActivityField(v)
	case fmt.Stringer:
		*f = ActivityField(v.String())
	default:
		return fmt.Errorf("cannot scan type %T into ActivityField: %v", value, value)
	}

	return nil
}

func New[E ~string](field E) ActivityField {
	return ActivityField(field)
}

func (a ActivityField) String() string {
	return string(a)
}

func (a ActivityField) WithActivityVal() FieldKey {
	return FieldKey{Field: a, Kind: activityVal}
}

func (a ActivityField) WithFunc() FieldKey {
	return FieldKey{Field: a, Kind: funcName}
}

func (a ActivityField) WithGetField() FieldKey {
	return FieldKey{Field: a, Kind: getField}
}

func (a ActivityField) WithFieldLog() FieldKey {
	return FieldKey{Field: a, Kind: fieldLog}
}

func (a ActivityField) WithUpdateScopeId() FieldKey {
	return FieldKey{Field: a, Kind: updateScopeId}
}
func (a ActivityField) WithKey() FieldKey {
	return FieldKey{Field: a, Kind: key}
}

func (a ActivityField) Only() FieldKey {
	return FieldKey{Field: a, Kind: ""}
}

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
	Owner         = FieldMapping{"owner_id", "owner"}
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

	VerbMoveDocWorkspace = "move_doc_to_workspace"
	VerbMoveDocDoc       = "move_doc_to_doc"
	VerbMoveWorkspaceDoc = "move_workspace_to_doc"
)

func ReqFieldMapping(in string) string {
	if v, ok := fieldChange[in]; ok {
		return v
	}
	return in
}
