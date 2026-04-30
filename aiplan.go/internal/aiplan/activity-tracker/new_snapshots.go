package tracker

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/opt"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
)

// SnapshotI - minimal interface for create/delete tracking
type SnapshotI interface {
	GetName() string
	GetID() uuid.UUID
	GetField() actField.ActivityField // optional - returns empty if not overridden
}

// ------ IssueSnapshot -------

type IssueSnapshot struct {
	ID           uuid.UUID
	Name         opt.Field[string]                 `act:"req:name;field:name;kind:scalar"`
	Assignees    opt.Field[[]EntityRef]            `act:"req:assignees_list;field:assignees;kind:collection;transform:uuid;table:users;preserve_id:true"`
	Watchers     opt.Field[[]EntityRef]            `act:"req:watchers_list;field:watchers;kind:collection;transform:uuid;table:users;preserve_id:true"`
	Description  opt.Field[string]                 `act:"req:description_html;field:description;kind:scalar"`
	Priority     opt.Field[string]                 `act:"req:priority;field:priority;kind:scalar"`
	State        opt.Field[EntityRef]              `act:"req:state;field:status;kind:scalar;transform:uuid;table:states;preserve_id:true"`
	TargetDate   opt.Field[*types.TargetDateTimeZ] `act:"req:target_date;field:target_date;kind:scalar"`
	StartDate    opt.Field[*types.TargetDateTimeZ] `act:"req:start_date;field:start_date;kind:scalar"`
	CompletedAt  opt.Field[*types.TargetDateTimeZ] `act:"req:completed_at;field:completed_at;kind:scalar"`
	Parent       opt.Field[EntityRef]              `act:"req:parent;field:parent;kind:scalar;transform:uuid;table:issues;preserve_id:true;linked_field:sub_issue"`
	BlockerList  opt.Field[[]EntityRef]            `act:"req:blocker_issues;field:blocking;kind:collection;table:issues;preserve_id:true;linked_field:blocks"`
	BlockedList  opt.Field[[]EntityRef]            `act:"req:blocked_issues;field:blocks;kind:collection;table:issues;preserve_id:true;linked_field:blocking"`
	SubIssues    opt.Field[[]EntityRef]            `act:"req:sub_issues;field:sub_issue;kind:collection;table:issues;preserve_id:true;linked_field:parent"`
	Links        opt.Field[[]EntityRef]            `act:"req:links;field:link;kind:collection;preserve_id:true"`
	LinkedIssues opt.Field[[]EntityRef]            `act:"req:linked_issues;field:linked;kind:collection;preserve_id:true;linked_field:linked"`
	Sprints      opt.Field[[]EntityRef]            `act:"req:sprints_list;field:sprint;kind:collection;transform:uuid;table:sprints;preserve_id:true;linked_field:issues"`
	Labels       opt.Field[[]EntityRef]            `act:"req:labels_list;field:label;kind:collection;transform:uuid;table:labels;preserve_id:true"`
}

func (i IssueSnapshot) GetName() string {
	if i.Name.IsSet() {
		return i.Name.Value()
	}
	return ""
}

func (i IssueSnapshot) GetID() uuid.UUID {
	return i.ID
}

func (i IssueSnapshot) GetField() actField.ActivityField {
	return actField.Issue.Field
}
func IssueToSnapshot(i dao.Issue, extraSubIssues ...dao.Issue) IssueSnapshot {
	return IssueSnapshot{
		ID:          i.ID,
		Name:        opt.Some(i.Name),
		Description: opt.Some(i.DescriptionHtml),
		Priority:    opt.Some(PtrToStr(i.Priority)),
		TargetDate:  opt.Some(i.TargetDate),
		StartDate:   opt.Some(i.StartDate),
		CompletedAt: opt.Some(i.CompletedAt),
		Assignees:   opt.Some(utils.SliceToSlice(i.Assignees, func(t *dao.User) EntityRef { return daoToEntityRef(t) })),
		Watchers:    opt.Some(utils.SliceToSlice(i.Watchers, func(t *dao.User) EntityRef { return daoToEntityRef(t) })),
		State:       opt.Some(daoToEntityRef(i.State)),

		BlockerList: opt.Some(utils.SliceToSlice(&i.BlockerIssuesIDs, func(t *dao.IssueBlocker) EntityRef {
			t.BlockedBy.Project = i.Project
			return daoToEntityRef(t.BlockedBy)
		})),
		BlockedList: opt.Some(utils.SliceToSlice(&i.BlockedIssuesIDs, func(t *dao.IssueBlocker) EntityRef {
			t.Block.Project = i.Project
			return daoToEntityRef(t.Block)
		})),
		SubIssues: func() opt.Field[[]EntityRef] {
			if len(extraSubIssues) > 0 {
				return opt.Some(utils.SliceToSlice(&extraSubIssues, func(t *dao.Issue) EntityRef { return daoToEntityRef(t) }))
			}
			return opt.None[[]EntityRef]()
		}(),
		Parent: func() opt.Field[EntityRef] {
			if i.ParentId.Valid && i.Parent != nil {
				i.Parent.Project = i.Project
				return opt.Some(EntityRef{ID: i.ParentId.UUID, NameValue: i.Parent.String(), NameField: "issue"})
			}
			return opt.None[EntityRef]()
		}(),
		Links: opt.Some(utils.SliceToSlice(i.Links, func(t *dao.IssueLink) EntityRef {
			return EntityRef{ID: t.Id, NameValue: t.Url, NameField: "link"}
		})),
		Labels: opt.Some(utils.SliceToSlice(i.Labels, func(t *dao.Label) EntityRef {
			return EntityRef{ID: t.ID, NameValue: t.Name, NameField: "label"}
		})),
		LinkedIssues: func() opt.Field[[]EntityRef] {

			refs := make([]EntityRef, len(i.LinkedIssues))
			for j, li := range i.LinkedIssues {
				li.Project = i.Project
				refs[j] = EntityRef{ID: li.ID, NameValue: li.String(), NameField: "issue"}
			}
			return opt.Some(refs)
		}(),
		Sprints: func() opt.Field[[]EntityRef] {
			if i.Sprints != nil {
				return opt.Some(utils.SliceToSlice(i.Sprints, func(t *dao.Sprint) EntityRef { return daoToEntityRef(t) }))
			}
			return opt.None[[]EntityRef]()
		}(),
	}
}

// ------ LabelSnapshot -------

type LabelSnapshot struct {
	ID          uuid.UUID
	Name        opt.Field[string] `act:"req:name;field:label_name;kind:scalar;preserve_id:true"`
	Color       opt.Field[string] `act:"req:color;field:label_color;kind:scalar;preserve_id:true"`
	Description opt.Field[string] `act:"req:description;field:label_description;kind:scalar;preserve_id:true"`
}

func (l LabelSnapshot) GetName() string {
	if l.Name.IsSet() {
		return l.Name.Value()
	}
	return ""
}

func (l LabelSnapshot) GetID() uuid.UUID {
	return l.ID
}

func (l LabelSnapshot) GetField() actField.ActivityField {
	return actField.Label.Field
}

func LabelToSnapshot(label *dao.Label) LabelSnapshot {
	return LabelSnapshot{
		ID:          label.ID,
		Name:        opt.Some(label.Name),
		Color:       opt.Some(label.Color),
		Description: opt.Some(label.Description),
	}
}

// ------ StateSnapshot -------

type StateSnapshot struct {
	ID          uuid.UUID
	Name        opt.Field[string] `act:"req:name;field:status_name;kind:scalar;preserve_id:true"`
	Description opt.Field[string] `act:"req:description;field:status_description;kind:scalar;preserve_id:true"`
	Color       opt.Field[string] `act:"req:color;field:status_color;kind:scalar;preserve_id:true"`
	Group       opt.Field[string] `act:"req:group;field:status_group;kind:scalar;preserve_id:true"`
	Default     opt.Field[bool]   `act:"req:default;field:status_default;kind:scalar;preserve_id:true"`
}

func (s StateSnapshot) GetName() string {
	if s.Name.IsSet() {
		return s.Name.Value()
	}
	return ""
}

func (s StateSnapshot) GetID() uuid.UUID {
	return s.ID
}

func (s StateSnapshot) GetField() actField.ActivityField {
	return actField.Status.Field
}

func StateToSnapshot(state *dao.State) StateSnapshot {
	return StateSnapshot{
		ID:          state.ID,
		Name:        opt.Some(state.Name),
		Description: opt.Some(state.Description),
		Color:       opt.Some(state.Color),
		Group:       opt.Some(state.Group),
		Default:     opt.Some(state.Default),
	}
}

// ------ IssueTemplateSnapshot -------

type IssueTemplateSnapshot struct {
	ID       uuid.UUID
	Name     opt.Field[string] `act:"req:name;field:template_name;kind:scalar;preserve_id:true"`
	Template opt.Field[string] `act:"req:template;field:template_template;kind:scalar;preserve_id:true"`
}

func (it IssueTemplateSnapshot) GetName() string {
	if it.Name.IsSet() {
		return it.Name.Value()
	}
	return ""
}

func (it IssueTemplateSnapshot) GetID() uuid.UUID {
	return it.ID
}

func (it IssueTemplateSnapshot) GetField() actField.ActivityField {
	return actField.Template.Field
}

func IssueTemplateToSnapshot(template *dao.IssueTemplate) IssueTemplateSnapshot {
	return IssueTemplateSnapshot{
		ID:       template.Id,
		Name:     opt.Some(template.Name),
		Template: opt.Some(template.Template.String()),
	}
}

// ------ WorkspaceSnapshot -------

type WorkspaceSnapshot struct {
	ID          uuid.UUID
	Name        opt.Field[string]      `act:"req:name;field:title;kind:scalar"`
	Description opt.Field[string]      `act:"req:description;field:description;kind:scalar"`
	LogoId      opt.Field[uuid.UUID]   `act:"req:logo_id;field:logo;kind:scalar"`
	OwnerId     opt.Field[uuid.UUID]   `act:"req:owner_id;field:owner;kind:ref"`
	Token       opt.Field[EntityRef]   `act:"req:integration_token;field:integration_token;kind:scalar;secret:true"`
	Integration opt.Field[[]EntityRef] `act:"req:integration;field:integration;kind:collection"`
	Members     opt.Field[[]EntityRef] `act:"req:member;field:member;kind:collection;transform:uuid;table:users;preserve_id:true"`
}

func (w WorkspaceSnapshot) GetName() string {
	if w.Name.IsSet() {
		return w.Name.Value()
	}
	return ""
}

func (w WorkspaceSnapshot) GetID() uuid.UUID {
	return w.ID
}

func (w WorkspaceSnapshot) GetField() actField.ActivityField {
	return actField.Workspace.Field
}

type WorkspaceEnricher func(*WorkspaceSnapshot)

func WorkspaceToSnapshot(workspace *dao.Workspace, enrichers ...WorkspaceEnricher) WorkspaceSnapshot {
	snapshot := WorkspaceSnapshot{
		ID:          workspace.ID,
		Name:        opt.Some(workspace.Name),
		Description: opt.Some(workspace.Description.String()),
		LogoId:      opt.Some(workspace.LogoId.UUID),
		OwnerId:     opt.Some(workspace.OwnerId),
		Token: opt.Some(func() EntityRef {
			return EntityRef{
				ID:        uuid.UUID{},
				NameValue: workspace.IntegrationToken,
			}
		}()),
	}

	for _, enricher := range enrichers {
		enricher(&snapshot)
	}

	return snapshot
}

func WithIntegration(id uuid.UUID, name string) WorkspaceEnricher {
	return func(s *WorkspaceSnapshot) {
		s.Integration = opt.Some([]EntityRef{{ID: id, NameValue: name}})
	}
}

func WithWorkspaceMembers(members []dao.WorkspaceMember, getNameValue func(m dao.WorkspaceMember) string) WorkspaceEnricher {
	return func(s *WorkspaceSnapshot) {
		refs := make([]EntityRef, len(members))
		for i, m := range members {
			refs[i] = EntityRef{
				ID:        m.GetId(),
				NameValue: getNameValue(m),
				NameField: string(m.GetEntityType()),
			}
		}
		s.Members = opt.Some(refs)
	}
}

// ------ DocSnapshot -------

type DocSnapshot struct {
	ID         uuid.UUID
	Title      opt.Field[string]      `act:"req:title;field:title;kind:scalar"`
	Content    opt.Field[string]      `act:"req:description;field:description;kind:scalar"`
	EditorRole opt.Field[int]         `act:"req:editor_role;field:editor_role;kind:scalar;preserve_id:true"`
	ReaderRole opt.Field[int]         `act:"req:reader_role;field:reader_role;kind:scalar;preserve_id:true"`
	Parent     opt.Field[EntityRef]   `act:"req:parent_doc_id;field:parent;kind:scalar;transform:uuid;table:docs;preserve_id:true"`
	Editors    opt.Field[[]EntityRef] `act:"req:editors_list;field:editors;kind:collection;transform:uuid;preserve_id:true"`
	Readers    opt.Field[[]EntityRef] `act:"req:readers_list;field:readers;kind:collection;transform:uuid;preserve_id:true"`
	Watchers   opt.Field[[]EntityRef] `act:"req:watchers_list;field:watchers;kind:collection;transform:uuid;preserve_id:true"`
}

func (d DocSnapshot) GetName() string {
	if d.Title.IsSet() {
		return d.Title.Value()
	}
	return ""
}

func (d DocSnapshot) GetID() uuid.UUID {
	return d.ID
}

func (d DocSnapshot) GetField() actField.ActivityField {
	return actField.Doc.Field
}

func DocToSnapshot(doc *dao.Doc) DocSnapshot {
	snapshot := DocSnapshot{
		ID:         doc.ID,
		Title:      opt.Some(doc.Title),
		Content:    opt.Some(doc.Content.String()),
		EditorRole: opt.Some(doc.EditorRole),
		ReaderRole: opt.Some(doc.ReaderRole),
		Watchers:   opt.Some(utils.SliceToSlice(doc.Watchers, func(t *dao.User) EntityRef { return daoToEntityRef(t) })),
		Readers:    opt.Some(utils.SliceToSlice(doc.Readers, func(t *dao.User) EntityRef { return daoToEntityRef(t) })),
		Editors:    opt.Some(utils.SliceToSlice(doc.Editors, func(t *dao.User) EntityRef { return daoToEntityRef(t) })),
	}

	if doc.ParentDoc != nil {
		snapshot.Parent = opt.Some(daoToEntityRef(doc.ParentDoc))
	} else if doc.ParentDocID.Valid {
		snapshot.Parent = opt.Some(EntityRef{ID: doc.ParentDocID.UUID, NameValue: doc.Title, NameField: "docs"})
	}
	return snapshot
}

// ------ ProjectSnapshot -------

type ProjectSnapshot struct {
	ID               uuid.UUID
	Name             opt.Field[string]      `act:"req:name;field:name;kind:scalar"`
	Public           opt.Field[bool]        `act:"req:public;field:public;kind:scalar"`
	Identifier       opt.Field[string]      `act:"req:identifier;field:identifier;kind:scalar"`
	ProjectLead      opt.Field[EntityRef]   `act:"req:project_lead;field:project_lead;kind:scalar;transform:uuid;table:users;preserve_id:true"`
	Emoji            opt.Field[int32]       `act:"req:emoji;field:emoji;kind:scalar"`
	LogoId           opt.Field[uuid.UUID]   `act:"req:logo_id;field:logo;kind:scalar"`
	RulesScript      opt.Field[string]      `act:"req:rules_script;field:rules_script;kind:scalar"`
	DefaultAssignees opt.Field[[]EntityRef] `act:"req:default_assignees;field:default_assignees;kind:collection;transform:uuid;table:users;preserve_id:true"`
	DefaultWatchers  opt.Field[[]EntityRef] `act:"req:default_watchers;field:default_watchers;kind:collection;transform:uuid;table:users;preserve_id:true"`
	Members          opt.Field[[]EntityRef] `act:"req:member;field:member;kind:collection;transform:uuid;table:users;preserve_id:true"`
	Issues           opt.Field[[]EntityRef] `act:"req:issues_list;field:issues;kind:collection;transform:uuid;table:issues;preserve_id:true"`
}

func (p ProjectSnapshot) GetName() string {
	if p.Name.IsSet() {
		return p.Name.Value()
	}
	return ""
}

func (p ProjectSnapshot) GetID() uuid.UUID {
	return p.ID
}

func (p ProjectSnapshot) GetField() actField.ActivityField {
	return actField.Project.Field
}

type ProjectEnricher func(*ProjectSnapshot)

func ProjectToSnapshot(p *dao.Project, enrichers ...ProjectEnricher) ProjectSnapshot {
	snapshot := ProjectSnapshot{
		ID:         p.ID,
		Name:       opt.Some(p.Name),
		Public:     opt.Some(p.Public),
		Identifier: opt.Some(p.Identifier),
		ProjectLead: func() opt.Field[EntityRef] {
			if p.ProjectLead != nil {
				return opt.Some(daoToEntityRef(p.ProjectLead))
			}
			return opt.None[EntityRef]()
		}(),
		Emoji:       opt.Some(p.Emoji),
		LogoId:      opt.Some(p.LogoId.UUID),
		RulesScript: opt.Some(PtrToStr(p.RulesScript)),
		DefaultAssignees: func() opt.Field[[]EntityRef] {
			refs := make([]EntityRef, len(p.DefaultAssigneesDetails))
			for i, pm := range p.DefaultAssigneesDetails {
				if pm.Member != nil {
					refs[i] = daoToEntityRef(pm.Member)
				}
			}
			return opt.Some(refs)
		}(),
		DefaultWatchers: func() opt.Field[[]EntityRef] {
			refs := make([]EntityRef, len(p.DefaultWatchersDetails))
			for i, pm := range p.DefaultWatchersDetails {
				if pm.Member != nil {
					refs[i] = daoToEntityRef(pm.Member)
				}
			}
			return opt.Some(refs)
		}(),
	}

	for _, enricher := range enrichers {
		enricher(&snapshot)
	}

	return snapshot
}

func WithProjectMembers(members []dao.ProjectMember, getNameValue func(m dao.ProjectMember) string) ProjectEnricher {
	return func(s *ProjectSnapshot) {
		refs := make([]EntityRef, len(members))
		for i, m := range members {
			refs[i] = EntityRef{
				ID:        m.GetId(),
				NameValue: getNameValue(m),
				NameField: string(m.GetEntityType()),
			}
		}
		s.Members = opt.Some(refs)
	}
}

func WithProjectIssues(issues []dao.Issue) ProjectEnricher {
	return func(s *ProjectSnapshot) {
		refs := make([]EntityRef, len(issues))
		for i, issue := range issues {
			refs[i] = daoToEntityRef(&issue)
		}
		s.Issues = opt.Some(refs)
	}
}

// ------ FormSnapshot -------

type FormSnapshot struct {
	ID                   uuid.UUID
	Title                opt.Field[string]            `act:"req:title;field:title;kind:scalar"`
	Description          opt.Field[string]            `act:"req:description;field:description;kind:scalar"`
	AuthRequire          opt.Field[bool]              `act:"req:auth_require;field:auth_require;kind:scalar"`
	EndDate              opt.Field[*types.TargetDate] `act:"req:end_date;field:end_date;kind:scalar"`
	TargetProjectId      opt.Field[uuid.UUID]         `act:"req:target_project_id;field:target_project_id;kind:scalar;transform:uuid;table:projects;preserve_id:true"`
	Fields               opt.Field[string]            `act:"req:fields;field:fields;kind:scalar"`                               // JSON string
	NotificationChannels opt.Field[string]            `act:"req:notification_channels;field:notification_channels;kind:scalar"` // JSON string
}

func (f FormSnapshot) GetName() string {
	if f.Title.IsSet() {
		return f.Title.Value()
	}
	return ""
}

func (f FormSnapshot) GetID() uuid.UUID {
	return f.ID
}

func (f FormSnapshot) GetField() actField.ActivityField {
	return actField.Form.Field
}

func FormToSnapshot(form *dao.Form) FormSnapshot {
	var fieldsStr string
	if form.Fields != nil {
		if b, err := json.Marshal(form.Fields); err == nil {
			fieldsStr = string(b)
		}
	}
	var notifyStr string
	if b, err := json.Marshal(form.NotificationChannels); err == nil {
		notifyStr = string(b)
	}
	var targetProjectId uuid.UUID
	if form.TargetProjectId.Valid {
		targetProjectId = form.TargetProjectId.UUID
	}
	var endDate *types.TargetDate
	if form.EndDate != nil {
		normalized := *form.EndDate
		normalized.Time = form.EndDate.Time.UTC()
		endDate = &normalized
	}
	return FormSnapshot{
		ID:                   form.ID,
		Title:                opt.Some(form.Title),
		Description:          opt.Some(form.Description.String()),
		AuthRequire:          opt.Some(form.AuthRequire),
		EndDate:              opt.Some(endDate),
		TargetProjectId:      opt.Some(targetProjectId),
		Fields:               opt.Some(fieldsStr),
		NotificationChannels: opt.Some(notifyStr),
	}
}

// ------ FormAnswerSnapshot -------

type FormAnswerSnapshot struct {
	ID       uuid.UUID
	SeqId    opt.Field[int]    `act:"req:seq_id;field:seq_id;kind:scalar"`
	FormDate opt.Field[string] `act:"req:form_date;field:form_date;kind:scalar"`
	Fields   opt.Field[string] `act:"req:fields;field:fields;kind:scalar"` // JSON string
}

func (fa FormAnswerSnapshot) GetName() string {
	if fa.SeqId.IsSet() {
		return fmt.Sprintf("answer #%d", fa.SeqId.Value())
	}
	return ""
}

func (fa FormAnswerSnapshot) GetID() uuid.UUID {
	return fa.ID
}

func (fa FormAnswerSnapshot) GetField() actField.ActivityField {
	return actField.ActivityField("form_answer")
}

func FormAnswerToSnapshot(answer *dao.FormAnswer) FormAnswerSnapshot {
	var fieldsStr string
	if answer.Fields != nil {
		if b, err := json.Marshal(answer.Fields); err == nil {
			fieldsStr = string(b)
		}
	}
	return FormAnswerSnapshot{
		ID:       answer.ID,
		SeqId:    opt.Some(answer.SeqId),
		FormDate: opt.Some(answer.FormDate.Format("2006-01-02T15:04:05Z")),
		Fields:   opt.Some(fieldsStr),
	}
}

// ------ ProjectMemberSnapshot -------

type ProjectMemberSnapshot struct {
	ID   uuid.UUID
	Role opt.Field[int] `act:"req:role;field:role;kind:scalar;preserve_id:true"`
}

func ProjectMemberToSnapshot(pm *dao.ProjectMember) ProjectMemberSnapshot {
	return ProjectMemberSnapshot{
		ID:   pm.MemberId,
		Role: opt.Some(pm.Role),
	}
}

func (pm ProjectMemberSnapshot) GetName() string {
	return ""
}

func (pm ProjectMemberSnapshot) GetID() uuid.UUID {
	return pm.ID
}

func (pm ProjectMemberSnapshot) GetField() actField.ActivityField {
	return actField.Member.Field
}

// -----

type MemberSnapshot struct {
	ID   uuid.UUID
	Role opt.Field[int] `act:"req:role;field:role;kind:scalar;preserve_id:true"`
}

func MemberToSnapshot[T dao.ProjectMember | dao.WorkspaceMember](m *T) MemberSnapshot {
	var id uuid.UUID
	var role int

	switch c := any(m).(type) {
	case *dao.ProjectMember:
		id = c.ID
		role = c.Role
	case *dao.WorkspaceMember:
		id = c.ID
		role = c.Role
	}

	return MemberSnapshot{
		ID:   id,
		Role: opt.Some(role),
	}
}

func (m MemberSnapshot) GetName() string {
	return ""
}

func (m MemberSnapshot) GetID() uuid.UUID {
	return m.ID
}

func (m MemberSnapshot) GetField() actField.ActivityField {
	return actField.Member.Field
}

// ------ SprintSnapshot -------

type SprintSnapshot struct {
	ID           uuid.UUID
	Name         opt.Field[string]                 `act:"req:name;field:name;kind:scalar"`
	Description  opt.Field[string]                 `act:"req:description;field:description;kind:scalar"`
	StartDate    opt.Field[*types.TargetDateTimeZ] `act:"req:start_date;field:start_date;kind:scalar"`
	EndDate      opt.Field[*types.TargetDateTimeZ] `act:"req:end_date;field:end_date;kind:scalar"`
	SprintFolder opt.Field[EntityRef]              `act:"req:sprint_folder;field:sprint_folder;kind:scalar;transform:uuid;table:sprint_folders;preserve_id:true"`
	Watchers     opt.Field[[]EntityRef]            `act:"req:watchers_list;field:watchers;kind:collection;transform:uuid;table:users;preserve_id:true"`
	Issues       opt.Field[[]EntityRef]            `act:"req:issues_list;field:issues;kind:collection;transform:uuid;table:issues;preserve_id:true;linked_field:sprint"`
}

func SprintToSnapshot(s *dao.Sprint) SprintSnapshot {
	snapshot := SprintSnapshot{
		ID:          s.Id,
		Name:        opt.Some(s.Name),
		Description: opt.Some(s.Description.String()),
		Watchers:    opt.Some(utils.SliceToSlice(&s.Watchers, func(t *dao.User) EntityRef { return daoToEntityRef(t) })),
		Issues:      opt.Some(utils.SliceToSlice(&s.Issues, func(t *dao.Issue) EntityRef { return daoToEntityRef(t) })),
	}

	if s.StartDate.Valid {
		startDate := types.TargetDateTimeZ{Time: s.StartDate.Time}
		snapshot.StartDate = opt.Some(&startDate)
	}

	if s.EndDate.Valid {
		endDate := types.TargetDateTimeZ{Time: s.EndDate.Time}
		snapshot.EndDate = opt.Some(&endDate)
	}

	if s.SprintFolder != nil {
		snapshot.SprintFolder = opt.Some(EntityRef{ID: s.SprintFolder.Id, NameValue: s.SprintFolder.Name, NameField: "sprint_folders"})
	} else if s.SprintFolderId.Valid {
		snapshot.SprintFolder = opt.Some(EntityRef{ID: s.SprintFolderId.UUID, NameValue: "", NameField: "sprint_folders"})
	}

	return snapshot
}

func (s SprintSnapshot) GetName() string {
	if s.Name.IsSet() {
		return s.Name.Value()
	}
	return ""
}

func (s SprintSnapshot) GetID() uuid.UUID {
	return s.ID
}

func (s SprintSnapshot) GetField() actField.ActivityField {
	return actField.Sprint.Field
}

// ------ CommentSnapshot -------

type CommentSnapshot struct {
	ID          uuid.UUID
	CommentHtml opt.Field[string] `act:"req:comment_html;field:comment;kind:scalar;preserve_id:true"`
}

func (c CommentSnapshot) GetName() string {
	if c.CommentHtml.IsSet() {
		return c.CommentHtml.Value()
	}
	return ""
}

func (c CommentSnapshot) GetID() uuid.UUID {
	return c.ID
}

func (c CommentSnapshot) GetField() actField.ActivityField {
	return actField.Comment.Field
}

func CommentToSnapshot[T dao.IssueComment | dao.DocComment](comment *T) CommentSnapshot {
	var id uuid.UUID
	var html types.RedactorHTML

	switch c := any(comment).(type) {
	case *dao.IssueComment:
		id = c.Id
		html = c.CommentHtml
	case *dao.DocComment:
		id = c.Id
		html = c.CommentHtml
	}

	return CommentSnapshot{
		ID:          id,
		CommentHtml: opt.Some(html.String()),
	}
}

// ------ AttachmentSnapshot -------

type AttachmentSnapshot struct {
	ID   uuid.UUID
	Name opt.Field[string] `act:"req:attachment;field:attachment;kind:scalar;preserve_id:true"`
}

func (c AttachmentSnapshot) GetName() string {
	if c.Name.IsSet() {
		return c.Name.Value()
	}
	return ""
}

func (c AttachmentSnapshot) GetID() uuid.UUID {
	return c.ID
}

func (c AttachmentSnapshot) GetField() actField.ActivityField {
	return actField.Attachment.Field
}

func AttachmentToSnapshot[T dao.IssueAttachment | dao.DocAttachment](a *T) AttachmentSnapshot {
	var id uuid.UUID
	var name string

	switch c := any(a).(type) {
	case *dao.IssueAttachment:
		id = c.Id
		name = c.Asset.Name
	case *dao.DocAttachment:
		id = c.Id
		name = c.Asset.Name
	}

	return AttachmentSnapshot{
		ID:   id,
		Name: opt.Some(name),
	}
}

func daoToEntityRef(entity dao.IDaoAct) EntityRef {
	if entity == nil {
		return EntityRef{}
	}
	if reflect.ValueOf(entity).IsNil() {
		return EntityRef{}
	}
	return EntityRef{
		ID:        entity.GetId(),
		NameValue: entity.GetString(),
		NameField: string(entity.GetEntityType()),
	}
}

func PtrToStr(str *string) string {
	if str == nil {
		return ""
	}
	return *str
}
