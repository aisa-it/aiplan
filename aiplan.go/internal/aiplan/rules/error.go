// Файл error.go определяет типы ошибок для системы правил.
//
// IRulesError — интерфейс ошибки, который помимо стандартного Error() предоставляет:
//   - GetTime/GetFnName — информация для логирования (когда и в какой функции)
//   - ScriptError — детали ошибки Lua (текст ошибки парсера/рантайма)
//   - ClientError — преобразование в HTTP-ошибку для ответа клиенту
//
// Ошибки делятся на два типа:
//   - errScript ("prohibition of committing an action") — скрипт вернул status=false,
//     действие запрещено бизнес-логикой
//   - errParseScript — синтаксическая ошибка в Lua-коде
package rules

import (
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/apierrors"
)

type IRulesError interface {
	error
	GetTime() time.Time
	GetFnName() *string
	ScriptError() (string, *string, bool)
	ClientError() apierrors.DefinedError
	SetClientError()
}

const errScript = "prohibition of committing an action"
const errParseScript = "error parsing lua script"

type rulesError struct {
	Err     string          `json:"err,omitempty"`
	FullErr *errDescription `json:"full_err,omitempty"`
	Info    *debugInfo      `json:"info,omitempty"`
	Fail    bool            `json:"fail"`
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

func (e *rulesError) ClientError() apierrors.DefinedError {
	if e.Err == errScript {
		return apierrors.ErrIssueScriptFail
	}
	if e.Fail {
		err := apierrors.ErrIssueCustomScriptFail
		err.RuErr = e.Err
		err.Err = e.Err
		return err
	}
	return apierrors.ErrGeneric
}

func (e *rulesError) SetClientError() {
	e.Fail = true
}
