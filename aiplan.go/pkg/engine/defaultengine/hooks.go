package defaultengine

import (
	"context"
	"log/slog"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/dao"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/engine"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/rules"
)

// Хуки задачи — пользовательские Lua-скрипты проекта (pkg/rules).
//
// Before-хуки не вызываются для администратора проекта: скрипты ограничивают
// участников. After-хук вызывается для всех. Результат каждого вызова
// пишется в rules_log сразу.

// luaHook — вызов одной функции скрипта для подготовленной задачи.
type luaHook func(user dao.User, issue dao.Issue) (rules.LuaResp, []rules.LuaOut, rules.IRulesError)

func (e *Engine) BeforeStateChange(_ context.Context, ev engine.StateTransition) (engine.Verdict, error) {
	if ev.Issue == nil || isProjectAdmin(ev.Subject) {
		return engine.Allow, nil
	}
	return e.runHook(ev.Subject, *ev.Issue, func(user dao.User, issue dao.Issue) (rules.LuaResp, []rules.LuaOut, rules.IRulesError) {
		return rules.BeforeStatusChange(user, issue, ev.To)
	})
}

func (e *Engine) AfterStateChange(_ context.Context, ev engine.StateTransition) error {
	if ev.Issue == nil {
		return nil
	}
	v, err := e.runHook(ev.Subject, *ev.Issue, func(user dao.User, issue dao.Issue) (rules.LuaResp, []rules.LuaOut, rules.IRulesError) {
		return rules.AfterStatusChange(user, issue, ev.To)
	})
	if err != nil {
		return err
	}
	if v.Decision == engine.DecisionDeny && v.Error != nil {
		return *v.Error
	}
	return nil
}

func (e *Engine) BeforeAssigneesChange(_ context.Context, s engine.Subject, issue dao.Issue, newAssignees []dao.User) (engine.Verdict, error) {
	if isProjectAdmin(s) {
		return engine.Allow, nil
	}
	return e.runHook(s, issue, func(user dao.User, issue dao.Issue) (rules.LuaResp, []rules.LuaOut, rules.IRulesError) {
		return rules.BeforeAssigneesChange(user, issue, newAssignees)
	})
}

func (e *Engine) BeforeWatchersChange(_ context.Context, s engine.Subject, issue dao.Issue, newWatchers []dao.User) (engine.Verdict, error) {
	if isProjectAdmin(s) {
		return engine.Allow, nil
	}
	return e.runHook(s, issue, func(user dao.User, issue dao.Issue) (rules.LuaResp, []rules.LuaOut, rules.IRulesError) {
		return rules.BeforeWatchersChange(user, issue, newWatchers)
	})
}

func (e *Engine) BeforeLabelsChange(_ context.Context, s engine.Subject, issue dao.Issue, newLabels []dao.Label) (engine.Verdict, error) {
	if isProjectAdmin(s) {
		return engine.Allow, nil
	}
	return e.runHook(s, issue, func(user dao.User, issue dao.Issue) (rules.LuaResp, []rules.LuaOut, rules.IRulesError) {
		return rules.BeforeLabelsChange(user, issue, newLabels)
	})
}

func (e *Engine) BeforePropertyChange(_ context.Context, s engine.Subject, issue dao.Issue, tpl dao.ProjectPropertyTemplate, newValue string) (engine.Verdict, error) {
	if isProjectAdmin(s) {
		return engine.Allow, nil
	}
	return e.runHook(s, issue, func(user dao.User, issue dao.Issue) (rules.LuaResp, []rules.LuaOut, rules.IRulesError) {
		return rules.BeforeIssuePropertyChange(user, issue, tpl, newValue)
	})
}

// runHook подготавливает задачу, вызывает функцию скрипта и записывает лог.
// Отказ скрипта — Deny с ошибкой для клиента.
func (e *Engine) runHook(s engine.Subject, issue dao.Issue, hook luaHook) (engine.Verdict, error) {
	user := s.User()
	if user == nil {
		return engine.Allow, nil
	}
	if err := prepareIssue(s, &issue); err != nil {
		return engine.Default, err
	}
	if issue.Project == nil || issue.Project.RulesScript == nil {
		return engine.Allow, nil
	}

	res, msg, rerr := hook(*user, issue)

	var logs []dao.RulesLog
	rules.AppendMsg(issue, *user, msg, &logs)
	rules.AppendError(issue, *user, rerr, &logs)
	rules.ResultToLog(issue, *user, res, rerr, &logs)
	if len(logs) > 0 {
		if err := rules.AddLog(s.DB(), logs); err != nil {
			slog.Error("Create rules log", "err", err)
		}
	}

	if !res.ClientResult {
		clientErr := rerr.ClientError()
		return engine.Verdict{Decision: engine.DecisionDeny, Error: &clientErr}, nil
	}
	return engine.Allow, nil
}

// prepareIssue догружает в копию задачи то, чего нет после обычной загрузки,
// но что читает скрипт: проект, статус, пространство, счётчик вложений и
// значения кастомных полей. Без скрипта у проекта ничего не грузится.
func prepareIssue(s engine.Subject, issue *dao.Issue) error {
	if issue.Project == nil {
		issue.Project = s.Project()
		if err := s.Err(); err != nil {
			return err
		}
	}
	if issue.Project == nil || issue.Project.RulesScript == nil {
		return nil
	}

	db := s.DB()
	if issue.State == nil {
		var state dao.State
		if err := db.Where("id = ?", issue.StateId).First(&state).Error; err != nil {
			return err
		}
		issue.State = &state
	}
	if issue.Workspace == nil {
		var workspace dao.Workspace
		if err := db.Where("id = ?", issue.WorkspaceId).First(&workspace).Error; err != nil {
			return err
		}
		issue.Workspace = &workspace
	}
	return rules.EnrichIssue(db, issue)
}

var _ engine.IssueHooks = (*Engine)(nil)
