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

type SnapshotI interface {
	GetName() string
	GetID() uuid.UUID
	GetField() actField.ActivityField
}

// ------ IssueSnapshot -------

type IssueSnapshot struct {
	ID           uuid.UUID
	Name         opt.Field[string]                 `act:"field:name;kind:scalar"`
	Assignees    opt.Field[[]EntityRef]            `act:"field:assignees;kind:collection;preserve_id:true"`
	Watchers     opt.Field[[]EntityRef]            `act:"field:watchers;kind:collection;preserve_id:true"`
	Description  opt.Field[string]                 `act:"field:description;kind:scalar"`
	Priority     opt.Field[string]                 `act:"field:priority;kind:scalar"`
	State        opt.Field[EntityRef]              `act:"field:status;kind:scalar;preserve_id:true"`
	TargetDate   opt.Field[*types.TargetDateTimeZ] `act:"field:target_date;kind:scalar"`
	StartDate    opt.Field[*types.TargetDateTimeZ] `act:"field:start_date;kind:scalar"`
	CompletedAt  opt.Field[*types.TargetDateTimeZ] `act:"field:completed_at;kind:scalar"`
	Parent       opt.Field[EntityRef]              `act:"field:parent;kind:scalar;preserve_id:true;linked_field:sub_issue"`
	BlockerList  opt.Field[[]EntityRef]            `act:"field:blocking;kind:collection;preserve_id:true;linked_field:blocks"`
	BlockedList  opt.Field[[]EntityRef]            `act:"field:blocks;kind:collection;preserve_id:true;linked_field:blocking"`
	SubIssues    opt.Field[[]EntityRef]            `act:"field:sub_issue;kind:collection;preserve_id:true;linked_field:parent"`
	Links        opt.Field[[]EntityRef]            `act:"field:link;kind:collection;preserve_id:true"`
	LinkedIssues opt.Field[[]EntityRef]            `act:"field:linked;kind:collection;preserve_id:true;linked_field:linked;verb:updated"`
	Sprints      opt.Field[[]EntityRef]            `act:"field:sprint;kind:collection;preserve_id:true;linked_field:issues;linked_layer:sprint"`
	Labels       opt.Field[[]EntityRef]            `act:"field:label;kind:collection;preserve_id:true"`
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
				return opt.Some(EntityRef{ID: i.ParentId.UUID, NameValue: i.Parent.String(), NameField: actField.Issue.Field.String()})
			}
			return opt.None[EntityRef]()
		}(),
		Links: opt.Some(utils.SliceToSlice(i.Links, func(t *dao.IssueLink) EntityRef {
			return EntityRef{ID: t.Id, NameValue: t.Url, NameField: actField.Link.Field.String()}
		})),
		Labels: opt.Some(utils.SliceToSlice(i.Labels, func(t *dao.Label) EntityRef {
			return EntityRef{ID: t.ID, NameValue: t.Name, NameField: actField.Label.Field.String()}
		})),
		LinkedIssues: func() opt.Field[[]EntityRef] {
			refs := make([]EntityRef, len(i.LinkedIssues))
			for j, li := range i.LinkedIssues {
				li.Project = i.Project
				refs[j] = EntityRef{ID: li.ID, NameValue: li.String(), NameField: actField.Issue.Field.String()}
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

// ------ IssueLinkSnapshot -------

type LinkSnapshot struct {
	ID    uuid.UUID
	Title opt.Field[string] `act:"field:link_title;kind:scalar;preserve_id:true"`
	Url   opt.Field[string] `act:"field:link_url;kind:scalar;preserve_id:true;"`
}

func (l LinkSnapshot) GetName() string {
	if l.Title.IsSet() {
		return l.Title.Value()
	}
	return ""
}

func (l LinkSnapshot) GetID() uuid.UUID {
	return l.ID
}

func (l LinkSnapshot) GetField() actField.ActivityField {
	return actField.Link.Field
}

func LinkToSnapshot(link *dao.IssueLink) LinkSnapshot {
	return LinkSnapshot{
		ID:    link.Id,
		Title: opt.Some(link.Title),
		Url:   opt.Some(link.Url),
	}
}

// ------ LabelSnapshot -------

type LabelSnapshot struct {
	ID          uuid.UUID
	Name        opt.Field[string] `act:"field:label_name;kind:scalar;preserve_id:true"`
	Color       opt.Field[string] `act:"field:label_color;kind:scalar;preserve_id:true"`
	Description opt.Field[string] `act:"field:label_description;kind:scalar;preserve_id:true"`
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
type StateEnricher func(*StateSnapshot)

type StateSnapshot struct {
	ID          uuid.UUID
	Name        opt.Field[string]    `act:"field:status_name;kind:scalar;preserve_id:true"`
	Description opt.Field[string]    `act:"field:status_description;kind:scalar;preserve_id:true"`
	Color       opt.Field[string]    `act:"field:status_color;kind:scalar;preserve_id:true"`
	Group       opt.Field[string]    `act:"field:status_group;kind:scalar;preserve_id:true"`
	Default     opt.Field[EntityRef] `act:"field:status_default;kind:scalar;preserve_id:true"`
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

func StateToSnapshot(state *dao.State, enrichers ...StateEnricher) StateSnapshot {
	snapshot := StateSnapshot{
		ID:          state.ID,
		Name:        opt.Some(state.Name),
		Description: opt.Some(state.Description),
		Color:       opt.Some(state.Color),
		Group:       opt.Some(state.Group),
	}
	for _, enricher := range enrichers {
		enricher(&snapshot)
	}

	return snapshot
}

func WithDefaultState(st dao.State) StateEnricher {
	return func(s *StateSnapshot) {
		s.Default = opt.Some(EntityRef{ID: st.ID, NameValue: st.Name})
	}
}

// ------ IssueTemplateSnapshot -------

type IssueTemplateSnapshot struct {
	ID       uuid.UUID
	Name     opt.Field[string] `act:"field:template_name;kind:scalar;preserve_id:true"`
	Template opt.Field[string] `act:"field:template_template;kind:scalar;preserve_id:true"`
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
	Name        opt.Field[string]      `act:"field:name;kind:scalar"`
	Description opt.Field[string]      `act:"field:description;kind:scalar"`
	LogoId      opt.Field[uuid.UUID]   `act:"field:logo;kind:scalar"`
	OwnerId     opt.Field[uuid.UUID]   `act:"field:owner;kind:scalar"`
	Token       opt.Field[string]      `act:"field:integration_token;kind:scalar;secret:true"`
	Integration opt.Field[[]EntityRef] `act:"field:integration;kind:collection;preserve_id:true"`
	Members     opt.Field[[]EntityRef] `act:"field:member;kind:collection;preserve_id:true"`
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
		Token:       opt.Some(workspace.IntegrationToken),
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
				ID:        m.MemberId,
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
	Title      opt.Field[string]      `act:"field:title;kind:scalar"`
	Content    opt.Field[string]      `act:"field:description;kind:scalar"`
	EditorRole opt.Field[int]         `act:"field:editor_role;kind:scalar"`
	ReaderRole opt.Field[int]         `act:"field:reader_role;kind:scalar"`
	Parent     opt.Field[EntityRef]   `act:"field:parent;kind:scalar;preserve_id:true"`
	Editors    opt.Field[[]EntityRef] `act:"field:editors;kind:collection;preserve_id:true"`
	Readers    opt.Field[[]EntityRef] `act:"field:readers;kind:collection;preserve_id:true"`
	Watchers   opt.Field[[]EntityRef] `act:"field:watchers;kind:collection;preserve_id:true"`
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
		snapshot.Parent = opt.Some(EntityRef{ID: doc.ParentDocID.UUID, NameValue: doc.Title, NameField: actField.Doc.Field.String()})
	}
	return snapshot
}

// ------ ProjectSnapshot -------

type ProjectSnapshot struct {
	ID               uuid.UUID
	Name             opt.Field[string]      `act:"field:name;kind:scalar"`
	Public           opt.Field[bool]        `act:"field:public;kind:scalar"`
	Identifier       opt.Field[string]      `act:"field:identifier;kind:scalar"`
	ProjectLead      opt.Field[EntityRef]   `act:"field:project_lead;kind:scalar;preserve_id:true"`
	Emoji            opt.Field[int32]       `act:"field:emoji;kind:scalar"`
	LogoId           opt.Field[uuid.UUID]   `act:"field:logo;kind:scalar"`
	RulesScript      opt.Field[string]      `act:"field:rules_script;kind:scalar"`
	DefaultAssignees opt.Field[[]EntityRef] `act:"field:default_assignees;kind:collection;preserve_id:true"`
	DefaultWatchers  opt.Field[[]EntityRef] `act:"field:default_watchers;kind:collection;preserve_id:true"`
	Members          opt.Field[[]EntityRef] `act:"field:member;kind:collection;preserve_id:true"`
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
		ID:               p.ID,
		Name:             opt.Some(p.Name),
		Public:           opt.Some(p.Public),
		Identifier:       opt.Some(p.Identifier),
		ProjectLead:      opt.Some(daoToEntityRef(p.ProjectLead)),
		Emoji:            opt.Some(p.Emoji),
		LogoId:           opt.Some(p.LogoId.UUID),
		RulesScript:      opt.Some(PtrToStr(p.RulesScript)),
		DefaultAssignees: opt.Some(utils.SliceToSlice(&p.DefaultAssigneesDetails, func(t *dao.ProjectMember) EntityRef { return daoToEntityRef(t.Member) })),
		DefaultWatchers:  opt.Some(utils.SliceToSlice(&p.DefaultWatchersDetails, func(t *dao.ProjectMember) EntityRef { return daoToEntityRef(t.Member) })),
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
				ID:        m.MemberId,
				NameValue: getNameValue(m),
				NameField: string(m.GetEntityType()),
			}
		}
		s.Members = opt.Some(refs)
	}
}

// ------ FormSnapshot -------

type FormSnapshot struct {
	ID            uuid.UUID
	Title         opt.Field[string]            `act:"field:title;kind:scalar"`
	Description   opt.Field[string]            `act:"field:description;kind:scalar"`
	AuthRequire   opt.Field[bool]              `act:"field:auth_require;kind:scalar"`
	EndDate       opt.Field[*types.TargetDate] `act:"field:end_date;kind:scalar"`
	TargetProject opt.Field[EntityRef]         `act:"field:target_project;kind:scalar;preserve_id:true"`
	Fields        opt.Field[string]            `act:"field:fields;kind:scalar"` // JSON string
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
	var endDate *types.TargetDate
	if form.EndDate != nil {
		normalized := *form.EndDate
		normalized.Time = form.EndDate.Time.UTC()
		endDate = &normalized
	}
	return FormSnapshot{
		ID:            form.ID,
		Title:         opt.Some(form.Title),
		Description:   opt.Some(form.Description.String()),
		AuthRequire:   opt.Some(form.AuthRequire),
		EndDate:       opt.Some(endDate),
		TargetProject: opt.Some(daoToEntityRef(form.TargetProject)),
		Fields:        opt.Some(fieldsStr)}
}

// ------ FormAnswerSnapshot -------

type FormAnswerSnapshot struct {
	ID       uuid.UUID
	SeqId    opt.Field[int]    `act:"field:seq_id;kind:scalar"`
	FormDate opt.Field[string] `act:"field:form_date;kind:scalar"`
	Fields   opt.Field[string] `act:"field:fields;kind:scalar"` // JSON string
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

// ------ SprintSnapshot -------

type SprintSnapshot struct {
	ID           uuid.UUID
	Name         opt.Field[string]                 `act:"field:name;kind:scalar"`
	Description  opt.Field[string]                 `act:"field:description;kind:scalar"`
	StartDate    opt.Field[*types.TargetDateTimeZ] `act:"field:start_date;kind:scalar"`
	EndDate      opt.Field[*types.TargetDateTimeZ] `act:"field:end_date;kind:scalar"`
	SprintFolder opt.Field[EntityRef]              `act:"field:sprint_folder;kind:scalar;preserve_id:true"`
	Watchers     opt.Field[[]EntityRef]            `act:"field:watchers;kind:collection;preserve_id:true"`
	Issues       opt.Field[[]EntityRef]            `act:"field:issue;kind:collection;preserve_id:true;linked_field:sprint;linked_layer:issue"`
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
		snapshot.StartDate = opt.Some(utils.ToPtr(types.TargetDateTimeZ{Time: s.StartDate.Time}))
	}

	if s.EndDate.Valid {
		snapshot.EndDate = opt.Some(utils.ToPtr(types.TargetDateTimeZ{Time: s.EndDate.Time}))
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

// ------ MemberSnapshot -------

type MemberSnapshot struct {
	ID   uuid.UUID
	Role opt.Field[int] `act:"field:role;kind:scalar;preserve_id:true"`
}

func MemberToSnapshot[T dao.ProjectMember | dao.WorkspaceMember](m *T) MemberSnapshot {
	var id uuid.UUID
	var role int

	switch c := any(m).(type) {
	case *dao.ProjectMember:
		id = c.MemberId
		role = c.Role
	case *dao.WorkspaceMember:
		id = c.MemberId
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

// ------ CommentSnapshot -------

type CommentSnapshot struct {
	ID          uuid.UUID
	CommentHtml opt.Field[string] `act:"field:comment;kind:scalar;preserve_id:true"`
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
	Name opt.Field[string] `act:"field:attachment;kind:scalar;preserve_id:true"`
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
