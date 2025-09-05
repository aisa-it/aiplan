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
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	policy "sheff.online/aiplan/internal/aiplan/redactor-policy"
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

	StateIds       []string `json:"states"`
	Priorities     []string `json:"priorities"`
	Labels         []string `json:"labels"`
	WorkspaceIds   []string `json:"workspaces"`
	WorkspaceSlugs []string `json:"workspace_slugs"`
	ProjectIds     []string `json:"projects"`

	AssignedToMe bool `json:"assigned_to_me"`
	WatchedByMe  bool `json:"watched_by_me"`
	AuthoredByMe bool `json:"authored_by_me"`
	OnlyActive   bool `json:"only_active"`

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
	DisableProjectIssue           bool `json:"disable_project_issue"`
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

	switch *field {
	case "name":
		if isIssue {
			return !ns.DisableName
		}
		if isPrAdmin {
			return !ns.DisableProjectName
		}
	case "issue":
		if isProject {
			if isPrAdmin {
				return !ns.DisableProjectIssue
			}
			return !ns.DisableIssueNew
		}

	case "parent":
		if isIssue {
			return !ns.DisableParent
		}
	case "priority":
		if isIssue {
			return !ns.DisablePriority
		}
	case "state":
		if isIssue {
			return !ns.DisableState
		}
		if isPrAdmin {
			return !ns.DisableProjectStatus
		}
	case "description":
		if isIssue {
			return !ns.DisableDesc
		}
	case "target_date":
		if isIssue {
			return !ns.DisableTargetDate
		}
	case "labels":
		if isIssue {
			return !ns.DisableLabels
		}
	case "assignees":
		if isIssue {
			return !ns.DisableAssignees
		}
	case "watchers":
		if isIssue {
			return !ns.DisableWatchers
		}
	case "blocks":
		if isIssue {
			return !ns.DisableBlocks
		}
	case "blocking":
		if isIssue {
			return !ns.DisableBlockedBy
		}
	case "link":
		if isIssue {
			return !ns.DisableLinks
		}
	case "comment":
		if isIssue {
			return !ns.DisableComments
		}
	case "attachment":
		if isIssue {
			return !ns.DisableAttachments
		}
	case "linked":
		if isIssue {
			return !ns.DisableLinked
		}
	case "sub_issue":
		if isIssue {
			return !ns.DisableSubIssue
		}
	case "deadline":
		if isIssue {
			return !ns.DisableDeadline
		}
	case "project":
		if isIssue {
			return !ns.DisableIssueTransfer
		}
	case "public":
		if isPrAdmin {
			return !ns.DisableProjectPublic
		}
	case "identifier":
		if isPrAdmin {
			return !ns.DisableProjectIdentifier
		}
	case "default_assignees":
		if isPrAdmin {
			return !ns.DisableProjectDefaultAssignee
		}
	case "default_watchers":
		if isPrAdmin {
			return !ns.DisableProjectDefaultWatcher
		}
	case "member":
		if isPrAdmin {
			return !ns.DisableProjectMember
		}
	case "owner":
		if isPrAdmin {
			return !ns.DisableProjectOwner
		}
	case "role":
		if isPrAdmin {
			return !ns.DisableProjectRole
		}
	case "status_default", "status_name", "status_color", "status_description", "status_group":
		if isPrAdmin {
			return !ns.DisableProjectStatus
		}
	case "label", "label_name", "label_color":
		if isPrAdmin {
			return !ns.DisableProjectLabel
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

	DisableDocEditor   bool `json:"disable_doc_editor"`
	DisableDocWatchers bool `json:"disable_doc_watchers"`
	DisableDocReader   bool `json:"disable_doc_reader"`
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
	isWorkspaceAdmin := entity == "workspace" && role == AdminRole

	switch *field {
	case "title":
		if isDoc {
			return !ns.DisableDocTitle
		}
	case "description":
		if isDoc {
			return !ns.DisableDocDesc
		}
		if isWorkspaceAdmin {
			return !ns.DisableWorkspaceDesc
		}
	case "reader_role", "editor_role":
		if isDoc {
			return !ns.DisableDocRole
		}
	case "attachment":
		if isDoc {
			return !ns.DisableDocAttachment
		}
	case "comment":
		if isDoc {
			return !ns.DisableDocComment
		}

	case "editors":
		if isDoc {
			return !ns.DisableDocEditor
		}
	case "watchers":
		if isDoc {
			return !ns.DisableDocWatchers
		}
	case "readers":
		if isDoc {
			return !ns.DisableDocReader
		}
	case "doc":
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
		if isWorkspaceAdmin {
			return !ns.DisableWorkspaceDoc
		}
	case "project":
		if isWorkspaceAdmin {
			return !ns.DisableWorkspaceProject
		}
	case "form":
		if isWorkspaceAdmin {
			return !ns.DisableWorkspaceForm
		}
	case "name":
		if isWorkspaceAdmin {
			return !ns.DisableWorkspaceName
		}
	case "integration_token":
		if isWorkspaceAdmin {
			return !ns.DisableWorkspaceToken
		}
	case "logo":
		if isWorkspaceAdmin {
			return !ns.DisableWorkspaceLogo
		}
	case "member":
		if isWorkspaceAdmin {
			return !ns.DisableWorkspaceMember
		}
	case "role":
		if isWorkspaceAdmin {
			return !ns.DisableWorkspaceRole
		}
	case "integration":
		if isWorkspaceAdmin {
			return !ns.DisableWorkspaceIntegration
		}
	case "owner":
		if isWorkspaceAdmin {
			return !ns.DisableWorkspaceOwner
		}
	}

	return false
}

type JsonURL struct {
	Url *url.URL
}

func (u *JsonURL) MarshalJSON() ([]byte, error) {
	if u == nil || u.Url == nil {
		return []byte("null"), nil
	}
	return []byte("\"" + u.Url.String() + "\""), nil
}
