// Содержит определения различных типов данных, используемых в приложении.  Включает типы для работы с датами, временными зонами, настройками темы, формами, фильтрами, URL, векторами, уведомлениями и JSON URL-ами.  Предоставляет методы для сериализации, десериализации и валидации данных в различных форматах и для различных целей.
//
// Основные возможности:
//   - Работа с датами и временем.
//   - Обработка JSON данных.
//   - Преобразование типов данных.
//   - Работа с URL.
//   - Фильтрация и настройка данных.
package types

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	policy "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/redactor-policy"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TargetDate type
type TargetDate struct {
	Time time.Time
}

func (d *TargetDate) UnmarshalJSON(b []byte) error {
	str := string(b)
	if str != "" && str[0] == '"' && str[len(str)-1] == '"' {
		str = str[1 : len(str)-1]
	}
	if strings.Contains(str, "T") {
		str = strings.Split(str, "T")[0]
	}
	t, err := time.Parse("2006-01-02", str)
	if err != nil {
		return err
	}
	*d = TargetDate{t}
	return nil
}

func (d *TargetDate) MarshalJSON() ([]byte, error) {
	return []byte(d.Time.Format("\"2006-01-02\"")), nil
}

func (d *TargetDate) Value() (driver.Value, error) {
	if d == nil {
		return nil, nil
	}
	return d.Time, nil
}

func (d *TargetDate) Scan(value interface{}) error {
	t, ok := value.(time.Time)
	if !ok {
		return fmt.Errorf("error unmarshal time: %v", value)
	}
	*d = TargetDate{t}
	return nil
}

func (d TargetDate) String() string {
	return d.Time.String()
}

func (td *TargetDate) ToNullTime() sql.NullTime {
	if td == nil {
		return sql.NullTime{Valid: false}
	}
	return sql.NullTime{
		Time:  td.Time,
		Valid: true,
	}
}

// TargetDateTimeZ type
type TargetDateTimeZ struct {
	Time time.Time
}

func (d *TargetDateTimeZ) UnmarshalJSON(b []byte) error {
	str := string(b)
	if str != "" && str[0] == '"' && str[len(str)-1] == '"' {
		str = str[1 : len(str)-1]
	}
	date, err := formatDate(str)
	if err != nil {
		return err
	}
	*d = TargetDateTimeZ{date}
	return nil
}

func (d *TargetDateTimeZ) MarshalJSON() ([]byte, error) {
	str, err := formatDateStr(d.Time.String(), time.RFC3339, nil)
	if err != nil {
		return nil, err
	}
	return []byte(`"` + str + `"`), nil
}

func (d *TargetDateTimeZ) Value() (driver.Value, error) {
	if d == nil {
		return nil, nil
	}
	return d.Time, nil
}

func (d *TargetDateTimeZ) Scan(value interface{}) error {
	t, ok := value.(time.Time)
	if !ok {
		return fmt.Errorf("error unmarshal time: %v", value)
	}
	*d = TargetDateTimeZ{t}
	return nil
}

func (d TargetDateTimeZ) String() string {
	return d.Time.String()
}

// TimeZone type
type TimeZone time.Location

func (tz *TimeZone) Scan(value interface{}) error {
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("failed to unmarshal timezone value: %v", value)
	}

	if str == "" {
		str = "Europe/Moscow"
	}

	loc, err := time.LoadLocation(str)
	if err != nil {
		return err
	}
	*tz = TimeZone(*loc)
	return nil
}

func (tz TimeZone) Value() (driver.Value, error) {
	loc := time.Location(tz)
	return (&loc).String(), nil
}

func (TimeZone) GormDataType() string {
	return "text"
}

func (tz TimeZone) MarshalJSON() ([]byte, error) {
	loc := time.Location(tz)
	return []byte(fmt.Sprintf("\"%s\"", &loc)), nil
}

// Theme type
type Theme struct {
	System    *bool `json:"system,omitempty" extensions:"x-nullable"`
	Dark      *bool `json:"dark,omitempty" extensions:"x-nullable"`
	Contrast  *bool `json:"contrast,omitempty" extensions:"x-nullable"`
	OpenInNew *bool `json:"open_in_new,omitempty" extensions:"x-nullable"`
}

func (theme Theme) Value() (driver.Value, error) {
	b, err := json.Marshal(theme)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (theme *Theme) Scan(value interface{}) error {
	if value == nil {
		*theme = Theme{}
		return nil
	}

	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return errors.New(fmt.Sprint("Failed to unmarshal JSONB value:", value))
	}

	return json.Unmarshal(bytes, theme)
}

// UserSettings type
type UserSettings struct {
	DeadlineNotification  time.Duration `json:"deadline_notification"`
	TgNotificationMute    bool          `json:"telegram_notification_mute"`
	EmailNotificationMute bool          `json:"email_notification_mute"`
	AppNotificationMute   bool          `json:"app_notification_mute"`
}

func (us UserSettings) Value() (driver.Value, error) {
	b, err := json.Marshal(us)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (us *UserSettings) Scan(value interface{}) error {
	if value == nil {
		*us = UserSettings{}
		return nil
	}

	var res []byte
	switch v := value.(type) {
	case []byte:
		res = v
	case string:
		res = []byte(v)
	default:
		return errors.New(fmt.Sprint("Failed to unmarshal JSONB value:", value))
	}

	return json.Unmarshal(res, us)
}
func (us UserSettings) IsEmpty() bool {
	b, _ := json.Marshal(us)
	return string(b) == "{}"
}

// RedactorHTML type
type RedactorHTML struct {
	Body             string
	stripped         string
	AlreadySanitized bool
}

func (r RedactorHTML) Value() (driver.Value, error) {
	if !r.AlreadySanitized {
		return policy.UgcPolicy.Sanitize(r.Body), nil
	}
	return r.Body, nil
}

func (r *RedactorHTML) Scan(value interface{}) error {
	if s, ok := value.(string); ok {
		r.Body = s
		return nil
	}
	return errors.New("unsupported type")
}

func (r RedactorHTML) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(r.Body); err != nil {
		return nil, err
	}

	return bytes.TrimSpace(buf.Bytes()), nil
}

func (r *RedactorHTML) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &r.Body); err != nil {
		return err
	}
	r.Body = policy.UgcPolicy.Sanitize(r.Body)
	r.Body = RemoveInvisibleChars(r.Body)
	r.AlreadySanitized = true

	return nil
}

func (r *RedactorHTML) StripTags() string {
	if r.stripped == "" {
		r.stripped = policy.StripTagsPolicy.Sanitize(r.Body)
		r.Body = RemoveInvisibleChars(r.Body)
	}
	return r.stripped
}

func (r RedactorHTML) String() string {
	return r.Body
}

func (RedactorHTML) GormDataType() string {
	return "text"
}

func RemoveInvisibleChars(s string) string {
	invisible := []string{
		"\u200B",
		"\u200C",
		"\u200D",
		"\uFEFF",
	}

	for _, ch := range invisible {
		s = strings.ReplaceAll(s, ch, "")
	}
	return s
}

// FormFieldsSlice type
type FormFieldsSlice []FormFields

type FormFields struct {
	Type     string          `json:"type"`
	Label    string          `json:"label,omitempty"`
	Val      interface{}     `json:"value,omitempty"`
	Required bool            `json:"required"`
	Validate *ValidationRule `json:"validate,omitempty" extensions:"x-nullable"`
}

type ValidationRule struct {
	ValidationType string        `json:"validation_type"`
	ValueType      string        `json:"value_type,omitempty"`
	Opt            []interface{} `json:"opt,omitempty"`
}

func (f FormFieldsSlice) Value() (driver.Value, error) {
	b, err := json.Marshal(f)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (f *FormFieldsSlice) Scan(value interface{}) error {
	if value == nil {
		*f = FormFieldsSlice{}
		return nil
	}

	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return errors.New(fmt.Sprint("Failed to unmarshal JSONB value:", value))
	}

	if err := json.Unmarshal(bytes, f); err != nil {
		return err
	}
	return nil
}

// IssuesListFilters type
type IssuesListFilters struct {
	AuthorIds   []string `json:"authors"`
	AssigneeIds []string `json:"assignees"`
	WatcherIds  []string `json:"watchers"`

	StateIds       []uuid.UUID `json:"states"`
	Priorities     []string    `json:"priorities"`
	Labels         []string    `json:"labels"`
	WorkspaceIds   []string    `json:"workspaces"`
	WorkspaceSlugs []string    `json:"workspace_slugs"`
	ProjectIds     []string    `json:"projects"`
	SprintIds      []string    `json:"sprints"`

	OnlyActive   bool `json:"only_active"`
	AssignedToMe bool `json:"assigned_to_me"`
	WatchedByMe  bool `json:"watched_by_me"`
	AuthoredByMe bool `json:"authored_by_me"`

	SearchQuery string `json:"search_query"`
}

func (filter *IssuesListFilters) Scan(value interface{}) error {
	if value == nil {
		*filter = IssuesListFilters{}
		return nil
	}

	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return errors.New(fmt.Sprint("Failed to unmarshal JSONB value:", value))
	}

	return json.Unmarshal(bytes, filter)
}

// NullDomain type
type NullDomain struct {
	URL   *url.URL
	Valid bool
}

func (d *NullDomain) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	raw, ok := value.(string)
	if !ok {
		return fmt.Errorf("failed unmarshal domain url: %v", value)
	}

	if raw == "" {
		return nil
	}

	u, err := url.ParseRequestURI(raw)
	if err != nil {
		return fmt.Errorf("failed unmarshal domain url: %w", err)
	}
	d.URL = u
	d.Valid = true
	return nil
}

func (d NullDomain) Value() (driver.Value, error) {
	if !d.Valid {
		return nil, nil
	}
	return d.URL.Scheme + "://" + d.URL.Host, nil
}

func (d *NullDomain) String() string {
	if !d.Valid {
		return ""
	}
	return d.URL.Scheme + "://" + d.URL.Host
}

// TsVector Postgres tsvector type
type TsVector struct {
	Vector string
}

func (TsVector) GormDataType() string {
	return "tsvector"
}

func (ts TsVector) GormValue(ctx context.Context, db *gorm.DB) clause.Expr {
	return clause.Expr{
		SQL:  "to_tsvector('russian', ?)",
		Vars: []interface{}{ts.Vector},
	}
}

func (ts *TsVector) Scan(v interface{}) error {
	if str, ok := v.(string); ok {
		*ts = TsVector{str}
		return nil
	}
	return errors.New("incorrect type of tsvector")
}

func (ts *TsVector) String() string {
	return ts.Vector
}

// JSONField generic field
type JSONField[T any] struct {
	Value   *T
	Defined bool
}

func (j *JSONField[T]) UnmarshalJSON(data []byte) error {
	j.Defined = true

	if string(data) == "null" {
		j.Value = nil
		return nil
	}

	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("invalid value for type %T: %s", value, err.Error())
	}
	j.Value = &value
	return nil
}

func (j JSONField[T]) MarshalJSON() ([]byte, error) {
	if !j.Defined {
		return []byte("null"), nil
	}
	if j.Value == nil {
		return []byte("null"), nil
	}
	return json.Marshal(*j.Value)
}

func (j JSONField[T]) String() string {
	if j.Value == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%v", *j.Value)
}

func (j JSONField[T]) IsDefined() bool {
	return j.Defined
}

func (j JSONField[T]) IsNull() bool {
	return j.Defined && j.Value == nil
}

func (j JSONField[T]) GetValue() (*T, bool) {
	return j.Value, j.Defined
}

// ProjectMemberNS type
type ProjectMemberNS struct {
	DisableName          bool `json:"disable_name"`
	DisableDesc          bool `json:"disable_desc"`
	DisableState         bool `json:"disable_state"`
	DisableAssignees     bool `json:"disable_assignees"`
	DisableWatchers      bool `json:"disable_watchers"`
	DisablePriority      bool `json:"disable_priority"`
	DisableParent        bool `json:"disable_parent"`
	DisableBlocks        bool `json:"disable_blocks"`
	DisableBlockedBy     bool `json:"disable_blockedBy"`
	DisableTargetDate    bool `json:"disable_targetDate"`
	DisableLabels        bool `json:"disable_labels"`
	DisableLinks         bool `json:"disable_links"`
	DisableComments      bool `json:"disable_comments"`
	DisableAttachments   bool `json:"disable_attachments"`
	DisableDeadline      bool `json:"disable_deadline"`
	DisableLinked        bool `json:"disable_linked"`
	DisableSubIssue      bool `json:"disable_sub_issue"`
	NotifyBeforeDeadline *int `json:"notify_before_deadline" extensions:"x-nullable"`
	DisableIssueTransfer bool `json:"disable_issue_transfer"`
	DisableIssueNew      bool `json:"disable_issue_new"`

	DisableProjectName            bool `json:"disable_project_name"`
	DisableProjectPublic          bool `json:"disable_project_public"`
	DisableProjectIdentifier      bool `json:"disable_project_identifier"`
	DisableProjectDefaultAssignee bool `json:"disable_project_default_assignee"`
	DisableProjectDefaultWatcher  bool `json:"disable_project_default_watcher"`
	DisableProjectMember          bool `json:"disable_project_member"`
	DisableProjectOwner           bool `json:"disable_project_owner"`
	DisableProjectRole            bool `json:"disable_project_role"`
	DisableProjectStatus          bool `json:"disable_project_status"`
	DisableProjectLabel           bool `json:"disable_project_label"`
	DisableProjectLogo            bool `json:"disable_project_logo"`
	DisableProjectTemplate        bool `json:"disable_project_template"`
}

func (ns ProjectMemberNS) Value() (driver.Value, error) {
	b, err := json.Marshal(ns)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (ns *ProjectMemberNS) Scan(value interface{}) error {
	if value == nil {
		*ns = ProjectMemberNS{}
		return nil
	}

	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return errors.New(fmt.Sprint("Failed to unmarshal JSONB value:", value))
	}

	if err := json.Unmarshal(bytes, ns); err != nil {
		return err
	}
	return nil
}

func (ns ProjectMemberNS) IsNotify(field *string, entity string, verb string, role int) bool {
	if field == nil {
		return false
	}

	isIssue := entity == "issue"
	isProject := entity == "project"
	isPrAdmin := entity == "project" && role == AdminRole

	switch actField.ActivityField(*field) {
	case actField.Name.Field:
		if isIssue {
			return !ns.DisableName
		}
		if isPrAdmin {
			return !ns.DisableProjectName
		}

	case actField.Issue.Field:
		if isProject {
			return !ns.DisableIssueNew
		}

		if isIssue {
			return !ns.DisableIssueTransfer
		}
	case
		actField.Logo.Field,
		actField.Emoj.Field:
		if isProject {
			return !ns.DisableProjectLogo
		}

	case actField.Parent.Field:
		if isIssue {
			return !ns.DisableParent
		}
	case actField.Priority.Field:
		if isIssue {
			return !ns.DisablePriority
		}
	case actField.Status.Field:
		if isIssue {
			return !ns.DisableState
		}
		if isPrAdmin {
			return !ns.DisableProjectStatus
		}
	case actField.Description.Field:
		if isIssue {
			return !ns.DisableDesc
		}
	case actField.TargetDate.Field:
		if isIssue {
			return !ns.DisableTargetDate
		}
	case actField.Label.Field:
		if isIssue {
			return !ns.DisableLabels
		}
		if isPrAdmin {
			return !ns.DisableProjectLabel
		}
	case actField.Assignees.Field:
		if isIssue {
			return !ns.DisableAssignees
		}
	case actField.Watchers.Field:
		if isIssue {
			return !ns.DisableWatchers
		}
	case actField.Blocks.Field:
		if isIssue {
			return !ns.DisableBlocks
		}
	case actField.Blocking.Field:
		if isIssue {
			return !ns.DisableBlockedBy
		}
	case
		actField.Link.Field,
		actField.LinkTitle.Field,
		actField.LinkUrl.Field:
		if isIssue {
			return !ns.DisableLinks
		}
	case actField.Comment.Field:
		if isIssue {
			return !ns.DisableComments
		}
	case actField.Attachment.Field:
		if isIssue {
			return !ns.DisableAttachments
		}
	case actField.Linked.Field:
		if isIssue {
			return !ns.DisableLinked
		}
	case actField.SubIssue.Field:
		if isIssue {
			return !ns.DisableSubIssue
		}
	case actField.Deadline.Field:
		if isIssue {
			return !ns.DisableDeadline
		}
	case actField.Project.Field:
		if isIssue {
			return !ns.DisableIssueTransfer
		}
	case actField.Public.Field:
		if isPrAdmin {
			return !ns.DisableProjectPublic
		}
	case actField.Identifier.Field:
		if isPrAdmin {
			return !ns.DisableProjectIdentifier
		}
	case actField.DefaultAssignees.Field:
		if isPrAdmin {
			return !ns.DisableProjectDefaultAssignee
		}
	case actField.DefaultWatchers.Field:
		if isPrAdmin {
			return !ns.DisableProjectDefaultWatcher
		}
	case actField.Member.Field:
		if isPrAdmin {
			return !ns.DisableProjectMember
		}
	case actField.ProjectLead.Field:
		if isPrAdmin {
			return !ns.DisableProjectOwner
		}
	case actField.Role.Field:
		if isPrAdmin {
			return !ns.DisableProjectRole
		}
	case
		actField.StatusDefault.Field,
		actField.StatusName.Field,
		actField.StatusColor.Field,
		actField.StatusDescription.Field,
		actField.StatusGroup.Field:
		if isPrAdmin {
			return !ns.DisableProjectStatus
		}
	case actField.LabelName.Field, actField.LabelColor.Field:
		if isPrAdmin {
			return !ns.DisableProjectLabel
		}
	case
		actField.Template.Field,
		actField.TemplateTemplate.Field,
		actField.TemplateName.Field:
		if isPrAdmin {
			return !ns.DisableProjectTemplate
		}
	}
	return false
}

// WorkspaceMemberNS type
type WorkspaceMemberNS struct {
	DisableDocTitle      bool `json:"disable_doc_title"`
	DisableDocDesc       bool `json:"disable_doc_desc"`
	DisableDocRole       bool `json:"disable_doc_role"`
	DisableDocAttachment bool `json:"disable_doc_attachment"`
	DisableDocComment    bool `json:"disable_doc_comment"`

	DisableDocWatchers bool `json:"disable_doc_watchers"`
	DisableDocCreate   bool `json:"disable_doc_create"`
	DisableDocDelete   bool `json:"disable_doc_delete"`
	DisableDocMove     bool `json:"disable_doc_move"`

	DisableWorkspaceProject     bool `json:"disable_workspace_project"`
	DisableWorkspaceForm        bool `json:"disable_workspace_form"`
	DisableWorkspaceDoc         bool `json:"disable_workspace_doc"`
	DisableWorkspaceName        bool `json:"disable_workspace_name"`
	DisableWorkspaceOwner       bool `json:"disable_workspace_owner"`
	DisableWorkspaceDesc        bool `json:"disable_workspace_desc"`
	DisableWorkspaceToken       bool `json:"disable_workspace_token"`
	DisableWorkspaceLogo        bool `json:"disable_workspace_logo"`
	DisableWorkspaceMember      bool `json:"disable_workspace_member"`
	DisableWorkspaceRole        bool `json:"disable_workspace_role"`
	DisableWorkspaceIntegration bool `json:"disable_workspace_integration"`
}

func (ns WorkspaceMemberNS) Value() (driver.Value, error) {
	b, err := json.Marshal(ns)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (ns *WorkspaceMemberNS) Scan(value interface{}) error {
	if value == nil {
		*ns = WorkspaceMemberNS{}
		return nil
	}

	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return errors.New(fmt.Sprint("Failed to unmarshal JSONB value:", value))
	}

	if err := json.Unmarshal(bytes, ns); err != nil {
		return err
	}
	return nil
}

func (ns WorkspaceMemberNS) IsNotify(field *string, entity string, verb string, role int) bool {
	if field == nil {
		return false
	}

	isDoc := entity == "doc"
	isWorkspace := entity == "workspace"
	isWorkspaceAdmin := entity == "workspace" && role == AdminRole

	switch actField.ActivityField(*field) {
	case actField.Title.Field:
		if isDoc {
			return !ns.DisableDocTitle
		}
	case actField.Description.Field:
		if isDoc {
			return !ns.DisableDocDesc
		}
		if isWorkspaceAdmin {
			return !ns.DisableWorkspaceDesc
		}
	case
		actField.ReaderRole.Field,
		actField.EditorRole.Field,
		actField.Editors.Field,
		actField.Readers.Field:
		if isDoc {
			return !ns.DisableDocRole
		}
	case actField.Attachment.Field:
		if isDoc {
			return !ns.DisableDocAttachment
		}
	case actField.Comment.Field:
		if isDoc {
			return !ns.DisableDocComment
		}
	case actField.Watchers.Field:
		if isDoc {
			return !ns.DisableDocWatchers
		}
	case actField.Doc.Field:
		if isDoc {
			switch verb {
			case "created":
				return !ns.DisableDocCreate
			case "deleted":
				return !ns.DisableDocDelete
			case "move_workspace_to_doc", "move_doc_to_workspace", "move_doc_to_doc", "added", "removed":
				return !ns.DisableDocMove
			}
		}
		if isWorkspace {
			switch verb {
			case "created":
				return !ns.DisableDocCreate
			case "added", "removed":
				return !ns.DisableDocMove
			case "deleted":
				return !ns.DisableDocDelete
			}

		}
		if isWorkspaceAdmin {
			return !ns.DisableWorkspaceDoc
		}
	case actField.Project.Field:
		if isWorkspaceAdmin {
			return !ns.DisableWorkspaceProject
		}
	case actField.Form.Field:
		if isWorkspaceAdmin {
			return false // TODO disabled BAK-317
			//return !ns.DisableWorkspaceForm
		}
	case actField.Name.Field:
		if isWorkspaceAdmin {
			return !ns.DisableWorkspaceName
		}
	case actField.Token.Field:
		if isWorkspaceAdmin {
			return !ns.DisableWorkspaceToken
		}
	case actField.Logo.Field:
		if isWorkspaceAdmin {
			return !ns.DisableWorkspaceLogo
		}
	case actField.Member.Field:
		if isWorkspaceAdmin {
			return !ns.DisableWorkspaceMember
		}
	case actField.Role.Field:
		if isWorkspaceAdmin {
			return !ns.DisableWorkspaceRole
		}
	case actField.Integration.Field:
		if isWorkspaceAdmin {
			return !ns.DisableWorkspaceIntegration
		}
	case actField.WorkspaceOwner.Field:
		if isWorkspaceAdmin {
			return !ns.DisableWorkspaceOwner
		}
	}

	return false
}

type JsonURL struct {
	Url *url.URL `swaggertype:"string" format:"uri"`
}

func (u *JsonURL) MarshalJSON() ([]byte, error) {
	if u == nil || u.Url == nil {
		return []byte("null"), nil
	}
	return []byte("\"" + u.Url.String() + "\""), nil
}

type IssueStatus int

const (
	Pending IssueStatus = iota
	InProgress
	Completed
	Cancelled
)

func (is IssueStatus) String() string {
	return [...]string{"Pending", "InProgress", "Completed", "Cancelled"}[is]
}

type IssueProcess struct {
	Status  IssueStatus `json:"status"`
	Overdue bool        `json:"overdue"`
}

type SprintStats struct {
	AllIssues  int `json:"all_issues"`
	Pending    int `json:"pending"`
	InProgress int `json:"in_progress"`
	Completed  int `json:"completed"`
	Cancelled  int `json:"cancelled"`
}

// -----
func formatDateStr(dateStr, outFormat string, tz *TimeZone) (string, error) {
	date, err := formatDate(dateStr)
	if err != nil {
		return "", err
	}

	if tz != nil {
		date = date.In((*time.Location)(tz))
	}
	return date.Format(outFormat), nil

}

func formatDate(dateStr string) (time.Time, error) {
	if dateStr == "" {
		return time.Time{}, fmt.Errorf("empty date string")
	}

	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02T15:04:05.000Z",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04:05 -0700 MST",
		"2006-01-02",
		"02.01.2006 15:04 MST",
		"02.01.2006 15:04 -0700",
		"02.01.2006",
	}

	var t time.Time
	var err error
	for _, layout := range layouts {
		t, err = time.Parse(layout, dateStr)
		if err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unsuported date format")
}
