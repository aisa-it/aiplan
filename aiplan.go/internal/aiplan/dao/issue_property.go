package dao

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dto"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
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

	Name      string   `gorm:"not null"`
	Type      string   `gorm:"not null"` // "string", "boolean", "select", "multiselect", "link", "lookup", "date", "datetime"
	Options   []string `gorm:"serializer:json"`
	OnlyAdmin bool     `gorm:"default:false"`
	SortOrder int      `gorm:"default:0"`

	// UniqueValues - для типа "multiselect": значения в списке не должны повторяться
	UniqueValues bool `gorm:"default:false"`

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
		UniqueValues: t.UniqueValues,
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
		result.UniqueValues = p.Template.UniqueValues
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
	case "multiselect":
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
// пустая строка (= не заполнено), остальное - fmt.Sprint
func SerializePropertyValue(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case map[string]any:
		if b, err := json.Marshal(v); err == nil {
			return string(b)
		}
	case []any:
		if len(v) == 0 {
			return ""
		}
		if b, err := json.Marshal(v); err == nil {
			return string(b)
		}
	case []string:
		if len(v) == 0 {
			return ""
		}
		if b, err := json.Marshal(v); err == nil {
			return string(b)
		}
	}
	return fmt.Sprint(value)
}

// ParsePropertyValue преобразует хранимое строковое значение поля в типизированное для DTO
func ParsePropertyValue(propType, value string) any {
	switch propType {
	case "boolean":
		return value == "true"
	case "select", "lookup", "date", "datetime":
		if value == "" {
			return nil
		}
		return value
	case "multiselect":
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
			UniqueValues: tmpl.UniqueValues,
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
	if converted, err := convertSelectMultiselectValues(tx, templateId, oldType, newType); converted {
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

// typeValuesNeedReset: старые значения невалидны для нового типа — в смене участвует
// lookup (значение — id строки справочника), link (значение — JSON-ссылка),
// multiselect (значение — JSON-массив; конвертации select↔multiselect обработаны
// выше) либо date/datetime (форматы дат несовместимы со свободным текстом и друг с другом)
func typeValuesNeedReset(oldType, newType string) bool {
	if oldType == "lookup" || newType == "lookup" {
		return true
	}
	if oldType == "multiselect" || newType == "multiselect" {
		return true
	}
	if oldType == "link" || newType == "link" {
		return true
	}
	return oldType == "date" || newType == "date" || oldType == "datetime" || newType == "datetime"
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
// двумя запросами вместо N вызовов ListIssuePropertiesDTO. OnlyAdmin-поля попадают
// только в задачи проектов, где пользователь админ. Options/Dependency в список
// осознанно не кладутся — колонка только показывает значение
func FillIssuesProperties(db *gorm.DB, user *User, issues []dto.IssueWithCount) (err error) {
	if len(issues) == 0 || user == nil {
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

	templatesByProject, err := visiblePropertyTemplatesByProject(db, user.ID, utils.SetToSlice(projectSet))
	if err != nil || len(templatesByProject) == 0 {
		return err
	}

	valuesByIssue, err := issuePropertyValuesByIssue(db, issueIds)
	if err != nil {
		return err
	}

	// Собираем в один плоский срез, чтобы резолвить lookup-подписи одним запросом,
	// а задачам раздаём подсрезы (общий backing array)
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

	if err = FillLookupValueLabels(db, all); err != nil {
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

// visiblePropertyTemplatesByProject - шаблоны полей проектов, доступные пользователю:
// OnlyAdmin-шаблоны только там, где он админ проекта
func visiblePropertyTemplatesByProject(db *gorm.DB, userId uuid.UUID, projectIds []uuid.UUID) (map[uuid.UUID][]ProjectPropertyTemplate, error) {
	var adminProjects []uuid.UUID
	if err := db.Model(&ProjectMember{}).Select("project_id").
		Where("member_id = ? AND role = ? AND project_id IN (?)", userId, types.AdminRole, projectIds).
		Find(&adminProjects).Error; err != nil {
		return nil, err
	}
	adminSet := make(map[uuid.UUID]struct{}, len(adminProjects))
	for _, id := range adminProjects {
		adminSet[id] = struct{}{}
	}

	var templates []ProjectPropertyTemplate
	if err := db.Where("project_id IN (?)", projectIds).
		Order("sort_order, created_at").
		Find(&templates).Error; err != nil {
		return nil, err
	}
	result := make(map[uuid.UUID][]ProjectPropertyTemplate, len(projectIds))
	for _, tmpl := range templates {
		if _, isAdmin := adminSet[tmpl.ProjectId]; tmpl.OnlyAdmin && !isAdmin {
			continue
		}
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
		Value:        DefaultPropertyValue(tmpl.Type),
	}
	if existing, ok := values[tmpl.Id]; ok {
		prop.Id = existing.Id
		prop.Value = ParsePropertyValue(tmpl.Type, existing.Value)
	}
	return prop
}
