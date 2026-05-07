// Пакет для валидации данных, используемых в AIPlan.  Содержит валидаторы для различных полей, таких как имя проекта, имя рабочего пространства, идентификатор и т.д.  Использует библиотеку go-playground/validator для выполнения проверок. Также включает в себя регулярные выражения для проверки соответствия формату данных.  Содержит валидаторы для эмодзи статусов.
//
// Основные возможности:
//   - Валидация различных полей данных с использованием предопределенных валидаторов.
//   - Настройка валидаторов для конкретных полей.
//   - Использование регулярных выражений для проверки формата данных.
//   - Валидация эмодзи статусов.
package aiplan

import (
	"regexp"
	"unicode/utf8"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/go-playground/validator"
)

type RequestValidator struct {
	validator *validator.Validate
}

func NewRequestValidator() *RequestValidator {
	v := validator.New()
	err := v.RegisterValidation("projectName", projectNameValidator)
	if err != nil {
		return nil
	}

	err = v.RegisterValidation("workspaceName", workspaceNameValidator)
	if err != nil {
		return nil
	}

	err = v.RegisterValidation("identifier", identifierValidator)
	if err != nil {
		return nil
	}

	err = v.RegisterValidation("slug", slugValidator)
	if err != nil {
		return nil
	}

	err = v.RegisterValidation("fullName", userFullNameValidator)
	if err != nil {
		return nil
	}

	err = v.RegisterValidation("username", usernameValidator)
	if err != nil {
		return nil
	}

	err = v.RegisterValidation("statusEmoji", statusEmojiValidator)
	if err != nil {
		return nil
	}
	return &RequestValidator{v}
}

func (rv *RequestValidator) Validate(i interface{}) error {
	if err := rv.validator.Struct(i); err != nil {
		_, ok := err.(validator.ValidationErrors)
		if !ok {
			return nil
		}
		return err
	}
	return nil
}

func projectNameValidator(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	lenStr := utf8.RuneCountInString(value)
	if !isValidLatinCyrillicDigitWithSymbol(value) {
		return false
	}

	return lenStr >= 1 && lenStr <= 100
}

func workspaceNameValidator(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	lenStr := utf8.RuneCountInString(value)
	if !isValidLatinCyrillicDigitWithSymbol(value) {
		return false
	}
	return lenStr >= 3 && lenStr <= 100
}

func identifierValidator(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	lenStr := utf8.RuneCountInString(value)
	if !isValidLatinUpperDigit(value) {
		return false
	}
	return lenStr >= 3 && lenStr <= 15
}

func slugValidator(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	lenStr := utf8.RuneCountInString(value)
	if !isValidLatinLowerDigitHyphen(value) {
		return false
	}
	return lenStr >= 3 && lenStr <= 50
}

func userFullNameValidator(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	lenStr := utf8.RuneCountInString(value)
	if !isValidLatinCyrillicHyphen(value) {
		return false
	}
	return lenStr >= 1 && lenStr <= 100
}

func usernameValidator(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	lenStr := utf8.RuneCountInString(value)
	if !isValidLatinWithSymbols(value) {
		return false
	}
	return lenStr >= 1 && lenStr <= 100
}

func statusEmojiValidator(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	_, ok := utils.ValidStatusEmoji[value]
	return ok
}

// ValidateAndSet
func isValidLatinCyrillicDigitWithSymbol(str string) bool {
	pt := `^[A-Za-zА-Яа-яёЁ0-9 ._\/\-\\!#\$%&'\"\(\)\*\+,\-.:;№<=>?@\[\\\]\^_\{\|\}~]+$`
	re := regexp.MustCompile(pt)
	return re.MatchString(str)
}

func isValidLatinUpperDigit(str string) bool {
	pt := `^[A-Z0-9]+$`
	re := regexp.MustCompile(pt)
	return re.MatchString(str)
}

func isValidLatinCyrillicHyphen(str string) bool {
	pt := `^[A-Za-zА-Яа-яёЁ-]+$`
	re := regexp.MustCompile(pt)
	return re.MatchString(str)
}

func isValidLatinWithSymbols(str string) bool {
	pt := `^[A-Za-z0-9._\/\-\\]+$`
	re := regexp.MustCompile(pt)
	return re.MatchString(str)
}

func isValidLatinLowerDigitHyphen(str string) bool {
	pt := `^[a-z0-9-]+$`
	re := regexp.MustCompile(pt)
	return re.MatchString(str)
}

var (
	validReactions = map[string]bool{
		"👍":  true,
		"👎":  true,
		"❤️": true,
		"😂":  true,
		"😮":  true,
		"🤡":  true,
		"💩":  true,
		"🤮":  true,
	}
)
