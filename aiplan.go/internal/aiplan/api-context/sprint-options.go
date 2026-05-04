package apicontext

import (
	"strings"

	"gorm.io/gorm"
)

type SprintFetchOptions struct {
	query *gorm.DB

	loaded map[string]struct{}
}

func (sfo *SprintFetchOptions) GetID() string {
	var str strings.Builder
	for k := range sfo.loaded {
		str.WriteString(k)
	}
	return str.String()
}

type SprintFetchOption (func(*SprintFetchOptions))

func WithSprintWorkspace() SprintFetchOption {
	return func(sfo *SprintFetchOptions) {
		sfo.query = sfo.query.Joins("Workspace")
		sfo.loaded["Workspace"] = struct{}{}
	}
}

func WithSprintCreatedBy() SprintFetchOption {
	return func(sfo *SprintFetchOptions) {
		sfo.query = sfo.query.Joins("CreatedBy")
		sfo.loaded["CreatedBy"] = struct{}{}
	}
}

func WithSprintUpdatedBy() SprintFetchOption {
	return func(sfo *SprintFetchOptions) {
		sfo.query = sfo.query.Joins("UpdatedBy")
		sfo.loaded["UpdatedBy"] = struct{}{}
	}
}

func WithSprintFolder() SprintFetchOption {
	return func(sfo *SprintFetchOptions) {
		sfo.query = sfo.query.Joins("SprintFolder")
		sfo.loaded["SprintFolder"] = struct{}{}
	}
}

func WithSprintWatchers() SprintFetchOption {
	return func(sfo *SprintFetchOptions) {
		sfo.query = sfo.query.Preload("Watchers")
		sfo.loaded["Watchers"] = struct{}{}
	}
}

func WithSprintIssues() SprintFetchOption {
	return func(sfo *SprintFetchOptions) {
		sfo.query = sfo.query.Preload("Issues.State")
		sfo.loaded["Issues"] = struct{}{}
	}
}

func WithSprintAll() SprintFetchOption {
	return func(sfo *SprintFetchOptions) {
		WithSprintWorkspace()(sfo)
		WithSprintCreatedBy()(sfo)
		WithSprintUpdatedBy()(sfo)
		WithSprintFolder()(sfo)
		WithSprintWatchers()(sfo)
		WithSprintIssues()(sfo)
	}
}
