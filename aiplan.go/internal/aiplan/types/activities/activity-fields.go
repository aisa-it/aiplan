package activities

import "fmt"

type ActivityField string

const (
	ReqFieldName             = "name"
	ReqFieldTitle            = "title"
	ReqFieldTemplate         = "template"
	ReqFieldWatchers         = "watchers_list"
	ReqFieldAssignees        = "assignees_list"
	ReqFieldReaders          = "readers_list"
	ReqFieldEditors          = "editors_list"
	ReqFieldIssues           = "issue_list"
	ReqFieldSprint           = "sprint"
	ReqFieldEmoj             = "emoji"
	ReqFieldPublic           = "public"
	ReqFieldIdentifier       = "identifier"
	ReqFieldProjectLead      = "project_lead"
	ReqFieldPriority         = "priority"
	ReqFieldRole             = "role"
	ReqFieldDefaultAssignees = "default_assignees"
	ReqFieldDefaultWatchers  = "default_watchers"
	ReqFieldDescription      = "description"
	ReqFieldDescriptionHtml  = "description_html"
	ReqFieldColor            = "color"
	ReqFieldTargetDate       = "target_date"
	ReqFieldStartDate        = "start_date"
	ReqFieldCompletedAt      = "completed_at"
	ReqFieldEndDate          = "end_date"
	ReqFieldLabel            = "labels_list"
	ReqFieldAuthRequire      = "auth_require"
	ReqFieldFields           = "fields"
	ReqFieldGroup            = "group"
	ReqFieldState            = "state"
	ReqFieldParent           = "parent"
	ReqFieldDefault          = "default"
	ReqFieldEstimatePoint    = "estimate_point"
	ReqFieldBlocksList       = "blocks_list"
	ReqFieldBlockersList     = "blockers_list"
	ReqFieldUrl              = "url"
	ReqFieldCommentHtml      = "comment_html"
	ReqFieldDocSort          = "doc_sort"
	ReqFieldReaderRole       = "reader_role"
	ReqFieldEditorRole       = "editor_role"
	ReqFieldLinked           = "linked_issues_ids"
	ReqFieldLogo             = "logo"
	ReqFieldToken            = "integration_token"
	ReqFieldOwner            = "owner_id"

	FieldReaders          ActivityField = "readers"
	FieldEditors          ActivityField = "editors"
	FieldWatchers         ActivityField = "watchers"
	FieldAssignees        ActivityField = "assignees"
	FieldLinked           ActivityField = "linked"
	FieldIssues           ActivityField = "issues"
	FieldIssue            ActivityField = "issue"
	FieldBlocks           ActivityField = "blocks"
	FieldBlocking         ActivityField = "blocking"
	FieldAttachment       ActivityField = "attachment"
	FieldComment          ActivityField = "comment"
	FieldDoc              ActivityField = "doc"
	FieldDescription      ActivityField = "description"
	FieldTitle            ActivityField = "title"
	FieldEmoj             ActivityField = "emoji"
	FieldPublic           ActivityField = "public"
	FieldIdentifier       ActivityField = "identifier"
	FieldProjectLead      ActivityField = "project_lead"
	FieldPriority         ActivityField = "priority"
	FieldRole             ActivityField = "role"
	FieldReaderRole       ActivityField = "reader_role"
	FieldEditorRole       ActivityField = "editor_role"
	FieldStatus           ActivityField = "status"
	FieldDefaultAssignees ActivityField = "default_assignees"
	FieldDefaultWatchers  ActivityField = "default_watchers"
	FieldSubIssue         ActivityField = "sub_issue"
	FieldToken            ActivityField = "integration_token"
	FieldOwner            ActivityField = "owner_id"
	FieldLogo             ActivityField = "logo"
	FieldParent           ActivityField = "parent"
	FieldDefault          ActivityField = "default"
	FieldEstimatePoint    ActivityField = "estimate_point"
	FieldUrl              ActivityField = "url"
	FieldCommentHtml      ActivityField = "comment_html"
	FieldDocSort          ActivityField = "doc_sort"
	FieldName             ActivityField = "name"
	FieldTemplate         ActivityField = "template"
	FieldColor            ActivityField = "color"
	FieldDescriptionHtml  ActivityField = "description_html"
	FieldTargetDate       ActivityField = "target_date"
	FieldStartDate        ActivityField = "start_date"
	FieldCompletedAt      ActivityField = "completed_at"
	FieldEndDate          ActivityField = "end_date"
	FieldLabel            ActivityField = "label"
	FieldAuthRequire      ActivityField = "auth_require"
	FieldFields           ActivityField = "fields"
	FieldGroup            ActivityField = "group"
	FieldMember           ActivityField = "member"

	FieldLink      ActivityField = "link"
	FieldLinkTitle ActivityField = "link_title"
	FieldLinkUrl   ActivityField = "link_url"

	FieldLabelName  ActivityField = "label_name"
	FieldLabelColor ActivityField = "label_color"

	FieldStatusColor       ActivityField = "status_color"
	FieldStatusGroup       ActivityField = "status_group"
	FieldStatusDescription ActivityField = "status_description"
	FieldStatusName        ActivityField = "status_name"
	FieldStatusDefault     ActivityField = "status_default"

	FieldTemplateName     ActivityField = "template_name"
	FieldTemplateTemplate ActivityField = "template_template"

	FieldProject  ActivityField = "project"
	FieldDeadline ActivityField = "deadline"

	FieldForm           ActivityField = "form"
	FieldIntegration    ActivityField = "integration"
	FieldWorkspaceOwner ActivityField = "owner"
	FieldIssueTransfer  ActivityField = "issue_transfer"
	FieldSprint         ActivityField = "sprint"
	FieldWorkspace      ActivityField = "workspace"
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
