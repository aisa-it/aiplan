// Обработка и логирование событий, возникающих в процессе выполнения правил.
// Сохраняет информацию о событиях, ошибках и результатах выполнения правил в базе данных.
//
// Основные возможности:
//   - Создание пакета логов в пакетном режиме.
//   - Запись результатов выполнения правил с указанием статуса, времени и сообщений.
//   - Добавление сообщений из Lua-скриптов в лог.
//   - Запись ошибок с подробной информацией, включая Lua-ошибки.
package rules

import (
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func AddLog(tx *gorm.DB, logs []dao.RulesLog) error {
	if err := tx.Omit(clause.Associations).CreateInBatches(logs, 10).Error; err != nil {
		return err
	}
	return nil
}

func ResultToLog(issue dao.Issue, user dao.User, result LuaResp, err IRulesError, logs *[]dao.RulesLog) {
	if !result.ScriptFlowResult {
		return
	}
	var t, msg string
	if result.ClientResult {
		t = "success"
	} else {
		t = "fail"
		msg = err.Error()
		err.SetClientError()
	}
	*logs = append(*logs, dao.RulesLog{
		Id:           dao.GenUUID(),
		CreatedAt:    time.Now(),
		ProjectId:    issue.ProjectId,
		Project:      issue.Project,
		WorkspaceId:  issue.WorkspaceId,
		Workspace:    issue.Workspace,
		IssueId:      issue.ID,
		Issue:        &issue,
		UserId:       uuid.NullUUID{UUID: user.ID, Valid: true},
		User:         &user,
		Time:         result.GetTime(),
		FunctionName: result.GetFnName(),
		Type:         t,
		Msg:          msg,
	})
}

func AppendMsg(issue dao.Issue, user dao.User, msg []LuaOut, logs *[]dao.RulesLog) {
	for _, out := range msg {
		*logs = append(*logs, dao.RulesLog{
			Id:           dao.GenUUID(),
			CreatedAt:    time.Now(),
			ProjectId:    issue.ProjectId,
			Project:      issue.Project,
			WorkspaceId:  issue.WorkspaceId,
			Workspace:    issue.Workspace,
			IssueId:      issue.ID,
			Issue:        &issue,
			UserId:       uuid.NullUUID{UUID: user.ID, Valid: true},
			User:         &user,
			Time:         out.Time,
			FunctionName: &out.FnName,
			Type:         "print",
			Msg:          out.Msg,
		})
	}
}

func AppendError(issue dao.Issue, user dao.User, err IRulesError, logs *[]dao.RulesLog) {
	if err == nil {
		return
	}
	if len(*logs) != 0 && (*logs)[len(*logs)-1].Msg == errParseScript {
		return
	}
	if str, luaErr, ok := err.ScriptError(); ok {
		*logs = append(*logs, dao.RulesLog{
			Id:           dao.GenUUID(),
			CreatedAt:    time.Now(),
			ProjectId:    issue.ProjectId,
			Project:      issue.Project,
			WorkspaceId:  issue.WorkspaceId,
			Workspace:    issue.Workspace,
			IssueId:      issue.ID,
			Issue:        &issue,
			UserId:       uuid.NullUUID{UUID: user.ID, Valid: true},
			User:         &user,
			Time:         err.GetTime(),
			FunctionName: err.GetFnName(),
			Type:         "error",
			Msg:          str,
			LuaErr:       luaErr,
		})
	}
}

func ptrString(s string) *string {
	return &s
}
