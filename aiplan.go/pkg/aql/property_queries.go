package aql

import (
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/aql/parser"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/dao"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/utils"
	"github.com/antlr4-go/antlr/v4"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// propertyCmpFuncs — типы параметров, которые сравниваются через <, <=, >, >=.
// Условия строятся по строкам issue_properties.
var propertyCmpFuncs = map[string]cmpFunc{
	"number": numberCmp,
	"date":   dayTextCmp,
	// Значение — unix-время в секундах.
	"datetime": dateCmp(clause.Expr{SQL: `CASE WHEN value ~ '^\d+$' THEN to_timestamp(value::bigint) END`}),
}

// propertyField — параметр задачи по названию. В разных проектах параметр с одним названием
// может быть разного типа, поэтому условие строится по каждому типу отдельно.
func (v *Visitor) propertyField(name string, ctx antlr.ParserRuleContext) fieldFuncs {
	templates, err := v.fetchTemplates(name)
	if err != nil {
		panic(at(err, ctx))
	}
	if len(templates) == 0 {
		panic(at(ErrFieldUnsupported, ctx))
	}

	byType := map[string][]dao.ProjectPropertyTemplate{}
	for _, t := range templates {
		byType[t.Type] = append(byType[t.Type], t)
	}
	f := fieldFuncs{
		in: func(v *Visitor, values []any) (*gorm.DB, error) {
			return propertyIn(v, byType, values)
		},
		cmp: func(v *Visitor, op int, value any) (*gorm.DB, error) {
			return propertyCmp(v, byType, op, value)
		},
	}
	// boolean пустым не бывает: без значения он считается false.
	notBool := slices.DeleteFunc(slices.Clone(templates), func(t dao.ProjectPropertyTemplate) bool {
		return t.Type == "boolean"
	})
	if len(notBool) > 0 {
		f.empty = func(v *Visitor, negate bool) *gorm.DB {
			return propertyEmpty(v, notBool, negate)
		}
	}
	return f
}

func propertyIn(v *Visitor, byType map[string][]dao.ProjectPropertyTemplate, values []any) (*gorm.DB, error) {
	cond := v.newQuery()
	supported := false
	for _, typ := range slices.Sorted(maps.Keys(byType)) {
		in, err := propertyTypeIn(v, typ, byType[typ], values)
		if err != nil {
			return nil, err
		}
		if in != nil {
			cond = cond.Or(in)
			supported = true
		}
	}
	if !supported {
		return nil, ErrPropertyType
	}
	return cond, nil
}

// propertyTypeIn — условие по параметрам одного типа; nil — тип пока не поддерживается.
func propertyTypeIn(v *Visitor, typ string, templates []dao.ProjectPropertyTemplate, values []any) (*gorm.DB, error) {
	switch typ {
	case "string", "select":
		return propertyNamesIn(v, templates, values, "LOWER(value) IN (?)")
	case "multiselect":
		return propertyNamesIn(v, templates, values, "EXISTS (SELECT 1 FROM jsonb_array_elements_text(NULLIF(issue_properties.value, '')::jsonb) AS e(v) WHERE LOWER(e.v) IN (?))")
	case "boolean":
		return propertyBoolIn(v, templates, values)
	case "user":
		return propertyUsersIn(v, templates, values, "value IN (SELECT id::text FROM (?) AS u)")
	case "users":
		return propertyUsersIn(v, templates, values, "EXISTS (SELECT 1 FROM jsonb_array_elements_text(NULLIF(issue_properties.value, '')::jsonb) AS e(v) WHERE e.v IN (SELECT id::text FROM (?) AS u))")
	case "lookup":
		return propertyLookupIn(v, templates, values)
	case "date":
		return propertyDaysIn(v, templates, values)
	case "number", "datetime":
		return propertyCmpIn(v, templates, values, propertyCmpFuncs[typ])
	}
	return nil, nil
}

// propertyCmp — сравнение по параметрам тех типов, которые его умеют, остальные пропускаются.
func propertyCmp(v *Visitor, byType map[string][]dao.ProjectPropertyTemplate, op int, value any) (*gorm.DB, error) {
	switch op {
	case parser.AQLParserLIKE:
		return propertyLike(v, byType["string"], value)
	case parser.AQLParserNOT_LIKE:
		like, err := propertyLike(v, byType["string"], value)
		if err != nil {
			return nil, err
		}
		return v.newQuery().Not(like), nil
	}

	cond := v.newQuery()
	supported := false
	for _, typ := range slices.Sorted(maps.Keys(byType)) {
		cmp := propertyCmpFuncs[typ]
		if cmp == nil {
			continue
		}
		rows, err := cmp(v, op, value)
		if err != nil {
			return nil, err
		}
		cond = cond.Or(propertyHas(v, byType[typ], rows))
		supported = true
	}
	if !supported {
		return nil, ErrOperator
	}
	return cond, nil
}

// propertyLike — ~ ищет подстроку без учёта регистра, только в параметрах-строках.
func propertyLike(v *Visitor, templates []dao.ProjectPropertyTemplate, value any) (*gorm.DB, error) {
	if len(templates) == 0 {
		return nil, ErrLikeOperator
	}
	text, err := lowerStrings([]any{value})
	if err != nil {
		return nil, err
	}
	if utf8.RuneCountInString(text[0]) < minLikeLen {
		return nil, ErrLikeLength
	}
	return propertyHas(v, templates, "value ILIKE ?", utils.PrepareSearchRequest(text[0])), nil
}

func propertyCmpIn(v *Visitor, templates []dao.ProjectPropertyTemplate, values []any, cmp cmpFunc) (*gorm.DB, error) {
	rows, err := cmpIn(cmp)(v, values)
	if err != nil {
		return nil, err
	}
	return propertyHas(v, templates, rows), nil
}

func propertyDaysIn(v *Visitor, templates []dao.ProjectPropertyTemplate, values []any) (*gorm.DB, error) {
	days := make([]string, 0, len(values))
	for _, value := range values {
		day, err := v.dayText(value)
		if err != nil {
			return nil, err
		}
		days = append(days, day)
	}
	return propertyHas(v, templates, "value IN (?)", days), nil
}

// numberCmp — CASE не даёт приведению к числу упасть на значении не по формату.
func numberCmp(v *Visitor, op int, value any) (*gorm.DB, error) {
	n, err := parseNumber(value)
	if err != nil {
		return nil, err
	}
	column := clause.Expr{SQL: `CASE WHEN value ~ '^[+-]{0,1}(\d+(\.\d*){0,1}|\.\d+)([eE][+-]{0,1}\d+){0,1}$' THEN value::numeric END`}
	expr := mapCmpExpr(op, column, n)
	if expr == nil {
		return nil, ErrNumberOperator
	}
	return v.newQuery().Where(expr), nil
}

// parseNumber — целое приходит из запроса числом, дробное — строкой.
func parseNumber(value any) (float64, error) {
	switch vv := value.(type) {
	case int:
		return float64(vv), nil
	case string:
		n, err := strconv.ParseFloat(vv, 64)
		if err == nil && !math.IsNaN(n) && !math.IsInf(n, 0) {
			return n, nil
		}
	}
	return 0, ErrNumber
}

// dayTextCmp — дата хранится строкой ГГГГ-ММ-ДД, такие строки сравниваются как даты.
func dayTextCmp(v *Visitor, op int, value any) (*gorm.DB, error) {
	day, err := v.dayText(value)
	if err != nil {
		return nil, err
	}
	expr := mapCmpExpr(op, clause.Expr{SQL: "NULLIF(value, '')"}, day)
	if expr == nil {
		return nil, ErrDateOperator
	}
	return v.newQuery().Where(expr), nil
}

func propertyNamesIn(v *Visitor, templates []dao.ProjectPropertyTemplate, values []any, where string) (*gorm.DB, error) {
	names, err := lowerStrings(values)
	if err != nil {
		return nil, err
	}
	return propertyHas(v, templates, where, names), nil
}

func propertyUsersIn(v *Visitor, templates []dao.ProjectPropertyTemplate, values []any, where string) (*gorm.DB, error) {
	users, err := refUsers(v, values)
	if err != nil {
		return nil, err
	}
	return propertyHas(v, templates, where, users), nil
}

// propertyLookupIn — в значении id строки справочника, ищем строку по id или по названию.
func propertyLookupIn(v *Visitor, templates []dao.ProjectPropertyTemplate, values []any) (*gorm.DB, error) {
	rows, err := refIds(v, values, &dao.DictionaryRow{}, "value")
	if err != nil {
		return nil, err
	}
	dictionaries := make([]uuid.NullUUID, 0, len(templates))
	for _, t := range templates {
		dictionaries = append(dictionaries, t.DictionaryId)
	}
	rows = rows.Where("dictionary_id IN (?)", dictionaries)
	return propertyHas(v, templates, "value IN (SELECT id::text FROM (?) AS r)", rows), nil
}

// propertyBoolIn — boolean без значения считается false.
func propertyBoolIn(v *Visitor, templates []dao.ProjectPropertyTemplate, values []any) (*gorm.DB, error) {
	names, err := lowerStrings(values)
	if err != nil {
		return nil, err
	}
	projects := make([]uuid.UUID, 0, len(templates))
	for _, t := range templates {
		projects = append(projects, t.ProjectId)
	}

	cond := v.newQuery()
	for _, name := range names {
		switch name {
		case "true":
			cond = cond.Or(propertyHas(v, templates, "value = 'true'"))
		case "false":
			cond = cond.Or("issues.project_id IN (?) AND NOT EXISTS (?)", projects, propertyRows(v, templates).Where("value = 'true'"))
		default:
			return nil, ErrBoolValue
		}
	}
	return cond, nil
}

// propertyEmpty — значения нет, или это пустая строка, или пустой список.
func propertyEmpty(v *Visitor, templates []dao.ProjectPropertyTemplate, negate bool) *gorm.DB {
	return exists(v, propertyRows(v, templates).Where("value NOT IN ('', '[]')"), negate)
}

// propertyHas — задачи, у которых значение параметра подходит под where.
func propertyHas(v *Visitor, templates []dao.ProjectPropertyTemplate, where any, args ...any) *gorm.DB {
	return v.newQuery().Where("EXISTS (?)", propertyRows(v, templates).Where(where, args...))
}

// propertyRows — значения параметров задачи issues.id, для EXISTS. Условия покрывают все колонки
// уникального индекса issue_properties, поэтому каждая проверка — поиск по индексу.
func propertyRows(v *Visitor, templates []dao.ProjectPropertyTemplate) *gorm.DB {
	workspaces := make([]uuid.UUID, 0, len(templates))
	projects := make([]uuid.UUID, 0, len(templates))
	ids := make([]uuid.UUID, 0, len(templates))
	for _, t := range templates {
		workspaces = append(workspaces, t.WorkspaceId)
		projects = append(projects, t.ProjectId)
		ids = append(ids, t.Id)
	}
	return v.newQuery().Model(&dao.IssueProperty{}).Select("1").
		Where("issue_id = issues.id AND workspace_id IN (?) AND project_id IN (?) AND template_id IN (?)", workspaces, projects, ids)
}

// dayText — день в часовом поясе пользователя, в формате хранения параметров-дат.
func (v *Visitor) dayText(value any) (string, error) {
	value, err := v.dateValue(value)
	if err != nil {
		return "", err
	}
	switch vv := value.(type) {
	case Date:
		return time.Time(vv).Format(time.DateOnly), nil
	case time.Time:
		return vv.In(v.now().Location()).Format(time.DateOnly), nil
	}
	return "", ErrDate
}

func (v *Visitor) fetchTemplates(name string) ([]dao.ProjectPropertyTemplate, error) {
	name = strings.ToLower(name)
	cached, ok := v.templatesCache[name]
	if ok {
		return cached, nil
	}

	var templates []dao.ProjectPropertyTemplate
	if err := v.newQuery().Scopes(v.visibleTemplates).Where("LOWER(name) = ?", name).Find(&templates).Error; err != nil {
		return nil, err
	}
	v.templatesCache[name] = templates
	return templates, nil
}
