package aql

import (
	"errors"
	"fmt"

	"github.com/antlr4-go/antlr/v4"
)

// Error — ошибка в запросе. [Start, End) — место для подсветки, в символах запроса.
type Error struct {
	Code  string `json:"code"`
	Err   string `json:"error"`
	Ru    string `json:"ru_error"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

func (e Error) Error() string {
	return e.Err
}

const (
	maxQuerySize = 10_000
	maxListSize  = 100
	maxExprCount = 100
	maxFuncCount = 50
	minLikeLen   = 3
)

// Ошибка вызывающего, а не запроса: пользователю её не показывать.
var ErrNoUser = errors.New("aql: current user is required")

var (
	ErrInternal      = Error{Code: "internal", Err: "unexpected query tree", Ru: "Не удалось разобрать запрос"}
	ErrQueryOverflow = Error{Code: "query_overflow", Err: "query is too long", Ru: "Запрос слишком длинный"}

	ErrFieldName          = Error{Code: "field_name", Err: "field name expected", Ru: "Здесь ожидается имя поля"}
	ErrFieldUnsupported   = Error{Code: "field_unsupported", Err: "field is not supported", Ru: "Поиск по этому полю не поддерживается"}
	ErrOperator           = Error{Code: "operator", Err: "only =, !=, IN and NOT IN are supported for this field", Ru: "Для этого поля доступны только =, !=, IN и NOT IN"}
	ErrLikeOperator       = Error{Code: "like_operator", Err: "~ and !~ are supported only for text fields", Ru: "~ и !~ доступны только для текстовых полей"}
	ErrDateOperator       = Error{Code: "date_operator", Err: "only =, !=, <, <=, >, >= are supported for dates", Ru: "Для дат доступны только =, !=, <, <=, >, >="}
	ErrValue              = Error{Code: "value", Err: "expected name or id", Ru: "Ожидается название или id"}
	ErrCustomFieldID      = Error{Code: "cf_unsupported", Err: "cf[id] is not supported, use the property name in quotes", Ru: "cf[…] не поддерживается: укажите название параметра в кавычках"}
	ErrPropertyType       = Error{Code: "property_type", Err: "search by this property type is not supported yet", Ru: "Поиск по параметрам этого типа пока не поддерживается"}
	ErrBoolValue          = Error{Code: "bool_value", Err: "expected true or false", Ru: "Ожидается true или false"}
	ErrEmptyUnsupported   = Error{Code: "empty_unsupported", Err: "field does not support IS EMPTY", Ru: "Это поле нельзя проверить на пустоту"}
	ErrHistoryUnsupported = Error{Code: "history_unsupported", Err: "WAS and CHANGED are not supported yet", Ru: "WAS и CHANGED пока не поддерживаются"}
	ErrUnsupportedSort    = Error{Code: "sort_unsupported", Err: "unsupported sort field", Ru: "По этому полю нельзя сортировать"}
	ErrListSizeExceed     = Error{Code: "list_size_exceed", Err: fmt.Sprintf("list can have at most %d values", maxListSize), Ru: fmt.Sprintf("В списке может быть не больше %d значений", maxListSize)}
	ErrExprCountExceed    = Error{Code: "expr_count_exceed", Err: fmt.Sprintf("query can have at most %d conditions", maxExprCount), Ru: fmt.Sprintf("В запросе может быть не больше %d условий", maxExprCount)}
	ErrFuncCountExceed    = Error{Code: "func_count_exceed", Err: fmt.Sprintf("query can have at most %d function calls", maxFuncCount), Ru: fmt.Sprintf("В запросе может быть не больше %d вызовов функций", maxFuncCount)}
	ErrLikeLength         = Error{Code: "like_length", Err: fmt.Sprintf("~ and !~ need at least %d characters", minLikeLen), Ru: fmt.Sprintf("Для ~ и !~ нужно не меньше %d символов", minLikeLen)}

	ErrNumberRange      = Error{Code: "number_range", Err: "number out of range", Ru: "Слишком большое число"}
	ErrNumber           = Error{Code: "number", Err: "expected number like 5 or 2.5", Ru: "Ожидается число, например 5 или 2.5"}
	ErrNumberOperator   = Error{Code: "number_operator", Err: "only =, !=, <, <=, >, >= are supported for numbers", Ru: "Для чисел доступны только =, !=, <, <=, >, >="}
	ErrDate             = Error{Code: "date", Err: "expected date like 2024-01-31 or \"2024-01-31 15:30\"", Ru: "Неверная дата, пример: 2024-01-31 или \"2024-01-31 15:30\""}
	ErrDurationUnit     = Error{Code: "duration_unit", Err: "unknown duration unit", Ru: "Неизвестная единица времени"}
	ErrDurationRange    = Error{Code: "duration_range", Err: "duration out of range", Ru: "Слишком большая длительность"}
	ErrDurationFraction = Error{Code: "duration_fraction", Err: "years and months must be whole", Ru: "Годы и месяцы указываются целым числом"}

	ErrFuncNotFound = Error{Code: "func_not_found", Err: "function not found", Ru: "Неизвестная функция"}
	ErrListFunc     = Error{Code: "func_list", Err: "list function requires IN", Ru: "Функция возвращает список: используйте IN или NOT IN"}
	ErrScalarFunc   = Error{Code: "func_scalar", Err: "scalar function after IN must be in parentheses", Ru: "Функция возвращает одно значение: используйте = или возьмите её в скобки"}
	ErrFuncArgs     = Error{Code: "func_args", Err: "wrong number of arguments", Ru: "Неверное число аргументов функции"}
	ErrShiftArg     = Error{Code: "func_shift", Err: "shift must be a number of periods or a duration like -1, -7d or -1M", Ru: "Сдвиг задаётся числом периодов или длительностью, например -1, -7d или -1M"}
	ErrIssueRef     = Error{Code: "issue_ref", Err: "expected issue key like PRJ-123 or issue id", Ru: "Ожидается ключ задачи, например PRJ-123"}
)

// at привязывает ошибку к месту в запросе. Ошибка не из списка выше считается внутренней.
func at(err error, ctx antlr.ParserRuleContext) Error {
	var e Error
	if !errors.As(err, &e) {
		e = ErrInternal
		e.Err = err.Error()
	}
	e.Start = ctx.GetStart().GetStart()
	e.End = ctx.GetStop().GetStop() + 1
	return e
}
