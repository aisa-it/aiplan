// Пакет содержит правила для выполнения Lua-скриптов, которые могут быть запущены до и после изменения статуса задачи, а также при изменении assignee и watchers.  Он предоставляет функции для взаимодействия с Lua-скриптами, передачи параметров и обработки результатов.
//
// Основные возможности:
//   - Выполнение Lua-скриптов до и после изменения статуса задачи.
//   - Выполнение Lua-скриптов при изменении assignee и watchers задачи.
//   - Передача данных в Lua-скрипт через параметры.
//   - Обработка результатов Lua-скрипта и формирование сообщений.
package rules

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	lua "github.com/yuin/gopher-lua"
)

func BeforeStatusChange(issuer dao.User, currentIssue dao.Issue, newState dao.State) (LuaResp, []LuaOut, IRulesError) {
	return callEventFunction("BeforeStatusChange", nil, issuer, currentIssue, newState)
}

func AfterStatusChange(issuer dao.User, currentIssue dao.Issue, newState dao.State) (LuaResp, []LuaOut, IRulesError) {
	return callEventFunction("AfterStatusChange", nil, issuer, currentIssue, newState)
}

func BeforeAssigneesChange(issuer dao.User, currentIssue dao.Issue, newAssigners []dao.User) (LuaResp, []LuaOut, IRulesError) {
	state := lua.NewState()
	assigneesTable := getUsers(state, newAssigners)
	return callEventFunction("BeforeAssigneesChange", state, issuer, currentIssue, assigneesTable)
}

func BeforeWatchersChange(issuer dao.User, currentIssue dao.Issue, newWatchers []dao.User) (LuaResp, []LuaOut, IRulesError) {
	state := lua.NewState()
	watchersTable := getUsers(state, newWatchers)
	return callEventFunction("BeforeWatchersChange", state, issuer, currentIssue, watchersTable)
}

func BeforeLabelsChange(issuer dao.User, currentIssue dao.Issue, labels []dao.Label) (LuaResp, []LuaOut, IRulesError) {
	state := lua.NewState()
	assigneesTable := getLabels(state, labels)
	return callEventFunction("BeforeLabelsChange", state, issuer, currentIssue, assigneesTable)
}

func callEventFunction(fnName string, state *lua.LState, issuer dao.User, currentIssue dao.Issue, params ...interface{}) (LuaResp, []LuaOut, IRulesError) {

	info := &debugInfo{
		Function:    &fnName,
		ProjectId:   currentIssue.ProjectId,
		IssuerId:    issuer.ID.String(),
		IssuerEmail: issuer.Email,
	}

	errFull := &errDescription{}

	newErr := func(err string) IRulesError {
		info.Time = time.Now()
		return &rulesError{
			Err:     err,
			Info:    info,
			FullErr: errFull,
		}
	}

	resp := func(client, script bool) LuaResp {
		info.Time = time.Now()
		return LuaResp{
			ClientResult:     client,
			ScriptFlowResult: script,
			Info:             info,
		}
	}

	if currentIssue.Project == nil || currentIssue.Project.RulesScript == nil {
		return resp(true, false), nil, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if state == nil {
		state = lua.NewState()
	}

	defer state.Close()

	deniedLib(state)
	registerLogger(state)

	errChan := make(chan IRulesError, 1)
	go func() {
		if err := state.DoString(*currentIssue.Project.RulesScript); err != nil {
			luaErr := strings.TrimSpace(err.Error())
			errFull.ErrMsg = errParseScript
			errFull.LuaError = &luaErr
			errChan <- newErr(errScript)
		}
		close(errChan)
	}()

	select {
	case <-ctx.Done():
		errFull.ErrMsg = "Lua execution timed out"
		return resp(true, false), nil, newErr(errScript)
	case err := <-errChan:
		if err != nil {
			return resp(true, false), nil, err
		}
	}

	fn := state.GetGlobal(fnName)
	if fn == lua.LNil {
		return resp(true, false), nil, nil
	}

	args := make([]lua.LValue, len(params)+1)
	args[0] = getCallParams(state, issuer, currentIssue)

	for i, param := range params {
		args[i+1] = getStructLTable(state, param)
	}

	resultChan := make(chan lua.LValue, 1)
	go func() {
		if err := state.CallByParam(lua.P{
			Fn:      fn,
			NRet:    1,
			Protect: true,
		}, args...); err != nil {
			luaErr := strings.TrimSpace(err.Error())
			errFull.ErrMsg = "Script error"
			errFull.LuaError = &luaErr
			resultChan <- lua.LNil
		} else {
			resultChan <- state.Get(-1)
		}
	}()

	select {
	case <-ctx.Done():
		errFull.ErrMsg = "Lua execution timed out"
		return resp(true, false), nil, newErr(errScript)
	case ret := <-resultChan:
		var messages []LuaOut
		if messagesTable := state.GetGlobal("messages"); messagesTable.Type() == lua.LTTable {
			messagesTableLen := messagesTable.(*lua.LTable).Len()
			for i := 1; i <= messagesTableLen; i++ {
				entry := messagesTable.(*lua.LTable).RawGetInt(i).(*lua.LTable)
				msg := entry.RawGetString("msg").String()
				timeStr := entry.RawGetString("time").String()
				var msgTime time.Time
				if len(timeStr) > 0 {
					seconds, err := strconv.ParseInt(timeStr[:10], 10, 64)
					nanoseconds, err2 := strconv.ParseInt(timeStr[11:], 10, 64)
					if err == nil && err2 == nil {
						msgTime = time.Unix(seconds, nanoseconds)
					}
				}

				messages = append(messages, LuaOut{
					Msg:    msg,
					Time:   msgTime,
					FnName: fnName,
				})
			}
		}

		if ret == lua.LNil {
			return resp(true, false), messages, newErr(errScript)
		}

		retTable, ok := ret.(*lua.LTable)
		if !ok {
			luaErr := strings.TrimSpace(fmt.Sprintf("%T", ret))
			errFull.ErrMsg = "Unexpected return type from Lua script expected table"
			errFull.LuaError = &luaErr
			return resp(true, false), messages, newErr(errScript)
		}

		status := retTable.RawGetString("status")
		errStr := retTable.RawGetString("error")

		if status == lua.LNil {
			errFull.ErrMsg = "Lua table missing 'status' key"
			return resp(true, false), messages, newErr(errScript)
		}

		if errStr != lua.LNil {
			return resp(false, true), messages, newErr(errStr.String())
		}

		if status == lua.LTrue {
			return resp(true, true), messages, nil
		} else {
			return resp(false, true), messages, newErr(errScript)
		}
	}
}

func getStructLTable(state *lua.LState, obj interface{}) *lua.LTable {
	table := state.NewTable()

	if luaTable, ok := obj.(*lua.LTable); ok {
		return luaTable
	}

	v := reflect.ValueOf(obj)
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	t := v.Type()

	for i := 0; i < v.NumField(); i++ {
		tag := t.Field(i).Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}

		field := v.Field(i)

		switch f := field.Interface().(type) {
		case int, int8, int16, int32, int64:
			table.RawSetString(tag, lua.LNumber(field.Int()))
		case float32, float64:
			table.RawSetString(tag, lua.LNumber(field.Float()))
		case string:
			table.RawSetString(tag, lua.LString(field.String()))
		case time.Time:
			table.RawSetString(tag, lua.LNumber(f.Unix()))
		default:
			continue
		}
	}
	return table
}

func getCallParams(state *lua.LState, issuer dao.User, currentIssue dao.Issue) *lua.LTable {
	params := state.NewTable()
	params.RawSetString("user", getStructLTable(state, issuer))
	params.RawSetString("status", getStructLTable(state, *currentIssue.State))
	params.RawSetString("project", getStructLTable(state, *currentIssue.Project))
	params.RawSetString("space", getStructLTable(state, *currentIssue.Workspace))
	params.RawSetString("issue", getStructLTable(state, currentIssue))

	metaTable := state.NewTable()

	state.SetFuncs(metaTable, map[string]lua.LGFunction{
		"compareUserEmail": func(L *lua.LState) int {
			self := L.CheckTable(1)
			inputEmail := L.CheckString(2)
			userTable := self.RawGetString("user").(*lua.LTable)
			userEmail := userTable.RawGetString("email").String()
			if inputEmail == userEmail {
				L.Push(lua.LBool(true))
			} else {
				L.Push(lua.LBool(false))
			}

			return 1
		},
	})

	state.SetFuncs(metaTable, map[string]lua.LGFunction{
		"compareStatusName": func(L *lua.LState) int {
			self := L.CheckTable(1)
			inputStatus := L.CheckString(2)
			statusTable := self.RawGetString("status").(*lua.LTable)
			oldStatus := statusTable.RawGetString("name").String()
			if inputStatus == oldStatus {
				L.Push(lua.LBool(true))
			} else {
				L.Push(lua.LBool(false))
			}

			return 1
		},
	})
	state.SetField(metaTable, "__index", metaTable)
	state.SetMetatable(params, metaTable)

	return params
}
