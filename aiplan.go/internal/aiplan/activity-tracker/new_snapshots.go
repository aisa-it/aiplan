package tracker

import (
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/opt"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
)

type IssueSnapshot struct {
	Name        opt.Field[string]      `act:"req:name;field:name;kind:scalar"`
	Assignees   opt.Field[[]EntityRef] `act:"req:assignees_list;field:assignees;kind:collection;transform:uuid;table:users"`
	Watchers    opt.Field[[]EntityRef] `act:"req:watchers_list;field:watchers;kind:collection;transform:uuid;table:users"`
	Description opt.Field[string]      `act:"req:description;field:description;kind:scalar"`
	State       opt.Field[EntityRef]   `act:"req:state;field:status;kind:scalar;transform:uuid;table:states"`
}

func IssueToSnapshot(i dao.Issue) IssueSnapshot {
	return IssueSnapshot{
		Name:      opt.Some(i.Name),
		Assignees: opt.Some(utils.SliceToSlice(i.Assignees, func(t *dao.User) EntityRef { return daoToEntityRef(t) })),
		Watchers:  opt.Some(utils.SliceToSlice(i.Watchers, func(t *dao.User) EntityRef { return daoToEntityRef(t) })),
		State:     opt.Some(daoToEntityRef(i.State)),
	}
}

type DocSnapshot struct {
	Title      opt.Field[string]      `act:"req:title;field:title;kind:scalar"`
	Content    opt.Field[string]      `act:"req:description;field:description;kind:scalar"`
	EditorRole opt.Field[int]         `act:"req:editor_role;field:editor_role;kind:scalar"`
	ReaderRole opt.Field[int]         `act:"req:reader_role;field:reader_role;kind:scalar"`
	Parent     opt.Field[EntityRef]   `act:"req:parent_doc_id;field:parent;kind:scalar;transform:uuid;table:docs"`
	Editors    opt.Field[[]EntityRef] `act:"req:editors_list;field:editors;kind:collection;transform:uuid"`
	Readers    opt.Field[[]EntityRef] `act:"req:readers_list;field:readers;kind:collection;transform:uuid"`
	Watchers   opt.Field[[]EntityRef] `act:"req:watchers_list;field:watchers;kind:collection;transform:uuid"`
}


func DocToSnapshot(doc *dao.Doc) DocSnapshot {
	snapshot := DocSnapshot{
		Title:      opt.Some(doc.Title),
		Content:    opt.Some(string(doc.Content.Body)),
		EditorRole: opt.Some(doc.EditorRole),
		ReaderRole: opt.Some(doc.ReaderRole),
    Watchers: opt.Some(utils.SliceToSlice(doc.Watchers, func(t *dao.User) EntityRef { return daoToEntityRef(t) })),
    Readers: opt.Some(utils.SliceToSlice(doc.Readers, func(t *dao.User) EntityRef { return daoToEntityRef(t) })),
    Editors: opt.Some(utils.SliceToSlice(doc.Editors, func(t *dao.User) EntityRef { return daoToEntityRef(t) })),
  }

	if doc.ParentDoc != nil {
		snapshot.Parent = opt.Some(daoToEntityRef(doc.ParentDoc))
	}
	return snapshot
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
