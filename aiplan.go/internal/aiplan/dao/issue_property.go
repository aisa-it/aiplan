package dao

import (
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dto"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/gofrs/uuid"
)

// ProjectPropertyTemplate - шаблон поля на уровне проекта
type ProjectPropertyTemplate struct {
	Id          uuid.UUID `gorm:"primaryKey;type:uuid"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	CreatedById uuid.NullUUID `gorm:"type:uuid" extensions:"x-nullable"`
	UpdatedById uuid.NullUUID `gorm:"type:uuid" extensions:"x-nullable"`

	WorkspaceId uuid.UUID `gorm:"index:ppt_ws_proj_idx,priority:1;type:uuid"`
	ProjectId   uuid.UUID `gorm:"index:ppt_ws_proj_idx,priority:2;type:uuid"`

	Name      string   `gorm:"not null"`
	Type      string   `gorm:"not null"` // "string", "boolean", "select"
	Options   []string `gorm:"serializer:json"`
	OnlyAdmin bool     `gorm:"default:false"`
	SortOrder int      `gorm:"default:0"`

	Workspace *Workspace `gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	Project   *Project   `gorm:"foreignKey:ProjectId" extensions:"x-nullable"`
	CreatedBy *User      `gorm:"foreignKey:CreatedById;references:ID" extensions:"x-nullable"`
	UpdatedBy *User      `gorm:"foreignKey:UpdatedById;references:ID" extensions:"x-nullable"`
}

func (ProjectPropertyTemplate) TableName() string { return "project_property_templates" }

// ToDTO преобразует ProjectPropertyTemplate в DTO
func (t *ProjectPropertyTemplate) ToDTO() *dto.ProjectPropertyTemplate {
	if t == nil {
		return nil
	}
	return &dto.ProjectPropertyTemplate{
		Id:          t.Id,
		ProjectId:   t.ProjectId,
		WorkspaceId: t.WorkspaceId,
		Name:        t.Name,
		Type:        t.Type,
		Options:     t.Options,
		OnlyAdmin:   t.OnlyAdmin,
		SortOrder:   t.SortOrder,
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
	}
}

// IssueProperty - значение поля для конкретной задачи
type IssueProperty struct {
	Id          uuid.UUID `gorm:"primaryKey;type:uuid"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	CreatedById uuid.NullUUID `gorm:"type:uuid" extensions:"x-nullable"`
	UpdatedById uuid.NullUUID `gorm:"type:uuid" extensions:"x-nullable"`

	WorkspaceId uuid.UUID `gorm:"uniqueIndex:issue_property_unique_idx,priority:1;type:uuid"`
	ProjectId   uuid.UUID `gorm:"uniqueIndex:issue_property_unique_idx,priority:2;type:uuid"`
	TemplateId  uuid.UUID `gorm:"uniqueIndex:issue_property_unique_idx,priority:3;type:uuid"`
	IssueId     uuid.UUID `gorm:"uniqueIndex:issue_property_unique_idx,priority:4;type:uuid"`

	Value string `gorm:"type:text"`

	Workspace *Workspace               `gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	Project   *Project                 `gorm:"foreignKey:ProjectId" extensions:"x-nullable"`
	Issue     *Issue                   `gorm:"foreignKey:IssueId"`
	Template  *ProjectPropertyTemplate `gorm:"foreignKey:TemplateId"`
	CreatedBy *User                    `gorm:"foreignKey:CreatedById;references:ID" extensions:"x-nullable"`
	UpdatedBy *User                    `gorm:"foreignKey:UpdatedById;references:ID" extensions:"x-nullable"`
}

func (IssueProperty) TableName() string { return "issue_properties" }

// ToDTO преобразует IssueProperty в DTO
func (p *IssueProperty) ToDTO() *dto.IssueProperty {
	if p == nil {
		return nil
	}
	result := &dto.IssueProperty{
		Id:          p.Id,
		IssueId:     p.IssueId,
		TemplateId:  p.TemplateId,
		ProjectId:   p.ProjectId,
		WorkspaceId: p.WorkspaceId,
		Value:       p.Value,
	}

	// Если шаблон загружен, добавляем информацию о нём
	if p.Template != nil {
		result.Name = p.Template.Name
		result.Type = p.Template.Type
		result.Options = p.Template.Options
	}

	return result
}

// GenSchema генерирует JSON Schema для валидации значения свойства
func (t ProjectPropertyTemplate) GenSchema() types.IssuePropertySchema {
	return types.IssuePropertySchema{
		Schema:   "issue-property-schema",
		Type:     "object",
		Required: []string{"name", "type", "value"},
		Properties: types.SchemaProperties{
			Name:  types.SchemaType{Const: t.Name},
			Type:  types.SchemaType{Const: t.Type},
			Value: types.SchemaType{Type: t.Type},
		},
		AdditionalProperties: true,
	}
}
