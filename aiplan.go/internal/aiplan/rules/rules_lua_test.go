// Тесты Lua-контекста правил без базы данных: проверяют, что значения
// кастомных полей (properties, getProp) и attachment_count корректно
// попадают в рантайм Lua-скрипта.
package rules

import (
	"testing"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	lua "github.com/yuin/gopher-lua"
)

// runLuaStatusChange выполняет BeforeStatusChange для задачи со скриптом;
// скрипт должен вернуть {status=true} — иначе тест падает с текстом ошибки скрипта
func runLuaStatusChange(t *testing.T, script string, issue dao.Issue) {
	t.Helper()

	issue.Project = &dao.Project{RulesScript: &script}
	if issue.State == nil {
		issue.State = &dao.State{Name: "Новая", Group: "unstarted"}
	}
	if issue.Workspace == nil {
		issue.Workspace = &dao.Workspace{}
	}

	res, _, err := BeforeStatusChange(
		dao.User{Email: "tester@example.com"},
		issue,
		dao.State{Name: "Выполнена", Group: "completed"},
	)
	if err != nil {
		t.Fatalf("script check failed: %v", err)
	}
	if !res.ClientResult {
		t.Fatal("script returned status=false without error")
	}
}

func luaTestProps() []dao.IssueProperty {
	return []dao.IssueProperty{
		{Value: "готово", Template: &dao.ProjectPropertyTemplate{Name: "Резолюция", Type: "string"}},
		{Value: "true", Template: &dao.ProjectPropertyTemplate{Name: "Срочно", Type: "boolean"}},
		{Value: "false", Template: &dao.ProjectPropertyTemplate{Name: "Согласовано", Type: "boolean"}},
		{Value: "", Template: &dao.ProjectPropertyTemplate{Name: "Канал", Type: "select"}},
		{Value: "", Template: &dao.ProjectPropertyTemplate{Name: "Комментарий", Type: "string"}},
	}
}

func TestLuaGetProp(t *testing.T) {
	script := `
	function BeforeStatusChange(ctx, newstatus)
		if ctx:getProp("Резолюция") ~= "готово" then
			return { status = false, error = "string: " .. tostring(ctx:getProp("Резолюция")) }
		end
		if ctx:getProp("Срочно") ~= true then
			return { status = false, error = "boolean true: " .. tostring(ctx:getProp("Срочно")) }
		end
		if ctx:getProp("Согласовано") ~= false then
			return { status = false, error = "boolean false: " .. tostring(ctx:getProp("Согласовано")) }
		end
		if ctx:getProp("Канал") ~= nil then
			return { status = false, error = "empty select must be nil: " .. tostring(ctx:getProp("Канал")) }
		end
		if ctx:getProp("Комментарий") ~= "" then
			return { status = false, error = "empty string must be ''" }
		end
		if ctx:getProp("Несуществующее") ~= nil then
			return { status = false, error = "missing prop must be nil" }
		end
		return { status = true }
	end
	`
	runLuaStatusChange(t, script, dao.Issue{Properties: luaTestProps()})
}

func TestLuaPropertiesList(t *testing.T) {
	script := `
	function BeforeStatusChange(ctx, newstatus)
		local count = 0
		local found = false
		for _, p in ipairs(ctx.properties) do
			count = count + 1
			if p.name == "Резолюция" and p.type == "string" and p.value == "готово" then
				found = true
			end
		end
		if count ~= 5 then
			return { status = false, error = "expected 5 properties, got " .. count }
		end
		if not found then
			return { status = false, error = "property {Резолюция, string, готово} not found in list" }
		end
		return { status = true }
	end
	`
	runLuaStatusChange(t, script, dao.Issue{Properties: luaTestProps()})
}

func TestLuaAttachmentCount(t *testing.T) {
	script := `
	function BeforeStatusChange(ctx, newstatus)
		if ctx.issue.attachment_count ~= 2 then
			return { status = false, error = "attachment_count: " .. tostring(ctx.issue.attachment_count) }
		end
		return { status = true }
	end
	`
	runLuaStatusChange(t, script, dao.Issue{AttachmentCount: 2})
}

func TestLuaAttachmentCountZeroBlocksClose(t *testing.T) {
	script := `
	function BeforeStatusChange(ctx, newstatus)
		if newstatus.group == "completed" and ctx.issue.attachment_count == 0 then
			return { status = false, error = "no attachments" }
		end
		return { status = true }
	end
	`
	issue := dao.Issue{
		State:     &dao.State{Name: "Новая", Group: "unstarted"},
		Project:   &dao.Project{RulesScript: &script},
		Workspace: &dao.Workspace{},
	}

	res, _, err := BeforeStatusChange(
		dao.User{Email: "tester@example.com"},
		issue,
		dao.State{Name: "Выполнена", Group: "completed"},
	)
	if err == nil || err.Error() != "no attachments" {
		t.Fatalf("expected script error 'no attachments', got: %v", err)
	}
	if res.ClientResult {
		t.Fatal("transition must be blocked by script")
	}
}

func TestLuaPropertyWithoutTemplateSkipped(t *testing.T) {
	script := `
	function BeforeStatusChange(ctx, newstatus)
		if #ctx.properties ~= 1 then
			return { status = false, error = "expected 1 property, got " .. #ctx.properties }
		end
		return { status = true }
	end
	`
	props := []dao.IssueProperty{
		{Value: "без шаблона", Template: nil},
		{Value: "готово", Template: &dao.ProjectPropertyTemplate{Name: "Резолюция", Type: "string"}},
	}
	runLuaStatusChange(t, script, dao.Issue{Properties: props})
}

func TestPropertyValueToLua(t *testing.T) {
	cases := []struct {
		name     string
		propType string
		value    string
		expected lua.LValue
	}{
		{"boolean true", "boolean", "true", lua.LTrue},
		{"boolean false", "boolean", "false", lua.LFalse},
		{"boolean empty", "boolean", "", lua.LFalse},
		{"select filled", "select", "вариант", lua.LString("вариант")},
		{"select empty", "select", "", lua.LNil},
		{"link filled", "link", `{"url":"http://x"}`, lua.LString(`{"url":"http://x"}`)},
		{"link empty", "link", "", lua.LNil},
		{"string filled", "string", "текст", lua.LString("текст")},
		{"string empty", "string", "", lua.LString("")},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := propertyValueToLua(c.propType, c.value); got != c.expected {
				t.Fatalf("propertyValueToLua(%q, %q) = %v, expected %v", c.propType, c.value, got, c.expected)
			}
		})
	}
}
