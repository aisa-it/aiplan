package apicontext

import (
	"github.com/aisa-it/aiplan/aiplan.go/pkg/dao"
	"gorm.io/gorm"
)

// Методы-адаптеры к контракту engine.Subject.
//
// Именование в engine короче (User вместо GetUser), чтобы движок читался
// как правила предметной области. Существующие GetX остаются основным
// API для обработчиков ядра.

// DB возвращает соединение, привязанное к запросу.
func (a *APIContext) DB() *gorm.DB { return a.db }

// Err возвращает первую ошибку ленивой загрузки сущностей.
func (a *APIContext) Err() error { return a.Error() }

// User возвращает пользователя запроса (nil для роутов без авторизации).
func (a *APIContext) User() *dao.User { return a.GetUser() }

// Workspace возвращает пространство запроса.
func (a *APIContext) Workspace() *dao.Workspace { return a.GetWorkspace() }

// WorkspaceMember возвращает членство пользователя в пространстве.
func (a *APIContext) WorkspaceMember() *dao.WorkspaceMember { return a.GetWorkspaceMember() }

// Project возвращает проект запроса.
func (a *APIContext) Project() *dao.Project { return a.GetProject() }

// ProjectMember возвращает членство пользователя в проекте.
func (a *APIContext) ProjectMember() *dao.ProjectMember { return a.GetProjectMember() }

// Sprint возвращает спринт запроса.
func (a *APIContext) Sprint() *dao.Sprint { return a.GetSprint() }

// Doc возвращает документ запроса вместе с персональными правами доступа:
// без них списки читателей и редакторов пусты и решение движка неверно.
func (a *APIContext) Doc() *dao.Doc { return a.GetDoc(WithDocAccessRules()) }

// Issue возвращает задачу запроса вместе с исполнителями: движку они
// нужны почти всегда, а повторное обращение берётся из кеша контекста.
func (a *APIContext) Issue() *dao.Issue { return a.GetIssue(WithAssignees()) }
