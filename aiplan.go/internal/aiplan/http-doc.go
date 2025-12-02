// Пакет aiplan предоставляет функциональность для управления документами в системе, включая создание, редактирование, перемещение, добавление реакций и историю изменений. Он предназначен для организации и отслеживания работы с документами в рамках рабочих пространств, обеспечивая удобный интерфейс для пользователей.  Пакет включает в себя механизмы для работы с версиями документов, добавления комментариев и управления правами доступа.
package aiplan

import (
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/url"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/apierrors"
	errStack "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/stack-error"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/limiter"

	tracker "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/activity-tracker"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dto"
	filestorage "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/file-storage"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/gofrs/uuid"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type DocContext struct {
	WorkspaceContext
	Doc dao.Doc
}

func (s *Services) DocMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		docId := c.Param("docId")
		workspace := c.(WorkspaceContext).Workspace
		workspaceMember := c.(WorkspaceContext).WorkspaceMember
		var doc dao.Doc
		if err := s.db.
			Set("member_id", workspaceMember.MemberId).
			Set("member_role", workspaceMember.Role).
			Set("breadcrumbs", true).
			Preload("AccessRules.Member").
			Preload("ParentDoc").
			Preload("Author").
			Preload("Workspace").
			Preload("InlineAttachments").
			Where("docs.workspace_id = ?", workspace.ID).
			Where("docs.reader_role <= ? OR docs.editor_role <= ? OR EXISTS (SELECT 1 FROM doc_access_rules dar WHERE dar.doc_id = docs.id AND dar.member_id = ?) OR docs.created_by_id = ?",
				workspaceMember.Role, workspaceMember.Role, workspaceMember.MemberId, workspaceMember.MemberId).
			Where("docs.id = ?", docId).
			First(&doc).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return EErrorDefined(c, apierrors.ErrDocNotFound)
			}
			return EErrorDefined(c, apierrors.ErrGeneric)
		}

		return next(DocContext{c.(WorkspaceContext), doc})
	}
}

func (s *Services) AddDocServices(g *echo.Group) {
	workspaceGroup := g.Group("workspaces/:workspaceSlug",
		s.WorkspaceMiddleware,
		s.WorkspacePermissionMiddleware,
		s.LastVisitedWorkspaceMiddleware,
	)
	docGroup := workspaceGroup.Group("/doc/:docId",
		s.DocMiddleware,
		s.DocPermissionMiddleware,
	)

	workspaceGroup.GET("/doc/", s.getRootDocList)
	workspaceGroup.POST("/doc/", s.createRootDoc)

	workspaceGroup.POST("/user-favorite-docs/", s.addDocToFavorites)
	workspaceGroup.GET("/user-favorite-docs/", s.getFavoriteDocList)
	workspaceGroup.DELETE("/user-favorite-docs/:docId/", s.removeDocFromFavorites)

	docGroup.GET("/", s.getDoc)
	docGroup.POST("/", s.createDoc)
	docGroup.PATCH("/", s.updateDoc)
	docGroup.DELETE("/", s.deleteDoc)
	docGroup.POST("/move/", s.moveDoc)

	docGroup.GET("/child/", s.getChildDocList)
	docGroup.GET("/history/", s.getDocHistoryList)
	docGroup.GET("/history/:versionId/", s.getDocHistory)
	docGroup.PATCH("/history/:versionId/", s.updateDocFromHistory)

	docGroup.GET("/comments/", s.getDocCommentList)
	docGroup.POST("/comments/", s.createDocComment)
	docGroup.GET("/comments/:commentId/", s.getDocComment)
	docGroup.PATCH("/comments/:commentId/", s.updateDocComment)
	docGroup.DELETE("/comments/:commentId/", s.deleteDocComment)

	docGroup.POST("/comments/:commentId/reactions/", s.addDocCommentReaction)
	docGroup.DELETE("/comments/:commentId/reactions/:reaction/", s.removeDocCommentReaction)

	docGroup.GET("/doc-attachments/", s.getDocAttachmentList)
	docGroup.POST("/doc-attachments/", s.createDocAttachments)
	docGroup.DELETE("/doc-attachments/:attachmentId/", s.deleteDocAttachment)

	docGroup.GET("/activities/", s.getDocActivityList)
}

// getRootDocList godoc
// @id getRootDocList
// @Summary doc: получение всех корневых документов
// @Description Возвращает список коневых документов
// @Tags Docs
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Success 200 {array} dto.DocLight "информация о документах"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Ошибка: рабочее пространство не найдено"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/doc/ [get]
func (s *Services) getRootDocList(c echo.Context) error {
	workspace := c.(WorkspaceContext).Workspace
	workspaceMember := c.(WorkspaceContext).WorkspaceMember

	var docs []dao.Doc
	if err := s.db.
		Set("member_role", workspaceMember.Role).
		Set("member_id", workspaceMember.MemberId).
		Preload("Author").
		Where("docs.workspace_id = ?", workspace.ID).
		Where("docs.reader_role <= ? OR docs.editor_role <= ? OR EXISTS (SELECT 1 FROM doc_access_rules dar WHERE dar.doc_id = docs.id AND dar.member_id = ?) OR docs.created_by_id = ?",
			workspaceMember.Role, workspaceMember.Role, workspaceMember.MemberId, workspaceMember.MemberId).
		Where("docs.parent_doc_id IS NULL").
		Order("seq_id ASC").
		Group("docs.id").
		Find(&docs).Error; err != nil {
		return EError(c, apierrors.ErrGeneric)
	}

	return c.JSON(http.StatusOK,
		utils.SliceToSlice(&docs, func(d *dao.Doc) dto.DocLight { return *d.ToLightDTO() }))
}

// getDoc godoc
// @id getDoc
// @Summary doc: получение документа
// @Description получение документа
// @Tags Docs
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param docId path string true "Id документа"
// @Success 200 {object} dto.Doc "документ"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Ошибка: не найдено"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/doc/{docId}/ [get]
func (s *Services) getDoc(c echo.Context) error {
	doc := c.(DocContext).Doc
	return c.JSON(http.StatusOK, doc.ToDTO())
}

// createRootDoc godoc
// @id createRootDoc
// @Summary doc: добавление корневого документа
// @Description добавление корневого документа
// @Tags Docs
// @Security ApiKeyAuth
// @Accept multipart/form-data
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param doc formData string true "документ в формате JSON"  example({"title": "title text", "content": "<p>HTML-контент</p>", "reader_role": 5, "editor_role":10, "seq_id": 0, "draft": false})
// @Param files formData file false "Вложения для документа"
// @Success 200 {object} dto.Doc "документ"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Ошибка: рабочее пространство не найдено"
// @Failure 413 {object} apierrors.DefinedError "Большой объем файла"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/doc/ [post]
func (s *Services) createRootDoc(c echo.Context) error {
	user := c.(WorkspaceContext).User
	workspace := c.(WorkspaceContext).Workspace
	workspaceMember := c.(WorkspaceContext).WorkspaceMember

	if workspaceMember.Role <= types.GuestRole {
		return EErrorDefined(c, apierrors.ErrDocForbidden)
	}

	doc, _, err := BindDoc(c, nil)
	if err != nil {
		return EError(c, err)
	}

	form, _ := c.MultipartForm()

	if err := s.db.Transaction(func(tx *gorm.DB) error {

		if err := dao.CreateDoc(tx, doc, user); err != nil {
			return err
		}

		fileAsset := dao.FileAsset{
			Id:          dao.GenUUID(),
			CreatedById: &user.ID,
			WorkspaceId: &workspace.ID,
			DocId: uuid.NullUUID{
				UUID:  doc.ID,
				Valid: true,
			},
		}

		attachments, err := s.uploadDocAttachments(tx, form, "files", fileAsset)
		if err != nil {
			return err
		}

		doc.InlineAttachments = attachments
		return nil
	}); err != nil {
		if err.Error() == "forbidden" {
			return EErrorDefined(c, apierrors.ErrDocUpdateForbidden)
		}
		return EError(c, err)
	}

	err = tracker.TrackActivity[dao.Doc, dao.WorkspaceActivity](s.tracker, tracker.ENTITY_CREATE_ACTIVITY, nil, nil, *doc, user)
	if err != nil {
		errStack.GetError(c, err)
	}

	return c.JSON(http.StatusOK, doc.ToDTO())
}

// createDoc godoc
// @id createDoc
// @Summary doc: добавление документа
// @Description добавление документа
// @Tags Docs
// @Security ApiKeyAuth
// @Accept multipart/form-data
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param docId path string true "Id документа"
// @Param doc formData string true "документ в формате JSON" example({"title": "title text", "content": "<p>HTML-контент</p>", "reader_role": 5, "editor_role":10, "seq_id": 0, "draft": false})
// @Param files formData file false "Вложения для документа"
// @Success 200 {object} dto.Doc "документ"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Ошибка: не найдено"
// @Failure 413 {object} apierrors.DefinedError "Большой объем файла"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/doc/{docId}/  [post]
func (s *Services) createDoc(c echo.Context) error {
	parentDoc := c.(DocContext).Doc
	workspace := c.(DocContext).Workspace
	user := c.(DocContext).User

	doc, fields, err := BindDoc(c, nil)
	if err != nil {
		return EError(c, err)
	}

	fieldMap := utils.SliceToSet(fields)
	if _, ok := fieldMap["editor_role"]; !ok {
		doc.EditorRole = parentDoc.EditorRole
	}
	if _, ok := fieldMap["reader_role"]; !ok {
		doc.ReaderRole = parentDoc.ReaderRole
	}

	form, _ := c.MultipartForm()
	doc.ParentDocID = uuid.NullUUID{UUID: parentDoc.ID, Valid: true}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := dao.CreateDoc(tx, doc, user); err != nil {
			return err
		}

		if doc.ReaderRole < parentDoc.ReaderRole {
			return apierrors.ErrDocChildRoleTooLow
		}

		if doc.EditorRole < parentDoc.EditorRole {
			return apierrors.ErrDocChildRoleTooLow
		}

		fileAsset := dao.FileAsset{
			Id:          dao.GenUUID(),
			CreatedById: &user.ID,
			WorkspaceId: &workspace.ID,
			DocId: uuid.NullUUID{
				UUID:  doc.ID,
				Valid: true,
			},
		}

		attachments, err := s.uploadDocAttachments(tx, form, "files", fileAsset)
		if err != nil {
			return err
		}

		doc.InlineAttachments = attachments
		return nil
	}); err != nil {
		if err.Error() == "forbidden" {
			return EErrorDefined(c, apierrors.ErrDocUpdateForbidden)
		}
		return EError(c, err)
	}

	reqMap := make(map[string]interface{})
	reqMap["entityParent"] = parentDoc

	err = tracker.TrackActivity[dao.Doc, dao.DocActivity](s.tracker, tracker.ENTITY_CREATE_ACTIVITY, reqMap, nil, *doc, user)
	if err != nil {
		errStack.GetError(c, err)
	}

	return c.JSON(http.StatusOK, doc.ToDTO())
}

// updateDoc godoc
// @id updateDoc
// @Summary doc: изменение документа
// @Description изменение документа
// @Tags Docs
// @Security ApiKeyAuth
// @Accept multipart/form-data
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param docId path string true "Id документа"
// @Param doc formData string true "документ в формате JSON" example({"title": "title text", "content": "<p>HTML-контент</p>", "reader_role": 5, "editor_role":10, "seq_id": 0, "draft": false})
// @Param files formData file false "Вложения для документа"
// @Success 200 {object} dto.Doc "документ"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Ошибка: не найдено"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/doc/{docId}/  [patch]
func (s *Services) updateDoc(c echo.Context) error {
	doc := c.(DocContext).Doc
	user := c.(DocContext).User
	workspace := c.(DocContext).Workspace
	workspaceMember := c.(DocContext).WorkspaceMember

	oldDocMap := StructToJSONMap(doc)

	newDoc, fields, err := BindDoc(c, &doc)
	if err != nil {
		return EError(c, err)
	}
	form, _ := c.MultipartForm()

	if utils.CheckInSet(utils.SliceToSet(fields), "editor_role", "reader_role", "editor_list", "reader_list", "watcher_list") {
		if doc.CreatedById != user.ID && workspaceMember.Role != types.AdminRole {
			return EErrorDefined(c, apierrors.ErrDocForbidden)
		}
	}

	var editorListOk, readerListOk, watcherListOk bool

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		fileAsset := dao.FileAsset{
			Id:          dao.GenUUID(),
			CreatedById: &user.ID,
			WorkspaceId: &workspace.ID,
			DocId: uuid.NullUUID{
				UUID:  doc.ID,
				Valid: true,
			},
		}

		attachments, err := s.uploadDocAttachments(tx, form, "files", fileAsset)
		if err != nil {
			return err
		}

		userIds := append(newDoc.EditorsIDs, append(newDoc.WatcherIDs, newDoc.ReaderIDs...)...)
		var users []dao.User
		if len(userIds) > 0 {
			if err := tx.Where("id IN (?)", userIds).Find(&users).Error; err != nil {
				return err
			}
		}

		if len(fields) > 0 {
			workspaceUUID := uuid.Must(uuid.FromString(workspace.ID))
			userUUID := uuid.Must(uuid.FromString(user.ID))

			memberAccess := make(map[string]dao.DocAccessRules)

			newIdsSet := make(map[string]bool)
			updateIdsSet := make(map[string]bool)
			deleteIdsSet := make(map[string]bool)

			oldRoleAccess := utils.SliceToMap(&doc.AccessRules, func(r *dao.DocAccessRules) string { return r.MemberId.String() })

			newMemberAccess := func(id string) dao.DocAccessRules {
				return dao.DocAccessRules{
					Id:          dao.GenUUID(),
					MemberId:    uuid.Must(uuid.FromString(id)),
					CreatedById: userUUID,
					DocId:       doc.ID,
					UpdatedById: uuid.NullUUID{},
					WorkspaceId: workspaceUUID,
					Edit:        false,
					Watch:       false,
					Workspace:   &workspace,
					Doc:         &doc,
					Member:      user,
				}
			}

			updateMember := func(id string, editor, watcher *bool) {
				if v, exists := oldRoleAccess[id]; exists {
					if editor != nil {
						v.Edit = *editor
					}
					if watcher != nil {
						v.Watch = *watcher
					}
					v.UpdatedById = uuid.NullUUID{UUID: userUUID, Valid: true}
					memberAccess[id] = v
					updateIdsSet[id] = true
				} else if val, ok := memberAccess[id]; ok {
					if editor != nil {
						val.Edit = *editor
					}
					if watcher != nil {
						val.Watch = *watcher
					}
					val.UpdatedById = uuid.NullUUID{UUID: userUUID, Valid: true}
					memberAccess[id] = val
					updateIdsSet[id] = true
				} else {
					ma := newMemberAccess(id)
					if editor != nil {
						ma.Edit = *editor
					}
					if watcher != nil {
						ma.Watch = *watcher
					}
					memberAccess[id] = ma
					newIdsSet[id] = true
				}
			}

			for _, field := range fields {

				switch field {
				case "editor_role", "reader_role":
					if field == "reader_role" {
						if newDoc.ParentDoc != nil && newDoc.ReaderRole < newDoc.ParentDoc.ReaderRole {
							return apierrors.ErrDocChildRoleTooLow
						}
					}
					if field == "editor_role" {
						if newDoc.ParentDoc != nil && newDoc.EditorRole < newDoc.ParentDoc.EditorRole {
							return apierrors.ErrDocChildRoleTooLow
						}
					}

					var childDocs []dao.Doc
					if err := tx.Where("parent_doc_id = ?", newDoc.ID).Find(&childDocs).Error; err != nil {
						return err
					}

					for _, childDoc := range childDocs {
						if field == "reader_role" {
							if childDoc.ReaderRole < newDoc.ReaderRole {
								return apierrors.ErrDocParentRoleTooLow
							}
						}
						if field == "editor_role" {
							if childDoc.EditorRole < newDoc.EditorRole {
								return apierrors.ErrDocParentRoleTooLow
							}
						}
					}

				case "reader_list":
					readerListOk = true
					for _, readerID := range newDoc.ReaderIDs {
						updateMember(readerID, utils.ToPtr(false), nil)
					}

				case "editor_list":
					editorListOk = true
					for _, editorID := range newDoc.EditorsIDs {
						updateMember(editorID, utils.ToPtr(true), nil)
					}

				case "watcher_list":
					watcherListOk = true
					for _, watcherID := range newDoc.WatcherIDs {
						updateMember(watcherID, nil, utils.ToPtr(true))
					}

					_, removedWatchers := calculateChanges(newDoc.WatcherIDs, doc.WatcherIDs)
					for _, removedID := range removedWatchers {
						if existing, exists := oldRoleAccess[removedID]; exists {
							existing.Watch = false
							existing.UpdatedById = uuid.NullUUID{UUID: userUUID, Valid: true}
							memberAccess[removedID] = existing
							updateIdsSet[removedID] = true
						}
					}
				}
			}

			if readerListOk || editorListOk {
				oldAccessIds := utils.MergeSlices(doc.EditorsIDs, doc.ReaderIDs)
				newAccessIds := utils.MergeSlices(newDoc.EditorsIDs, newDoc.ReaderIDs)

				_, removedAccessIds := calculateChanges(newAccessIds, oldAccessIds)
				for _, removedID := range removedAccessIds {
					deleteIdsSet[removedID] = true
				}
			}

			if len(newIdsSet) > 0 {
				newRoles := make([]dao.DocAccessRules, 0, len(newIdsSet))
				for id := range newIdsSet {
					newRoles = append(newRoles, memberAccess[id])
				}
				if err := tx.Omit(clause.Associations).CreateInBatches(&newRoles, 10).Error; err != nil {
					return err
				}
			}

			if len(updateIdsSet) > 0 {
				for id := range updateIdsSet {
					accessRule := memberAccess[id]
					if err := tx.Model(&dao.DocAccessRules{}).
						Where("doc_id = ?", doc.ID, id).
						Where("member_id = ?", id).
						Updates(map[string]interface{}{
							"edit":          accessRule.Edit,
							"watch":         accessRule.Watch,
							"updated_by_id": accessRule.UpdatedById,
						}).Error; err != nil {
						return err
					}
				}
			}

			if len(deleteIdsSet) > 0 {
				deleteIds := make([]string, 0, len(deleteIdsSet))
				for id := range deleteIdsSet {
					deleteIds = append(deleteIds, id)
				}
				if err := tx.Where("doc_id = ? AND member_id IN (?)", doc.ID, deleteIds).Delete(&dao.DocAccessRules{}).Error; err != nil {
					return err
				}
			}

			if err := tx.Omit(clause.Associations).Select(fields).Updates(&newDoc).Error; err != nil {
				return err
			}
		}

		newDoc.InlineAttachments = attachments

		return nil
	}); err != nil {
		if err.Error() == "forbidden" {
			return EErrorDefined(c, apierrors.ErrDocUpdateForbidden)
		}
		return EError(c, err)
	}

	newDocMap := StructToJSONMap(newDoc)
	if watcherListOk {
		newDocMap["watchers_list"] = utils.SliceToSlice(&newDoc.WatcherIDs, func(t *string) interface{} {
			return *t
		})
	}
	if editorListOk {
		newDocMap["editors_list"] = utils.SliceToSlice(&newDoc.EditorsIDs, func(t *string) interface{} {
			return *t
		})
	}
	if readerListOk {
		newDocMap["readers_list"] = utils.SliceToSlice(&newDoc.ReaderIDs, func(t *string) interface{} {
			return *t
		})
	}
	err = tracker.TrackActivity[dao.Doc, dao.DocActivity](s.tracker, tracker.ENTITY_UPDATED_ACTIVITY, newDocMap, oldDocMap, doc, user)
	if err != nil {
		errStack.GetError(c, err)
	}

	return c.JSON(http.StatusOK, newDoc.ToDTO())
}

func calculateChanges(newIds, oldIds []string) (added []string, removed []string) {
	newSet := make(map[string]bool)
	oldSet := make(map[string]bool)

	for _, id := range newIds {
		newSet[id] = true
	}
	for _, id := range oldIds {
		oldSet[id] = true
	}

	for id := range newSet {
		if !oldSet[id] {
			added = append(added, id)
		}
	}
	for id := range oldSet {
		if !newSet[id] {
			removed = append(removed, id)
		}
	}

	return added, removed
}

// deleteDoc godoc
// @id deleteDoc
// @Summary doc: удаление документа
// @Description удаление документа
// @Tags Docs
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param docId path string true "Id документа"
// @Success 200 "документ удален"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Ошибка: не найдено"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/doc/{docId}/  [delete]
func (s *Services) deleteDoc(c echo.Context) error {
	doc := c.(DocContext).Doc
	user := c.(DocContext).User

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if len(doc.ChildDocs) > 0 {
			return EErrorDefined(c, apierrors.ErrDocDeleteHasChild)
		}
		data := make(map[string]interface{})
		if err := createDocActivity(s.tracker, tracker.ENTITY_DELETE_ACTIVITY, data, nil, doc, user, nil); err != nil {
			errStack.GetError(c, err)
			return err
		}

		return s.db.Delete(&doc).Error
	}); err != nil {
		if err.Error() == "forbidden" {
			return EErrorDefined(c, apierrors.ErrDocUpdateForbidden)
		}
		return EError(c, err)
	}

	return c.NoContent(http.StatusOK)
}

type DocMoveAction int

const (
	ActionAdd DocMoveAction = iota
	ActionSort
	ActionDelete
)

type docMove struct {
	OldSecId  int
	NewSecId  int
	Type      DocMoveAction
	ActionDoc bool
}

type docChanges struct {
	FromDoc *dao.Doc
	ToDoc   *dao.Doc
}

// moveDoc godoc
// @id moveDoc
// @Summary doc: перенос документа
// @Description перенос документа
// @Tags Docs
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param docId path string true "Id документа"
// @Success 200 "документ перемещен"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Ошибка: не найдено"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/doc/{docId}/move/  [post]
func (s *Services) moveDoc(c echo.Context) error {
	doc := c.(DocContext).Doc
	user := c.(DocContext).User

	var groupChanges docChanges
	changes := make(map[string]docMove)

	var req DocMoveParams
	if err := c.Bind(&req); err != nil {
		return EError(c, err)
	}
	var allDocs []dao.Doc
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		var currentGroup, newGroup []dao.Doc
		var newParent dao.Doc
		if err := buildGroupQuery(tx, doc.WorkspaceId, doc.ParentDocID).
			Preload("ParentDoc").
			Preload("Workspace").
			Find(&currentGroup).Error; err != nil {
			return err
		}

		sortOnly := (!doc.ParentDocID.Valid && req.ParentId == nil) ||
			(doc.ParentDocID.Valid && req.ParentId != nil && doc.ParentDocID.UUID.String() == *req.ParentId)

		if sortOnly {
			if err := groupChanges.reorderDocs(&currentGroup, ActionSort, &doc, req.PreviousId, req.NextId, changes); err != nil {
				return err
			}

		} else {
			if d := currentGroup[0].ParentDoc; d != nil {
				groupChanges.FromDoc = d
			}

			if req.ParentId != nil {
				if err := tx.
					Where("workspace_id = ?", doc.WorkspaceId).
					Where("id = ?", req.ParentId).
					Set("breadcrumbs", true).
					First(&newParent).Error; err != nil {
					return err
				}
				if utils.CheckInSlice(newParent.Breadcrumbs, doc.ID.String()) {
					return apierrors.ErrDocMoveIntoOwnChild
				}
			}

			if err := buildGroupQuery(tx, doc.WorkspaceId, parseNullableUUID(req.ParentId)).
				Preload("ParentDoc").
				Preload("Workspace").
				Find(&newGroup).Error; err != nil {
				return err
			}
			var parent *dao.Doc
			if len(newGroup) == 0 {
				if req.ParentId != nil {
					if err := tx.
						Where("workspace_id = ?", doc.WorkspaceId).
						Where("id = ?", req.ParentId).
						First(&parent).Error; err != nil {
						return err
					}
				}
			} else {
				parent = newGroup[0].ParentDoc
			}

			groupChanges.ToDoc = parent

			doc.ParentDocID = parseNullableUUID(req.ParentId)

			if err := groupChanges.reorderDocs(&currentGroup, ActionDelete, &doc, nil, nil, changes); err != nil {
				return err
			}

			if err := groupChanges.reorderDocs(&newGroup, ActionAdd, &doc, req.PreviousId, req.NextId, changes); err != nil {
				return err
			}

		}

		allDocs = mergeDocGroups(sortOnly, currentGroup, newGroup)
		if len(allDocs) == 0 {
			return nil
		}

		return tx.Omit(clause.Associations).Save(&allDocs).Error
	}); err != nil {
		return EError(c, err)
	}

	for _, docTmp := range allDocs {
		if v, ok := changes[docTmp.ID.String()]; ok {
			newDocMap := make(map[string]interface{})
			oldDocMap := make(map[string]interface{})
			newDocMap["doc_sort"] = v.NewSecId
			oldDocMap["doc_sort"] = v.OldSecId

			if v.ActionDoc {
				if req.ParentId == nil {
					docTmp.ParentDoc = nil
				}

				switch v.Type {
				case ActionAdd, ActionDelete:
					if err := createDocActivity(s.tracker, tracker.ENTITY_MOVE_ACTIVITY, newDocMap, oldDocMap, docTmp, user, &groupChanges); err != nil {
						errStack.GetError(c, err)
					}
					if err := createDocActivity(s.tracker, tracker.ENTITY_ADD_ACTIVITY, newDocMap, oldDocMap, docTmp, user, &groupChanges); err != nil {
						errStack.GetError(c, err)
					}
					if err := createDocActivity(s.tracker, tracker.ENTITY_REMOVE_ACTIVITY, newDocMap, oldDocMap, docTmp, user, &groupChanges); err != nil {
						errStack.GetError(c, err)
					}

				case ActionSort:
					newDocMap["doc_sort"] = "re-sorting"
					oldDocMap["doc_sort"] = ""
					if docTmp.ParentDoc != nil {
						if err := createDocActivity(s.tracker, tracker.ENTITY_UPDATED_ACTIVITY, newDocMap, oldDocMap, *docTmp.ParentDoc, user, nil); err != nil {
							errStack.GetError(c, err)
						}
					} else {
						if err := tracker.TrackActivity[dao.Workspace, dao.WorkspaceActivity](s.tracker, tracker.ENTITY_UPDATED_ACTIVITY, newDocMap, oldDocMap, c.(DocContext).Workspace, user); err != nil {
							errStack.GetError(c, err)
						}
					}

				}
			}
		}
	}
	return c.NoContent(http.StatusOK)
}

func buildGroupQuery(db *gorm.DB, workspaceID string, parent uuid.NullUUID) *gorm.DB {
	query := db.Where("workspace_id = ?", workspaceID)
	if parent.Valid {
		query = query.Where("parent_doc_id = ?", parent.UUID)
	} else {
		query = query.Where("parent_doc_id IS NULL")
	}
	return query.Order("seq_id ASC")
}

func parseNullableUUID(id *string) uuid.NullUUID {
	if id == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: uuid.FromStringOrNil(*id), Valid: true}
}

func mergeDocGroups(sortOnly bool, current, new []dao.Doc) []dao.Doc {
	if sortOnly {
		return current
	}
	return append(current, new...)
}

func (dc *docChanges) reorderDocs(docs *[]dao.Doc, action DocMoveAction, currentDoc *dao.Doc, prevId, nextId *string, changes map[string]docMove) error {
	indexMap := make(map[string]int)
	currentIdx := -1

	for i, d := range *docs {
		indexMap[d.ID.String()] = i
		if d.ID == currentDoc.ID {
			currentIdx = i
		}
	}

	prevIdx, prevExists := getDocIndex(indexMap, prevId)
	nextIdx, nextExists := getDocIndex(indexMap, nextId)

	if prevExists && nextExists && prevIdx >= nextIdx {
		return apierrors.ErrDocOrderBadRequest
	}

	switch action {
	case ActionDelete:
		if currentIdx != -1 {
			*docs = append((*docs)[:currentIdx], (*docs)[currentIdx+1:]...)
		}
	case ActionAdd, ActionSort:

		if currentIdx != -1 {
			*docs = append((*docs)[:currentIdx], (*docs)[currentIdx+1:]...)
			delete(indexMap, currentDoc.ID.String())

			for i, d := range *docs {
				indexMap[d.ID.String()] = i
				if d.ID == currentDoc.ID {
					currentIdx = i
				}
			}

			prevIdx, prevExists = getDocIndex(indexMap, prevId)
			nextIdx, nextExists = getDocIndex(indexMap, nextId)
		}

		switch {
		case !prevExists && nextExists:
			docInsertAt(docs, 0, *currentDoc)
		case prevExists && !nextExists:
			*docs = append(*docs, *currentDoc)
		case prevExists && nextExists:
			docInsertAt(docs, prevIdx+1, *currentDoc)
		case prevId == nil && nextId == nil:
			docInsertAt(docs, 0, *currentDoc)
		}

	}

	for i, v := range *docs {
		actDoc := v.ID.String() == currentDoc.ID.String()
		if !actDoc && v.SeqId == i {
			continue
		}

		tmp := docMove{
			OldSecId:  v.SeqId,
			NewSecId:  i,
			Type:      action,
			ActionDoc: actDoc,
		}

		changes[v.ID.String()] = tmp
		(*docs)[i].SeqId = i
	}

	return nil
}

func getDocIndex(m map[string]int, id *string) (int, bool) {
	if id == nil {
		return -1, false
	}
	i, ok := m[*id]
	return i, ok
}

func docInsertAt(docs *[]dao.Doc, index int, doc dao.Doc) {
	*docs = append((*docs)[:index], append([]dao.Doc{doc}, (*docs)[index:]...)...)
}

// getChildDocList godoc
// @id getChildDocList
// @Summary doc: получение все дочерние документы
// @Description Возвращает все дочерние документы
// @Tags Docs
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param docId path string true "Id документа"
// @Success 200 {array} dto.DocLight "информация о документах"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Ошибка: не найдено"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/doc/{docId}/child/  [get]
func (s *Services) getChildDocList(c echo.Context) error {
	currentDoc := c.(DocContext).Doc
	workspace := c.(DocContext).Workspace
	workspaceMember := c.(DocContext).WorkspaceMember

	var docs []dao.Doc
	if err := s.db.
		Set("member_id", workspaceMember.MemberId).
		Set("member_role", workspaceMember.Role).
		Preload("AccessRules.Member").
		Preload("Author").
		Where("docs.workspace_id = ?", workspace.ID).
		Where("docs.reader_role <= ? OR docs.editor_role <= ? OR EXISTS (SELECT 1 FROM doc_access_rules dar WHERE dar.doc_id = docs.id AND dar.member_id = ?) OR docs.created_by_id = ?",
			workspaceMember.Role, workspaceMember.Role, workspaceMember.MemberId, workspaceMember.MemberId).
		Where("docs.parent_doc_id = ?", currentDoc.ID).
		Order("seq_id ASC").
		Group("docs.id").
		Find(&docs).
		Error; err != nil {
		return EError(c, apierrors.ErrGeneric)
	}

	return c.JSON(http.StatusOK, utils.SliceToSlice(&docs, func(d *dao.Doc) dto.DocLight { return *d.ToLightDTO() }))
}

// getDocCommentList godoc
// @id getDocCommentList
// @Summary doc: комментарии документа
// @Description комментарии документа
// @Tags Docs
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param docId path string true "Id документа"
// @Param offset query int false "Смещение для пагинации" default(0)
// @Param limit query int false "Лимит записей" default(100)
// @Success 200 {object} dao.PaginationResponse{result=[]dto.DocComment} "комментарии"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Ошибка: не найдено"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/doc/{docId}/comments/  [get]
func (s *Services) getDocCommentList(c echo.Context) error {
	currentDoc := c.(DocContext).Doc
	workspace := c.(DocContext).Workspace

	offset := 0
	limit := 100

	if err := echo.QueryParamsBinder(c).
		Int("offset", &offset).
		Int("limit", &limit).
		BindError(); err != nil {
		return EError(c, err)
	}

	query := s.db.
		Joins("Actor").
		Joins("OriginalComment").
		Joins("OriginalComment.Actor").
		Preload("Reactions").
		Where("doc_comments.workspace_id = ?", workspace.ID).
		Where("doc_comments.doc_id = ?", currentDoc.ID).
		Order("doc_comments.created_at DESC")

	var docComments []dao.DocComment
	result, err := dao.PaginationRequest(
		offset,
		limit,
		query,
		&docComments,
	)
	if err != nil {
		return EError(c, err)
	}

	comments := make([]dto.DocComment, len(docComments))
	for i := range docComments {
		comments[i] = *docComments[i].ToDTO()
	}
	result.Result = comments

	return c.JSON(http.StatusOK, result)
}

// createDocComment godoc
// @id createDocComment
// @Summary doc: создание комментария
// @Description создание комментария
// @Tags Docs
// @Security ApiKeyAuth
// @Accept multipart/form-data
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param docId path string true "Id документа"
// @Param comment formData string true "комментарий в формате JSON" example({"comment_html": "<p>HTML-контент</p>", "reply_to_comment_id": null})
// @Param files formData file false "Вложения для документа"
// @Success 200 {object} dto.DocComment "комментарий"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 413 {object} apierrors.DefinedError "Большой объем файла"
// @Failure 404 {object} apierrors.DefinedError "Ошибка: не найдено"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/doc/{docId}/comments/  [post]
func (s *Services) createDocComment(c echo.Context) error {
	workspace := c.(DocContext).Workspace
	user := c.(DocContext).User

	var lastCommentTime time.Time
	if err := s.db.Select("created_at").
		Where("workspace_id = ?", workspace.ID).
		Where("actor_id = ?", user.ID).
		Order("created_at desc").
		Model(&dao.DocComment{}).
		First(&lastCommentTime).Error; err != nil && err != gorm.ErrRecordNotFound {
		return EError(c, err)
	}

	if time.Since(lastCommentTime) <= commentsCooldown {
		return EErrorDefined(c, apierrors.ErrTooManyComments)
	}
	comment, _, err := BindDocComment(c, nil)
	if err != nil {
		return EError(c, err)
	}
	form, _ := c.MultipartForm()

	if comment.CommentHtml.StripTags() == "" {
		return EErrorDefined(c, apierrors.ErrDocCommentEmpty)
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if comment.ReplyToCommentId.Valid {
			if err := tx.Where("id = ?", comment.ReplyToCommentId).First(&comment.OriginalComment).Error; err != nil {
				if err == gorm.ErrRecordNotFound {
					return apierrors.ErrDocCommentNotFound
				}
				return err
			}
		}

		if err := tx.Omit(clause.Associations).Create(&comment).Error; err != nil {
			return err
		}

		fileAsset := dao.FileAsset{
			Id:           dao.GenUUID(),
			CreatedById:  &user.ID,
			WorkspaceId:  &workspace.ID,
			DocCommentId: uuid.NullUUID{UUID: comment.Id, Valid: true},
		}

		attachments, err := s.uploadDocAttachments(tx, form, "files", fileAsset)
		if err != nil {
			return err
		}

		comment.Attachments = attachments

		return nil
	}); err != nil {
		if err.Error() == "forbidden" {
			return EErrorDefined(c, apierrors.ErrDocUpdateForbidden)
		}
		return EError(c, err)
	}

	err = tracker.TrackActivity[dao.DocComment, dao.DocActivity](s.tracker, tracker.ENTITY_CREATE_ACTIVITY, nil, nil, *comment, user)
	if err != nil {
		errStack.GetError(c, err)
	}

	return c.JSON(http.StatusOK, comment.ToDTO())
}

// getDocComment godoc
// @id getDocComment
// @Summary doc: получение комментария
// @Description Получает данные комментария
// @Tags Docs
// @Security ApiKeyAuth
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param docId path string true "Id документа"
// @Param commentId path string true "ID комментария"
// @Success 200 {object} dto.DocComment "Комментарий"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/doc/{docId}/comments/{commentId}/ [get]
func (s *Services) getDocComment(c echo.Context) error {
	workspace := c.(DocContext).Workspace
	docId := c.(DocContext).Doc.ID
	commentId := c.Param("commentId")

	if _, err := uuid.FromString(commentId); err != nil {
		return EErrorDefined(c, apierrors.ErrDocBadRequest)
	}

	query := s.db.
		Joins("Actor").
		Joins("OriginalComment").
		Joins("OriginalComment.Actor").
		Preload("Reactions").
		Where("doc_comments.workspace_id = ?", workspace.ID).
		Where("doc_comments.doc_id = ?", docId.String()).
		Where("doc_comments.id = ?", commentId)

	var comment dao.DocComment
	if err := query.First(&comment).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return EErrorDefined(c, apierrors.ErrDocCommentNotFound)
		}
		return EError(c, err)
	}

	return c.JSON(http.StatusOK, comment.ToDTO())
}

// updateDocComment godoc
// @id updateDocComment
// @Summary doc: обновление комментария
// @Description обновление комментария
// @Tags Docs
// @Security ApiKeyAuth
// @Accept multipart/form-data
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param docId path string true "Id документа"
// @Param commentId path string true "Id комментария"
// @Param comment formData string true "комментарий в формате JSON" example({"comment_html": "<p>HTML-контент</p>", "reply_to_comment_id": null})
// @Param files formData file false "Вложения для документа"
// @Success 200 {object} dto.DocComment "комментарий"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Ошибка: не найдено"
// @Failure 413 {object} apierrors.DefinedError "Большой объем файла"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/doc/{docId}/comments/{commentId}/ [patch]
func (s *Services) updateDocComment(c echo.Context) error {
	user := *c.(DocContext).User
	workspace := c.(DocContext).Workspace
	commentId := c.Param("commentId")

	var commentOld dao.DocComment
	if err := s.db.
		Where("id = ?", commentId).Preload(clause.Associations).Find(&commentOld).Error; err != nil {
		return EError(c, err)
	}

	oldMap := StructToJSONMap(commentOld)

	if *commentOld.ActorId != user.ID {
		return EErrorDefined(c, apierrors.ErrCommentEditForbidden)
	}

	comment, fields, err := BindDocComment(c, &commentOld)
	if err != nil {
		return EError(c, err)
	}

	if comment.CommentHtml.StripTags() == "" {
		return EErrorDefined(c, apierrors.ErrDocCommentEmpty)
	}

	form, _ := c.MultipartForm()
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		fileAsset := dao.FileAsset{
			Id:           dao.GenUUID(),
			CreatedById:  &user.ID,
			WorkspaceId:  &workspace.ID,
			DocCommentId: uuid.NullUUID{UUID: comment.Id, Valid: true},
		}

		attachments, err := s.uploadDocAttachments(tx, form, "files", fileAsset)
		if err != nil {
			return err
		}

		comment.Attachments = attachments

		if comment.ReplyToCommentId.Valid {
			if err := tx.Where("id = ?", comment.ReplyToCommentId).First(&comment.OriginalComment).Error; err != nil {
				if err == gorm.ErrRecordNotFound {
					return apierrors.ErrDocCommentNotFound
				}
				return err
			}
		}

		if err := s.db.Omit(clause.Associations).Select(fields).Updates(&comment).Error; err != nil {
			return err
		}

		return nil
	}); err != nil {
		if err.Error() == "forbidden" {
			return EErrorDefined(c, apierrors.ErrDocUpdateForbidden)
		}
		return EError(c, err)
	}
	newMap := StructToJSONMap(comment)
	newMap["updateScopeId"] = commentId
	newMap["field_log"] = "comment"

	oldMap["updateScope"] = "comment"
	oldMap["updateScopeId"] = commentId
	err = tracker.TrackActivity[dao.DocComment, dao.DocActivity](s.tracker, tracker.ENTITY_UPDATED_ACTIVITY, newMap, oldMap, *comment, &user)
	if err != nil {
		errStack.GetError(c, err)
	}

	return c.JSON(http.StatusOK, comment.ToDTO())
}

// deleteDocComment godoc
// @id deleteDocComment
// @Summary doc: удаление комментария
// @Description удаление комментария
// @Tags Docs
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param docId path string true "Id документа"
// @Param commentId path string true "Id комментария"
// @Success 200 "комментарий удален"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Ошибка: не найдено"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/doc/{docId}/comments/{commentId}/ [delete]
func (s *Services) deleteDocComment(c echo.Context) error {
	user := *c.(DocContext).User
	workspace := c.(DocContext).Workspace
	workspaceMember := c.(DocContext).WorkspaceMember
	doc := c.(DocContext).Doc
	commentId := c.Param("commentId")

	var comment dao.DocComment
	if err := s.db.Where("workspace_id = ?", workspace.ID).
		Where("doc_id = ?", doc.ID.String()).
		Where("id = ?", commentId).
		Preload("Attachments").
		First(&comment).Error; err != nil {
		return EError(c, err)
	}

	if workspaceMember.Role != types.AdminRole && *comment.ActorId != user.ID {
		return EErrorDefined(c, apierrors.ErrCommentEditForbidden)
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		err := tracker.TrackActivity[dao.DocComment, dao.DocActivity](s.tracker, tracker.ENTITY_DELETE_ACTIVITY, nil, nil, comment, &user)
		if err != nil {
			errStack.GetError(c, err)
			return err
		}

		return s.db.Delete(&comment).Error
	}); err != nil {
		return EError(c, err)
	}

	return c.NoContent(http.StatusOK)
}

// addDocCommentReaction godoc
// @id addDocCommentReaction
// @Summary doc: добавление реакции
// @Description добавление реакции
// @Tags Docs
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param docId path string true "Id документа"
// @Param commentId path string true "Id комментария"
// @Param data body ReactionRequest true "Реакция (пример: 👍, 👎, ❤️)"
// @Success 200 {object} dto.CommentReaction "реакция добавлена ранее"
// @Success 201 {object} dto.CommentReaction "реакция добавлена"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Ошибка: не найдено"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/doc/{docId}/comments/{commentId}/reactions/ [post]
func (s *Services) addDocCommentReaction(c echo.Context) error {
	user := *c.(DocContext).User
	doc := c.(DocContext).Doc
	commentId := c.Param("commentId")

	var reactionRequest ReactionRequest

	if err := c.Bind(&reactionRequest); err != nil {
		return EError(c, err)
	}

	if !validReactions[reactionRequest.Reaction] {
		return EErrorDefined(c, apierrors.ErrInvalidReaction)
	}

	var comment dao.DocComment
	if err := s.db.Where("id = ?", commentId).Where("doc_id = ?", doc.ID).First(&comment).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return EErrorDefined(c, apierrors.ErrDocCommentNotFound)
		}
		return EError(c, err)
	}

	// Проверяем, есть ли уже такая реакция от пользователя
	var existingReaction dao.DocCommentReaction
	err := s.db.Where("user_id = ? AND comment_id = ? AND reaction = ?", user.ID, commentId, reactionRequest.Reaction).First(&existingReaction).Error
	if err == nil {
		return c.JSON(http.StatusOK, existingReaction.ToDTO())
	} else if err != gorm.ErrRecordNotFound {
		return EError(c, err)
	}

	// Создаем новую реакцию
	reaction := dao.DocCommentReaction{
		Id:        dao.GenUUID(),
		CreatedAt: time.Now(),
		UserId:    user.ID,
		CommentId: comment.Id,
		Reaction:  reactionRequest.Reaction,
	}

	if err := s.db.Create(&reaction).Error; err != nil {
		return EError(c, err)
	}

	return c.JSON(http.StatusCreated, reaction.ToDTO())
}

// removeDocCommentReaction godoc
// @id removeDocCommentReaction
// @Summary doc: удаление реакции
// @Description удаление реакции
// @Tags Docs
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param docId path string true "Id документа"
// @Param commentId path string true "Id комментария"
// @Param reaction path string true "реакция"
// @Success 204  "удален"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Ошибка: не найдено"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/doc/{docId}/comments/{commentId}/reactions/{reaction}/ [delete]
func (s *Services) removeDocCommentReaction(c echo.Context) error {
	user := *c.(DocContext).User
	commentId := c.Param("commentId")
	reactionStr := c.Param("reaction")

	if err := s.db.Where("user_id = ? AND comment_id = ? AND reaction = ?",
		user.ID, commentId, reactionStr).Delete(&dao.DocCommentReaction{}).Error; err != nil {
		return EError(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}

// ############# Doc attachments methods ###################

// getDocAttachmentList godoc
// @id getDocAttachmentList
// @Summary Doc (вложения): получение вложений документа
// @Description Возвращает список всех вложений, прикрепленных к документу
// @Tags Docs
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param docId path string true "Id документа"
// @Success 200 {array} dto.Attachment "Список вложений"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/doc/{docId}/doc-attachments/ [get]
func (s *Services) getDocAttachmentList(c echo.Context) error {
	workspace := c.(DocContext).Workspace
	docId := c.(DocContext).Doc.ID

	var attachments []dao.DocAttachment
	if err := s.db.
		Joins("Asset").
		Where("doc_attachments.workspace_id = ?", workspace.ID).
		Where("doc_attachments.doc_id = ?", docId).
		Order("doc_attachments.created_at").
		Find(&attachments).Error; err != nil {
		return EError(c, err)
	}

	return c.JSON(
		http.StatusOK,
		utils.SliceToSlice(&attachments, func(da *dao.DocAttachment) dto.Attachment { return *da.ToLightDTO() }),
	)
}

// createDocAttachments godoc
// @id createDocAttachments
// @Summary Doc (вложения): загрузка вложения в документ
// @Description Загружает новое вложение и прикрепляет его к документу
// @Tags Docs
// @Security ApiKeyAuth
// @Accept multipart/form-data
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param docId path string true "Id документа"
// @Param asset formData file true "Файл для загрузки"
// @Success 201 {object} dto.Attachment "Созданное вложение"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/doc/{docId}/doc-attachments/ [post]
func (s *Services) createDocAttachments(c echo.Context) error {
	user := *c.(DocContext).User
	doc := c.(DocContext).Doc
	workspace := c.(DocContext).Workspace

	if !limiter.Limiter.CanAddAttachment(uuid.Must(uuid.FromString(workspace.ID))) {
		return EErrorDefined(c, apierrors.ErrAssetsLimitExceed)
	}

	asset, err := c.FormFile("asset")
	if err != nil {
		return EError(c, err)
	}

	assetSrc, err := asset.Open()
	if err != nil {
		return EError(c, err)
	}

	fileName := asset.Filename

	if decodedFilename, err := url.QueryUnescape(asset.Filename); err == nil {
		fileName = decodedFilename
	}

	assetId := dao.GenUUID()

	if err := s.storage.SaveReader(
		assetSrc,
		asset.Size,
		assetId,
		asset.Header.Get("Content-Type"),
		&filestorage.Metadata{
			WorkspaceId: workspace.ID,
			DocId:       doc.ID.String(),
		},
	); err != nil {
		return EError(c, err)
	}

	docAttachment := dao.DocAttachment{
		Id:          dao.GenID(),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		CreatedById: &user.ID,
		UpdatedById: &user.ID,
		AssetId:     assetId,
		DocId:       doc.ID.String(),
		WorkspaceId: workspace.ID,
	}

	fa := dao.FileAsset{
		Id:          assetId,
		CreatedById: &user.ID,
		WorkspaceId: &workspace.ID,
		Name:        fileName,
		ContentType: asset.Header.Get("Content-Type"),
		FileSize:    int(asset.Size),
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := s.db.Create(&fa).Error; err != nil {
			return err
		}

		if err := s.db.Create(&docAttachment).Error; err != nil {
			return err
		}
		docAttachment.Asset = &fa
		return nil
	}); err != nil {
		return EError(c, err)
	}

	err = tracker.TrackActivity[dao.DocAttachment, dao.DocActivity](s.tracker, tracker.ENTITY_CREATE_ACTIVITY, nil, nil, docAttachment, &user)
	if err != nil {
		errStack.GetError(c, err)
	}

	return c.JSON(http.StatusCreated, docAttachment.ToLightDTO())
}

// deleteDocAttachment godoc
// @id deleteDocAttachment
// @Summary Doc (вложения): удаление вложения из документа
// @Description Удаляет указанное вложение, прикрепленное к документу
// @Tags Docs
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param docId path string true "Id документа"
// @Param attachmentId path string true "ID вложения"
// @Success 200 "Вложение успешно удалено"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/doc/{docId}/doc-attachments/{attachmentId} [delete]
func (s *Services) deleteDocAttachment(c echo.Context) error {
	workspace := c.(DocContext).Workspace
	docId := c.(DocContext).Doc.ID.String()
	user := c.(DocContext).User
	attachmentId := c.Param("attachmentId")

	var attachment dao.DocAttachment
	if err := s.db.
		Preload("Asset").
		Where("workspace_id = ?", workspace.ID).
		Where("doc_id = ?", docId).
		Where("doc_attachments.id = ?", attachmentId).
		Find(&attachment).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return EErrorDefined(c, apierrors.ErrDocAttachmentNotFound)
		}
		return EError(c, err)
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		err := tracker.TrackActivity[dao.DocAttachment, dao.DocActivity](s.tracker, tracker.ENTITY_DELETE_ACTIVITY, nil, nil, attachment, user)
		if err != nil {
			errStack.GetError(c, err)
			return err
		}

		return s.db.Omit(clause.Associations).
			Delete(&attachment).Error
	}); err != nil {
		return EError(c, err)
	}

	return c.NoContent(http.StatusOK)
}

// ############# User favorite Doc methods ###################

// addDocToFavorites godoc
// @id addDocToFavorites
// @Summary Doc (Favorites): добавление документа в избранное
// @Description Добавляет указанный документ в список избранных текущего пользователя в рабочем пространстве.
// @Tags Docs
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param project body AddDocToFavoritesRequest true "ID документа для добавления в избранное"
// @Success 201 {object} dto.DocFavorites "Добавленный документ в избранных"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Документ не найден"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/user-favorite-docs/ [post]
func (s *Services) addDocToFavorites(c echo.Context) error {
	workspace := c.(WorkspaceContext).Workspace
	workspaceMember := c.(WorkspaceContext).WorkspaceMember

	var req AddDocToFavoritesRequest
	if err := c.Bind(&req); err != nil {
		return EError(c, err)
	}
	doc, err := dao.GetDoc(s.db, workspace.ID, req.DocID, workspaceMember)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return EErrorDefined(c, apierrors.ErrDocNotFound)
		}
		return EError(c, err)
	}

	docFavorite := dao.DocFavorites{
		Id:          dao.GenID(),
		CreatedById: &workspaceMember.MemberId,
		DocId:       doc.ID.String(),
		UserId:      workspaceMember.MemberId,
		WorkspaceId: workspace.ID,
		Workspace:   &workspace,
		Doc:         &doc,
	}

	if err := s.db.Create(&docFavorite).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return c.NoContent(http.StatusOK)
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.NoContent(http.StatusForbidden)
		}
		return EError(c, err)
	}
	docFavorite.Doc.IsFavorite = true

	return c.JSON(http.StatusCreated, docFavorite.ToDTO())
}

// getFavoriteDocList godoc
// @id getFavoriteDocList
// @Summary Doc (Favorites): получение списка избранных документов
// @Description Возвращает список избранных документов текущего пользователя в рабочем пространстве.
// @Tags Docs
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Success 200 {array} dto.DocFavorites "Список избранных документов"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Пространство не найдено"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/user-favorite-docs/ [get]
func (s *Services) getFavoriteDocList(c echo.Context) error {
	user := *c.(WorkspaceContext).User
	workspace := c.(WorkspaceContext).Workspace
	workspaceMember := c.(WorkspaceContext).WorkspaceMember

	var favorites []dao.DocFavorites
	if err := s.db.
		Set("member_id", workspaceMember.MemberId).
		Set("member_role", workspaceMember.Role).
		Preload("Doc").
		Joins("LEFT JOIN docs ON docs.id = doc_favorites.doc_id").
		Where("doc_favorites.user_id = ?", user.ID).
		Where("doc_favorites.workspace_id = ?", workspace.ID).
		Where("docs.reader_role <= ? OR docs.editor_role <= ? OR EXISTS (SELECT 1 FROM doc_access_rules dar WHERE dar.doc_id = doc_favorites.doc_id AND dar.member_id = ?) OR docs.created_by_id = ?",
			workspaceMember.Role, workspaceMember.Role, workspaceMember.MemberId, workspaceMember.MemberId).
		Order("lower(docs.title)").
		Group("doc_favorites.id, docs.title").
		Find(&favorites).Error; err != nil {
		return EError(c, err)
	}

	return c.JSON(http.StatusOK, utils.SliceToSlice(&favorites, func(df *dao.DocFavorites) dto.DocFavorites { return *df.ToDTO() }))
}

// removeDocFromFavorites godoc
// @id removeDocFromFavorites
// @Summary Doc (Favorites): удаление документа из избранных
// @Description Удаляет указанный документ из списка избранных текущего пользователя в рабочем пространстве.
// @Tags Docs
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param docId path string true "Id документа"
// @Success 200 "Успешно удалено"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Документ не найден"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/user-favorite-docs/{docId} [delete]
func (s *Services) removeDocFromFavorites(c echo.Context) error {
	user := *c.(WorkspaceContext).User
	workspace := c.(WorkspaceContext).Workspace
	docId := c.Param("docId")

	if err := s.db.Where("user_id = ?", user.ID).
		Where("workspace_id = ?", workspace.ID).
		Where("doc_id = ?", docId).
		Delete(&dao.DocFavorites{}).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.NoContent(http.StatusNotFound)
		}
		return EError(c, err)
	}

	return c.NoContent(http.StatusOK)
}

// getDocActivityList godoc
// @id getDocActivityList
// @Summary Doc: получение активности по документу
// @Description Возвращает активность по документу
// @Tags Docs
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param docId path string true "Id документа"
// @Param offset query int false "Смещение для пагинации" default(0)
// @Param limit query int false "Лимит записей" default(100)
// @Success 200 {object} dao.PaginationResponse{result=[]dto.EntityActivityFull} "Список активностей с пагинацией"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/doc/{docId}/activities/ [get]
func (s *Services) getDocActivityList(c echo.Context) error {
	docId := c.(DocContext).Doc.ID.String()
	workspaceId := c.(DocContext).Workspace.ID

	offset := 0
	limit := 100

	if err := echo.QueryParamsBinder(c).
		Int("offset", &offset).
		Int("limit", &limit).
		BindError(); err != nil {
		return EError(c, err)
	}

	var doc dao.DocActivity
	doc.UnionCustomFields = "'doc' AS entity_type"
	unionTable := dao.BuildUnionSubquery(s.db, "fa", dao.FullActivity{}, doc)

	query := unionTable.Joins("Doc").
		Preload(clause.Associations).
		Where("fa.doc_id = ?", docId).
		Where("fa.workspace_id = ?", workspaceId).
		Order("fa.created_at DESC")

	var activities []dao.FullActivity
	resp, err := dao.PaginationRequest(
		offset,
		limit,
		query,
		&activities,
	)
	if err != nil {
		return EErrorDefined(c, apierrors.ErrGeneric)
	}

	resp.Result = utils.SliceToSlice(resp.Result.(*[]dao.FullActivity), func(da *dao.FullActivity) dto.EntityActivityFull { return *da.ToDTO() })

	return c.JSON(http.StatusOK, resp)
}

// getDocHistoryList godoc
// @id getDocHistoryList
// @Summary Doc: получение истории изменений по документу
// @Description Возвращает истории изменений  по документу
// @Tags Docs
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param docId path string true "Id документа"
// @Param offset query int false "Смещение для пагинации" default(0)
// @Param limit query int false "Лимит записей" default(100)
// @Success 200 {object} dao.PaginationResponse{result=[]dto.HistoryBodyLight} "Список с пагинацией"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/doc/{docId}/history/ [get]
func (s *Services) getDocHistoryList(c echo.Context) error {
	doc := c.(DocContext).Doc

	offset := -1
	limit := 100

	if err := echo.QueryParamsBinder(c).
		Int("offset", &offset).
		Int("limit", &limit).BindError(); err != nil {
		return EError(c, err)
	}

	var activities []dao.DocActivity

	query := s.db.
		Preload("Actor").
		Where("workspace_id = ?", doc.WorkspaceId).
		Where("doc_id = ?", doc.ID).
		Where("field = ?", "description").
		Order("created_at DESC")

	resp, err := dao.PaginationRequest(
		offset,
		limit,
		query,
		&activities,
	)
	if err != nil {
		return EError(c, err)
	}

	resp.Result = utils.SliceToSlice(resp.Result.(*[]dao.DocActivity), func(da *dao.DocActivity) dto.HistoryBodyLight { return *da.ToHistoryLightDTO() })

	return c.JSON(http.StatusOK, resp)
}

// getDocHistory godoc
// @id getDocHistory
// @Summary Doc: получение старой версии по документу
// @Description Возвращает старую версию по документу
// @Tags Docs
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param docId path string true "Id документа"
// @Param versionId path string true "Id версии"
// @Success 200 {object} dto.HistoryBody "версия по id и текущая"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/doc/{docId}/history/{versionId} [get]
func (s *Services) getDocHistory(c echo.Context) error {
	doc := c.(DocContext).Doc
	versionId := c.Param("versionId")

	var activity dao.DocActivity
	if err := s.db.
		Preload("Actor").
		Preload("Doc.InlineAttachments").
		Where("workspace_id = ?", doc.WorkspaceId).
		Where("doc_id = ?", doc.ID).
		Where("field = ?", "description").
		Where("id = ?", versionId).
		First(&activity).Error; err != nil {
		return EErrorDefined(c, apierrors.ErrGeneric)
	}

	query := s.db.Where("workspace_id = ?", doc.WorkspaceId).Where("doc_id = ?", doc.ID)
	oldFiles, err := dao.GetFileAssetFromDescription(query, activity.OldValue)
	if err != nil {
		return EErrorDefined(c, apierrors.ErrGeneric)
	}

	query2 := s.db.Where("workspace_id = ?", doc.WorkspaceId).Where("doc_id = ?", doc.ID)
	currentFiles, err := dao.GetFileAssetFromDescription(query2, &doc.Content.Body)
	if err != nil {
		return EErrorDefined(c, apierrors.ErrGeneric)
	}

	resp := activity.ToHistoryLightDTO().ToFullHistory(
		activity.OldValue,
		&doc.Content.Body,
		utils.SliceToSlice(&oldFiles, func(fa *dao.FileAsset) dto.FileAsset { return *fa.ToDTO() }),
		utils.SliceToSlice(&currentFiles, func(fa *dao.FileAsset) dto.FileAsset { return *fa.ToDTO() }),
	)
	return c.JSON(http.StatusOK, resp)
}

// updateDocFromHistory godoc
// @id updateDocFromHistory
// @Summary Doc: Откат старой версии документа
// @Description Откатывает старую версию документа
// @Tags Docs
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param docId path string true "id документа"
// @Param versionId path string true "id версии"
// @Success 200 "успешно"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/doc/{docId}/history/{versionId} [patch]
func (s *Services) updateDocFromHistory(c echo.Context) error {
	doc := c.(DocContext).Doc
	user := c.(DocContext).User
	versionId := c.Param("versionId")

	oldDocMap := StructToJSONMap(doc)
	var activity dao.DocActivity
	if err := s.db.
		Preload("Actor").
		Preload("Doc.InlineAttachments").
		Where("workspace_id = ?", doc.WorkspaceId).
		Where("doc_id = ?", doc.ID).
		Where("field = ?", "description").
		Where("id = ?", versionId).
		First(&activity).Error; err != nil {
		return EErrorDefined(c, apierrors.ErrGeneric)
	}

	if activity.OldValue == nil {
		return EErrorDefined(c, apierrors.ErrGeneric)
	}

	doc.Content.Body = *activity.OldValue

	if err := s.db.Omit(clause.Associations).Save(&doc).Error; err != nil {
		return EError(c, err)
	}

	newDocMap := StructToJSONMap(doc)

	err := tracker.TrackActivity[dao.Doc, dao.DocActivity](s.tracker, tracker.ENTITY_UPDATED_ACTIVITY, newDocMap, oldDocMap, doc, user)
	if err != nil {
		errStack.GetError(c, err)
	}

	return c.NoContent(http.StatusOK)
}

type DocRequest struct {
	Title       string             `json:"title,omitempty" example:"title text" validate:"required,max=150"`
	Content     types.RedactorHTML `json:"content,omitempty" swaggertype:"string" example:"<p>HTML-контент</p>"`
	EditorRole  int                `json:"editor_role,omitempty" example:"5"`
	ReaderRole  int                `json:"reader_role,omitempty" example:"5"`
	SeqId       int                `json:"seq_id,omitempty"`
	Draft       bool               `json:"draft,omitempty" example:"false"`
	EditorList  []string           `json:"editor_list,omitempty"`
	ReaderList  []string           `json:"reader_list,omitempty"`
	WatcherList []string           `json:"watcher_list,omitempty"`
}

type DocCommentRequest struct {
	CommentHtml    types.RedactorHTML `json:"comment_html" swaggertype:"string" example:"<p>HTML-контент</p>"`
	ReplyToComment *string            `json:"reply_to_comment_id,omitempty"`
}

type DocMoveParams struct {
	ParentId   *string `json:"parent_id,omitempty"`
	PreviousId *string `json:"previous_id,omitempty"`
	NextId     *string `json:"next_id,omitempty"`
}

type AddDocToFavoritesRequest struct {
	DocID string `json:"doc" validate:"required"`
}

func BindDoc(c echo.Context, doc *dao.Doc) (*dao.Doc, []string, error) {
	var req DocRequest
	fields, err := BindData(c, "doc", &req)
	if err != nil {
		return nil, nil, apierrors.ErrDocBadRequest
	}

	if doc != nil && req.Title == "" {
		req.Title = doc.Title
	}
	if err := c.Validate(&req); err != nil {
		return nil, nil, apierrors.ErrDocRequestValidate
	}

	if doc == nil {
		var workspace dao.Workspace
		var user *dao.User
		if workspaceCtx, ok := c.(WorkspaceContext); ok {
			workspace = workspaceCtx.Workspace
			user = workspaceCtx.User
		} else {
			workspace = c.(DocContext).Workspace
			user = c.(DocContext).User
		}

		return &dao.Doc{
			ID:          dao.GenUUID(),
			Author:      user,
			UpdatedById: nil,
			Title:       req.Title,
			Content:     req.Content,
			EditorRole:  req.EditorRole,
			ReaderRole:  req.ReaderRole,
			WorkspaceId: workspace.ID,
			Workspace:   &workspace,
			ParentDocID: uuid.NullUUID{},
			SeqId:       req.SeqId,
			Draft:       req.Draft,
			EditorsIDs:  req.EditorList,
			ReaderIDs:   req.ReaderList,
			WatcherIDs:  req.WatcherList,
		}, fields, nil
	} else {
		docCopy := *doc

		var resFields []string
		for _, field := range fields {
			switch field {
			case "title":
				_ = CompareAndAddFields(&docCopy.Title, &req.Title, field, &resFields)
			case "content":
				if doc.Content.Body != req.Content.Body {
					doc.Content = req.Content
					resFields = append(resFields, field)
				}
			case "reader_role":
				_ = CompareAndAddFields(&docCopy.ReaderRole, &req.ReaderRole, field, &resFields)
			case "editor_role":
				_ = CompareAndAddFields(&docCopy.EditorRole, &req.EditorRole, field, &resFields)
			case "seq_id":
				_ = CompareAndAddFields(&docCopy.SeqId, &req.SeqId, field, &resFields)
			case "draft":
				_ = CompareAndAddFields(&docCopy.Draft, &req.Draft, field, &resFields)
			case "editor_list":
				_ = CompareAndAddFields(&docCopy.EditorsIDs, &req.EditorList, field, &resFields)
			case "reader_list":
				_ = CompareAndAddFields(&docCopy.ReaderIDs, &req.ReaderList, field, &resFields)
			case "watcher_list":
				_ = CompareAndAddFields(&docCopy.WatcherIDs, &req.WatcherList, field, &resFields)
			}
		}
		if len(resFields) > 0 {
			docCopy.UpdatedById = &c.(DocContext).User.ID
			resFields = append(resFields, "updated_by_id")
		}

		return &docCopy, resFields, nil
	}
}

func BindDocComment(c echo.Context, comment *dao.DocComment) (*dao.DocComment, []string, error) {
	var req DocCommentRequest
	fields, err := BindData(c, "comment", &req)
	if err != nil {
		return nil, nil, apierrors.ErrDocCommentBadRequest
	}
	if err := c.Validate(&req); err != nil {
		return nil, nil, err
	}
	var replyId uuid.NullUUID
	if req.ReplyToComment != nil {
		fromString, err := uuid.FromString(*req.ReplyToComment)
		if err != nil {
			return nil, nil, err
		}
		replyId = uuid.NullUUID{UUID: fromString, Valid: true}
	}
	if comment == nil {
		commentCreate := &dao.DocComment{
			Id:               dao.GenUUID(),
			CommentStripped:  "",
			CreatedById:      &c.(DocContext).User.ID,
			WorkspaceId:      c.(DocContext).Workspace.ID,
			DocId:            c.(DocContext).Doc.ID.String(),
			ActorId:          &c.(DocContext).User.ID,
			Actor:            c.(DocContext).User,
			CommentHtml:      req.CommentHtml,
			ReplyToCommentId: replyId,
			CommentType:      1,
			Attachments:      make([]dao.FileAsset, 0),
		}
		commentCreate.CommentStripped = commentCreate.CommentHtml.StripTags()

		return commentCreate, fields, nil
	} else {
		var resFields []string
		for _, field := range fields {
			switch field {
			case "comment_html":
				if comment.CommentHtml.Body != req.CommentHtml.Body {
					comment.CommentHtml = req.CommentHtml
					comment.CommentStripped = comment.CommentHtml.StripTags()
					resFields = append(resFields, "comment_html", "comment_stripped", "updated_by_id")
					comment.UpdatedById = &c.(DocContext).User.ID
				}
			}
		}
		return comment, resFields, nil
	}
}

func (s *Services) uploadDocAttachments(tx *gorm.DB, form *multipart.Form, name string, fa dao.FileAsset) ([]dao.FileAsset, error) {
	res := make([]dao.FileAsset, 0)
	if form == nil {
		return res, nil
	}
	for _, f := range form.File[name] {
		fa.Id = dao.GenUUID()
		fa.CreatedAt = time.Now()
		fa.Name = f.Filename
		fa.FileSize = int(f.Size)

		if err := s.uploadAssetForm(tx, f, &fa,
			filestorage.Metadata{
				WorkspaceId: *fa.WorkspaceId,
			}); err != nil {
			return res, err
		}
		res = append(res, fa)
	}
	return res, nil
}

func createDocActivity(track *tracker.ActivitiesTracker,
	activityType string,
	requestedData map[string]interface{},
	currentInstance map[string]interface{},
	doc dao.Doc,
	actor *dao.User, changes *docChanges) error {

	if requestedData != nil {
		requestedData["parent_key"] = "parent_doc_id"
	}
	if currentInstance != nil {
		currentInstance["parent_key"] = "parent_doc_id"
	}

	var err error

	createToDocActivity := tracker.TrackActivity[dao.Doc, dao.DocActivity]
	createToWorkspaceActivity := tracker.TrackActivity[dao.Doc, dao.WorkspaceActivity]

	changeAct := map[bool]string{true: "doc", false: "workspace"}

	if changes != nil {
		fromDoc := changes.FromDoc != nil
		toDoc := changes.ToDoc != nil

		requestedData["field_move"] = fmt.Sprintf("%s_to_%s", changeAct[fromDoc], changeAct[toDoc])
		if currentInstance != nil {
			if fromDoc {
				currentInstance["entity"] = *changes.FromDoc
				currentInstance["parent_title"] = changes.FromDoc.Title
				currentInstance["parent_doc_id"] = changes.FromDoc.ID.String()
			} else {
				currentInstance["parent_title"] = doc.Workspace.Name
			}
		}

		if requestedData != nil {
			if toDoc {
				requestedData["entity"] = *changes.ToDoc
				requestedData["parent_title"] = changes.ToDoc.Title
				requestedData["parent_doc_id"] = changes.ToDoc.ID.String()
			} else {
				requestedData["parent_title"] = doc.Workspace.Name
			}
		}
	}

	switch activityType {
	case
		tracker.ENTITY_UPDATED_ACTIVITY,
		tracker.ENTITY_MOVE_ACTIVITY:
		err = createToDocActivity(track, activityType, requestedData, currentInstance, doc, actor)
	case
		tracker.ENTITY_ADD_ACTIVITY:
		if changes != nil && changes.ToDoc != nil {
			err = createToDocActivity(track, activityType, requestedData, currentInstance, doc, actor)
		} else {
			err = createToWorkspaceActivity(track, activityType, requestedData, currentInstance, doc, actor)
		}
	case
		tracker.ENTITY_REMOVE_ACTIVITY:
		if changes != nil && changes.FromDoc != nil {
			err = createToDocActivity(track, activityType, requestedData, currentInstance, doc, actor)
		} else {
			err = createToWorkspaceActivity(track, activityType, requestedData, currentInstance, doc, actor)
		}
	case
		tracker.ENTITY_DELETE_ACTIVITY:
		if doc.ParentDoc != nil {
			requestedData["old_title"] = doc.Title
			err = createToDocActivity(track, activityType, requestedData, currentInstance, *doc.ParentDoc, actor)
		} else {
			err = createToWorkspaceActivity(track, activityType, requestedData, currentInstance, doc, actor)
		}
	default:
		err = createToWorkspaceActivity(track, activityType, requestedData, currentInstance, doc, actor)
	}

	if err != nil {
		return err
	}
	return nil
}
