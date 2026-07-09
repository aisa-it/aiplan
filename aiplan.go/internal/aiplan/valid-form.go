// Пакет предоставляет валидаторы для данных в форме, используемые в AIPlan.  Он определяет различные типы валидации, такие как минимальное/максимальное значение, длина строки, проверка на целочисленность и регулярные выражения.  Также включает в себя обработку различных типов полей формы, таких как текстовые поля, чекбоксы, выпадающие списки и т.д.  Валидаторы могут быть настроены с использованием различных параметров, таких как минимальное/максимальное значение, регулярное выражение и список допустимых значений.  Пакет также предоставляет механизм для определения последовательности валидаций, которые должны быть выполнены для каждого поля формы.  Он использует структуру `FormValidateStruct` для определения конфигурации каждого валидатора и функцию `FormValidatoor` для создания карты валидаторов.  Включает валидацию UUID, цветовых кодов и временных меток.
package aiplan

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
)

func validateFormEndDate(endDate *types.TargetDate) error {
	if endDate == nil {
		return nil
	}
	today := time.Now().Truncate(24 * time.Hour).UTC().Add(-time.Millisecond)
	if !endDate.Time.After(today) {
		return fmt.Errorf("end_date must be in the future")
	}
	return nil
}

var typeValueMapping = map[string]string{
	"numeric":     "numeric",
	"checkbox":    "bool",
	"input":       "string",
	"textarea":    "string",
	"color":       "string",
	"date":        "numeric",
	"attachment":  "uuid",
	"select":      "select",
	"multiselect": "multiselect",
}

var fieldRules = map[string]struct {
	countOpt   int
	fieldTypes []string
}{
	"min_max":      {countOpt: 2, fieldTypes: []string{"numeric", "date"}},
	"len_str":      {countOpt: 2, fieldTypes: []string{"input", "textarea"}},
	"only_integer": {countOpt: 0, fieldTypes: []string{"numeric", "date"}},
}

func validateForm(fields *types.FormFieldsSlice) error {
	var seenIssueNameField bool

	for i, field := range *fields {
		if _, ok := typeValueMapping[field.Type]; !ok {
			return fmt.Errorf("unknown field type: %s", field.Type)
		}

		if field.IssueNameField {
			if field.Type != "input" && field.Type != "string" {
				return fmt.Errorf("issue_name_field only input type")
			}
			if seenIssueNameField {
				return fmt.Errorf("issue_name_field duplicate")
			}
			seenIssueNameField = true
			(*fields)[i].Required = true
			(*fields)[i].DependOn = nil
		}

		(*fields)[i].Val = nil

		if (*fields)[i].Validate == nil {
			(*fields)[i].Validate = &types.ValidationRule{}
		}

		if vt, ok := typeValueMapping[field.Type]; ok {
			(*fields)[i].Validate.ValueType = vt
		}

		if field.Type == "date" {
			(*fields)[i].Validate.ValidationType = "only_integer min_max"
			(*fields)[i].Validate.Opt = []any{float64(math.MinInt64), float64(math.MaxInt64)}
		}

		if err := validateFieldRules(fields, i); err != nil {
			return err
		}

		if err := validateDependOnConfig(fields, i); err != nil {
			return err
		}
	}

	return nil
}

func validateFieldRules(fields *types.FormFieldsSlice, i int) error {
	vr := (*fields)[i].Validate
	if vr == nil || vr.ValidationType == "" {
		return nil
	}

	tokens := strings.Fields(vr.ValidationType)
	var consumed int

	for _, token := range tokens {
		rule, ok := fieldRules[token]
		if !ok {
			return fmt.Errorf("unknown validation rule: %s", token)
		}

		var supported bool
		for _, ft := range rule.fieldTypes {
			if ft == (*fields)[i].Type {
				supported = true
				break
			}
		}
		if !supported {
			return fmt.Errorf("validation rule %q not supported for field type %q", token, (*fields)[i].Type)
		}

		if consumed+rule.countOpt > len(vr.Opt) {
			return fmt.Errorf("validation rule %q requires %d options, got %d", token, rule.countOpt, len(vr.Opt)-consumed)
		}

		if rule.countOpt > 0 {
			for j := 0; j < rule.countOpt; j++ {
				if _, ok := vr.Opt[consumed+j].(float64); !ok {
					return fmt.Errorf("validation rule %q option must be number", token)
				}
			}
		}

		consumed += rule.countOpt
	}

	return nil
}

func validateDependOnConfig(fields *types.FormFieldsSlice, i int) error {
	field := (*fields)[i]
	if field.DependOn == nil {
		return nil
	}

	if i <= field.DependOn.FieldIndex {
		return fmt.Errorf("invalid depend_on order: %d must be greater than %d", i, field.DependOn.FieldIndex)
	}

	parentField := (*fields)[field.DependOn.FieldIndex]
	switch parentField.Type {
	case "checkbox":
		if field.DependOn.OptionIndex != nil {
			return fmt.Errorf("invalid depend_on config: checkbox must not have option_index")
		}
	case "select", "multiselect":
		if field.DependOn.OptionIndex == nil {
			return fmt.Errorf("depend_on option index required for select/multiselect")
		}
		if parentField.Validate == nil || parentField.Validate.Opt == nil ||
			*field.DependOn.OptionIndex >= len(parentField.Validate.Opt) {
			return fmt.Errorf("depend_on option index out of range")
		}
	default:
		return fmt.Errorf("unsupported depend_on field type: %s", parentField.Type)
	}

	return nil
}

func validateAnswers(validator *types.FormValidator, fields, answers types.FormFieldsSlice) (types.FormFieldsSlice, []types.FormAnswerError) {
	reqFields := make([]any, len(answers))
	for i, f := range answers {
		reqFields[i] = f.Val
	}

	fieldResults, vr := validator.Validate(reqFields)
	if !vr.Valid {
		return nil, vr.Errors
	}

	result := make(types.FormFieldsSlice, len(fieldResults))
	for i, r := range fieldResults {
		result[i] = types.FormFields{
			Type:           r.Type,
			Label:          r.Label,
			Val:            r.Val,
			Required:       fields[i].Required,
			IssueNameField: fields[i].IssueNameField,
			DependOn:       fields[i].DependOn,
		}
	}

	return result, nil
}
