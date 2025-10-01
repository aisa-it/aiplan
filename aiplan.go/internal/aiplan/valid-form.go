// Пакет предоставляет валидаторы для данных в форме, используемые в AIPlan.  Он определяет различные типы валидации, такие как минимальное/максимальное значение, длина строки, проверка на целочисленность и регулярные выражения.  Также включает в себя обработку различных типов полей формы, таких как текстовые поля, чекбоксы, выпадающие списки и т.д.  Валидаторы могут быть настроены с использованием различных параметров, таких как минимальное/максимальное значение, регулярное выражение и список допустимых значений.  Пакет также предоставляет механизм для определения последовательности валидаций, которые должны быть выполнены для каждого поля формы.  Он использует структуру `FormValidateStruct` для определения конфигурации каждого валидатора и функцию `FormValidatoor` для создания карты валидаторов.  Включает валидацию UUID, цветовых кодов и временных меток.
package aiplan

import (
	"fmt"
	"go/types"
	"regexp"
	"strings"

	types2 "github.com/aisa-it/aiplan/internal/aiplan/types"
	"github.com/gofrs/uuid"
)

const (
	ruleMinMax  = "min_max"
	ruleLen     = "len_str"
	ruleOnlyInt = "only_integer"
)

type FormValidateStruct struct {
	Name             string
	CountOpt         int
	TypeOpt          types.BasicKind
	Func             validateTypeFunc
	Pattern          *string
	FieldTypeSupport []string
}

var (
	formTypeValidator = map[string]FormValidateStruct{
		ruleMinMax:  {Name: ruleMinMax, CountOpt: 2, TypeOpt: types.Float64, Func: validateTypeMinMax, Pattern: nil, FieldTypeSupport: []string{"numeric"}},
		ruleLen:     {Name: ruleLen, CountOpt: 2, TypeOpt: types.Float64, Func: validateTypeLenStr, Pattern: nil, FieldTypeSupport: []string{"input", "textarea"}},
		ruleOnlyInt: {Name: ruleOnlyInt, CountOpt: 0, Func: validateTypeRegular, Pattern: strPtr("^[-+]?\\d+$"), FieldTypeSupport: []string{"numeric", "date"}},
	}
)

type validateTypeFunc func(val interface{}, opt []interface{}, pattern *string) bool
type validateFunc func(val interface{}, required bool, custom *types2.ValidationRule) bool

func FormValidator() map[string]validateFunc {
	validMap := make(map[string]validateFunc)
	validMap["numeric"] = validateNumeric
	validMap["checkbox"] = validateCheckbox
	validMap["input"] = validateString
	validMap["textarea"] = validateString
	validMap["color"] = validateColor
	validMap["date"] = validateTimestamp
	validMap["attachment"] = validateUuid
	validMap["select"] = validateSelect
	validMap["multiselect"] = validateMultiSelect
	return validMap
}

func validateCheckbox(val interface{}, required bool, custom *types2.ValidationRule) bool {
	skip := val == nil && !required
	_, ok := val.(bool)
	if !ok && !skip {
		return false
	}
	return true
}

func validateNumeric(val interface{}, required bool, custom *types2.ValidationRule) bool {
	skip := val == nil && !required
	_, ok := val.(float64)
	if !ok && !skip {
		return false
	}

	if custom == nil {
		return true
	}
	return answerValidateRun(val, custom) || skip
}

func validateString(val interface{}, required bool, custom *types2.ValidationRule) bool {
	skip := val == nil && !required
	_, ok := val.(string)
	if !ok && !skip {
		return false
	}

	if custom == nil {
		return true
	}

	return answerValidateRun(val, custom) || skip
}

func validateColor(val interface{}, required bool, custom *types2.ValidationRule) bool {
	skip := val == nil && !required

	v, ok := val.(string)
	if !ok && !skip {
		return false
	}

	if len(v) == 7 && v[0] == '#' {
		return true
	}

	return skip
}

func validateTimestamp(val interface{}, required bool, custom *types2.ValidationRule) bool {
	skip := val == nil && !required

	_, ok := val.(float64)
	if !ok && !skip {
		return false
	}
	return answerValidateRun(val, custom) || skip
}

func validateSelect(val interface{}, required bool, custom *types2.ValidationRule) bool {
	skip := val == nil && !required

	return contains(custom.Opt, val) || skip
}

func validateMultiSelect(val interface{}, required bool, custom *types2.ValidationRule) bool {
	skip := val == nil && !required

	options, ok := val.([]interface{})
	if !ok && !skip {
		return false
	}
	for _, option := range options {
		if !contains(custom.Opt, option) && !skip {
			return skip
		}
	}
	return true
}

func contains(arr []interface{}, val interface{}) bool {
	for _, item := range arr {
		if item == val {
			return true
		}
	}
	return false
}

func validateTypeLenStr(val interface{}, opt []interface{}, pattern *string) bool {
	str, ok := val.(string)
	if !ok {
		return false
	}
	minLen, okMin := opt[0].(float64)
	maxLen, okMax := opt[1].(float64)
	if okMin && okMax {
		if len(str) < int(minLen) || len(str) > int(maxLen) {
			return false
		}
	}
	return true
}

func validateTypeMinMax(val interface{}, opt []interface{}, pattern *string) bool {
	num, ok := val.(float64)
	if !ok {
		return false
	}
	minN, okMin := opt[0].(float64)
	maxN, okMax := opt[1].(float64)
	if okMin && okMax {
		if num < minN || num > maxN {
			return false
		}
	}
	return true
}

func validateTypeRegular(val interface{}, opt []interface{}, pattern *string) bool {
	if pattern == nil {
		return false
	}
	strVal := fmt.Sprintf("%v", val)
	re, err := regexp.Compile(*pattern)
	if err != nil || !re.MatchString(strVal) {
		return false
	}
	return true
}

func validateUuid(val interface{}, required bool, custom *types2.ValidationRule) bool {
	_, ok := val.(string)
	if !ok {
		return false
	}

	strVal := fmt.Sprintf("%v", val)
	_, err := uuid.FromString(strVal)
	if err != nil {
		return false
	}
	return true
}

func strPtr(str string) *string {
	return &str
}

func answerValidateRun(val interface{}, custom *types2.ValidationRule) bool {
	if len(custom.ValidationType) == 0 {
		return true
	}
	validationTypes := strings.Split(custom.ValidationType, " ")
	var countOpts int

	for _, vType := range validationTypes {
		if v, ok := formTypeValidator[vType]; ok {
			if v.CountOpt == 0 {
				continue
			}
			startEl := countOpts
			endEl := countOpts + v.CountOpt
			if valid := v.Func(val, custom.Opt[startEl:endEl], v.Pattern); !valid {
				return false
			}
			countOpts = endEl
		} else {
			return false
		}
	}
	return true
}
