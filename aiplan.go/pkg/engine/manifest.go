package engine

import "github.com/aisa-it/aiplan/aiplan.go/pkg/dto"

// Ключи сущностей, которые фронт умеет переименовывать (dto.EntityNames).
const (
	EntityIssue     = "issue"
	EntityProject   = "project"
	EntityWorkspace = "workspace"
	EntitySprint    = "sprint"
	EntityDoc       = "doc"
	EntityForm      = "form"
	EntityComment   = "comment"
	EntityState     = "state"
	EntityIssueType = "issue_type"
	EntityAssignee  = "assignee"
	EntityWatcher   = "watcher"
	EntityLabel     = "label"
)

// EntityNamer — движок, переименовывающий сущности для интерфейса.
// Опционален: без него фронт получает пустой словарь. Читается один раз
// после Init: ответ не зависит от пользователя и не меняется на ходу.
type EntityNamer interface {
	EntityNames() dto.EntityNames
}
