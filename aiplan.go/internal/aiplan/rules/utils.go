// Содержит правила проверки данных и функции для работы с данными в Lua. Используется для валидации и обработки данных в контексте Lua-скриптов.
//
// Основные возможности:
//   - Проверка соответствия строки значению в таблице.
//   - Проверка соответствия строки значению в таблице.
//   - Генерация таблиц с данными пользователей и меток для использования в Lua-скриптах.
//   - Обработка ошибок и логирование событий в Lua.
package rules

import (
	"fmt"
	"time"

	"github.com/aisa-it/aiplan/internal/aiplan/dao"
	lua "github.com/yuin/gopher-lua"
)

var (
	CheckEmail func(L *lua.LState) int
	CheckName  func(L *lua.LState) int
)

func init() {
	CheckEmail = createCheckFieldFunc("email")
	CheckName = createCheckFieldFunc("name")
}

type LuaOut struct {
	Msg    string
	Time   time.Time
	FnName string
}

type LuaResp struct {
	ClientResult     bool
	ScriptFlowResult bool
	Info             *debugInfo
}

type debugInfo struct {
	Function    *string   `json:"function"`
	ProjectId   string    `json:"project_id"`
	IssuerId    string    `json:"issuer_id"`
	IssuerEmail string    `json:"issuer_email"`
	Time        time.Time `json:"time"`
}

func (r *LuaResp) GetTime() time.Time {
	return r.Info.Time
}

func (r *LuaResp) GetFnName() *string {
	return r.Info.Function
}

func deniedLib(state *lua.LState) {
	state.SetGlobal("require", lua.LNil)
	state.SetGlobal("loadfile", lua.LNil)
	state.SetGlobal("dofile", lua.LNil)
	state.SetGlobal("net", lua.LNil)
	state.SetGlobal("debug", lua.LNil)
	state.SetGlobal("coroutine", lua.LNil)
	state.SetGlobal("socket", lua.LNil)
	state.SetGlobal("lfs", lua.LNil)
	state.SetGlobal("os", lua.LNil)
	state.SetGlobal("io", lua.LNil)
	state.SetGlobal("package", lua.LNil)
	state.SetGlobal("ffi", lua.LNil)
}

func createCheckFieldFunc(fieldName string) lua.LGFunction {
	return func(L *lua.LState) int {
		self := L.CheckTable(1)
		fieldValue := L.CheckString(2)
		for i := 1; i <= self.Len(); i++ {
			entry := self.RawGetInt(i).(*lua.LTable)
			valueInTable := entry.RawGetString(fieldName).String()

			if valueInTable == fieldValue {
				L.Push(lua.LTrue)
				return 1
			}
		}
		L.Push(lua.LFalse)
		return 1
	}
}

func registerLogger(state *lua.LState) {
	messages := state.NewTable()
	state.SetGlobal("messages", messages)
	state.SetGlobal("print", state.NewFunction(func(L *lua.LState) int {
		var message string
		numArgs := L.GetTop()
		for i := 1; i <= numArgs; i++ {
			arg := L.ToString(i)
			if i > 1 {
				message += " "
			}
			message += arg
		}
		msgTable := L.NewTable()
		msgTable.RawSetString("msg", lua.LString(message))
		currentTime := time.Now()
		formattedTime := fmt.Sprintf("%d.%09d", currentTime.Unix(), currentTime.Nanosecond())
		msgTable.RawSetString("time", lua.LString(formattedTime))
		messages.Append(msgTable)
		return 0
	}))
}

func getLabels(state *lua.LState, labels []dao.Label) *lua.LTable {
	labelTable := state.NewTable()
	for _, label := range labels {
		labelTable.Append(getStructLTable(state, label))
	}

	metaTable := state.NewTable()
	state.SetFuncs(metaTable, map[string]lua.LGFunction{"checkName": CheckName})
	state.SetField(metaTable, "__index", metaTable)
	state.SetMetatable(labelTable, metaTable)
	return labelTable
}

func getUsers(state *lua.LState, users []dao.User) *lua.LTable {
	usersTable := state.NewTable()
	for _, assigner := range users {
		usersTable.Append(getStructLTable(state, assigner))
	}

	metaTable := state.NewTable()

	state.SetFuncs(metaTable, map[string]lua.LGFunction{"checkEmail": CheckEmail})
	state.SetField(metaTable, "__index", metaTable)
	state.SetMetatable(usersTable, metaTable)

	return usersTable
}
