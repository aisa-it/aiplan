package tracker

import (
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

type IssueSnapshot struct {
	ID           uuid.UUID
	Name         opt.Field[string]                 `act:"req:name;field:name;kind:scalar"`
	Assignees    opt.Field[[]EntityRef]            `act:"req:assignees_list;field:assignees;kind:collection;transform:uuid;table:users;preserve_id:true"`
	Watchers     opt.Field[[]EntityRef]            `act:"req:watchers_list;field:watchers;kind:collection;transform:uuid;table:users;preserve_id:true"`
	Description  opt.Field[string]                 `act:"req:description_html;field:description;kind:scalar"`
	Priority     opt.Field[*string]                `act:"req:priority;field:priority;kind:scalar"`
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

func IssueToSnapshot(i dao.Issue) IssueSnapshot {
	return IssueSnapshot{
		ID:          i.ID,
		Name:        opt.Some(i.Name),
		Description: opt.Some(i.DescriptionHtml),
		Priority:    opt.Some(i.Priority),
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
		SubIssues: opt.None[[]EntityRef](),
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
	}
}

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

type ProjectSnapshot struct {
	ID               uuid.UUID
	Name             opt.Field[string]        `act:"req:name;field:name;kind:scalar"`
	Public           opt.Field[bool]          `act:"req:public;field:public;kind:scalar"`
	Identifier       opt.Field[string]        `act:"req:identifier;field:identifier;kind:scalar"`
	ProjectLead      opt.Field[EntityRef]     `act:"req:project_lead;field:project_lead;kind:scalar;transform:uuid;table:users;preserve_id:true"`
	Emoji            opt.Field[int32]         `act:"req:emoji;field:emoji;kind:scalar"`
	LogoId           opt.Field[uuid.NullUUID] `act:"req:logo_id;field:logo;kind:scalar"`
	CoverImage       opt.Field[*string]       `act:"req:cover_image;field:cover_image;kind:scalar"`
	EstimateId       opt.Field[*string]       `act:"req:estimate;field:estimate;kind:scalar"`
	RulesScript      opt.Field[*string]       `act:"req:rules_script;field:rules_script;kind:scalar"`
	DefaultAssignees opt.Field[[]EntityRef]   `act:"req:default_assignees;field:default_assignees;kind:collection;transform:uuid;table:users;preserve_id:true"`
	DefaultWatchers  opt.Field[[]EntityRef]   `act:"req:default_watchers;field:default_watchers;kind:collection;transform:uuid;table:users;preserve_id:true"`
}

func ProjectToSnapshot(p *dao.Project) ProjectSnapshot {
	return ProjectSnapshot{
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
		LogoId:      opt.Some(p.LogoId),
		CoverImage:  opt.Some(p.CoverImage),
		EstimateId:  opt.Some(p.EstimateId),
		RulesScript: opt.Some(p.RulesScript),
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

////

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
