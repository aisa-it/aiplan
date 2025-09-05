// Содержит типы ошибок, возникающих при обработке правил и скриптов.  Предоставляет информацию о времени возникновения ошибки, имени функции и детальное описание ошибки (включая Lua ошибки). Используется для логирования и обработки ошибок в системе правил.
//
// Основные возможности:
//   - Представление ошибок с информацией о времени, функции и детальном описании.
//   - Извлечение информации об ошибке для логирования и обработки.
//   - Предоставление детального описания ошибки, включая Lua ошибки, при наличии.
package rules

import "time"

type IRulesError interface {
	error
	GetTime() time.Time
	GetFnName() *string
	ScriptError() (string, *string, bool)
}

const errScript = "project script error"
const errParseScript = "error parsing lua script"

type rulesError struct {
	Err     string          `json:"err,omitempty"`
	FullErr *errDescription `json:"full_err,omitempty"`
	Info    *debugInfo      `json:"info,omitempty"`
}

type errDescription struct {
	ErrMsg   string  `json:"err_msg,omitempty"`
	LuaError *string `json:"lua_error,omitempty"`
}

func (e *rulesError) GetTime() time.Time {
	return e.Info.Time
}

func (e *rulesError) GetFnName() *string {
	return e.Info.Function
}

func (e *rulesError) Error() string {
	return e.Err
}

func (e *rulesError) ScriptError() (string, *string, bool) {
	if e.FullErr.ErrMsg == "" {
		return "", nil, false
	}
	return e.FullErr.ErrMsg, e.FullErr.LuaError, true
}
