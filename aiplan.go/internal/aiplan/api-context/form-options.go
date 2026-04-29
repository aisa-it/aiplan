package apicontext

import (
	"strings"

	"gorm.io/gorm"
)

type FormFetchOptions struct {
	query *gorm.DB

	loaded map[string]struct{}
}

func (ffo *FormFetchOptions) GetID() string {
	var str strings.Builder
	for k := range ffo.loaded {
		str.WriteString(k)
	}
	return str.String()
}

type FormFetchOption (func(*FormFetchOptions))

func WithFormAuthor() FormFetchOption {
	return func(ffo *FormFetchOptions) {
		ffo.query = ffo.query.Joins("Author")
		ffo.loaded["Author"] = struct{}{}
	}
}

func WithFormWorkspace() FormFetchOption {
	return func(ffo *FormFetchOptions) {
		ffo.query = ffo.query.Joins("Workspace")
		ffo.loaded["Workspace"] = struct{}{}
	}
}

func WithFormTargetProject() FormFetchOption {
	return func(ffo *FormFetchOptions) {
		ffo.query = ffo.query.Joins("TargetProject")
		ffo.loaded["TargetProject"] = struct{}{}
	}
}

func WithFormCurrentMember() FormFetchOption {
	return func(ffo *FormFetchOptions) {
		ffo.loaded["CurrentMember"] = struct{}{}
	}
}

func WithFormAll() FormFetchOption {
	return func(ffo *FormFetchOptions) {
		WithFormAuthor()(ffo)
		WithFormWorkspace()(ffo)
		WithFormTargetProject()(ffo)
	}
}
