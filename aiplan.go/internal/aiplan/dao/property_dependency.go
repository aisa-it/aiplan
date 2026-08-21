// Каскадные зависимости кастомных полей (types.PropertyDependency):
// валидация значения ребёнка против текущего значения родителя в задаче
// и сброс несовместимых значений детей при смене значения родителя.
package dao

import (
	"errors"
	"fmt"
	"slices"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

// ErrDependencyValueIncompatible - значение ребёнка недопустимо при текущем
// значении родителя (маппится на apierrors.ErrPropertyValueIncompatible в ручках)
var ErrDependencyValueIncompatible = errors.New("value is not allowed by the parent property value")

// propertyDisplayValue возвращает отображаемое значение поля: для lookup - value
// строки справочника, для остальных типов - хранимое значение как есть
func propertyDisplayValue(db *gorm.DB, template ProjectPropertyTemplate, valueStr string) (string, error) {
	if template.Type != "lookup" || valueStr == "" {
		return valueStr, nil
	}
	rowId := uuid.FromStringOrNil(valueStr)
	if rowId.IsNil() {
		return "", nil
	}
	labels, err := ResolveDictionaryRowValues(db, []uuid.UUID{rowId})
	if err != nil {
		return "", err
	}
	return labels[rowId], nil
}

// attrMatches сравнивает значение атрибута строки справочника с отображаемым
// значением родителя: строка - равенство, массив - вхождение
func attrMatches(attrValue any, parentDisplay string) bool {
	switch v := attrValue.(type) {
	case string:
		return v == parentDisplay
	case []any:
		for _, item := range v {
			if fmt.Sprint(item) == parentDisplay {
				return true
			}
		}
		return false
	case nil:
		return false
	default:
		return fmt.Sprint(v) == parentDisplay
	}
}

// CurrentParentDisplay возвращает отображаемое значение родительского поля зависимости
// для задачи. ok=false - каскад не ограничивает (родитель пуст или зависимость битая)
func CurrentParentDisplay(db *gorm.DB, template ProjectPropertyTemplate, issueId uuid.UUID) (string, bool, error) {
	dep := template.Dependency

	var parent ProjectPropertyTemplate
	if err := db.Where("id = ? AND project_id = ?", dep.ParentTemplateId, template.ProjectId).
		First(&parent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Родительский шаблон удалён - мягкая деградация, каскад не ограничивает
			return "", false, nil
		}
		return "", false, err
	}

	var parentProp IssueProperty
	if err := db.Where("issue_id = ? AND template_id = ?", issueId, parent.Id).
		First(&parentProp).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", false, nil
		}
		return "", false, err
	}
	if parentProp.Value == "" {
		return "", false, nil
	}

	display, err := propertyDisplayValue(db, parent, parentProp.Value)
	if err != nil {
		return "", false, err
	}
	return display, true, nil
}

// dependencyAllowsValue проверяет допустимость значения ребёнка при отображаемом
// значении родителя. lookupRow - строка справочника значения (для режима row_filter)
func dependencyAllowsValue(dep *types.PropertyDependency, parentDisplay string, valueStr string, lookupRow *DictionaryRow) bool {
	switch dep.Mode {
	case types.PropertyDependencyOptionsMap:
		return slices.Contains(dep.OptionsMap[parentDisplay], valueStr)
	case types.PropertyDependencyRowFilter:
		if lookupRow == nil {
			return false
		}
		var attrValue any
		if lookupRow.Attrs != nil {
			attrValue = lookupRow.Attrs[dep.RowFilterAttr]
		}
		return attrMatches(attrValue, parentDisplay)
	default:
		return true
	}
}

// CheckDependencyValue валидирует устанавливаемое значение поля с каскадной
// зависимостью: значение допустимо при текущем значении родителя в задаче.
// Пустой родитель не ограничивает. lookupRow - строка справочника значения
// (nil для не-lookup полей)
func CheckDependencyValue(db *gorm.DB, template ProjectPropertyTemplate, issueId uuid.UUID, valueStr string, lookupRow *DictionaryRow) error {
	if template.Dependency == nil || valueStr == "" {
		return nil
	}

	parentDisplay, restricted, err := CurrentParentDisplay(db, template, issueId)
	if err != nil {
		return err
	}
	if !restricted {
		return nil
	}

	if !dependencyAllowsValue(template.Dependency, parentDisplay, valueStr, lookupRow) {
		return ErrDependencyValueIncompatible
	}
	return nil
}

// childValueCompatible проверяет совместимость текущего значения ребёнка
// с новым отображаемым значением родителя
func childValueCompatible(db *gorm.DB, child ProjectPropertyTemplate, childValue string, parentDisplay string) (bool, error) {
	var lookupRow *DictionaryRow
	if child.Type == "lookup" && child.DictionaryId.Valid {
		rowId := uuid.FromStringOrNil(childValue)
		if rowId.IsNil() {
			return false, nil
		}
		var err error
		lookupRow, err = GetDictionaryRow(db, child.DictionaryId.UUID, rowId)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return false, nil
			}
			return false, err
		}
	}
	return dependencyAllowsValue(child.Dependency, parentDisplay, childValue, lookupRow), nil
}

// ResetIncompatibleChildren сбрасывает значения зависимых полей задачи, ставшие
// недопустимыми после смены значения родительского поля. Возвращает имена
// сброшенных полей. Пустое новое значение родителя детей не сбрасывает
// (пустой родитель не ограничивает)
func ResetIncompatibleChildren(db *gorm.DB, parentTemplate ProjectPropertyTemplate, issueId uuid.UUID, newValueStr string, userId uuid.UUID) ([]string, error) {
	var children []ProjectPropertyTemplate
	if err := db.Where("project_id = ?", parentTemplate.ProjectId).
		Where("dependency->>'parent_template_id' = ?", parentTemplate.Id.String()).
		Find(&children).Error; err != nil {
		return nil, err
	}
	if len(children) == 0 || newValueStr == "" {
		return nil, nil
	}

	parentDisplay, err := propertyDisplayValue(db, parentTemplate, newValueStr)
	if err != nil {
		return nil, err
	}

	var reset []string
	for _, child := range children {
		wasReset, err := resetChildIfIncompatible(db, child, issueId, parentDisplay, userId)
		if err != nil {
			return nil, err
		}
		if wasReset {
			reset = append(reset, child.Name)
		}
	}
	return reset, nil
}

// resetChildIfIncompatible сбрасывает значение одного зависимого поля задачи,
// если оно недопустимо при новом отображаемом значении родителя
func resetChildIfIncompatible(db *gorm.DB, child ProjectPropertyTemplate, issueId uuid.UUID, parentDisplay string, userId uuid.UUID) (bool, error) {
	var childProp IssueProperty
	if err := db.Where("issue_id = ? AND template_id = ?", issueId, child.Id).
		First(&childProp).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	if childProp.Value == "" {
		return false, nil
	}

	compatible, err := childValueCompatible(db, child, childProp.Value, parentDisplay)
	if err != nil || compatible {
		return false, err
	}

	if err := db.Model(&childProp).Updates(map[string]any{
		"value":         "",
		"updated_by_id": uuid.NullUUID{UUID: userId, Valid: true},
	}).Error; err != nil {
		return false, err
	}
	return true, nil
}
