// Пакет предоставляет middleware для проверки прав доступа к документам в AIPlan.
//
//	Проверяет, имеет ли пользователь права на чтение, редактирование или просмотр документа, основываясь на его роли и правах пользователя.
//	Поддерживает различные сценарии доступа, включая права автора, суперпользователя, администратора и права на чтение/редактирование.
//
// Основные возможности:
//   - Проверка прав доступа к документам.
//   - Поддержка различных ролей пользователей.
//   - Разграничение прав на чтение, редактирование и просмотр.
//   - Обработка различных HTTP-методов (GET, OPTIONS, HEAD).
package aiplan

import (
	"errors"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"sheff.online/aiplan/internal/aiplan/apierrors"
	"sheff.online/aiplan/internal/aiplan/dao"
	"sheff.online/aiplan/internal/aiplan/utils"
)

func (s *Services) DocPermissionMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		has, err := s.hasDocPermissions(c)
		if err != nil {
			return EError(c, err)
		}
		if !has {
			return EErrorDefined(c, apierrors.ErrDocForbidden)
		}
		return next(c)
	}
}

func (s *Services) hasDocPermissions(c echo.Context) (bool, error) {
	docContext, ok := c.(DocContext)
	if !ok {
		return false, errors.New("wrong context")
	}
	workspaceMember := docContext.WorkspaceMember
	doc := docContext.Doc
	user := docContext.User

	// Allow Author
	if user.ID == doc.CreatedById {
		return true, nil
	}

	if docContext.User.IsSuperuser {
		return true, nil
	}

	//// Allow workspace admin all
	//if docContext.WorkspaceMember.Role == AdminRole {
	//	return true, nil
	//}

	readerSet := utils.SliceToSet(doc.ReaderIDs)
	editorSet := utils.SliceToSet(doc.EditorsIDs)
	watcherSet := utils.SliceToSet(doc.WatcherIDs)

	if onlyReadMethod(c) {
		if hasReadAccess(user.ID, workspaceMember.Role, &doc, readerSet, editorSet, watcherSet) {
			return true, nil
		}
		return false, nil
	}

	if strings.Contains(c.Path(), "/comments/") {
		if hasReadAccess(user.ID, workspaceMember.Role, &doc, readerSet, editorSet, watcherSet) {
			return true, nil
		}
	}

	if hasEditAccess(user.ID, workspaceMember.Role, &doc, editorSet) {
		return true, nil
	}

	return false, nil
}

func onlyReadMethod(c echo.Context) bool {
	switch c.Request().Method {
	case http.MethodGet, http.MethodOptions, http.MethodHead:
		return true
	default:
		return false
	}
}

func hasReadAccess(userID string, role int, doc *dao.Doc, readers, editors, watchers map[string]struct{}) bool {
	if role >= doc.ReaderRole || role >= doc.EditorRole {
		return true
	}
	if _, ok := readers[userID]; ok {
		return true
	}
	if _, ok := editors[userID]; ok {
		return true
	}
	if _, ok := watchers[userID]; ok {
		return true
	}
	return false
}

func hasEditAccess(userID string, role int, doc *dao.Doc, editors map[string]struct{}) bool {
	if role >= doc.EditorRole {
		return true
	}
	if _, ok := editors[userID]; ok {
		return true
	}
	return false
}
