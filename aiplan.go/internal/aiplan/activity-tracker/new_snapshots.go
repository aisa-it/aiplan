package tracker

import (
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/opt"
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
	ID          uuid.UUID
	Name        opt.Field[string]      `act:"req:name;field:name;kind:scalar"`
	Assignees   opt.Field[[]EntityRef] `act:"req:assignees_list;field:assignees;kind:collection;transform:uuid;table:users;preserve_id:true"`
	Watchers    opt.Field[[]EntityRef] `act:"req:watchers_list;field:watchers;kind:collection;transform:uuid;table:users;preserve_id:true"`
	Description opt.Field[string]      `act:"req:description_html;field:description;kind:scalar"`
	State       opt.Field[EntityRef]   `act:"req:state;field:status;kind:scalar;transform:uuid;table:states;preserve_id:true"`
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
	return ""
}

func IssueToSnapshot(i dao.Issue) IssueSnapshot {
	return IssueSnapshot{
		ID:          i.ID,
		Name:        opt.Some(i.Name),
		Description: opt.Some(i.DescriptionHtml),
		Assignees:   opt.Some(utils.SliceToSlice(i.Assignees, func(t *dao.User) EntityRef { return daoToEntityRef(t) })),
		Watchers:    opt.Some(utils.SliceToSlice(i.Watchers, func(t *dao.User) EntityRef { return daoToEntityRef(t) })),
		State:       opt.Some(daoToEntityRef(i.State)),
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
	return ""
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

func CommentToSnapshot(comment *dao.DocComment) CommentSnapshot {
	return CommentSnapshot{
		ID:          comment.Id,
		CommentHtml: opt.Some(comment.CommentHtml.String()),
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

func AttachmentToSnapshot(a *dao.DocAttachment) AttachmentSnapshot {
	return AttachmentSnapshot{
		ID:   a.Id,
		Name: opt.Some(a.Asset.Name),
	}
}

func daoToEntityRef(entity dao.IDaoAct) EntityRef {
	if entity == nil {
		return EntityRef{}
	}
	nameField := string(entity.GetEntityType())
	switch entity.GetEntityType() {
	case actField.Doc.Field:
		nameField = "docs"
	case actField.Issue.Field:
		nameField = "issues"
	case actField.Project.Field:
		nameField = "projects"
	case actField.Workspace.Field:
		nameField = "workspaces"
	}
	return EntityRef{
		ID:        entity.GetId(),
		NameValue: entity.GetString(),
		NameField: nameField,
	}
}
