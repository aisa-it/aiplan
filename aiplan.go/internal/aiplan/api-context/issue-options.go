package apicontext

import (
	"strings"

	"gorm.io/gorm"
)

type IssueFetchOptions struct {
	query *gorm.DB

	loaded map[string]struct{}
}

func (ifo *IssueFetchOptions) GetID() string {
	var str strings.Builder
	for k := range ifo.loaded {
		str.WriteString(k)
	}
	return str.String()
}

type FetchOption (func(*IssueFetchOptions))

func WithParent() FetchOption {
	return func(ifo *IssueFetchOptions) {
		ifo.query = ifo.query.Joins("Parent")
		ifo.loaded["Parent"] = struct{}{}
	}
}

func WithState() FetchOption {
	return func(ifo *IssueFetchOptions) {
		ifo.query = ifo.query.Joins("State")
		ifo.loaded["State"] = struct{}{}
	}
}
func WithSprints() FetchOption {
	return func(ifo *IssueFetchOptions) {
		ifo.query = ifo.query.Preload("Sprints")
		ifo.loaded["Sprints"] = struct{}{}
	}
}

func WithAssignees() FetchOption {
	return func(ifo *IssueFetchOptions) {
		ifo.query = ifo.query.Preload("Assignees")
		ifo.loaded["Assignees"] = struct{}{}
	}
}

func WithWatchers() FetchOption {
	return func(ifo *IssueFetchOptions) {
		ifo.query = ifo.query.Preload("Watchers")
		ifo.loaded["Watchers"] = struct{}{}
	}
}

func WithLabels() FetchOption {
	return func(ifo *IssueFetchOptions) {
		ifo.query = ifo.query.Preload("Labels")
		ifo.loaded["Labels"] = struct{}{}
	}
}

func WithLinks() FetchOption {
	return func(ifo *IssueFetchOptions) {
		ifo.query = ifo.query.Preload("Links")
		ifo.loaded["Links"] = struct{}{}
	}
}

func WithAuthor() FetchOption {
	return func(ifo *IssueFetchOptions) {
		ifo.query = ifo.query.Joins("Author")
		ifo.loaded["Author"] = struct{}{}
	}
}

func WithLinksCreatedBy() FetchOption {
	return func(ifo *IssueFetchOptions) {
		ifo.query = ifo.query.Preload("Links.CreatedBy")
		ifo.loaded["Links.CreatedBy"] = struct{}{}
	}
}

func WithLabelsWorkspace() FetchOption {
	return func(ifo *IssueFetchOptions) {
		ifo.query = ifo.query.Preload("Labels.Workspace")
		ifo.loaded["Labels.Workspace"] = struct{}{}
	}
}

func WithLabelsProject() FetchOption {
	return func(ifo *IssueFetchOptions) {
		ifo.query = ifo.query.Preload("Labels.Project")
		ifo.loaded["Labels.Project"] = struct{}{}
	}
}

func WithBlockers() FetchOption {
	return func(ifo *IssueFetchOptions) {
		ifo.loaded["Blockers"] = struct{}{}
	}
}

func WithLinkedIssues() FetchOption {
	return func(ifo *IssueFetchOptions) {
		ifo.loaded["LinkedIssues"] = struct{}{}
	}
}

func WithAll() FetchOption {
	return func(ifo *IssueFetchOptions) {
		WithParent()(ifo)
		WithState()(ifo)
		WithAuthor()(ifo)
		WithSprints()(ifo)
		WithAssignees()(ifo)
		WithWatchers()(ifo)
		WithLabels()(ifo)
		WithLinks()(ifo)
		WithLinksCreatedBy()(ifo)
		WithLabelsWorkspace()(ifo)
		WithLabelsProject()(ifo)
		WithBlockers()(ifo)
	}
}
