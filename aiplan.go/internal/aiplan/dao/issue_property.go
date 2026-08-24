package dao

import (
	"encoding/json"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dto"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
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
	Type      string   `gorm:"not null"` // "string", "boolean", "select", "link", "lookup"
	Options   []string `gorm:"serializer:json"`
	OnlyAdmin bool     `gorm:"default:false"`
	SortOrder int      `gorm:"default:0"`

	// DictionaryId - справочник для типа "lookup" (значение поля - id строки справочника)
	DictionaryId uuid.NullUUID `gorm:"type:uuid" extensions:"x-nullable"`

	// Dependency - каскадная зависимость от родительского поля (nil - независимое поле)
	Dependency *types.PropertyDependency `gorm:"type:jsonb;serializer:json" extensions:"x-nullable"`

	Workspace  *Workspace  `gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	Project    *Project    `gorm:"foreignKey:ProjectId" extensions:"x-nullable"`
	Dictionary *Dictionary `gorm:"foreignKey:DictionaryId" extensions:"x-nullable"`
	CreatedBy  *User       `gorm:"foreignKey:CreatedById;references:ID;belongsTo" extensions:"x-nullable"`
	UpdatedBy  *User       `gorm:"foreignKey:UpdatedById;references:ID;belongsTo" extensions:"x-nullable"`
}

func (ProjectPropertyTemplate) TableName() string { return "project_property_templates" }

// ToDTO преобразует ProjectPropertyTemplate в DTO
func (t *ProjectPropertyTemplate) ToDTO() *dto.ProjectPropertyTemplate {
	if t == nil {
		return nil
	}
	return &dto.ProjectPropertyTemplate{
		Id:           t.Id,
		ProjectId:    t.ProjectId,
		WorkspaceId:  t.WorkspaceId,
		Name:         t.Name,
		Type:         t.Type,
		Options:      t.Options,
		DictionaryId: t.DictionaryId,
		Dependency:   t.Dependency,
		OnlyAdmin:    t.OnlyAdmin,
		SortOrder:    t.SortOrder,
		CreatedAt:    t.CreatedAt,
		UpdatedAt:    t.UpdatedAt,
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

	// ResolvedValue - отображаемое значение lookup-поля (Value хранит id строки
	// справочника). Заполняется вызывающей стороной (rules.EnrichIssue), в БД не хранится
	ResolvedValue string `gorm:"-" json:"-"`

	Workspace *Workspace               `gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	Project   *Project                 `gorm:"foreignKey:ProjectId" extensions:"x-nullable"`
	Issue     *Issue                   `gorm:"foreignKey:IssueId"`
	Template  *ProjectPropertyTemplate `gorm:"foreignKey:TemplateId"`
	CreatedBy *User                    `gorm:"foreignKey:CreatedById;references:ID;belongsTo" extensions:"x-nullable"`
	UpdatedBy *User                    `gorm:"foreignKey:UpdatedById;references:ID;belongsTo" extensions:"x-nullable"`
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
		result.DictionaryId = p.Template.DictionaryId
		result.Dependency = p.Template.Dependency
	}

	return result
}

// DefaultPropertyValue возвращает значение по умолчанию для незаполненного поля по его типу
func DefaultPropertyValue(propType string) any {
	switch propType {
	case "string":
		return ""
	case "boolean":
		return false
	default:
		return nil
	}
}

// ParsePropertyValue преобразует хранимое строковое значение поля в типизированное для DTO
func ParsePropertyValue(propType, value string) any {
	switch propType {
	case "boolean":
		return value == "true"
	case "select", "lookup":
		if value == "" {
			return nil
		}
		return value
	case "link":
		if value == "" {
			return nil
		}
		var m json.RawMessage
		if err := json.Unmarshal([]byte(value), &m); err != nil {
			return value
		}
		return m
	default:
		return value
	}
}

// ListIssuePropertiesDTO собирает все кастомные поля задачи: шаблоны проекта,
// склеенные с существующими значениями или значениями по умолчанию. OnlyAdmin-поля
// возвращаются только админам, lookup-значениям заполняется value_label.
// Единая точка сборки для HTTP- и MCP-каналов
func ListIssuePropertiesDTO(db *gorm.DB, issue *Issue, isAdmin bool) ([]dto.IssueProperty, error) {
	var templates []ProjectPropertyTemplate
	if err := db.Where("project_id = ?", issue.ProjectId).
		Where("only_admin = ? OR only_admin = ?", false, isAdmin).
		Order("sort_order, created_at").
		Find(&templates).Error; err != nil {
		return nil, err
	}

	var existingProps []IssueProperty
	if err := db.Where("issue_id = ?", issue.ID).Find(&existingProps).Error; err != nil {
		return nil, err
	}

	propsMap := make(map[uuid.UUID]IssueProperty, len(existingProps))
	for _, p := range existingProps {
		propsMap[p.TemplateId] = p
	}

	result := make([]dto.IssueProperty, 0, len(templates))
	for _, tmpl := range templates {
		if tmpl.OnlyAdmin && !isAdmin {
			continue
		}
		prop := dto.IssueProperty{
			TemplateId:   tmpl.Id,
			IssueId:      issue.ID,
			ProjectId:    issue.ProjectId,
			WorkspaceId:  issue.WorkspaceId,
			Name:         tmpl.Name,
			Type:         tmpl.Type,
			DictionaryId: tmpl.DictionaryId,
			Dependency:   tmpl.Dependency,
			Value:        DefaultPropertyValue(tmpl.Type),
		}
		if tmpl.Type == "select" {
			prop.Options = tmpl.Options
		}
		if existing, ok := propsMap[tmpl.Id]; ok {
			prop.Id = existing.Id
			prop.Value = ParsePropertyValue(tmpl.Type, existing.Value)
		}
		result = append(result, prop)
	}

	// Для lookup-полей резолвим отображаемые значения строк справочников
	if err := FillLookupValueLabels(db, result); err != nil {
		return nil, err
	}
	return result, nil
}

// MigratePropertyValuesOnTypeChange приводит существующие значения задач к новой
// конфигурации шаблона при смене типа или справочника. lookup → string: id строки
// заменяется отображаемым значением строки справочника. Прочие смены с участием
// lookup (уход в другой тип, приход в lookup, смена справочника) или link (значение —
// JSON-ссылка, в других типах это мусор) сбрасывают значения — они перестают быть
// валидными. Смены между остальными типами значения не трогают
func MigratePropertyValuesOnTypeChange(tx *gorm.DB, templateId uuid.UUID, oldType, newType string, oldDictionaryId, newDictionaryId uuid.NullUUID) error {
	if oldType == newType && oldDictionaryId == newDictionaryId {
		return nil
	}
	if oldType == "lookup" && newType == "string" && oldDictionaryId.Valid {
		return convertLookupValuesToStrings(tx, templateId, oldDictionaryId.UUID)
	}
	if !typeValuesNeedReset(oldType, newType) {
		return nil
	}
	return tx.Model(&IssueProperty{}).
		Where("template_id = ? AND value <> ''", templateId).
		Update("value", "").Error
}

// typeValuesNeedReset: старые значения невалидны для нового типа — в смене участвует
// lookup (значение — id строки справочника) или link (значение — JSON-ссылка)
func typeValuesNeedReset(oldType, newType string) bool {
	if oldType == "lookup" || newType == "lookup" {
		return true
	}
	return oldType == "link" || newType == "link"
}

// convertLookupValuesToStrings заменяет id строк справочника в значениях задач
// отображаемыми значениями строк (включая архивные)
func convertLookupValuesToStrings(tx *gorm.DB, templateId uuid.UUID, dictionaryId uuid.UUID) error {
	// Порядок важен: сначала сброс битых ссылок (пока все значения ещё id),
	// затем замена валидных id отображаемыми значениями строк
	if err := tx.Exec(`UPDATE issue_properties SET value = ''
		WHERE template_id = ? AND value <> ''
		AND NOT EXISTS (SELECT 1 FROM project_dictionary_rows r WHERE r.dictionary_id = ? AND r.id::text = issue_properties.value)`,
		templateId, dictionaryId).Error; err != nil {
		return err
	}
	return tx.Exec(`UPDATE issue_properties SET value = r.value
		FROM project_dictionary_rows r
		WHERE issue_properties.template_id = ? AND r.dictionary_id = ? AND r.id::text = issue_properties.value`,
		templateId, dictionaryId).Error
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
