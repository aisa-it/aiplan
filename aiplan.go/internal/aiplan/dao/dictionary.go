// Справочники проекта: именованные наборы строк с произвольными атрибутами.
// Используются полями типа "lookup" (ProjectPropertyTemplate.DictionaryId) —
// значение IssueProperty.Value хранит id строки справочника.
package dao

import (
	"errors"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dto"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

// Ошибки валидации значения lookup-поля (маппятся на apierrors в ручках)
var (
	ErrLookupRowNotFound = errors.New("dictionary row not found")
	ErrLookupRowArchived = errors.New("dictionary row is archived")
)

// Dictionary - справочник уровня проекта
type Dictionary struct {
	Id          uuid.UUID `gorm:"primaryKey;type:uuid"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	CreatedById uuid.NullUUID `gorm:"type:uuid" extensions:"x-nullable"`
	UpdatedById uuid.NullUUID `gorm:"type:uuid" extensions:"x-nullable"`

	WorkspaceId uuid.UUID `gorm:"index:dict_ws_proj_idx,priority:1;type:uuid"`
	ProjectId   uuid.UUID `gorm:"index:dict_ws_proj_idx,priority:2;type:uuid"`

	Name string `gorm:"not null"`

	Workspace *Workspace `gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	Project   *Project   `gorm:"foreignKey:ProjectId" extensions:"x-nullable"`
	CreatedBy *User      `gorm:"foreignKey:CreatedById;references:ID;belongsTo" extensions:"x-nullable"`
	UpdatedBy *User      `gorm:"foreignKey:UpdatedById;references:ID;belongsTo" extensions:"x-nullable"`
}

func (Dictionary) TableName() string { return "project_dictionaries" }

// ToDTO преобразует Dictionary в DTO
func (d *Dictionary) ToDTO() *dto.Dictionary {
	if d == nil {
		return nil
	}
	return &dto.Dictionary{
		Id:          d.Id,
		ProjectId:   d.ProjectId,
		WorkspaceId: d.WorkspaceId,
		Name:        d.Name,
		CreatedAt:   d.CreatedAt,
		UpdatedAt:   d.UpdatedAt,
	}
}

// DictionaryRow - строка справочника: отображаемое значение + произвольные
// атрибуты (attrs) для фильтраций. Архивная строка не выбирается в новых
// значениях, но отображается в уже установленных
type DictionaryRow struct {
	Id          uuid.UUID `gorm:"primaryKey;type:uuid"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	CreatedById uuid.NullUUID `gorm:"type:uuid" extensions:"x-nullable"`
	UpdatedById uuid.NullUUID `gorm:"type:uuid" extensions:"x-nullable"`

	WorkspaceId  uuid.UUID `gorm:"type:uuid"`
	ProjectId    uuid.UUID `gorm:"type:uuid"`
	DictionaryId uuid.UUID `gorm:"index:dict_row_lookup_idx,priority:1;type:uuid"`

	// Составной индекс (dictionary_id, archived, value) покрывает выборку строк
	// селекта: фильтр по archived и сортировку по value
	Value    string         `gorm:"not null;index:dict_row_lookup_idx,priority:3"`
	Attrs    map[string]any `gorm:"serializer:json;type:jsonb"`
	Archived bool           `gorm:"default:false;index:dict_row_lookup_idx,priority:2"`

	Dictionary *Dictionary `gorm:"foreignKey:DictionaryId" extensions:"x-nullable"`
	CreatedBy  *User       `gorm:"foreignKey:CreatedById;references:ID;belongsTo" extensions:"x-nullable"`
	UpdatedBy  *User       `gorm:"foreignKey:UpdatedById;references:ID;belongsTo" extensions:"x-nullable"`
}

func (DictionaryRow) TableName() string { return "project_dictionary_rows" }

// ToDTO преобразует DictionaryRow в DTO
func (r *DictionaryRow) ToDTO() *dto.DictionaryRow {
	if r == nil {
		return nil
	}
	return &dto.DictionaryRow{
		Id:           r.Id,
		DictionaryId: r.DictionaryId,
		Value:        r.Value,
		Attrs:        r.Attrs,
		Archived:     r.Archived,
		CreatedAt:    r.CreatedAt,
		UpdatedAt:    r.UpdatedAt,
	}
}

// IsDictionaryExists сообщает, существует ли справочник (guard для middleware;
// принадлежность проекту проверяется в apicontext.GetDictionary)
func IsDictionaryExists(db *gorm.DB, dictionaryId string) (bool, error) {
	id := uuid.FromStringOrNil(dictionaryId)
	if id.IsNil() {
		return false, nil
	}
	return Exists(db, db.Session(&gorm.Session{}).Model(&Dictionary{}).Where("id = ?", id).Select("1"))
}

// GetDictionaryRow возвращает строку справочника по id с проверкой принадлежности
func GetDictionaryRow(db *gorm.DB, dictionaryId uuid.UUID, rowId uuid.UUID) (*DictionaryRow, error) {
	var row DictionaryRow
	if err := db.Where("id = ? AND dictionary_id = ?", rowId, dictionaryId).First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

// CheckLookupValue валидирует значение lookup-поля (id строки справочника шаблона):
// строка должна существовать в справочнике шаблона и быть не архивной.
// Для пустого значения (сброс) возвращает (nil, nil)
func CheckLookupValue(db *gorm.DB, template ProjectPropertyTemplate, valueStr string) (*DictionaryRow, error) {
	if valueStr == "" {
		return nil, nil
	}
	if !template.DictionaryId.Valid {
		return nil, ErrLookupRowNotFound
	}
	rowId, err := uuid.FromString(valueStr)
	if err != nil {
		return nil, ErrLookupRowNotFound
	}
	row, err := GetDictionaryRow(db, template.DictionaryId.UUID, rowId)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrLookupRowNotFound
		}
		return nil, err
	}
	if row.Archived {
		return nil, ErrLookupRowArchived
	}
	return row, nil
}

// IsDictionaryRowUsed сообщает, ссылаются ли значения lookup-полей задач на строку
func IsDictionaryRowUsed(db *gorm.DB, rowId uuid.UUID) (bool, error) {
	var used bool
	err := db.Model(&IssueProperty{}).
		Select("count(*) > 0").
		Joins("JOIN project_property_templates ppt ON ppt.id = issue_properties.template_id").
		Where("ppt.type = 'lookup'").
		Where("issue_properties.value = ?", rowId.String()).
		Find(&used).Error
	return used, err
}

// lookupRowId извлекает id строки справочника из значения lookup-поля
func lookupRowId(prop dto.IssueProperty) (uuid.UUID, bool) {
	if prop.Type != "lookup" {
		return uuid.Nil, false
	}
	value, ok := prop.Value.(string)
	if !ok || value == "" {
		return uuid.Nil, false
	}
	rowId, err := uuid.FromString(value)
	if err != nil {
		return uuid.Nil, false
	}
	return rowId, true
}

// FillLookupValueLabels батчем проставляет отображаемые значения (value_label)
// для lookup-полей, значение которых - id строки справочника
func FillLookupValueLabels(db *gorm.DB, props []dto.IssueProperty) error {
	var rowIds []uuid.UUID
	for _, prop := range props {
		if rowId, ok := lookupRowId(prop); ok {
			rowIds = append(rowIds, rowId)
		}
	}
	if len(rowIds) == 0 {
		return nil
	}

	labels, err := ResolveDictionaryRowValues(db, rowIds)
	if err != nil {
		return err
	}

	for i := range props {
		rowId, ok := lookupRowId(props[i])
		if !ok {
			continue
		}
		if label, ok := labels[rowId]; ok {
			props[i].ValueLabel = &label
		}
	}
	return nil
}

// ResolveDictionaryRowValues возвращает отображаемые значения строк по их id
func ResolveDictionaryRowValues(db *gorm.DB, rowIds []uuid.UUID) (map[uuid.UUID]string, error) {
	result := make(map[uuid.UUID]string, len(rowIds))
	if len(rowIds) == 0 {
		return result, nil
	}
	var rows []DictionaryRow
	if err := db.Select("id, value").Where("id IN (?)", rowIds).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		result[row.Id] = row.Value
	}
	return result, nil
}
