package apicontext

import (
	"strings"

	"gorm.io/gorm"
)

type DocFetchOptions struct {
	query *gorm.DB

	loaded map[string]struct{}
}

func (dfo *DocFetchOptions) GetID() string {
	var str strings.Builder
	for k := range dfo.loaded {
		str.WriteString(k)
	}
	return str.String()
}

type DocFetchOption (func(*DocFetchOptions))

func WithDocAuthor() DocFetchOption {
	return func(dfo *DocFetchOptions) {
		dfo.query = dfo.query.Preload("Author")
		dfo.loaded["Author"] = struct{}{}
	}
}

func WithDocWorkspace() DocFetchOption {
	return func(dfo *DocFetchOptions) {
		dfo.query = dfo.query.Preload("Workspace")
		dfo.loaded["Workspace"] = struct{}{}
	}
}

func WithDocParent() DocFetchOption {
	return func(dfo *DocFetchOptions) {
		dfo.query = dfo.query.Preload("ParentDoc")
		dfo.loaded["ParentDoc"] = struct{}{}
	}
}

func WithDocAccessRules() DocFetchOption {
	return func(dfo *DocFetchOptions) {
		dfo.query = dfo.query.Preload("AccessRules.Member")
		dfo.loaded["AccessRules"] = struct{}{}
	}
}

func WithDocInlineAttachments() DocFetchOption {
	return func(dfo *DocFetchOptions) {
		dfo.query = dfo.query.Preload("InlineAttachments")
		dfo.loaded["InlineAttachments"] = struct{}{}
	}
}

func WithDocBreadcrumbs() DocFetchOption {
	return func(dfo *DocFetchOptions) {
		dfo.loaded["Breadcrumbs"] = struct{}{}
	}
}

func WithDocAll() DocFetchOption {
	return func(dfo *DocFetchOptions) {
		WithDocAuthor()(dfo)
		WithDocWorkspace()(dfo)
		WithDocParent()(dfo)
		WithDocAccessRules()(dfo)
		WithDocInlineAttachments()(dfo)
		WithDocBreadcrumbs()(dfo)
	}
}
