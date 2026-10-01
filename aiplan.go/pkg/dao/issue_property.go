package dao

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/apierrors"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/dto"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/types"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/utils"
	"github.com/gofrs/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
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

	Name string `gorm:"not null"`
	// Type - тип поля: "string", "boolean", "select", "multiselect", "link", "lookup", "date",
	// "datetime", "number", "file" (значение - id вложения этой же задачи), "user" и "users"
	// (значение - id участника проекта, для users - JSON-массив id)
	Type      string   `gorm:"not null"`
	Options   []string `gorm:"serializer:json"`
	SortOrder int      `gorm:"default:0"`

	// ReaderRole/EditorRole - минимальная роль в проекте (5 гость, 10 участник,
	// 15 администратор) для просмотра и для изменения значения поля.
	// Роли шаблона только сужают общее правило на изменение задачи: 5 - без
	// дополнительного ограничения. Редактирование не ниже просмотра:
	// reader_role <= editor_role. Видимость применяет движок
	// (PropertyTemplateScope), право менять - правило движка по объекту
	ReaderRole int `gorm:"default:5"`
	EditorRole int `gorm:"default:5"`

	// UniqueValues - для типа "multiselect": значения в списке не должны повторяться
	UniqueValues bool `gorm:"default:false"`

	// Required - поле обязательно для заполнения: пустое значение не принимается
	// (для boolean не проверяется). Включение задним числом существующие задачи не проверяет
	Required bool `gorm:"default:false"`

	// Unit - единица измерения для типа "number" (свободный текст, у других типов пустая)
	Unit string

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

// PropertyTemplateScope - ограничение видимости шаблонов полей для субъекта.
// Строит движок (policy.Enforcer.PropertyTemplateScope), dao только применяет
// его к своим выборкам по project_property_templates
type PropertyTemplateScope func(*gorm.DB) *gorm.DB

// applyTemplateScope применяет ограничение видимости; без него выборка
// пустая - пустая выдача безопаснее полной
func applyTemplateScope(q *gorm.DB, scope PropertyTemplateScope) *gorm.DB {
	if scope == nil {
		return q.Where("1 = 0")
	}
	return scope(q)
}

// PropertyRolesForOnlyAdmin - роли по устаревшему флагу only_admin:
// true - только администратор (15/15), false - значения по умолчанию (5/5)
func PropertyRolesForOnlyAdmin(onlyAdmin bool) (reader, editor int) {
	if onlyAdmin {
		return types.AdminRole, types.AdminRole
	}
	return types.GuestRole, types.GuestRole
}

// CheckPropertyRoles проверяет роли доступа к полю: обе - роли проекта,
// изменение не ниже просмотра
func CheckPropertyRoles(reader, editor int) error {
	validRole := func(role int) bool {
		return role == types.GuestRole || role == types.MemberRole || role == types.AdminRole
	}
	if !validRole(reader) || !validRole(editor) || reader > editor {
		return apierrors.ErrPropertyRolesInvalid
	}
	return nil
}

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
		ReaderRole:   t.ReaderRole,
		EditorRole:   t.EditorRole,
		OnlyAdmin:    t.ReaderRole == types.AdminRole,
		UniqueValues: t.UniqueValues,
		Required:     t.Required,
		Unit:         t.Unit,
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
	// справочника) или file-поля (Value хранит id вложения, здесь - имя файла).
	// Заполняется вызывающей стороной (rules.EnrichIssue), в БД не хранится
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
		result.UniqueValues = p.Template.UniqueValues
		result.Required = p.Template.Required
		result.Unit = p.Template.Unit
		// Число отдаём числом; остальные типы - хранимой строкой (формат ответа установки)
		if p.Template.Type == "number" {
			result.Value = ParsePropertyValue(p.Template.Type, p.Value)
		}
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
	case "multiselect", "users":
		return []string{}
	default:
		return nil
	}
}

// IsOptionsPropertyType: тип поля с фиксированным набором вариантов (Options)
func IsOptionsPropertyType(propType string) bool {
	return propType == "select" || propType == "multiselect"
}

// ParseMultiselectValue разбирает хранимое значение multiselect-поля (JSON-массив
// строк). Пустое или некорректное значение - пустой список
func ParseMultiselectValue(value string) []string {
	if value == "" {
		return []string{}
	}
	var items []string
	if err := json.Unmarshal([]byte(value), &items); err != nil || items == nil {
		return []string{}
	}
	return items
}

// SerializePropertyValue сериализует значение поля в строку для хранения в БД:
// nil - пустая строка, объект (link) и массив (multiselect) - JSON, пустой массив -
// пустая строка (= не заполнено), JSON-число (float64) - каноническая запись без
// экспоненты, остальное - fmt.Sprint
func SerializePropertyValue(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case map[string]any:
		return marshalOrSprint(v)
	case []any:
		if len(v) == 0 {
			return ""
		}
		return marshalOrSprint(v)
	case []string:
		if len(v) == 0 {
			return ""
		}
		return marshalOrSprint(v)
	}
	return fmt.Sprint(value)
}

// marshalOrSprint - JSON значения; при ошибке сериализации - fmt.Sprint
func marshalOrSprint(value any) string {
	if b, err := json.Marshal(value); err == nil {
		return string(b)
	}
	return fmt.Sprint(value)
}

// ParsePropertyValue преобразует хранимое строковое значение поля в типизированное для DTO
func ParsePropertyValue(propType, value string) any {
	switch propType {
	case "boolean":
		return value == "true"
	case "select", "lookup", "file", "date", "datetime", "user":
		if value == "" {
			return nil
		}
		return value
	case "multiselect", "users":
		return ParseMultiselectValue(value)
	case "link":
		if value == "" {
			return nil
		}
		var m json.RawMessage
		if err := json.Unmarshal([]byte(value), &m); err != nil {
			return value
		}
		return m
	case "number":
		return parseNumberValue(value)
	default:
		return value
	}
}

// parseNumberValue - хранимое значение number-поля как json.Number (в JSON уходит
// числом без шума float). Пустое или неразборчивое значение - nil: json.Number
// с мусором сломал бы сериализацию всего ответа
func parseNumberValue(value string) any {
	if value == "" {
		return nil
	}
	f, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return nil
	}
	return json.Number(strconv.FormatFloat(f, 'f', -1, 64))
}

// IsEmptyPropertyValue: хранимое значение поля пустое для своего типа - "" для
// скалярных типов, пустой список для multiselect. boolean не бывает пустым
func IsEmptyPropertyValue(propType, value string) bool {
	switch propType {
	case "boolean":
		return false
	case "multiselect", "users":
		return len(ParseMultiselectValue(value)) == 0
	default:
		return value == ""
	}
}

// PrepareIssuePropertyValue проверяет устанавливаемое значение поля по шаблону и
// возвращает строку для хранения: форма и семантика значения (types.ValidatePropertyValue),
// уникальность multiselect, обязательность заполнения; number приводится к канонической
// записи. Ошибки - apierrors. Единая точка для HTTP- и MCP-каналов; проверки, требующие
// БД (lookup-строка, каскад), остаются у вызывающего
func PrepareIssuePropertyValue(template ProjectPropertyTemplate, value any) (string, error) {
	if err := types.ValidatePropertyValue(template.Type, template.Options, value); err != nil {
		return "", apierrors.ErrPropertyValueValidationFailed
	}
	if !types.CheckUniqueValues(template.Type, template.UniqueValues, value) {
		return "", apierrors.ErrPropertyValuesNotUnique
	}
	valueStr := SerializePropertyValue(value)
	if template.Type == "number" {
		// Валидность уже проверена, здесь только каноническая запись
		valueStr, _ = types.NormalizeNumberValue(value)
	}
	if template.Required && IsEmptyPropertyValue(template.Type, valueStr) {
		return "", apierrors.ErrPropertyRequired
	}
	return valueStr, nil
}

// MissingRequiredProperties - обязательные шаблоны полей проекта задачи, у которых в
// задаче нет значения или оно пустое для своего типа (IsEmptyPropertyValue).
// Движок по умолчанию запрещает по нему завершение задачи
func MissingRequiredProperties(db *gorm.DB, issue *Issue) ([]ProjectPropertyTemplate, error) {
	var templates []ProjectPropertyTemplate
	if err := db.Where("project_id = ? AND required = true", issue.ProjectId).
		Order("sort_order, created_at").
		Find(&templates).Error; err != nil {
		return nil, err
	}
	if len(templates) == 0 {
		return nil, nil
	}

	var props []IssueProperty
	if err := db.Where("issue_id = ?", issue.ID).Find(&props).Error; err != nil {
		return nil, err
	}
	values := make(map[uuid.UUID]string, len(props))
	for _, p := range props {
		values[p.TemplateId] = p.Value
	}

	missing := make([]ProjectPropertyTemplate, 0)
	for _, tmpl := range templates {
		if IsEmptyPropertyValue(tmpl.Type, values[tmpl.Id]) {
			missing = append(missing, tmpl)
		}
	}
	return missing, nil
}

// ListIssuePropertiesDTO собирает все кастомные поля задачи: шаблоны проекта,
// склеенные с существующими значениями или значениями по умолчанию. Видимость
// шаблонов ограничивает scope движка, lookup- и file-значениям заполняется
// value_label. Единая точка сборки для HTTP- и MCP-каналов
func ListIssuePropertiesDTO(db *gorm.DB, issue *Issue, scope PropertyTemplateScope) ([]dto.IssueProperty, error) {
	var templates []ProjectPropertyTemplate
	if err := applyTemplateScope(db.Where("project_id = ?", issue.ProjectId), scope).
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
		prop := dto.IssueProperty{
			TemplateId:   tmpl.Id,
			IssueId:      issue.ID,
			ProjectId:    issue.ProjectId,
			WorkspaceId:  issue.WorkspaceId,
			Name:         tmpl.Name,
			Type:         tmpl.Type,
			DictionaryId: tmpl.DictionaryId,
			Dependency:   tmpl.Dependency,
			UniqueValues: tmpl.UniqueValues,
			Required:     tmpl.Required,
			Unit:         tmpl.Unit,
			Value:        DefaultPropertyValue(tmpl.Type),
		}
		if IsOptionsPropertyType(tmpl.Type) {
			prop.Options = tmpl.Options
		}
		if existing, ok := propsMap[tmpl.Id]; ok {
			prop.Id = existing.Id
			prop.Value = ParsePropertyValue(tmpl.Type, existing.Value)
		}
		result = append(result, prop)
	}

	if err := FillPropertyValueLabels(db, result); err != nil {
		return nil, err
	}
	return result, nil
}

// FillPropertyValueLabels проставляет отображаемые значения (value_label) полям,
// хранящим id: lookup - строка справочника, file - имя файла вложения, user/users -
// имена пользователей. По одному запросу на тип
func FillPropertyValueLabels(db *gorm.DB, props []dto.IssueProperty) error {
	if err := FillLookupValueLabels(db, props); err != nil {
		return err
	}
	if err := FillFileValueLabels(db, props); err != nil {
		return err
	}
	return FillUserValueLabels(db, props)
}

// propertyValueUUID извлекает UUID-ссылку из значения поля указанного типа
// (lookup - id строки справочника, file - id вложения); пустое или не-UUID - false
func propertyValueUUID(prop dto.IssueProperty, propType string) (uuid.UUID, bool) {
	if prop.Type != propType {
		return uuid.Nil, false
	}
	value, ok := prop.Value.(string)
	if !ok || value == "" {
		return uuid.Nil, false
	}
	id, err := uuid.FromString(value)
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}

// CheckFilePropertyValue валидирует значение file-поля (id вложения этой же задачи):
// не UUID - ErrPropertyValueValidationFailed, вложения нет в задаче - ErrPropertyFileNotFound.
// Возвращает вложение с подгруженным Asset (одна строка - AfterFind допустим).
// Для пустого значения (сброс) возвращает (nil, nil)
func CheckFilePropertyValue(db *gorm.DB, issueId uuid.UUID, valueStr string) (*IssueAttachment, error) {
	if valueStr == "" {
		return nil, nil
	}
	attachmentId, err := uuid.FromString(valueStr)
	if err != nil {
		return nil, apierrors.ErrPropertyValueValidationFailed
	}
	var attachment IssueAttachment
	if err := db.Where("id = ? AND issue_id = ?", attachmentId, issueId).First(&attachment).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apierrors.ErrPropertyFileNotFound
		}
		return nil, err
	}
	return &attachment, nil
}

// FillFileValueLabels батчем проставляет отображаемые значения (value_label) для
// file-полей - имена файлов вложений, id которых хранятся в значениях
func FillFileValueLabels(db *gorm.DB, props []dto.IssueProperty) error {
	var attachmentIds []uuid.UUID
	for _, prop := range props {
		if id, ok := propertyValueUUID(prop, "file"); ok {
			attachmentIds = append(attachmentIds, id)
		}
	}
	if len(attachmentIds) == 0 {
		return nil
	}

	names, err := ResolveAttachmentFileNames(db, attachmentIds)
	if err != nil {
		return err
	}

	for i := range props {
		id, ok := propertyValueUUID(props[i], "file")
		if !ok {
			continue
		}
		if name, ok := names[id]; ok {
			props[i].ValueLabel = &name
		}
	}
	return nil
}

// ResolveAttachmentFileNames возвращает имена файлов вложений задач по id вложений
// одним запросом (JOIN на file_assets, без хуков модели - AfterFind на каждую строку
// дал бы N+1)
func ResolveAttachmentFileNames(db *gorm.DB, attachmentIds []uuid.UUID) (map[uuid.UUID]string, error) {
	result := make(map[uuid.UUID]string, len(attachmentIds))
	if len(attachmentIds) == 0 {
		return result, nil
	}
	var rows []struct {
		Id   uuid.UUID
		Name string
	}
	if err := db.Table("issue_attachments").
		Select("issue_attachments.id, file_assets.name").
		Joins("JOIN file_assets ON file_assets.id = issue_attachments.asset_id").
		Where("issue_attachments.id IN (?)", attachmentIds).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		result[row.Id] = row.Name
	}
	return result, nil
}

// MigratePropertyValuesOnTypeChange приводит существующие значения задач к новой
// конфигурации шаблона при смене типа или справочника. lookup → string: id строки
// заменяется отображаемым значением строки справочника. string ↔ number: текст
// остаётся, в number проходят только числовые значения. Прочие смены с участием
// lookup (уход в другой тип, приход в lookup, смена справочника), file (id вложения),
// link (значение — JSON-ссылка, в других типах это мусор) или number сбрасывают
// значения — они перестают быть валидными. Смены между остальными типами значения не трогают
func MigratePropertyValuesOnTypeChange(tx *gorm.DB, templateId uuid.UUID, oldType, newType string, oldDictionaryId, newDictionaryId uuid.NullUUID) error {
	if oldType == newType && oldDictionaryId == newDictionaryId {
		return nil
	}
	if oldType == "lookup" && newType == "string" && oldDictionaryId.Valid {
		return convertLookupValuesToStrings(tx, templateId, oldDictionaryId.UUID)
	}
	if converted, err := convertSelectMultiselectValues(tx, templateId, oldType, newType); converted {
		return err
	}
	if converted, err := convertStringNumberValues(tx, templateId, oldType, newType); converted {
		return err
	}
	if !typeValuesNeedReset(oldType, newType) {
		return nil
	}
	return tx.Model(&IssueProperty{}).
		Where("template_id = ? AND value <> ''", templateId).
		Update("value", "").Error
}

// convertSelectMultiselectValues конвертирует значения при смене select ↔ multiselect:
// select → multiselect оборачивает значение в список из одного элемента,
// multiselect → select оставляет первый элемент списка (битый JSON — сброс).
// converted=false — смена не из этих двух, значения не тронуты
func convertSelectMultiselectValues(tx *gorm.DB, templateId uuid.UUID, oldType, newType string) (bool, error) {
	switch {
	case oldType == "select" && newType == "multiselect":
		return true, tx.Exec(`UPDATE issue_properties SET value = jsonb_build_array(value)::text
			WHERE template_id = ? AND value <> ''`, templateId).Error
	case oldType == "multiselect" && newType == "select":
		return true, tx.Exec(`UPDATE issue_properties
			SET value = CASE WHEN value ~ '^\[' THEN coalesce(value::jsonb->>0, '') ELSE '' END
			WHERE template_id = ? AND value <> ''`, templateId).Error
	}
	return false, nil
}

// convertStringNumberValues конвертирует значения при смене string ↔ number:
// number → string оставляет число как текст, string → number оставляет значения,
// разбираемые как число (обрезав пробелы), остальные сбрасывает. Каноническую
// запись хранимого числа при чтении восстанавливает parseNumberValue.
// converted=false — смена не из этих двух, значения не тронуты
func convertStringNumberValues(tx *gorm.DB, templateId uuid.UUID, oldType, newType string) (bool, error) {
	switch {
	case oldType == "number" && newType == "string":
		return true, nil
	case oldType == "string" && newType == "number":
		// Квантификаторы {0,1} вместо ? — GORM считает ? в тексте запроса плейсхолдером
		return true, tx.Exec(`UPDATE issue_properties
			SET value = CASE WHEN btrim(value) ~ '^[+-]{0,1}(\d+(\.\d*){0,1}|\.\d+)([eE][+-]{0,1}\d+){0,1}$' THEN btrim(value) ELSE '' END
			WHERE template_id = ? AND value <> ''`, templateId).Error
	}
	return false, nil
}

// typeValuesNeedReset: старые значения невалидны для нового типа — в смене участвует
// lookup (значение — id строки справочника), file (значение — id вложения задачи),
// link (значение — JSON-ссылка), multiselect (значение — JSON-массив; конвертации
// select↔multiselect обработаны выше), number (конвертация string↔number обработана
// выше), date/datetime (форматы дат несовместимы со свободным текстом и друг с другом)
// либо user/users (значение — id пользователя или их список)
func typeValuesNeedReset(oldType, newType string) bool {
	resetTypes := []string{"lookup", "file", "multiselect", "link", "number", "date", "datetime", "user", "users"}
	return slices.Contains(resetTypes, oldType) || slices.Contains(resetTypes, newType)
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

// FillIssuesProperties батчем подкачивает значения дополнительных параметров в
// задачи списка (колонки таблицы): шаблоны проектов выдачи + значения по id задач
// двумя запросами вместо N вызовов ListIssuePropertiesDTO. Видимость шаблонов
// ограничивает scope движка - он работает по project_id строки, поэтому один
// на задачи многих проектов. Options/Dependency в список осознанно не
// кладутся — колонка только показывает значение
func FillIssuesProperties(db *gorm.DB, scope PropertyTemplateScope, issues []dto.IssueWithCount) (err error) {
	if len(issues) == 0 {
		return nil
	}

	// Дочерний спан под спаном запроса; контекст со спаном возвращаем в db,
	// чтобы SQL-спаны gorm-плагина трассировки легли под него
	ctx, span := otel.Tracer("aiplan/dao").Start(db.Statement.Context, "dao.FillIssuesProperties")
	defer func() { endSpan(span, err) }()
	db = db.WithContext(ctx)
	span.SetAttributes(attribute.Int("issues.count", len(issues)))

	issueIds := make([]uuid.UUID, 0, len(issues))
	projectSet := make(map[uuid.UUID]struct{})
	for _, issue := range issues {
		issueIds = append(issueIds, issue.Id)
		projectSet[issue.ProjectId] = struct{}{}
	}

	span.SetAttributes(attribute.Int("projects.count", len(projectSet)))

	templatesByProject, err := visiblePropertyTemplatesByProject(db, scope, utils.SetToSlice(projectSet))
	if err != nil || len(templatesByProject) == 0 {
		return err
	}

	valuesByIssue, err := issuePropertyValuesByIssue(db, issueIds)
	if err != nil {
		return err
	}

	// Собираем в один плоский срез, чтобы резолвить lookup- и file-подписи по одному
	// запросу на тип, а задачам раздаём подсрезы (общий backing array)
	all := make([]dto.IssueProperty, 0, len(issues))
	ranges := make([][2]int, len(issues))
	for i, issue := range issues {
		start := len(all)
		for _, tmpl := range templatesByProject[issue.ProjectId] {
			all = append(all, buildIssuePropertyDTO(issue, tmpl, valuesByIssue[issue.Id]))
		}
		ranges[i] = [2]int{start, len(all)}
	}

	span.SetAttributes(attribute.Int("properties.count", len(all)))

	if err = FillPropertyValueLabels(db, all); err != nil {
		return err
	}
	assignIssuesProperties(issues, all, ranges)
	return nil
}

// assignIssuesProperties раздаёт задачам их подсрезы плоского списка полей
// (cap ограничен концом диапазона — append в один подсрез не затрёт соседний)
func assignIssuesProperties(issues []dto.IssueWithCount, all []dto.IssueProperty, ranges [][2]int) {
	for i := range issues {
		if ranges[i][0] == ranges[i][1] {
			continue
		}
		issues[i].Properties = all[ranges[i][0]:ranges[i][1]:ranges[i][1]]
	}
}

// endSpan закрывает спан, помечая его ошибкой при err != nil
func endSpan(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
}

// visiblePropertyTemplatesByProject - шаблоны полей проектов, видимые
// пользователю по scope движка, сгруппированные по проекту
func visiblePropertyTemplatesByProject(db *gorm.DB, scope PropertyTemplateScope, projectIds []uuid.UUID) (map[uuid.UUID][]ProjectPropertyTemplate, error) {
	var templates []ProjectPropertyTemplate
	if err := applyTemplateScope(db.Where("project_id IN (?)", projectIds), scope).
		Order("sort_order, created_at").
		Find(&templates).Error; err != nil {
		return nil, err
	}
	result := make(map[uuid.UUID][]ProjectPropertyTemplate, len(projectIds))
	for _, tmpl := range templates {
		result[tmpl.ProjectId] = append(result[tmpl.ProjectId], tmpl)
	}
	return result, nil
}

// issuePropertyValuesByIssue - сохранённые значения полей задач: issue id → template id → значение
func issuePropertyValuesByIssue(db *gorm.DB, issueIds []uuid.UUID) (map[uuid.UUID]map[uuid.UUID]IssueProperty, error) {
	var values []IssueProperty
	if err := db.Where("issue_id IN (?)", issueIds).Find(&values).Error; err != nil {
		return nil, err
	}
	result := make(map[uuid.UUID]map[uuid.UUID]IssueProperty, len(issueIds))
	for _, v := range values {
		if result[v.IssueId] == nil {
			result[v.IssueId] = make(map[uuid.UUID]IssueProperty)
		}
		result[v.IssueId][v.TemplateId] = v
	}
	return result, nil
}

// buildIssuePropertyDTO - DTO поля задачи по шаблону и (если есть) сохранённому значению
func buildIssuePropertyDTO(issue dto.IssueWithCount, tmpl ProjectPropertyTemplate, values map[uuid.UUID]IssueProperty) dto.IssueProperty {
	prop := dto.IssueProperty{
		TemplateId:   tmpl.Id,
		IssueId:      issue.Id,
		ProjectId:    issue.ProjectId,
		WorkspaceId:  issue.WorkspaceId,
		Name:         tmpl.Name,
		Type:         tmpl.Type,
		DictionaryId: tmpl.DictionaryId,
		UniqueValues: tmpl.UniqueValues,
		Required:     tmpl.Required,
		Unit:         tmpl.Unit,
		Value:        DefaultPropertyValue(tmpl.Type),
	}
	if existing, ok := values[tmpl.Id]; ok {
		prop.Id = existing.Id
		prop.Value = ParsePropertyValue(tmpl.Type, existing.Value)
	}
	return prop
}
