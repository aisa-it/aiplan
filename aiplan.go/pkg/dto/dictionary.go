package dto

import (
	"time"

	"github.com/gofrs/uuid"
)

// Dictionary - справочник проекта
type Dictionary struct {
	Id          uuid.UUID `json:"id"`
	ProjectId   uuid.UUID `json:"project_id"`
	WorkspaceId uuid.UUID `json:"workspace_id"`
	Name        string    `json:"name"`
	RowsCount   int64     `json:"rows_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// DictionaryRow - строка справочника
type DictionaryRow struct {
	Id           uuid.UUID      `json:"id"`
	DictionaryId uuid.UUID      `json:"dictionary_id"`
	Value        string         `json:"value"`
	Attrs        map[string]any `json:"attrs,omitempty"`
	Archived     bool           `json:"archived"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

// CreateDictionaryRequest - запрос на создание справочника
type CreateDictionaryRequest struct {
	Name string `json:"name" validate:"required,min=1,max=255"`
}

// UpdateDictionaryRequest - запрос на обновление справочника
type UpdateDictionaryRequest struct {
	Name *string `json:"name,omitempty" extensions:"x-nullable"`
}

// CreateDictionaryRowRequest - запрос на создание строки справочника
type CreateDictionaryRowRequest struct {
	Value string         `json:"value" validate:"required,min=1"`
	Attrs map[string]any `json:"attrs,omitempty"`
}

// UpdateDictionaryRowRequest - запрос на обновление строки справочника
type UpdateDictionaryRowRequest struct {
	Value    *string         `json:"value,omitempty" extensions:"x-nullable"`
	Attrs    *map[string]any `json:"attrs,omitempty" extensions:"x-nullable"`
	Archived *bool           `json:"archived,omitempty" extensions:"x-nullable"`
}

// ImportDictionaryRowsRequest - батч-импорт строк справочника.
// replace=true: существующие строки без ссылок из задач удаляются,
// строки со ссылками архивируются, затем вставляются новые
type ImportDictionaryRowsRequest struct {
	Replace bool                         `json:"replace"`
	Rows    []CreateDictionaryRowRequest `json:"rows" validate:"required,min=1,dive"`
}

// ImportDictionaryRowsResult - результат батч-импорта строк
type ImportDictionaryRowsResult struct {
	Created  int `json:"created"`
	Deleted  int `json:"deleted"`
	Archived int `json:"archived"`
}
