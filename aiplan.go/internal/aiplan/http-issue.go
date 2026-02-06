// Пакет предоставляет функциональность для работы со связанными задачами в системе управления проектами. Он включает в себя операции добавления, удаления и получения списка связанных задач, а также поддержку загрузки и экспорта вложений для этих задач. Также реализована поддержка получения PDF-версии задачи.
//
// Основные возможности:
//   - Добавление связанных задач к задаче.
//   - Удаление связанных задач из задачи.
//   - Получение списка связанных задач.
//   - Загрузка вложений для задачи.
//   - Экспорт задачи в PDF-формат.
package aiplan

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/apierrors"
	errStack "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/stack-error"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/limiter"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/export"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/search"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dto"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications"

	tracker "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/activity-tracker"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	filestorage "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/file-storage"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/rules"
	"github.com/gofrs/uuid"
	"github.com/labstack/echo/v4"
	"github.com/santhosh-tekuri/jsonschema/v6"
	tusd "github.com/tus/tusd/v2/pkg/handler"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	commentsCooldown = time.Second * 5

	descriptionLockTime = time.Minute * 15
)

type IssueContext struct {
	ProjectContext
	Issue dao.Issue
}

// AddIssueServices - добавление сервисов задач
func (s *Services) AddIssueServices(g *echo.Group) {
	issueGroup := g.Group("workspaces/:workspaceSlug/projects/:projectId/issues/:issueIdOrSeq",
		s.WorkspaceMiddleware,
		s.LastVisitedWorkspaceMiddleware,
		s.ProjectMiddleware,
		s.FindIssueByIdOrSeqMiddleware,
		s.IssuePermissionMiddleware,
	)

	g.POST("issues/search/", s.getIssueList)
	g.POST("issues/search/export/", s.exportIssueList)

	issueGroup.GET("/", s.getIssue)
	issueGroup.PATCH("/", s.updateIssue)
	issueGroup.DELETE("/", s.deleteIssue)

	issueGroup.GET("/sub-issues/", s.getSubIssueList)
	issueGroup.POST("/sub-issues/", s.addSubIssueList)
	issueGroup.POST("/sub-issues/:subIssueId/up/", s.moveSubIssueUp)
	issueGroup.POST("/sub-issues/:subIssueId/down/", s.moveSubIssueDown)

	issueGroup.GET("/sub-issues/available/", s.getAvailableSubIssueList)
	issueGroup.GET("/parent-issues/available/", s.getAvailableParentIssueList)
	issueGroup.GET("/blocks-issues/available/", s.getAvailableBlocksIssueList)
	issueGroup.GET("/blockers-issues/available/", s.getAvailableBlockersIssueList)
	issueGroup.GET("/linked-issues/available/", s.getAvailableLinkedIssueList)

	issueGroup.GET("/issue-links/", s.getIssueLinkList)
	issueGroup.POST("/issue-links/", s.createIssueLink)
	issueGroup.PATCH("/issue-links/:linkId/", s.updateIssueLink)
	issueGroup.DELETE("/issue-links/:linkId/", s.deleteIssueLink)

	issueGroup.GET("/history/", s.getIssueHistoryList)

	issueGroup.GET("/comments/", s.getIssueCommentList)
	issueGroup.POST("/comments/", s.createIssueComment)
	issueGroup.GET("/comments/:commentId/", s.getIssueComment)
	issueGroup.PATCH("/comments/:commentId/", s.updateIssueComment)
	issueGroup.DELETE("/comments/:commentId/", s.deleteIssueComment)

	issueGroup.POST("/comments/:commentId/reactions/", s.addCommentReaction)
	issueGroup.DELETE("/comments/:commentId/reactions/:reaction", s.removeCommentReaction)

	issueGroup.GET("/activities/", s.getIssueActivityList)

	issueGroup.GET("/issue-attachments/", s.getIssueAttachmentList)
	issueGroup.POST("/issue-attachments/", s.createIssueAttachments)
	issueGroup.GET("/issue-attachments/all/", s.downloadIssueAttachments)
	issueGroup.DELETE("/issue-attachments/:attachmentId/", s.deleteIssueAttachment)

	issueGroup.GET("/linked-issues/", s.getIssueLinkedIssueList)
	issueGroup.POST("/linked-issues/", s.addIssueLinkedIssueList)

	issueGroup.GET("/pdf/", s.getIssuePdf)

	issueGroup.POST("/description-lock/", s.issueDescriptionLock)
	issueGroup.POST("/description-unlock/", s.issueDescriptionUnlock)

	issueGroup.POST("/pin/", s.issuePin)
	issueGroup.POST("/unpin/", s.issueUnpin)

	g.Any("attachments/tus/*", s.storage.GetTUSHandler(cfg, "/api/auth/attachments/tus/", s.attachmentsUploadValidator, s.attachmentsPostUploadHook))

	// Issue Properties (значения полей задачи)
	issueGroup.GET("/properties/", s.getIssueProperties)
	issueGroup.POST("/properties/:templateId/", s.setIssueProperty)
}

func (s *Services) attachmentsUploadValidator(hook tusd.HookEvent) (tusd.HTTPResponse, tusd.FileInfoChanges, error) {
	entityType, tOk := hook.Upload.MetaData["entity_type"]
	issueId, iOk := hook.Upload.MetaData["issue_id"]
	docId, dOk := hook.Upload.MetaData["doc_id"]
	fileName, fOk := hook.Upload.MetaData["file_name"]

	var filteredMetadata tusd.MetaData

	req := http.Request{Header: hook.HTTPRequest.Header}
	accessCookie, _ := req.Cookie("access_token")
	if accessCookie == nil {
		return tusd.HTTPResponse{}, tusd.FileInfoChanges{}, apierrors.ErrGeneric.TusdError()
	}
	user_id, err := getUserIdFromJWT(accessCookie.Value)
	if err != nil {
		return tusd.HTTPResponse{}, tusd.FileInfoChanges{}, apierrors.ErrGeneric.TusdError()
	}

	if !tOk {
		return tusd.HTTPResponse{}, tusd.FileInfoChanges{}, apierrors.ErrGeneric.TusdError()
	}

	if hook.Upload.Size > 4*1024*1024*1024 {
		return tusd.HTTPResponse{}, tusd.FileInfoChanges{}, apierrors.ErrAttachmentIsTooBig.TusdError()
	}

	switch entityType {
	case "issue":
		if !(iOk && fOk) {
			return tusd.HTTPResponse{}, tusd.FileInfoChanges{}, apierrors.ErrAttachmentsIncorrectMetadata.TusdError()
		}
		priv, err := dao.GetUserPrivilegesOverIssue(issueId, user_id, s.db)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return tusd.HTTPResponse{}, tusd.FileInfoChanges{}, apierrors.ErrIssueNotFound.TusdError()
			}
			return tusd.HTTPResponse{}, tusd.FileInfoChanges{}, apierrors.ErrGeneric.TusdError()
		}

		if priv.ProjectRole == types.GuestRole || (!priv.IsAuthor && !priv.IsAssigner && priv.ProjectRole == types.MemberRole) {
			return tusd.HTTPResponse{}, tusd.FileInfoChanges{}, apierrors.ErrNotEnoughRights.TusdError()
		}

		filteredMetadata = tusd.MetaData{
			"issue_id":  priv.IssueId,
			"user_id":   priv.UserId,
			"file_name": fileName,
			"filetype":  hook.Upload.MetaData["file_type"], // Passed as content-type to minio, https://github.com/mackinleysmith/tusd/blob/d95c0d59ba14a202fbcd8556b5435fef3cb96040/pkg/s3store/s3store.go#L334
		}
	case "doc":
		if !(dOk && fOk) {
			return tusd.HTTPResponse{}, tusd.FileInfoChanges{}, apierrors.ErrAttachmentsIncorrectMetadata.TusdError()
		}
		priv, err := dao.GetUserPrivilegesOverDoc(docId, user_id, s.db)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return tusd.HTTPResponse{}, tusd.FileInfoChanges{}, apierrors.ErrDocNotFound.TusdError()
			}
			return tusd.HTTPResponse{}, tusd.FileInfoChanges{}, apierrors.ErrGeneric.TusdError()
		}

		if !priv.IsAuthor && !priv.IsEditor {
			return tusd.HTTPResponse{}, tusd.FileInfoChanges{}, apierrors.ErrNotEnoughRights.TusdError()
		}

		filteredMetadata = tusd.MetaData{
			"doc_id":    priv.DocId,
			"user_id":   priv.UserId,
			"file_name": fileName,
			"filetype":  hook.Upload.MetaData["file_type"],
		}
	default:
		return tusd.HTTPResponse{}, tusd.FileInfoChanges{}, apierrors.ErrGeneric.TusdError()
	}

	return tusd.HTTPResponse{}, tusd.FileInfoChanges{ID: dao.GenID(), MetaData: filteredMetadata}, nil
}

func (s *Services) attachmentsPostUploadHook(event tusd.HookEvent) {
	assetName, err := uuid.FromString(strings.Split(event.Upload.ID, "+")[0])
	if err != nil {
		slog.Error("Parse uploaded file id", "id", event.Upload.ID, "err", err)
		return
	}
	userId := event.Upload.MetaData["user_id"]
	issueId, iOk := event.Upload.MetaData["issue_id"]
	docId, dOk := event.Upload.MetaData["doc_id"]
	fileName := event.Upload.MetaData["file_name"]

	var user dao.User
	if err := s.db.Where("id = ?", userId).First(&user).Error; err != nil {
		slog.Error("Find new attachment user", "err", err)
		return
	}

	if iOk {
		var issue dao.Issue
		if err := s.db.Select("issues.id", "issues.workspace_id", "issues.project_id").Joins("Project").Where("issues.id = ?", issueId).First(&issue).Error; err != nil {
			slog.Error("Find issue for attachment", "issueId", issueId, "err", err)
			return
		}

		userID := uuid.NullUUID{UUID: user.ID, Valid: true}
		issueAttachment := dao.IssueAttachment{
			Id:          dao.GenUUID(),
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
			CreatedById: userID,
			UpdatedById: userID,
			AssetId:     assetName,
			IssueId:     issue.ID,
			ProjectId:   issue.ProjectId,
			WorkspaceId: issue.WorkspaceId,
		}

		fa := dao.FileAsset{
			Id:          assetName,
			CreatedById: userID,
			WorkspaceId: uuid.NullUUID{UUID: issue.WorkspaceId, Valid: true},
			Name:        fileName,
			FileSize:    int(event.Upload.Size),
		}

		if err := s.db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Create(&fa).Error; err != nil {
				return err
			}

			if err := tx.Create(&issueAttachment).Error; err != nil {
				return err
			}
			return nil
		}); err != nil {
			slog.Error("Save attachment info to db", "err", err)
			return
		}
		//TODO check it
		data := map[string]interface{}{
			"attachment_activity_val": fileName,
		}

		if err := tracker.TrackActivity[dao.IssueAttachment, dao.IssueActivity](s.tracker, actField.EntityCreateActivity, data, nil, issueAttachment, &user); err != nil {
			errStack.GetError(nil, errStack.TrackErrorStack(fmt.Errorf("track new issue attachment activity")))
		}

		//if err := s.tracker.TrackActivity(tracker.ATTACHMENT_CREATED_ACTIVITY, nil, map[string]interface{}{"id": issueAttachment.Id}, issue.ID.String(), tracker.ENTITY_TYPE_ISSUE, issue.Project, user); err != nil {
		//	slog.Error("Track new attachment activity", "err", err)
		//}

	} else if dOk {
		var doc dao.Doc
		if err := s.db.Select("docs.id", "docs.workspace_id").Joins("Workspace").Where("docs.id = ?", docId).First(&doc).Error; err != nil {
			slog.Error("Find doc for attachment", "docId", docId, "err", err)
			return
		}

		userID := uuid.NullUUID{UUID: user.ID, Valid: true}
		docAttachment := dao.DocAttachment{
			Id:          dao.GenUUID(),
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
			CreatedById: userID,
			UpdatedById: userID,
			AssetId:     assetName,
			DocId:       doc.ID,
			WorkspaceId: doc.WorkspaceId,
		}

		fa := dao.FileAsset{
			Id:          assetName,
			CreatedById: userID,
			WorkspaceId: uuid.NullUUID{UUID: doc.WorkspaceId, Valid: true},
			Name:        fileName,
			FileSize:    int(event.Upload.Size),
		}

		if err := s.db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Create(&fa).Error; err != nil {
				return err
			}

			if err := tx.Create(&docAttachment).Error; err != nil {
				return err
			}
			return nil
		}); err != nil {
			slog.Error("Save attachment info to db", "err", err)
			return
		}
		data := map[string]interface{}{
			"attachment_activity_val": fileName,
		}
		if err := tracker.TrackActivity[dao.DocAttachment, dao.DocActivity](s.tracker, actField.EntityCreateActivity, data, nil, docAttachment, &user); err != nil {
			errStack.GetError(nil, errStack.TrackErrorStack(fmt.Errorf("track new doc attachment activity")))
		}
	}
}

// ############# Issue search middleware ###################

func (s *Services) FindIssueByIdOrSeqMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		project := c.(ProjectContext).Project
		issueIdOrSeq := c.Param("issueIdOrSeq")

		if issueIdOrSeq == "" {
			return EErrorDefined(c, apierrors.ErrIssueNotFound)
		}

		query := s.db.
			Joins("Parent").
			//Joins("Workspace").
			Joins("State").
			//Joins("Project").
			Preload("Sprints").
			Preload("Assignees").
			Preload("Watchers").
			Preload("Labels").
			Preload("Links").
			Joins("Author").
			Preload("Links.CreatedBy").
			Preload("Labels.Workspace").
			Preload("Labels.Project").
			Where("issues.project_id = ?", project.ID)

		var issue dao.Issue
		issue.Project = &project
		issue.Workspace = project.Workspace
		issue.FullLoad = true
		if _, err := uuid.FromString(issueIdOrSeq); err == nil {
			// uuid id of issue
			if err := query.
				Where("issues.id = ?", issueIdOrSeq).
				First(&issue).Error; err != nil {
				if err == gorm.ErrRecordNotFound {
					return EErrorDefined(c, apierrors.ErrIssueNotFound)
				}
				return EError(c, err)
			}
		} else if sequenceId, err := strconv.Atoi(issueIdOrSeq); err == nil {
			// sequence id of issue
			if err := query.
				Where("issues.sequence_id = ?", sequenceId).
				First(&issue).Error; err != nil {
				if err == gorm.ErrRecordNotFound {
					return EErrorDefined(c, apierrors.ErrIssueNotFound)
				}
				return EError(c, err)
			}
		} else {
			return EErrorDefined(c, apierrors.ErrIssueNotFound)
		}

		// Fetch Author details
		if err := issue.Author.AfterFind(s.db); err != nil {
			return EError(c, err)
		}

		return next(IssueContext{c.(ProjectContext), issue})
	}
}

// ############# Issue methods ###################

// getIssueList godoc
// @id getIssueList
// @Summary Задачи: поиск задач
// @Description Выполняет поиск задач с использованием фильтров и сортировки
// @Tags Issues
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param hide_sub_issues query bool false "Выключить подзадачи" default(false)
// @Param order_by query string false "Поле для сортировки" default("sequence_id") enum(id, created_at, updated_at, name, priority, target_date, sequence_id, state, labels, sub_issues_count, link_count, attachment_count, linked_issues_count, assignees, watchers, author, search_rank)
// @Param group_by query string false "Поле для группировки результатов" default("") enum(priority, author, state, labels, assignees, watchers, project)
// @Param offset query int false "Смещение для пагинации" default(-1)
// @Param limit query int false "Лимит записей" default(100)
// @Param desc query bool false "Сортировка по убыванию" default(true)
// @Param only_count query bool false "Вернуть только количество" default(false)
// @Param only_active query bool false "Вернуть только активные задачи" default(false)
// @Param only_pinned query bool false "Вернуть только закрепленные задачи" default(false)
// @Param stream query bool false "Ответ ввиде стриминга json сгруппированных таблиц, работает только при группировке" default(false)
// @Param filters body types.IssuesListFilters false "Фильтры для поиска задач"
// @Success 200 {object} dto.IssueSearchResult "Результат поиска задач"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/issues/search [post]
func (s *Services) getIssueList(c echo.Context) error {
	globalSearch := false
	var user dao.User
	var projectMember dao.ProjectMember
	var sprint *dao.Sprint
	if context, ok := c.(ProjectContext); ok {
		projectMember = context.ProjectMember
		user = *context.User
	}
	if context, ok := c.(AuthContext); ok {
		user = *context.User
		globalSearch = true
	}
	if context, ok := c.(SprintContext); ok {
		user = *context.User
		sprint = &context.Sprint
		globalSearch = true
	}

	searchParams, err := types.ParseSearchParams(c)
	if err != nil {
		return EError(c, err)
	}

	// Для streaming режима создаем callback
	var streamCallback search.StreamCallback
	if searchParams.Stream && searchParams.GroupByParam != "" {
		c.Response().Header().Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		c.Response().WriteHeader(http.StatusOK)
		enc := json.NewEncoder(c.Response())

		streamCallback = func(group dto.IssuesGroupResponse) error {
			if err := enc.Encode(group); err != nil {
				return err
			}
			c.Response().Flush()
			return nil
		}
	}

	// Валидация
	if searchParams.Limit > 100 {
		return EErrorDefined(c, apierrors.ErrLimitTooHigh)
	}

	result, err := search.GetIssueListData(s.db, user, projectMember, sprint, globalSearch, searchParams, streamCallback)
	if err != nil {
		if definedErr, ok := err.(apierrors.DefinedError); ok {
			return EErrorDefined(c, definedErr)
		}
		return EError(c, err)
	}

	// Для streaming режима данные уже отправлены
	if streamCallback != nil {
		return nil
	}

	return c.JSON(http.StatusOK, result)
}

// exportIssueList godoc
// @id exportIssueList
// @Summary Задачи: экспорт задач в CSV
// @Description Экспортирует задачи в ZIP архив с CSV файлами. При группировке создаётся отдельный CSV файл для каждой группы.
// @Tags Issues
// @Security ApiKeyAuth
// @Accept json
// @Produce application/zip
// @Param hide_sub_issues query bool false "Выключить подзадачи" default(false)
// @Param order_by query string false "Поле для сортировки" default("sequence_id") enum(id, created_at, updated_at, name, priority, target_date, sequence_id, state, labels, sub_issues_count, link_count, attachment_count, linked_issues_count, assignees, watchers, author, search_rank)
// @Param group_by query string false "Поле для группировки результатов" default("") enum(priority, author, state, labels, assignees, watchers, project)
// @Param offset query int false "Смещение для пагинации" default(-1)
// @Param limit query int false "Лимит записей" default(100)
// @Param desc query bool false "Сортировка по убыванию" default(true)
// @Param only_active query bool false "Вернуть только активные задачи" default(false)
// @Param only_pinned query bool false "Вернуть только закрепленные задачи" default(false)
// @Param filters body types.IssuesListFilters false "Фильтры для поиска задач"
// @Success 200 {file} binary "ZIP архив с CSV файлами"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/issues/search/export/ [post]
func (s *Services) exportIssueList(c echo.Context) error {
	user := c.(AuthContext).User

	searchParams, err := types.ParseSearchParams(c)
	if err != nil {
		return EError(c, err)
	}
	searchParams.LightSearch = false
	searchParams.Offset = 0
	searchParams.Limit = 1_000_000

	result, err := search.GetIssueListData(s.db, *user, dao.ProjectMember{}, nil, true, searchParams, nil)
	if err != nil {
		if definedErr, ok := err.(apierrors.DefinedError); ok {
			return EErrorDefined(c, definedErr)
		}
		return EError(c, err)
	}

	f, err := os.CreateTemp("", "export-*.zip")
	if err != nil {
		return EError(c, err)
	}
	defer os.Remove(f.Name())

	z := zip.NewWriter(f)
	z.RegisterCompressor(zip.Deflate, func(w io.Writer) (io.WriteCloser, error) {
		return flate.NewWriter(w, flate.BestCompression)
	})

	switch res := result.(type) {
	case dto.IssuesGroupedResponse:
		for i, group := range res.Issues {
			fileName := getGroupFileName(group.Entity, i)
			entry, err := z.Create(fileName)
			if err != nil {
				return EError(c, err)
			}
			w := csv.NewWriter(entry)

			if err := w.Write(csvExportHeader()); err != nil {
				return EError(c, err)
			}

			for _, item := range group.Issues {
				issue, ok := item.(*dto.IssueWithCount)
				if !ok {
					continue
				}
				if err := w.Write(issueToCSVRow(issue)); err != nil {
					return EError(c, err)
				}
			}

			w.Flush()
			if err := w.Error(); err != nil {
				return EError(c, err)
			}
		}
	case dto.IssuesSearchResponse:
		entry, err := z.Create("issues.csv")
		if err != nil {
			return EError(c, err)
		}
		w := csv.NewWriter(entry)
		w.Comma = ';'

		if err := w.Write(csvExportHeader()); err != nil {
			return EError(c, err)
		}

		for _, issue := range res.Issues {
			if err := w.Write(issueToCSVRow(&issue)); err != nil {
				return EError(c, err)
			}
		}

		w.Flush()
		if err := w.Error(); err != nil {
			return EError(c, err)
		}
	}

	if err := z.Close(); err != nil {
		return EError(c, err)
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return EError(c, err)
	}

	c.Response().Header().Set("Content-Disposition", "attachment; filename=issues-export.zip")
	return c.Stream(http.StatusOK, "application/zip", f)
}

// getIssue godoc
// @id getIssue
// @Summary Задачи: получение задачи
// @Description Возвращает информацию о задаче в проекте по её идентификатору или номеру последовательности
// @Tags Issues
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param issueIdOrSeq path string true "Идентификатор или номеру последовательности задачи"
// @Success 200 {object} dto.Issue "Детали задачи"
// @Failure 204 "Нет контента (пользователь не имеет доступа)"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/issues/{issueIdOrSeq} [get]
func (s *Services) getIssue(c echo.Context) error {
	issue := c.(IssueContext).Issue
	return c.JSON(http.StatusOK, issue.ToDTO())
}

// updateIssue godoc
// @id updateIssue
// @Summary Задачи: частичное обновление задачи
// @Description Обновляет указанные поля задачи, такие как состояние, исполнители, наблюдатели, метки и вложения.
// @Tags Issues
// @Security ApiKeyAuth
// @Accept multipart/form-data
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param issueIdOrSeq path string true "Идентификатор или последовательный номер задачи"
// @Param issue formData string true "задача в формате JSON"
// @Param files formData file false "Файлы для добавления в задачу"
// @Success 200 {object} dto.Issue "Обновленные данные задачи"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/issues/{issueIdOrSeq} [patch]
func (s *Services) updateIssue(c echo.Context) error {
	user := *c.(IssueContext).User
	issue := c.(IssueContext).Issue
	projectMember := c.(IssueContext).ProjectMember
	project := c.(IssueContext).Project

	oldIssue := issue
	issueMapOld := StructToJSONMap(issue)

	var data map[string]interface{}
	form, _ := c.MultipartForm()

	// If comment without attachments
	if form == nil {
		if err := c.Bind(&data); err != nil {
			return EError(c, err)
		}
	} else {
		// else get comment data from "comment" value
		if c.FormValue("issue") == "" {
			//TODO: defined error
			return EError(c, nil)
		}
		if err := json.Unmarshal([]byte(c.FormValue("issue")), &data); err != nil {
			return EError(c, err)
		}

		if !limiter.Limiter.CanAddAttachment(project.WorkspaceId) {
			return EErrorDefined(c, apierrors.ErrAssetsLimitExceed)
		}
	}

	if val, ok := data["description_html"]; ok {
		if val == "" {
			delete(data, "description_html")
		} else {
			var locked bool
			if err := s.db.Model(&dao.IssueDescriptionLock{}).
				Select("EXISTS(?)",
					s.db.Model(&dao.IssueDescriptionLock{}).
						Select("1").
						Where("issue_id = ?", issue.ID).
						Where("user_id <> ?", user.ID).
						Where("locked_until > NOW()"),
				).
				Find(&locked).Error; err != nil {
				return EError(c, err)
			}

			if locked {
				return EErrorDefined(c, apierrors.ErrIssueDescriptionLocked)
			}
		}
	}

	var targetDateOk bool
	var targetDate *string
	if val, ok := data["target_date"]; ok {
		if issue.TargetDate != nil {
			if date, err := utils.FormatDateStr(issue.TargetDate.String(), "2006-01-02T15:04:05Z07:00", nil); err != nil {
				return EErrorDefined(c, apierrors.ErrGeneric)
			} else {
				issueMapOld["target_date_activity_val"] = date
			}
		}
		if val != nil {
			date, err := utils.FormatDateStr(val.(string), "2006-01-02T15:04:05Z07:00", nil)
			if err != nil {
				return EErrorDefined(c, apierrors.ErrGeneric)
			}

			if d, err := utils.FormatDate(date); err != nil {
				return EErrorDefined(c, apierrors.ErrGeneric)
			} else {
				if time.Now().After(d) {
					return EErrorDefined(c, apierrors.ErrIssueTargetDateExp)
				}
			}
			data["target_date"] = date
			targetDate = &date
		}
		targetDateOk = true
	}

	if name, ok := data["name"]; ok {
		if len(strings.TrimSpace(name.(string))) == 0 {
			return EErrorDefined(c, apierrors.ErrIssueNameEmpty)
		}
	}

	var unpinTask bool
	if parentId, ok := data["parent"]; ok {
		if parentId != nil {
			parentUUID, err := uuid.FromString(parentId.(string))
			if err != nil {
				return EError(c, err)
			}

			if !(oldIssue.ParentId.Valid && oldIssue.ParentId.UUID == parentUUID) {
				var ancestorIDs []uuid.UUID
				err = s.db.Raw(`
                WITH RECURSIVE ancestor_chain AS (
                    SELECT id, parent_id FROM issues WHERE id = ?
                    UNION ALL
                    SELECT i.id, i.parent_id FROM issues i
                    JOIN ancestor_chain ac ON i.id = ac.parent_id
                )
                SELECT id FROM ancestor_chain WHERE id != ?;
            `, parentUUID, parentUUID).Scan(&ancestorIDs).Error

				if err != nil {
					return EError(c, err)
				}

				if slices.Contains(ancestorIDs, issue.ID) {
					return EErrorDefined(c, apierrors.ErrChildDependency)
				}
			}

		} else {
			if issue.Parent != nil {
				unpinTask = issue.Parent.CreatedById == user.ID && len(data) == 4
			}
		}
		data["parent_id"] = parentId
	}
	var rulesLog []dao.RulesLog
	defer func() {
		if err := rules.AddLog(s.db, rulesLog); err != nil {
			slog.Error("Create rules log", "err", err)
		}
	}()

	var statusChange bool
	var newState dao.State
	if stateId, ok := data["state"]; ok {
		statusChange = true
		stateUUID, err := uuid.FromString(stateId.(string))
		if err != nil {
			return EError(c, err)
		}
		data["state_id"] = stateUUID

		if err := s.db.Where("id = ?", stateUUID).
			Where("project_id = ?", issue.ProjectId).
			First(&newState).Error; err != nil {
			return EError(c, err)
		}

		issueMapOld[actField.Status.Field.WithActivityValStr()] = issue.State.Name
		data[actField.Status.Field.WithActivityValStr()] = newState.Name
		if newState.Group == "started" && issue.State.Group != "started" {
			data["start_date"] = &types.TargetDate{Time: time.Now()}
		}

		if issue.State.Group == "started" && (newState.Group == "backlog" || newState.Group == "unstarted") {
			data["start_date"] = nil
		}

		if newState.Group == "completed" && issue.State.Group != "completed" {
			data["completed_at"] = &types.TargetDate{Time: time.Now()}
		} else if newState.Group != "completed" {
			if newState.Group == "started" {
				data["start_date"] = &types.TargetDate{Time: time.Now()}
			}
			// Reset completed at date on open status
			data["completed_at"] = nil
		} else {
			data["completed_at"] = issue.CompletedAt
		}

		if projectMember.Role != types.AdminRole {
			res, msg, err := rules.BeforeStatusChange(user, oldIssue, newState)

			rules.AppendMsg(issue, user, msg, &rulesLog)
			rules.AppendError(issue, user, err, &rulesLog)
			rules.ResultToLog(issue, user, res, err, &rulesLog)

			if !res.ClientResult {
				return EError(c, err.ClientError())
			}
		}
		issue.StateId = newState.ID
	}

	prepareUUIDParams := func(in []interface{}) []interface{} {
		var res []interface{}
		for _, i2 := range in {
			if id := uuid.FromStringOrNil(i2.(string)); !id.IsNil() {
				res = append(res, id)
			}
		}
		return utils.SetToSlice(utils.SliceToSet(res))
	}

	blockers, blockersOk := data["blockers_list"].([]interface{}) // задача блокирует [blocker_issues]
	if blockersOk {
		blockers = prepareUUIDParams(blockers)
		data["blockers_list"] = blockers
	}
	assignees, assigneesOk := data["assignees_list"].([]interface{})
	if assigneesOk {
		assignees = prepareUUIDParams(assignees)
		data["assignees_list"] = assignees
	}
	watchers, watchersOk := data["watchers_list"].([]interface{})
	if watchersOk {
		watchers = prepareUUIDParams(watchers)
		data["watchers_list"] = watchers
	}
	labels, labelsOk := data["labels_list"].([]interface{})
	if labelsOk {
		labels = prepareUUIDParams(labels)
		data["labels_list"] = labels
	}
	blocks, blocksOk := data["blocks_list"].([]interface{}) // блокируют эту задачу [blocked_issues]
	if blocksOk {
		blocks = prepareUUIDParams(blocks)
		data["blocks_list"] = blocks
	}
	parent, parentOk := data["parent"]

	//var getProjectMemberList []dao.ProjectMember
	var allProjectMembers []dao.User
	var allProjectLabels []dao.Label
	if watchersOk || assigneesOk {
		if err := s.db.
			Where("id in (?)", s.db.Model(&dao.ProjectMember{}).
				Select("member_id").
				Where("project_id = ?", project.ID)).
			Find(&allProjectMembers).Error; err != nil {
			return EError(c, err)
		}
	}

	if labelsOk {
		if err := s.db.Where("project_id = ?", project.ID).Find(&allProjectLabels).Error; err != nil {
			return EError(c, err)
		}
		data[actField.Label.Field.WithGetFieldStr()] = "labels"
	}

	// Reset sort order
	if parentOk && parent == nil {
		data["sort_order"] = 0
	} else if parentOk && parent != nil {
		var sortOrder int
		if err := s.db.Select("coalesce(max(sort_order), 0)").Model(&dao.Issue{}).Where("parent_id = ?", parent).Find(&sortOrder).Error; err != nil {
			return EError(c, err)
		}
		data["sort_order"] = sortOrder + 1
	}

	// Pre rules hooks
	{
		if assigneesOk && projectMember.Role != types.AdminRole {
			assigneesUser := dao.GetUserFromProjectMember(allProjectMembers, assignees)
			res, msg, err := rules.BeforeAssigneesChange(user, oldIssue, assigneesUser)

			rules.AppendMsg(issue, user, msg, &rulesLog)
			rules.AppendError(issue, user, err, &rulesLog)
			rules.ResultToLog(issue, user, res, err, &rulesLog)

			if !res.ClientResult {
				return EError(c, err.ClientError())
			}
		}

		if watchersOk && projectMember.Role != types.AdminRole {
			watchersUser := dao.GetUserFromProjectMember(allProjectMembers, watchers)
			res, msg, err := rules.BeforeWatchersChange(user, oldIssue, watchersUser)

			rules.AppendMsg(issue, user, msg, &rulesLog)
			rules.AppendError(issue, user, err, &rulesLog)
			rules.ResultToLog(issue, user, res, err, &rulesLog)

			if !res.ClientResult {
				return EError(c, err.ClientError())
			}
		}

		if labelsOk && projectMember.Role != types.AdminRole {
			var currentLabels []dao.Label
			for _, daoLabel := range allProjectLabels {
				for _, label := range labels {
					if daoLabel.ID == label {
						currentLabels = append(currentLabels, daoLabel)
					}
				}
			}
			res, msg, err := rules.BeforeLabelsChange(user, oldIssue, currentLabels)

			rules.AppendMsg(issue, user, msg, &rulesLog)
			rules.AppendError(issue, user, err, &rulesLog)
			rules.ResultToLog(issue, user, res, err, &rulesLog)

			if !res.ClientResult {
				return EError(c, err.ClientError())
			}
		}
	}

	updateAll := projectMember.Role == types.AdminRole || issue.CreatedById == user.ID || unpinTask

	userID := uuid.NullUUID{UUID: user.ID, Valid: true}
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		// Upload new files
		if form != nil {
			// Save issue attachments to
			for _, f := range form.File["files"] {
				fileAsset := dao.FileAsset{
					Id:          dao.GenUUID(),
					CreatedAt:   time.Now(),
					CreatedById: userID,
					Name:        f.Filename,
					FileSize:    int(f.Size),
					WorkspaceId: uuid.NullUUID{UUID: issue.WorkspaceId, Valid: true},
					IssueId:     uuid.NullUUID{Valid: true, UUID: issue.ID},
				}

				if err := s.uploadAssetForm(tx, f, &fileAsset,
					filestorage.Metadata{
						WorkspaceId: issue.WorkspaceId.String(),
						ProjectId:   issue.ProjectId.String(),
						IssueId:     issue.ID.String(),
					}); err != nil {
					return err
				}

				issue.InlineAttachments = append(issue.InlineAttachments, fileAsset)
			}
		}

		dataField := utils.MapToSlice(data, func(k string, v interface{}) string { return actField.ReqFieldMapping(k) })
		if hasRecentFieldUpdate[dao.IssueActivity](tx.Where("issue_id = ?", issue.ID), user.ID, dataField...) {
			return apierrors.ErrUpdateTooFrequent
		}

		// Update blockers
		if blockersOk && updateAll {
			// Delete all blockers
			if err := tx.Where("block_id = ?", issue.ID).Unscoped().Delete(&dao.IssueBlocker{}).Error; err != nil {
				return err
			}

			var newBlockers []dao.IssueBlocker
			for _, blocker := range blockers {
				blockerUUID, err := uuid.FromString(fmt.Sprint(blocker))
				if err != nil {
					return err
				}
				newBlockers = append(newBlockers, dao.IssueBlocker{
					Id:          dao.GenUUID(),
					BlockedById: blockerUUID,
					BlockId:     issue.ID,
					ProjectId:   issue.ProjectId,
					WorkspaceId: issue.WorkspaceId,
					CreatedById: userID,
					UpdatedById: userID,
				})
			}
			if err := tx.CreateInBatches(&newBlockers, 10).Error; err != nil {
				return err
			}
		}

		// Update assignees
		if assigneesOk {

			// Delete all assignees
			if err := tx.Where("issue_id = ?", issue.ID).Unscoped().Delete(&dao.IssueAssignee{}).Error; err != nil {
				return err
			}

			var newAssignees []dao.IssueAssignee
			for _, assignee := range assignees {
				assigneeUUID := uuid.FromStringOrNil(fmt.Sprint(assignee))
				newAssignees = append(newAssignees, dao.IssueAssignee{
					Id:          dao.GenUUID(),
					AssigneeId:  assigneeUUID,
					IssueId:     issue.ID,
					ProjectId:   issue.ProjectId,
					WorkspaceId: issue.WorkspaceId,
					CreatedById: userID,
					UpdatedById: userID,
				})
			}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(&newAssignees, 10).Error; err != nil {
				return err
			}
		}

		// Update watchers
		if watchersOk {
			// Delete all watchers
			if err := tx.Where("issue_id = ?", issue.ID).Unscoped().Delete(&dao.IssueWatcher{}).Error; err != nil {
				return err
			}

			var newWatchers []dao.IssueWatcher
			for _, watcher := range watchers {
				watcherUUID := uuid.FromStringOrNil(fmt.Sprint(watcher))
				newWatchers = append(newWatchers, dao.IssueWatcher{
					Id:          dao.GenUUID(),
					WatcherId:   watcherUUID,
					IssueId:     issue.ID,
					ProjectId:   issue.ProjectId,
					WorkspaceId: issue.WorkspaceId,
					CreatedById: userID,
					UpdatedById: userID,
				})
			}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(&newWatchers, 10).Error; err != nil {
				return err
			}
		}

		// Update labels
		if labelsOk {
			// Delete all labels
			if err := tx.Where("issue_id = ?", issue.ID).Unscoped().Delete(&dao.IssueLabel{}).Error; err != nil {
				return err
			}

			var newLabels []dao.IssueLabel
			for _, label := range labels {
				newLabels = append(newLabels, dao.IssueLabel{
					Id:          dao.GenUUID(),
					LabelId:     uuid.Must(uuid.FromString(fmt.Sprint(label))),
					IssueId:     issue.ID,
					ProjectId:   issue.ProjectId,
					WorkspaceId: issue.WorkspaceId,
					CreatedById: userID,
					UpdatedById: userID,
				})
			}
			if err := tx.CreateInBatches(&newLabels, 10).Error; err != nil {
				return err
			}
		}

		// Update blocked
		if blocksOk && updateAll {
			// Delete all blocked
			if err := tx.Where("blocked_by_id = ?", issue.ID).Unscoped().Delete(&dao.IssueBlocker{}).Error; err != nil {
				return err
			}

			var newBlocked []dao.IssueBlocker
			for _, block := range blocks {
				blockUUID, err := uuid.FromString(fmt.Sprint(block))
				if err != nil {
					return err
				}
				newBlocked = append(newBlocked, dao.IssueBlocker{
					Id:          dao.GenUUID(),
					BlockId:     blockUUID,
					BlockedById: issue.ID,
					ProjectId:   issue.ProjectId,
					WorkspaceId: issue.WorkspaceId,
					CreatedById: userID,
					UpdatedById: userID,
				})
			}
			if err := tx.CreateInBatches(&newBlocked, 10).Error; err != nil {
				return err
			}
		}

		if targetDateOk || issue.TargetDate != nil {
			if targetDateOk || assigneesOk || watchersOk {
				assigneeIds := &issue.AssigneeIDs
				watcherIds := &issue.WatcherIDs
				var notifyUserIds []uuid.UUID

				if targetDateOk {
					notifyUserIds = issue.AssigneeIDs
					notifyUserIds = append(notifyUserIds, issue.WatcherIDs...)
				}

				if assigneesOk || watchersOk {
					dateStr, err := notifications.FormatDate(issue.TargetDate.Time.String(), "2006-01-02", nil)
					if err != nil {
						return EErrorDefined(c, apierrors.ErrGeneric)
					}

					if assigneesOk {
						notifyUserIds = []uuid.UUID{}
						for _, v := range assignees {
							if str, ok := v.(uuid.UUID); ok {
								notifyUserIds = append(notifyUserIds, str)
							}
						}
						notifyUserIds = append(notifyUserIds, *watcherIds...)
					}
					if watchersOk {
						notifyUserIds = []uuid.UUID{}

						for _, v := range watchers {
							if str, ok := v.(uuid.UUID); ok {
								notifyUserIds = append(notifyUserIds, str)
							}
						}
						notifyUserIds = append(notifyUserIds, *assigneeIds...)
					}

					targetDate = &dateStr
				}
				notifyUserIds = append(notifyUserIds, issue.Author.ID)

				err := notifications.CreateDeadlineNotification(tx, &issue, targetDate, notifyUserIds)
				if err != nil {
					return err
				}
			}

		}

		issue.UpdatedAt = time.Now()
		issue.UpdatedById = uuid.NullUUID{UUID: user.ID, Valid: true}

		data["updated_at"] = time.Now()
		data["updated_by_id"] = userID

		var err error
		if updateAll {
			err = tx.Model(&issue).Select(issue.FieldsAllowedForUpdate()).Updates(data).Error
		} else {
			err = tx.Model(&issue).Select(issue.FieldsAllowedForAllUpdate()).Updates(data).Error
		}

		if err := tx.Where("issue_id = ?", issue.ID).Delete(&dao.IssueDescriptionLock{}).Error; err != nil && err != gorm.ErrRecordNotFound {
			return err
		}

		return err
	}); err != nil {
		if err.Error() == "forbidden" {
			return EErrorDefined(c, apierrors.ErrIssueUpdateForbidden)
		}
		return EError(c, err)
	}

	err := tracker.TrackActivity[dao.Issue, dao.IssueActivity](s.tracker, actField.EntityUpdatedActivity, data, issueMapOld, issue, &user)
	if err != nil {
		errStack.GetError(c, err)
	}

	//s.tracker.TrackActivity(tracker.ISSUE_UPDATED_ACTIVITY, data, issueMapOld, issue.ID.String(), tracker.ENTITY_TYPE_ISSUE, &project, user)

	if statusChange {
		res, msg, err := rules.AfterStatusChange(user, oldIssue, newState)

		rules.AppendMsg(issue, user, msg, &rulesLog)
		rules.AppendError(issue, user, err, &rulesLog)
		rules.ResultToLog(issue, user, res, err, &rulesLog)

		if !res.ClientResult {
			return EError(c, err.ClientError())
		}
	}

	return c.JSON(http.StatusOK, issue.ToDTO())
}

// deleteIssue godoc
// @id deleteIssue
// @Summary Задачи: удаление задачи
// @Description Удаляет задачу из проекта. Только автор задачи или администратор могут выполнить удаление.
// @Tags Issues
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param issueIdOrSeq path string true "Идентификатор или последовательный номер задачи"
// @Success 200 "Задача успешно удалена"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/issues/{issueIdOrSeq} [delete]
func (s *Services) deleteIssue(c echo.Context) error {
	user := *c.(IssueContext).User
	//project := c.(IssueContext).Project
	issue := c.(IssueContext).Issue
	//currentInst := StructToJSONMap(issue)
	if c.(IssueContext).ProjectMember.Role != types.AdminRole && issue.CreatedById != user.ID {
		return EErrorDefined(c, apierrors.ErrDeleteIssueForbidden)
	}

	//if err := s.tracker.TrackActivity(tracker.ISSUE_DELETED_ACTIVITY, nil, currentInst, issue.ID.String(), tracker.ENTITY_TYPE_ISSUE, &project, user); err != nil {
	//	return EError(c, err)
	//}

	if err := s.db.Transaction(func(tx *gorm.DB) error {

		err := tracker.TrackActivity[dao.Issue, dao.ProjectActivity](s.tracker, actField.EntityDeleteActivity, nil, nil, issue, &user)
		if err != nil {
			errStack.GetError(c, err)
			return err
		}

		return s.db.Delete(&issue).Error
	}); err != nil {
		if err.Error() == "forbidden" {
			return EErrorDefined(c, apierrors.ErrDocUpdateForbidden)
		}
		return EError(c, err)
	}

	return c.NoContent(http.StatusOK)
}

// ############# Sub-issues methods ###################

// getSubIssueList godoc
// @id getSubIssueList
// @Summary Задачи: получение списка подзадач
// @Description Возвращает список подзадач для указанной задачи с распределением по состояниям
// @Tags Issues
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param issueIdOrSeq path string true "Идентификатор или последовательный номер задачи"
// @Success 200 {object} dto.ResponseSubIssueList "Список подзадач и распределение состояний"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/issues/{issueIdOrSeq}/sub-issues [get]
func (s *Services) getSubIssueList(c echo.Context) error {
	project := c.(IssueContext).Project
	issueId := c.(IssueContext).Issue.ID

	manualSort := false

	if err := echo.QueryParamsBinder(c).Bool("manual_sort", &manualSort).BindError(); err != nil {
		return EError(c, err)
	}

	query := s.db.
		Where(&dao.Issue{ParentId: uuid.NullUUID{UUID: issueId, Valid: true}, ProjectId: project.ID}).
		Joins("State").
		Joins("Project").
		Joins("Workspace").
		Joins("Author")

	if !manualSort {
		query = query.Order(`"State".sequence, sequence_id`)
		//Select(`issues.*, case "State".group when 'backlog' then 1 when 'unstarted' then 2 when 'started' then 3 when 'completed' then 4 when 'cancelled' then 5 end as state_sort`).
		//Order("state_sort, sequence_id")
	} else {
		query = query.Order("sort_order, sequence_id")
	}

	var subIssues []dao.Issue
	if err := query.
		Find(&subIssues).Error; err != nil {
		return EError(c, err)
	}

	stateDistribution := make(map[string]int)
	for _, issue := range subIssues {
		if _, ok := stateDistribution[issue.State.Group]; !ok {
			stateDistribution[issue.State.Group] = 0
		}
		stateDistribution[issue.State.Group] = stateDistribution[issue.State.Group] + 1
	}

	resp := dto.ResponseSubIssueList{
		SubIssues:         utils.SliceToSlice(&subIssues, func(i *dao.Issue) dto.Issue { return *i.ToDTO() }),
		StateDistribution: stateDistribution,
	}
	return c.JSON(http.StatusOK, resp)
}

// addSubIssueList godoc
// @id addSubIssueList
// @Summary Задачи: добавление подзадач
// @Description Добавляет указанные задачи как подзадачи для текущей задачи
// @Tags Issues
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param issueIdOrSeq path string true "Идентификатор или последовательный номер задачи"
// @Param data body SubIssuesIds true "Идентификаторы подзадач для добавления"
// @Success 200 {array} dto.IssueLight "Список добавленных подзадач"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/issues/{issueIdOrSeq}/sub-issues [post]
func (s *Services) addSubIssueList(c echo.Context) error {
	project := c.(IssueContext).Project
	parentIssue := c.(IssueContext).Issue
	user := c.(IssueContext).User

	var req SubIssuesIds
	if err := c.Bind(&req); err != nil {
		return EError(c, err)
	}

	if len(req.SubIssueIDs) == 0 {
		return c.JSON(http.StatusOK, []dao.Issue{})
	}

	var subIssueIDs []string
	for _, id := range req.SubIssueIDs {
		rootId, err := s.getRootAncestorID(s.db, id)
		if err != nil {
			return EError(c, err)
		}
		if rootId != parentIssue.ID.String() {
			subIssueIDs = append(subIssueIDs, id.String())
		}
	}

	query := s.db.
		Preload("Project").
		Where("project_id = ?", project.ID).
		Where("parent_id is null").
		Where("id in ?", subIssueIDs)

	if project.CurrentUserMembership.Role < types.AdminRole {
		query = query.Where("created_by_id = ?", user.ID)
	}

	var subIssues []dao.Issue
	if err := query.
		Find(&subIssues).Error; err != nil {
		return EError(c, err)
	}

	var maxSortOrder int
	if err := s.db.Select("coalesce(max(sort_order), 0)").Where("parent_id = ?", parentIssue.ID).Model(&dao.Issue{}).Find(&maxSortOrder).Error; err != nil {
		return EError(c, err)
	}

	// Save old data for activity tracking
	oldSubIssuesData := make([]map[string]interface{}, len(subIssues))
	for i, issue := range subIssues {
		oldSubIssuesData[i] = StructToJSONMap(issue)
		oldSubIssuesData[i]["parent"] = uuid.NullUUID{}
	}

	id, err := uuid.FromString(parentIssue.ID.String())
	if err != nil {
		return EError(c, err)
	}
	parentId := uuid.NullUUID{UUID: id, Valid: true}

	for i := range subIssues {
		if project.CurrentUserMembership.Role != types.AdminRole && subIssues[i].CreatedById != user.ID {
			return EErrorDefined(c, apierrors.ErrPermissionParentIssue)
		}
		subIssues[i].ParentId = parentId
		subIssues[i].UpdatedById = uuid.NullUUID{UUID: user.ID, Valid: true}
		subIssues[i].SortOrder = i + maxSortOrder + 1
	}

	if err := s.db.Save(&subIssues).Error; err != nil {
		return EError(c, err)
	}

	// Save new data for activity tracking
	newSubIssuesData := make([]map[string]interface{}, len(subIssues))
	for i := range subIssues {
		newSubIssuesData[i] = make(map[string]interface{})
		newSubIssuesData[i]["parent"] = parentId.UUID
	}

	// Activity tracking
	for i := range subIssues {
		err := tracker.TrackActivity[dao.Issue, dao.IssueActivity](s.tracker, actField.EntityUpdatedActivity, newSubIssuesData[i], oldSubIssuesData[i], subIssues[i], user)
		if err != nil {
			errStack.GetError(c, err)
		}

		//if err := s.tracker.TrackActivity(
		//	tracker.ISSUE_UPDATED_ACTIVITY,
		//	newSubIssuesData[i],
		//	oldSubIssuesData[i],
		//	subIssues[i].ID.String(),
		//	tracker.ENTITY_TYPE_ISSUE,
		//	&project,
		//	*user,
		//); err != nil {
		//	return EError(c, err)
		//}
	}

	return c.JSON(http.StatusOK, utils.SliceToSlice(&subIssues, func(i *dao.Issue) dto.IssueLight { return *i.ToLightDTO() }))
}

// moveSubIssueUp godoc
// @id moveSubIssueUp
// @Summary Задачи: перемещение подзадачи вверх в списке
// @Description Перемещает подзадачу выше по списку при ручной сортировке детей
// @Tags Issues
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path int true "ID проекта"
// @Param issueIdOrSeq path string true "Идентификатор или последовательный номер задачи"
// @Param subIssueId path string true "Идентификатор подзадачи"
// @Success 200 "Подзадача успешно перемещена"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/issues/{issueIdOrSeq}/sub-issues/{subIssueId}/up [post]
func (s *Services) moveSubIssueUp(c echo.Context) error {
	project := c.(IssueContext).Project
	parentIssue := c.(IssueContext).Issue

	subIssueId := c.Param("subIssueId")

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		var subIssue dao.Issue
		if err := tx.Model(&subIssue).
			Clauses(clause.Returning{Columns: []clause.Column{{Name: "sort_order"}}}).
			Where("id = ?", subIssueId).
			Where("workspace_id = ?", project.WorkspaceId).
			Where("project_id = ?", project.ID).
			Where("parent_id = ?", parentIssue.ID).
			Update("sort_order", gorm.Expr("GREATEST(sort_order - 1, 0)")).Error; err != nil {
			return err
		}

		return tx.
			Model(&dao.Issue{}).
			Where("workspace_id = ?", project.WorkspaceId).
			Where("project_id = ?", project.ID).
			Where("parent_id = ?", parentIssue.ID).
			Where("sort_order = ?", subIssue.SortOrder).
			Where("id != ?", subIssueId).
			Update("sort_order", subIssue.SortOrder+1).Error
	}); err != nil {
		if err == gorm.ErrRecordNotFound {
			return EErrorDefined(c, apierrors.ErrIssueNotFound)
		}
		return EError(c, err)
	}

	return c.NoContent(http.StatusOK)
}

// moveSubIssueDown godoc
// @id moveSubIssueDown
// @Summary Задачи: перемещение подзадачи вверх в списке
// @Description Перемещает подзадачу выше по списку при ручной сортировке детей
// @Tags Issues
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path int true "ID проекта"
// @Param issueIdOrSeq path string true "Идентификатор или последовательный номер задачи"
// @Param subIssueId path string true "Идентификатор подзадачи"
// @Success 200 "Подзадача успешно перемещена"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/issues/{issueIdOrSeq}/sub-issues/{subIssueId}/down [post]
func (s *Services) moveSubIssueDown(c echo.Context) error {
	project := c.(IssueContext).Project
	parentIssue := c.(IssueContext).Issue

	subIssueId := c.Param("subIssueId")

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		var subIssue dao.Issue
		if err := tx.Model(&subIssue).
			Clauses(clause.Returning{Columns: []clause.Column{{Name: "sort_order"}}}).
			Where("id = ?", subIssueId).
			Where("workspace_id = ?", project.WorkspaceId).
			Where("project_id = ?", project.ID).
			Where("parent_id = ?", parentIssue.ID).
			Update("sort_order",
				gorm.Expr("LEAST(sort_order + 1, (?))",
					s.db.Select("max(sort_order) + 1").
						Where("workspace_id = ?", project.WorkspaceId).
						Where("project_id = ?", project.ID).
						Where("parent_id = ?", parentIssue.ID).
						Where("id != ?", subIssueId).
						Model(&dao.Issue{}),
				)).Error; err != nil {
			return err
		}

		return tx.
			Model(&dao.Issue{}).
			Where("workspace_id = ?", project.WorkspaceId).
			Where("project_id = ?", project.ID).
			Where("parent_id = ?", parentIssue.ID).
			Where("sort_order = ?", subIssue.SortOrder).
			Where("id != ?", subIssueId).
			Update("sort_order", subIssue.SortOrder-1).Error
	}); err != nil {
		if err == gorm.ErrRecordNotFound {
			return EErrorDefined(c, apierrors.ErrIssueNotFound)
		}
		return EError(c, err)
	}

	return c.NoContent(http.StatusOK)
}

// ############# Search for available issues methods ###################

// getAvailableSubIssueList godoc
// @id getAvailableSubIssueList
// @Summary Задачи: поиск доступных подзадач
// @Description Поиск задач, доступных для добавления в качестве подзадач
// @Tags Issues
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param issueIdOrSeq path string true "Идентификатор или последовательный номер задачи"
// @Param offset query int false "Смещение для пагинации"
// @Param limit query int false "Лимит записей"
// @Param order_by query string false "Поле для сортировки"
// @Param desc query bool false "Сортировка по убыванию"
// @Param search_query query string false "Поисковый запрос"
// @Success 200 {object} dao.PaginationResponse{result=[]dto.Issue} "Список доступных задач для добавления"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/issues/{issueIdOrSeq}/sub-issues/available [get]
func (s *Services) getAvailableSubIssueList(c echo.Context) error {
	return s.availableIssues(c, SearchSubIssues)
}

// getAvailableParentIssueList godoc
// @id getAvailableParentIssueList
// @Summary Задачи: поиск доступных родительских задач
// @Description Поиск задач, доступных для назначения в качестве родительских для текущей задачи
// @Tags Issues
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param issueIdOrSeq path string true "Идентификатор или последовательный номер задачи"
// @Param offset query int false "Смещение для пагинации"
// @Param limit query int false "Лимит записей"
// @Param order_by query string false "Поле для сортировки"
// @Param desc query bool false "Сортировка по убыванию"
// @Param search_query query string false "Поисковый запрос"
// @Success 200 {object} dao.PaginationResponse{result=[]dto.Issue} "Список доступных родительских задач"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/issues/{issueIdOrSeq}/parent-issues/available [get]
func (s *Services) getAvailableParentIssueList(c echo.Context) error {
	return s.availableIssues(c, SearchParentIssues)
}

// getAvailableBlocksIssueList godoc
// @id getAvailableBlocksIssueList
// @Summary Задачи: поиск доступных блокируемых задач
// @Description Поиск задач, которые могут быть добавлены в качестве блокируемых текущей задачей
// @Tags Issues
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param issueIdOrSeq path string true "Идентификатор или последовательный номер задачи"
// @Param offset query int false "Смещение для пагинации"
// @Param limit query int false "Лимит записей"
// @Param order_by query string false "Поле для сортировки"
// @Param desc query bool false "Сортировка по убыванию"
// @Param search_query query string false "Поисковый запрос"
// @Success 200 {object} dao.PaginationResponse{result=[]dto.Issue} "Список доступных блокируемых задач"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/issues/{issueIdOrSeq}/blocks-issues/available [get]
func (s *Services) getAvailableBlocksIssueList(c echo.Context) error {
	return s.availableIssues(c, SearchBlocksIssues)
}

// getAvailableBlockersIssueList godoc
// @id getAvailableBlockersIssueList
// @Summary Задачи: поиск доступных блокирующих задач
// @Description Поиск задач, которые могут быть добавлены в качестве блокирующих для текущей задачи
// @Tags Issues
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param issueIdOrSeq path string true "Идентификатор или последовательный номер задачи"
// @Param offset query int false "Смещение для пагинации"
// @Param limit query int false "Лимит записей"
// @Param order_by query string false "Поле для сортировки"
// @Param desc query bool false "Сортировка по убыванию"
// @Param search_query query string false "Поисковый запрос"
// @Success 200 {object} dao.PaginationResponse{result=[]dto.Issue}"Список доступных блокирующих задач"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/issues/{issueIdOrSeq}/blockers-issues/available [get]
func (s *Services) getAvailableBlockersIssueList(c echo.Context) error {
	return s.availableIssues(c, SearchBlockersIssues)
}

// getAvailableLinkedIssueList godoc
// @id getAvailableLinkedIssueList
// @Summary Задачи: поиск доступных для связывания задач
// @Description Поиск задач, которые могут быть связаны с текущей задачи
// @Tags Issues
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param issueIdOrSeq path string true "Идентификатор или последовательный номер задачи"
// @Param offset query int false "Смещение для пагинации"
// @Param limit query int false "Лимит записей"
// @Param order_by query string false "Поле для сортировки"
// @Param desc query bool false "Сортировка по убыванию"
// @Param search_query query string false "Поисковый запрос"
// @Success 200 {object} dao.PaginationResponse{result=[]dto.Issue} "Список доступных блокирующих задач"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/issues/{issueIdOrSeq}/linked-issues/available/ [get]
func (s *Services) getAvailableLinkedIssueList(c echo.Context) error {
	return s.availableIssues(c, SearchLinkedIssues)
}

const (
	SearchParentIssues = iota
	SearchSubIssues
	SearchBlocksIssues
	SearchBlockersIssues
	SearchLinkedIssues
)

func (s *Services) availableIssues(c echo.Context, issuesType int) error {
	currentIssue := c.(IssueContext).Issue
	member := c.(IssueContext).ProjectMember

	offset := 0
	limit := 100
	orderBy := "name"

	desc := false
	searchQuery := ""

	if err := echo.QueryParamsBinder(c).
		Int("offset", &offset).
		Int("limit", &limit).
		String("order_by", &orderBy).
		Bool("desc", &desc).
		String("search_query", &searchQuery).
		BindError(); err != nil {
		return EError(c, err)
	}

	if limit > 100 {
		limit = 100
	}

	query := s.db.
		Preload("Workspace").
		Preload("Watchers").
		Preload("Assignees").
		Preload("Author").
		Joins("join projects p on p.id = issues.project_id").
		Where("issues.id != ?", currentIssue.ID).
		Where("project_id = ?", currentIssue.ProjectId).
		Order(clause.OrderByColumn{
			Column: clause.Column{Name: orderBy},
			Desc:   desc,
		})

	if member.Role < types.AdminRole && (issuesType == SearchParentIssues || issuesType == SearchSubIssues) {
		query = query.Where("issues.created_by_id = ?", member.MemberId)
	}

	switch issuesType {
	case SearchParentIssues:
		familyIDs, err := s.getDescendantIssueIDs(s.db, currentIssue.ID)
		if err != nil {
			return EError(c, err)
		}
		if len(familyIDs) > 0 {
			query = query.Where("issues.id NOT IN (?)", familyIDs)
		}
		if currentIssue.ParentId.Valid {
			query = query.Where("issues.id != ?", currentIssue.ParentId)
		}
	case SearchSubIssues:
		query = query.Where("parent_id is null")
		if currentIssue.ParentId.Valid {
			rootId, err := s.getRootAncestorID(s.db, currentIssue.ID)
			if err != nil {
				return EError(c, err)
			}
			query = query.Where("issues.id != ?", rootId)
		}
	case SearchBlocksIssues:
		blockedIssueIDs, err := s.getBlockedIssueIDs(s.db, currentIssue.ID)
		if err != nil {
			return EError(c, fmt.Errorf("error in recursive query for blocked issues: %v", err))
		}
		if len(blockedIssueIDs) > 0 {
			query = query.Where("issues.id NOT IN (?)", blockedIssueIDs)
		}
		query = query.Where("issues.id NOT IN (?)",
			s.db.
				Select("block_id").
				Where("blocked_by_id = ?", currentIssue.ID).
				Where("project_id = ?", currentIssue.ProjectId).
				Model(&dao.IssueBlocker{}),
		)
	case SearchBlockersIssues:
		blockingIssueIDs, err := s.getBlockingIssueIDs(s.db, currentIssue.ID)
		if err != nil {
			return EError(c, fmt.Errorf("error in recursive query for blockers: %v", err))
		}
		if len(blockingIssueIDs) > 0 {
			query = query.Where("issues.id NOT IN (?)", blockingIssueIDs)
		}
		query = query.Where("issues.id NOT IN (?)",
			s.db.
				Select("blocked_by_id").
				Where("block_id = ?", currentIssue.ID).
				Where("project_id = ?", currentIssue.ProjectId).
				Model(&dao.IssueBlocker{}),
		)
	case SearchLinkedIssues:
		if member.Role == types.GuestRole || (member.Role == types.MemberRole && currentIssue.Author.ID != c.(IssueContext).User.ID) {
			query = query.Where("1 = 0")
		}
	default:
		return EError(c, fmt.Errorf("unsupported available issues search type %d", issuesType))
	}

	if searchQuery != "" {
		query = query.Where(dao.Issue{}.FullTextSearch(s.db, searchQuery))
	}

	var issues []dao.Issue
	resp, err := dao.PaginationRequest(
		offset,
		limit,
		query,
		&issues,
	)
	if err != nil {
		return EError(c, err)
	}

	resp.Result = utils.SliceToSlice(resp.Result.(*[]dao.Issue), func(i *dao.Issue) dto.Issue { return *i.ToDTO() })
	return c.JSON(http.StatusOK, resp)
}

// getDescendantIssueIDs:все задачи, которые являются потомками (или наследниками) указанной задачи
func (s *Services) getDescendantIssueIDs(tx *gorm.DB, issueID uuid.UUID) ([]string, error) {
	var descendantIssueIDs []string
	err := tx.Raw(`
		WITH RECURSIVE descendant_chain AS (
			SELECT id, parent_id
			FROM issues
			WHERE id = ? OR parent_id = ?
			UNION ALL
			SELECT i.id, i.parent_id
			FROM issues i
			INNER JOIN descendant_chain dc ON i.parent_id = dc.id
		)
		SELECT id FROM descendant_chain WHERE id != ?;
	`, issueID, issueID, issueID).Scan(&descendantIssueIDs).Error

	return descendantIssueIDs, err
}

// getRootAncestorID: Получение ID самого верхнего предка (корня) для указанной задачи
func (s *Services) getRootAncestorID(tx *gorm.DB, issueID uuid.UUID) (string, error) {
	var rootAncestorID string
	err := tx.Raw(`
		WITH RECURSIVE ancestor_chain AS (
			SELECT id, parent_id
			FROM issues
			WHERE id = ?
			UNION ALL
			SELECT i.id, i.parent_id
			FROM issues i
			INNER JOIN ancestor_chain ac ON i.id = ac.parent_id
			WHERE ac.parent_id IS NOT NULL
		)
		SELECT id FROM ancestor_chain
		WHERE parent_id IS NULL;
	`, issueID).Scan(&rootAncestorID).Error

	return rootAncestorID, err
}

// getBlockedIssueIDs: для получения всех задач, которые заблокированы указанной задачей.
func (s *Services) getBlockedIssueIDs(tx *gorm.DB, issueID uuid.UUID) ([]string, error) {
	var blockedIssueIDs []string
	err := tx.Raw(`
		WITH RECURSIVE blocked_chain AS (
			SELECT block_id
			FROM issue_blockers
			WHERE blocked_by_id = ?
			UNION ALL
			SELECT ib.block_id
			FROM issue_blockers ib
			INNER JOIN blocked_chain bc ON ib.blocked_by_id = bc.block_id
		)
		SELECT DISTINCT block_id FROM blocked_chain;
	`, issueID).Scan(&blockedIssueIDs).Error
	return blockedIssueIDs, err
}

// getBlockingIssueIDs: получение всех задач, которые блокируют указанную задачу.
func (s *Services) getBlockingIssueIDs(tx *gorm.DB, issueID uuid.UUID) ([]string, error) {
	var blockingIssueIDs []string
	err := tx.Raw(`
		WITH RECURSIVE blocking_chain AS (
			SELECT blocked_by_id
			FROM issue_blockers
			WHERE block_id = ?
			UNION ALL
			SELECT ib.blocked_by_id
			FROM issue_blockers ib
			INNER JOIN blocking_chain bc ON ib.block_id = bc.blocked_by_id
		)
		SELECT DISTINCT blocked_by_id FROM blocking_chain;
	`, issueID).Scan(&blockingIssueIDs).Error

	return blockingIssueIDs, err
}

// ############# Issue links methods ###################

// getIssueLinkList godoc
// @id getIssueLinkList
// @Summary Задачи (ссылки): получение списка ссылок в задаче
// @Description Возвращает список всех ссылок, прикрепленных к задаче
// @Tags Issues
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param issueIdOrSeq path string true "Идентификатор или последовательный номер задачи"
// @Success 200 {array} dto.IssueLinkLight "Список ссылок"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/issues/{issueIdOrSeq}/issue-links [get]
func (s *Services) getIssueLinkList(c echo.Context) error {
	project := c.(IssueContext).Project
	issueId := c.(IssueContext).Issue.ID

	var links []dao.IssueLink
	if err := s.db.
		Where("project_id = ?", project.ID).Where("issue_id = ?", issueId).Order("created_at").Find(&links).Error; err != nil {
		return EError(c, err)
	}
	return c.JSON(
		http.StatusOK,
		utils.SliceToSlice(&links, func(il *dao.IssueLink) dto.IssueLinkLight { return *il.ToLightDTO() }),
	)
}

// createIssueLink godoc
// @id createIssueLink
// @Summary Задачи (ссылки): создание ссылки в задаче
// @Description Создает новую ссылку, прикрепленную к задаче
// @Tags Issues
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param issueIdOrSeq path string true "Идентификатор или последовательный номер задачи"
// @Param data body IssueLinkRequest true "Данные новой ссылки"
// @Success 200 {object} dto.IssueLinkLight "Созданная ссылка"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/issues/{issueIdOrSeq}/issue-links [post]
func (s *Services) createIssueLink(c echo.Context) error {
	user := *c.(IssueContext).User
	project := c.(IssueContext).Project
	issue := c.(IssueContext).Issue

	var req IssueLinkRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return EError(c, err)
	}

	if req.Url == "" || req.Title == "" {
		return EErrorDefined(c, apierrors.ErrURLAndTitleRequired)
	}

	userID := uuid.NullUUID{UUID: user.ID, Valid: true}
	link := dao.IssueLink{
		Id:          dao.GenUUID(),
		Title:       req.Title,
		Url:         req.Url,
		CreatedById: userID,
		UpdatedById: userID,
		IssueId:     issue.ID,
		ProjectId:   project.ID,
		WorkspaceId: project.WorkspaceId,
	}

	if err := s.db.Create(&link).Error; err != nil {
		return EError(c, err)
	}

	//if err := s.tracker.TrackActivity(tracker.LINK_CREATED_ACTIVITY, StructToJSONMap(link), nil, issueId, tracker.ENTITY_TYPE_ISSUE, &project, user); err != nil {
	//	return EError(c, err)
	//}

	err := tracker.TrackActivity[dao.IssueLink, dao.IssueActivity](s.tracker, actField.EntityCreateActivity, nil, nil, link, &user)
	if err != nil {
		errStack.GetError(c, err)
	}

	return c.JSON(http.StatusOK, link.ToLightDTO())
}

// updateIssueLink godoc
// @id updateIssueLink
// @Summary Задачи (ссылки): обновление ссылки в задаче
// @Description Обновляет существующую ссылку в задаче
// @Tags Issues
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param issueIdOrSeq path string true "Идентификатор или последовательный номер задачи"
// @Param linkId path string true "ID ссылки"
// @Param data body IssueLinkRequest true "Данные для обновления ссылки"
// @Success 200 {object} dto.IssueLinkLight "Обновленная ссылка"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/issues/{issueIdOrSeq}/issue-links/{linkId} [patch]
func (s *Services) updateIssueLink(c echo.Context) error {
	user := *c.(IssueContext).User
	//project := c.(IssueContext).Project
	//issueId := c.(IssueContext).Issue.ID.String()

	linkId := c.Param("linkId")

	var oldLink dao.IssueLink
	if err := s.db.Where("id = ?", linkId).First(&oldLink).Error; err != nil {
		return EError(c, err)
	}

	oldMap := StructToJSONMap(oldLink)
	var newLink IssueLinkRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&newLink); err != nil {
		return EError(c, err)
	}

	if newLink.Url == "" || newLink.Title == "" {
		return EErrorDefined(c, apierrors.ErrURLAndTitleRequired)
	}
	if newLink.Url == oldLink.Url && newLink.Title == oldLink.Title {
		return c.JSON(http.StatusOK, oldLink)
	}

	userID := uuid.NullUUID{UUID: user.ID, Valid: true}
	oldLink.UpdatedAt = time.Now()
	oldLink.UpdatedById = userID
	oldLink.Title = newLink.Title
	oldLink.Url = newLink.Url

	{ //rateLimit
		dataField := utils.MapToSlice(oldMap, func(k string, v interface{}) string { return fmt.Sprintf("link_%s", actField.ReqFieldMapping(k)) })
		if hasRecentFieldUpdate[dao.IssueActivity](s.db.Where("issue_id = ?", oldLink.IssueId), user.ID, dataField...) {
			return EErrorDefined(c, apierrors.ErrUpdateTooFrequent)
		}
	}

	if err := s.db.Omit(clause.Associations).Save(&oldLink).Error; err != nil {
		return EError(c, err)
	}
	newMap := StructToJSONMap(oldLink)
	newMap["updateScopeId"] = oldLink.Id

	oldMap["updateScope"] = "link"
	oldMap["updateScopeId"] = oldLink.Id

	err := tracker.TrackActivity[dao.IssueLink, dao.IssueActivity](s.tracker, actField.EntityUpdatedActivity, newMap, oldMap, oldLink, &user)
	if err != nil {
		errStack.GetError(c, err)
	}

	return c.JSON(http.StatusOK, oldLink.ToLightDTO())
}

// deleteIssueLink godoc
// @id deleteIssueLink
// @Summary Задачи (ссылки): удаление ссылки в задаче
// @Description Удаляет указанную ссылку, прикрепленную к задаче
// @Tags Issues
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param issueIdOrSeq path string true "Идентификатор или последовательный номер задачи"
// @Param linkId path string true "ID ссылки"
// @Success 200 "Ссылка успешно удалена"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/issues/{issueIdOrSeq}/issue-links/{linkId} [delete]
func (s *Services) deleteIssueLink(c echo.Context) error {
	user := *c.(IssueContext).User
	project := c.(IssueContext).Project
	issueId := c.(IssueContext).Issue.ID
	linkId := c.Param("linkId")

	var link dao.IssueLink

	if err := s.db.Where("project_id = ?", project.ID).
		Where("issue_id = ?", issueId).
		Where("issue_links.id = ?", linkId).First(&link).Error; err != nil {
		return EError(c, err)
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		err := tracker.TrackActivity[dao.IssueLink, dao.IssueActivity](s.tracker, actField.EntityDeleteActivity, nil, nil, link, &user)
		if err != nil {
			errStack.GetError(c, err)
			return err
		}

		return s.db.Delete(&link).Error
	}); err != nil {
		return EError(c, err)
	}

	return c.NoContent(http.StatusOK)
}

// ############# Issue history methods ###################

// getIssueHistoryList godoc
// @id getIssueHistoryList
// @Summary Задачи: получение истории задачи
// @Description Возвращает историю изменений и комментариев для задачи, отсортированных по времени создания
// @Tags Issues
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param issueIdOrSeq path string true "Идентификатор или последовательный номер задачи"
// @Success 200 {array} interface{} "История изменений и комментариев задачи"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/issues/{issueIdOrSeq}/history [get]
func (s *Services) getIssueHistoryList(c echo.Context) error {
	project := c.(IssueContext).Project
	issueId := c.(IssueContext).Issue.ID

	var issueActivities []dao.EntityActivity
	if err := s.db.Preload(clause.Associations).
		Where("issue_id = ?", issueId).
		Where("project_id = ?", project.ID).
		Where("field != ?", "comment").
		Order("created_at DESC").Find(&issueActivities).Error; err != nil {
		return EError(c, err)
	}

	var issueComments []dao.IssueComment
	if err := s.db.Where("issue_id = ?", issueId).
		Where("project_id = ?", project.ID).
		Order("created_at DESC").Preload(clause.Associations).Find(&issueComments).Error; err != nil {
		return EError(c, err)
	}

	result := make([]interface{}, 0)
	for _, activity := range issueActivities {
		result = append(result, *activity.ToFullDTO())
	}
	for _, comment := range issueComments {
		result = append(result, *comment.ToDTO())
	}

	sort.Slice(result, func(i, j int) bool {
		var iTime, jTime time.Time
		if c, ok := result[i].(dto.EntityActivityFull); ok {
			iTime = c.CreatedAt
		} else {
			iTime = result[i].(dto.IssueComment).CreatedAt
		}

		if c, ok := result[j].(dto.EntityActivityFull); ok {
			jTime = c.CreatedAt
		} else {
			jTime = result[j].(dto.IssueComment).CreatedAt
		}

		return iTime.After(jTime)
	})

	return c.JSON(http.StatusOK, result)
}

// ############# Issue comments methods ###################

// getIssueCommentList godoc
// @id getIssueCommentList
// @Summary Задачи (комментарии): получение списка комментариев к задаче
// @Description Возвращает список комментариев к задаче с возможностью пагинации
// @Tags Issues
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param issueIdOrSeq path string true "Идентификатор или последовательный номер задачи"
// @Param offset query int false "Смещение для пагинации" default(0)
// @Param limit query int false "Лимит записей" default(100)
// @Success 200 {object} dao.PaginationResponse{result=[]dto.IssueComment} "Список комментариев с пагинацией и количеством реакций"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/issues/{issueIdOrSeq}/comments [get]
func (s *Services) getIssueCommentList(c echo.Context) error {
	project := c.(IssueContext).Project
	issueId := c.(IssueContext).Issue.ID

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
		Where("issue_comments.project_id = ?", project.ID).
		Where("issue_comments.issue_id = ?", issueId).
		Order("issue_comments.created_at DESC")

	var comments []dao.IssueComment
	resp, err := dao.PaginationRequest(
		offset,
		limit,
		query,
		&comments,
	)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return EErrorDefined(c, apierrors.ErrIssueNotFound)
		}
		return EError(c, err)
	}

	resp.Result = utils.SliceToSlice(resp.Result.(*[]dao.IssueComment), func(ic *dao.IssueComment) *dto.IssueComment { return ic.ToDTO() })

	//TODO удалить после полного перехода на gen-ts-api
	commentsResp := struct {
		dao.PaginationResponse
		Comments any `json:"comments"`
	}{
		resp,
		resp.Result,
	}

	return c.JSON(http.StatusOK, commentsResp)
}

// createIssueComment godoc
// @Id createIssueComment
// @Summary Задачи (комментарии): добавление комментария к задаче
// @Description Добавляет новый комментарий к задаче с возможностью добавления вложений
// @Tags Issues
// @Security ApiKeyAuth
// @Accept multipart/form-data
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param issueIdOrSeq path string true "Идентификатор или последовательный номер задачи"
// @Param comment formData string true "Текст комментария в формате JSON"
// @Param files formData file false "Вложения для комментария"
// @Success 201 {object} dto.IssueComment "Созданный комментарий"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 429 {object} apierrors.DefinedError "Слишком часто создаются комментарии"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/issues/{issueIdOrSeq}/comments [post]
func (s *Services) createIssueComment(c echo.Context) error {
	user := *c.(IssueContext).User
	project := c.(IssueContext).Project
	issue := c.(IssueContext).Issue
	var lastCommentTime time.Time
	if err := s.db.Select("created_at").
		Where("workspace_id = ?", issue.WorkspaceId).
		Where("actor_id = ?", user.ID).
		Order("created_at desc").
		Model(&dao.IssueComment{}).
		First(&lastCommentTime).Error; err != nil && err != gorm.ErrRecordNotFound {
		return EError(c, err)
	}
	if time.Since(lastCommentTime) <= commentsCooldown {
		return EErrorDefined(c, apierrors.ErrTooManyComments)
	}

	form, _ := c.MultipartForm()

	var comment dao.IssueComment

	// If comment without attachments
	if form == nil {
		if err := c.Bind(&comment); err != nil {
			return EError(c, err)
		}
	} else {
		// else get comment data from "comment" value
		if c.FormValue("comment") == "" {
			//TODO: defined error
			return EError(c, nil)
		}
		if err := json.Unmarshal([]byte(c.FormValue("comment")), &comment); err != nil {
			return EError(c, err)
		}

		if !limiter.Limiter.CanAddAttachment(project.WorkspaceId) {
			return EErrorDefined(c, apierrors.ErrAssetsLimitExceed)
		}
	}

	if comment.CommentHtml.StripTags() == "" && comment.CommentStripped == "" {
		return EErrorDefined(c, apierrors.ErrIssueCommentEmpty)
	}

	userID := uuid.NullUUID{UUID: user.ID, Valid: true}
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		comment.Id = dao.GenUUID()
		comment.ProjectId = project.ID
		comment.Project = &project
		comment.IssueId = issue.ID
		comment.WorkspaceId = project.WorkspaceId
		comment.Workspace = issue.Workspace
		comment.ActorId = userID
		comment.Issue = &issue
		comment.CommentStripped = types.RemoveInvisibleChars(comment.CommentStripped)

		if err := tx.Omit(clause.Associations).Create(&comment).Error; err != nil {
			return err
		}

		issue.UpdatedAt = time.Now()
		if err := tx.Select("updated_at").Updates(&issue).Error; err != nil {
			return err
		}

		if form != nil {
			// Save issue attachments to
			for _, f := range form.File["files"] {
				fileAsset := dao.FileAsset{
					Id:          dao.GenUUID(),
					CreatedAt:   time.Now(),
					CreatedById: userID,
					Name:        f.Filename,
					FileSize:    int(f.Size),
					WorkspaceId: uuid.NullUUID{UUID: issue.WorkspaceId, Valid: true},
					CommentId:   uuid.NullUUID{UUID: comment.Id, Valid: true},
				}

				if err := s.uploadAssetForm(tx, f, &fileAsset,
					filestorage.Metadata{
						WorkspaceId: issue.WorkspaceId.String(),
						ProjectId:   issue.ProjectId.String(),
						IssueId:     issue.ID.String(),
					}); err != nil {
					return err
				}

				comment.Attachments = append(comment.Attachments, fileAsset)
			}
		}

		var authorOriginalComment *dao.User
		var replyNotMember bool
		if comment.ReplyToCommentId.Valid {
			if err := tx.Where(
				"id = (?)", tx.
					Select("actor_id").
					Model(&dao.IssueComment{}).
					Where("id = ?", comment.ReplyToCommentId.UUID)).
				First(&authorOriginalComment).Error; err != nil {
				return err
			}
			authorId := authorOriginalComment.ID
			if authorOriginalComment.ID != issue.CreatedById {
				for _, id := range issue.AssigneeIDs {
					if id == authorId {
						replyNotMember = true
						break
					}
				}
				for _, id := range issue.WatcherIDs {
					if id == authorId {
						replyNotMember = true
						break
					}
				}
			} else {
				replyNotMember = true
			}
			if !replyNotMember {
				if notify, countNotify, err := notifications.CreateUserNotificationAddComment(tx, authorOriginalComment.ID, comment); err == nil {
					s.notificationsService.Ws.Send(authorOriginalComment.ID, notify.ID, comment, countNotify)
				}
			}
		}

		users, err := dao.GetMentionedUsers(tx, comment.CommentHtml)
		if err != nil {
			return err
		}
		comment.Actor = &user
		for _, u := range users {
			if authorOriginalComment != nil && !replyNotMember {
				if u.ID == authorOriginalComment.ID {
					continue
				}
			}

			if notify, countNotify, err := notifications.CreateUserNotificationAddComment(tx, u.ID, comment); err == nil {
				s.notificationsService.Ws.Send(u.ID, notify.ID, notifications.Mention{IssueComment: comment}, countNotify)
			}
		}

		return nil
	}); err != nil {
		return EError(c, err)
	}

	//if err := s.tracker.TrackActivity(tracker.COMMENT_CREATED_ACTIVITY, StructToJSONMap(comment), nil, issue.ID.String(), tracker.ENTITY_TYPE_ISSUE, &project, user); err != nil {
	//	return EError(c, err)
	//}

	err := tracker.TrackActivity[dao.IssueComment, dao.IssueActivity](s.tracker, actField.EntityCreateActivity, nil, nil, comment, &user)
	if err != nil {
		errStack.GetError(c, err)
	}

	return c.JSON(http.StatusCreated, comment.ToDTO())
}

// getIssueComment godoc
// @id getIssueComment
// @Summary Задачи (комментарии): получение комментария к задаче
// @Description Получает данные комментария к задаче
// @Tags Issues
// @Security ApiKeyAuth
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param issueIdOrSeq path string true "Идентификатор или последовательный номер задачи"
// @Param commentId path string true "ID комментария"
// @Success 200 {object} dto.IssueComment "Комментарий"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/issues/{issueIdOrSeq}/comments/{commentId} [get]
func (s *Services) getIssueComment(c echo.Context) error {
	project := c.(IssueContext).Project
	issueId := c.(IssueContext).Issue.ID
	commentId := c.Param("commentId")

	query := s.db.
		Joins("Actor").
		Joins("OriginalComment").
		Joins("OriginalComment.Actor").
		Preload("Reactions").
		Where("issue_comments.project_id = ?", project.ID).
		Where("issue_comments.issue_id = ?", issueId).
		Where("issue_comments.id = ? or issue_comments.original_id = ?", commentId, commentId).
		Order("issue_comments.created_at DESC")

	var comment dao.IssueComment
	if err := query.First(&comment).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return EErrorDefined(c, apierrors.ErrIssueCommentNotFound)
		}
		return EError(c, err)
	}

	return c.JSON(http.StatusOK, comment.ToDTO())
}

// updateIssueComment godoc
// @id updateIssueComment
// @Summary Задачи (комментарии): изменение комментария к задаче
// @Description Изменяет текст и вложения для указанного комментария задачи
// @Tags Issues
// @Security ApiKeyAuth
// @Accept multipart/form-data
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param issueIdOrSeq path string true "Идентификатор или последовательный номер задачи"
// @Param commentId path string true "ID комментария"
// @Param comment formData string true "Обновленный текст комментария в формате JSON"
// @Param files formData file false "Новые вложения для комментария"
// @Success 200 {object} dto.IssueComment "Обновленный комментарий"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/issues/{issueIdOrSeq}/comments/{commentId} [patch]
func (s *Services) updateIssueComment(c echo.Context) error {
	user := *c.(IssueContext).User
	issue := c.(IssueContext).Issue
	commentId := c.Param("commentId")

	var comment dao.IssueComment
	var commentOld dao.IssueComment
	if err := s.db.
		Where("id = ?", commentId).Preload(clause.Associations).Preload("Issue.Workspace").Find(&commentOld).Error; err != nil {
		return EError(c, err)
	}

	if !commentOld.ActorId.Valid || commentOld.ActorId.UUID != user.ID {
		return EErrorDefined(c, apierrors.ErrCommentEditForbidden)
	}

	oldMap := StructToJSONMap(commentOld)

	form, _ := c.MultipartForm()

	// If comment without attachments
	if form == nil {
		if err := c.Bind(&comment); err != nil {
			return EError(c, err)
		}
	} else {
		// else get comment data from "comment" value
		if c.FormValue("comment") == "" {
			//TODO: defined error
			return EError(c, nil)
		}
		if err := json.Unmarshal([]byte(c.FormValue("comment")), &comment); err != nil {
			return EError(c, err)
		}

		if !limiter.Limiter.CanAddAttachment(issue.WorkspaceId) {
			return EErrorDefined(c, apierrors.ErrAssetsLimitExceed)
		}
	}

	if commentOld.CommentHtml.Body == comment.CommentHtml.Body {
		return c.JSON(http.StatusOK, commentOld)
	}

	if comment.CommentHtml.StripTags() == "" && comment.CommentStripped == "" {
		return EErrorDefined(c, apierrors.ErrIssueCommentEmpty)
	}

	userID := uuid.NullUUID{UUID: user.ID, Valid: true}
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		commentOld.Attachments = comment.Attachments
		commentOld.CommentHtml = comment.CommentHtml
		commentOld.CommentStripped = comment.CommentStripped
		commentOld.ReplyToCommentId = comment.ReplyToCommentId
		commentOld.UpdatedById = userID

		if err := tx.Omit(clause.Associations).Save(&commentOld).Error; err != nil {
			return err
		}

		if form != nil {
			// Save issue attachments to
			for _, f := range form.File["files"] {
				fileAsset := dao.FileAsset{
					Id:          dao.GenUUID(),
					CreatedAt:   time.Now(),
					CreatedById: userID,
					Name:        f.Filename,
					FileSize:    int(f.Size),
					WorkspaceId: uuid.NullUUID{UUID: issue.WorkspaceId, Valid: true},
					CommentId:   uuid.NullUUID{UUID: commentOld.Id, Valid: true},
				}

				if err := s.uploadAssetForm(tx, f, &fileAsset,
					filestorage.Metadata{
						WorkspaceId: issue.WorkspaceId.String(),
						ProjectId:   issue.ProjectId.String(),
						IssueId:     issue.ID.String(),
					}); err != nil {
					return err
				}

				commentOld.Attachments = append(commentOld.Attachments, fileAsset)
			}
		}

		var authorOriginalComment *dao.User
		var replyNotMember bool
		if comment.ReplyToCommentId.Valid {
			if err := tx.Where(
				"id = (?)", tx.
					Select("actor_id").
					Model(&dao.IssueComment{}).
					Where("id = ?", comment.ReplyToCommentId.UUID)).
				First(&authorOriginalComment).Error; err != nil {
				return err
			}
			authorId := authorOriginalComment.ID
			if authorOriginalComment.ID != issue.CreatedById {
				for _, id := range issue.AssigneeIDs {
					if id == authorId {
						replyNotMember = true
						break
					}
				}
				for _, id := range issue.WatcherIDs {
					if id == authorId {
						replyNotMember = true
						break
					}
				}
			} else {
				replyNotMember = true
			}
			if !replyNotMember {
				commentUUID := uuid.Must(uuid.FromString(commentId))
				comment.Id = commentUUID
				comment.IssueId = issue.ID
				comment.WorkspaceId = issue.WorkspaceId
				comment.Issue = &issue
				if notify, countNotify, err := notifications.CreateUserNotificationAddComment(tx, authorOriginalComment.ID, comment); err == nil {
					s.notificationsService.Ws.Send(authorOriginalComment.ID, notify.ID, comment, countNotify)
				}
			}
		}

		users, err := dao.GetMentionedUsers(tx, commentOld.CommentHtml)
		if err != nil {
			return err
		}
		commentOld.Actor = &user
		for _, u := range users {
			if authorOriginalComment != nil && !replyNotMember {
				if u.ID == authorOriginalComment.ID {
					continue
				}
			}
			if notify, countNotify, err := notifications.CreateUserNotificationAddComment(tx, u.ID, commentOld); err == nil {
				s.notificationsService.Ws.Send(u.ID, notify.ID, commentOld, countNotify)
			}
		}

		return nil
	}); err != nil {
		return EError(c, err)
	}

	newMap := StructToJSONMap(commentOld)
	newMap["updateScopeId"] = commentOld.Id
	newMap["field_log"] = actField.Comment.Field

	oldMap["updateScope"] = "comment"
	oldMap["updateScopeId"] = commentOld.Id
	//if err := s.tracker.TrackActivity(tracker.COMMENT_UPDATED_ACTIVITY, newMap, oldMap, issue.ID.String(), tracker.ENTITY_TYPE_ISSUE, commentOld.Project, user); err != nil {
	//	return EError(c, err)
	//}

	err := tracker.TrackActivity[dao.IssueComment, dao.IssueActivity](s.tracker, actField.EntityUpdatedActivity, newMap, oldMap, commentOld, &user)
	if err != nil {
		errStack.GetError(c, err)
	}

	return c.JSON(http.StatusOK, commentOld.ToDTO())
}

// deleteIssueComment godoc
// @id deleteIssueComment
// @Summary Задачи(комментарии): удаление комментария к задаче
// @Description Удаляет указанный комментарий из задачи
// @Tags Issues
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param issueIdOrSeq path string true "Идентификатор или последовательный номер задачи"
// @Param commentId path string true "ID комментария"
// @Success 204 "Комментарий успешно удален"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/issues/{issueIdOrSeq}/comments/{commentId} [delete]
func (s *Services) deleteIssueComment(c echo.Context) error {
	user := *c.(IssueContext).User
	project := c.(IssueContext).Project
	projectMember := c.(IssueContext).ProjectMember
	issueId := c.(IssueContext).Issue.ID
	commentId := c.Param("commentId")

	var comment dao.IssueComment
	if err := s.db.Where("project_id = ?", project.ID).
		Where("issue_id = ?", issueId).
		Where("id = ?", commentId).
		Preload("Attachments").
		First(&comment).Error; err != nil {
		return EError(c, err)
	}

	if projectMember.Role != types.AdminRole && (!comment.ActorId.Valid || comment.ActorId.UUID != user.ID) {
		return EErrorDefined(c, apierrors.ErrCommentEditForbidden)
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		err := tracker.TrackActivity[dao.IssueComment, dao.IssueActivity](s.tracker, actField.EntityDeleteActivity, nil, nil, comment, &user)
		if err != nil {
			errStack.GetError(c, err)
			return err
		}

		return s.db.Delete(&comment).Error
	}); err != nil {
		return EError(c, err)
	}

	//if err := s.tracker.TrackActivity(tracker.COMMENT_DELETED_ACTIVITY, nil, nil, issueId, tracker.ENTITY_TYPE_ISSUE, &project, user); err != nil {
	//	return EError(c, err)
	//}
	return c.NoContent(http.StatusOK)
}

// ############# Methods for comment (issue) reactions ###################

// addCommentReaction godoc
// @id addCommentReaction
// @Summary Задачи (комментарии): добавление реакции к комментарию
// @Description Добавляет реакцию пользователя к комментарию задачи
// @Tags Issues
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param issueIdOrSeq path string true "Идентификатор или последовательный номер задачи"
// @Param commentId path string true "ID комментария"
// @Param data body map[string]string true "Реакция (пример: 👍, 👎, ❤️)"
// @Success 201 {object} dto.CommentReaction "Созданная реакция"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса или реакция"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/issues/{issueIdOrSeq}/comments/{commentId}/reactions [post]
func (s *Services) addCommentReaction(c echo.Context) error {
	user := *c.(IssueContext).User
	commentId := c.Param("commentId")

	var reactionRequest ReactionRequest

	if err := c.Bind(&reactionRequest); err != nil {
		return EError(c, err)
	}

	if !validReactions[reactionRequest.Reaction] {
		return EErrorDefined(c, apierrors.ErrInvalidReaction)
	}

	// Проверяем, есть ли уже такая реакция от пользователя
	var existingReaction dao.CommentReaction
	err := s.db.Where("user_id = ? AND comment_id = ? AND reaction = ?", user.ID, commentId, reactionRequest.Reaction).First(&existingReaction).Error
	if err == nil {
		return c.JSON(http.StatusOK, existingReaction)
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return EError(c, err)
	}

	// Создаем новую реакцию
	commentUUID := uuid.Must(uuid.FromString(commentId))
	reaction := dao.CommentReaction{
		Id:        dao.GenUUID(),
		CreatedAt: time.Now(),
		UserId:    user.ID,
		CommentId: commentUUID,
		Reaction:  reactionRequest.Reaction,
	}

	if err := s.db.Create(&reaction).Error; err != nil {
		return EError(c, err)
	}

	return c.JSON(http.StatusCreated, reaction.ToDTO())
}

// removeCommentReaction godoc
// @id removeCommentReaction
// @Summary Задачи (комментарии): удаление реакции с комментария
// @Description Удаляет указанную реакцию пользователя с комментария задачи
// @Tags Issues
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param issueIdOrSeq path string true "Идентификатор или последовательный номер задачи"
// @Param commentId path string true "ID комментария"
// @Param reaction path string true "Реакция для удаления (пример: 👍, 👎, ❤️)"
// @Success 204 "Реакция успешно удалена"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/issues/{issueIdOrSeq}/comments/{commentId}/reactions/{reaction} [delete]
func (s *Services) removeCommentReaction(c echo.Context) error {
	user := *c.(IssueContext).User
	commentId := c.Param("commentId")
	reactionStr := strings.TrimSuffix(c.Param("reaction"), "/")

	if err := s.db.Where("user_id = ? AND comment_id = ? AND reaction = ?",
		user.ID, commentId, reactionStr).Delete(&dao.CommentReaction{}).Error; err != nil {
		return EError(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}

// ############# Activities methods ###################

// getIssueActivityList godoc
// @id getIssueActivityList
// @Summary Задачи: получение активности по задаче
// @Description Возвращает активность по задаче без учета комментариев, с возможностью фильтрации по полю
// @Tags Issues
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param issueIdOrSeq path string true "Идентификатор или последовательный номер задачи"
// @Param offset query int false "Смещение для пагинации" default(0)
// @Param limit query int false "Лимит записей" default(100)
// @Param field query string false "Поле активности для фильтрации (например: state)"
// @Success 200 {object} dao.PaginationResponse{result=[]dto.EntityActivityFull} "Список активностей с пагинацией"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/issues/{issueIdOrSeq}/activities [get]
func (s *Services) getIssueActivityList(c echo.Context) error {
	projectId := c.(IssueContext).Project.ID
	issueId := c.(IssueContext).Issue.ID

	offset := 0
	limit := 100
	field := ""

	if err := echo.QueryParamsBinder(c).
		Int("offset", &offset).
		Int("limit", &limit).
		String("field", &field).
		BindError(); err != nil {
		return EError(c, err)
	}

	var issue dao.IssueActivity
	issue.UnionCustomFields = "'issue' AS entity_type"
	unionTable := dao.BuildUnionSubquery(s.db, "ia", dao.FullActivity{}, issue)

	query := unionTable.
		Joins("Issue").
		Joins("Actor").
		Joins("Project").
		Joins("Workspace").
		Where("ia.project_id = ?", projectId).
		Where("ia.issue_id = ?", issueId).
		Order("ia.created_at DESC")

	if field != "" {
		query = query.Where("ia.field = ?", actField.Status.Field.String())
		if field == "state" {
			query = query.Select("ia.*, round(extract('epoch' from ia.created_at - (LAG(ia.created_at, 1, \"Issue\".created_at) over (order by ia.created_at))) * 1000) as state_lag")

		} else {
			query = query.Select("ia.*")
		}
	} else {
		query = query.Where("ia.field <> ?", actField.Status.Field.String())
	}

	type fullActivityWithLag struct {
		dao.FullActivity
		StateLag int `json:"state_lag,omitempty" gorm:"state_lag"`
	}

	toDto := func(fa *fullActivityWithLag) *dto.EntityActivityFull {
		if fa == nil {
			return nil
		}

		res := fa.ToDTO()
		res.StateLag = fa.StateLag
		return res
	}

	var activities []fullActivityWithLag
	res, err := dao.PaginationRequest(offset, limit, query, &activities)
	if err != nil {
		return EError(c, err)
	}

	res.Result = utils.SliceToSlice(res.Result.(*[]fullActivityWithLag), func(ea *fullActivityWithLag) dto.EntityActivityFull { return *toDto(ea) })

	//res.
	//return c.JSON(http.StatusOK, res)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"offset":     res.Offset,
		"limit":      res.Limit,
		"count":      res.Count,
		"activities": res.Result,
	})
}

//func findActivities[T any](query *gorm.DB, offset, limit int) ([]T, error) {
//	var activities []T
//	if err := query.Offset(offset).Limit(limit).Find(&activities).Error; err != nil {
//		return nil, err
//	}
//	return activities, nil
//}

// ############# Issue attachments methods ###################

// getIssueAttachmentList godoc
// @id getIssueAttachmentList
// @Summary Задачи (вложения): получение вложений задачи
// @Description Возвращает список всех вложений, прикрепленных к задаче
// @Tags Issues
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param issueIdOrSeq path string true "Идентификатор или последовательный номер задачи"
// @Success 200 {array} dao.PaginationResponse{result=[]dto.Attachment}  "Список вложений"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/issues/{issueIdOrSeq}/issue-attachments [get]
func (s *Services) getIssueAttachmentList(c echo.Context) error {
	project := c.(IssueContext).Project
	issueId := c.(IssueContext).Issue.ID

	var attachments []dao.IssueAttachment
	if err := s.db.
		Joins("Asset").
		Where("issue_attachments.project_id = ?", project.ID).
		Where("issue_attachments.issue_id = ?", issueId).
		Order("issue_attachments.created_at").
		Find(&attachments).Error; err != nil {
		return EError(c, err)
	}
	return c.JSON(
		http.StatusOK,
		utils.SliceToSlice(&attachments, func(ia *dao.IssueAttachment) dto.Attachment { return *ia.ToLightDTO() }),
	)
}

// createIssueAttachments godoc
// @id createIssueAttachments
// @Summary Задачи (вложения): загрузка вложения в задачу
// @Description Загружает новое вложение и прикрепляет его к задаче
// @Tags Issues
// @Security ApiKeyAuth
// @Accept multipart/form-data
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param issueIdOrSeq path string true "Идентификатор или последовательный номер задачи"
// @Param asset formData file true "Файл для загрузки"
// @Success 201 {object} dto.Attachment "Созданное вложение"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/issues/{issueIdOrSeq}/issue-attachments [post]
func (s *Services) createIssueAttachments(c echo.Context) error {
	user := *c.(IssueContext).User
	project := c.(IssueContext).Project
	issue := c.(IssueContext).Issue

	if !limiter.Limiter.CanAddAttachment(project.WorkspaceId) {
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

	assetName := dao.GenUUID()

	attributes := make(map[string]interface{})
	attributes["name"] = fileName
	attributes["size"] = asset.Size

	if err := s.storage.SaveReader(
		assetSrc,
		asset.Size,
		assetName,
		asset.Header.Get("Content-Type"),
		&filestorage.Metadata{
			WorkspaceId: issue.WorkspaceId.String(),
			ProjectId:   issue.ProjectId.String(),
			IssueId:     issue.ID.String(),
		},
	); err != nil {
		return EError(c, err)
	}

	issueAttachment := dao.IssueAttachment{
		Id:          dao.GenUUID(),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		CreatedById: uuid.NullUUID{UUID: user.ID, Valid: true},
		UpdatedById: uuid.NullUUID{UUID: user.ID, Valid: true},
		Attributes:  attributes,
		AssetId:     assetName,
		IssueId:     issue.ID,
		ProjectId:   project.ID,
		WorkspaceId: issue.WorkspaceId,
	}

	issueAttachment.Asset = &dao.FileAsset{
		Id:          assetName,
		CreatedById: uuid.NullUUID{UUID: user.ID, Valid: true},
		WorkspaceId: uuid.NullUUID{UUID: issue.WorkspaceId, Valid: true},
		Name:        asset.Filename,
		FileSize:    int(asset.Size),
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {

		if err := s.db.Create(&issueAttachment.Asset).Error; err != nil {
			return err
		}

		if err := s.db.Create(&issueAttachment).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		return EError(c, err)
	}

	//if err := s.tracker.TrackActivity(tracker.ATTACHMENT_CREATED_ACTIVITY, nil, map[string]interface{}{"id": issueAttachment.Id}, issue.ID.String(), tracker.ENTITY_TYPE_ISSUE, issue.Project, user); err != nil {
	//	return EError(c, err)
	//}
	err = tracker.TrackActivity[dao.IssueAttachment, dao.IssueActivity](s.tracker, actField.EntityCreateActivity, nil, nil, issueAttachment, &user)
	if err != nil {
		errStack.GetError(c, err)
	}

	return c.JSON(http.StatusCreated, issueAttachment.ToLightDTO())
}

// downloadIssueAttachments godoc
// @id downloadIssueAttachments
// @Summary Задачи (вложения): скачать все вложения в ZIP
// @Description Скачивает все вложения задачи в виде ZIP-архива
// @Tags Issues
// @Security ApiKeyAuth
// @Produce application/zip
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param issueIdOrSeq path string true "Идентификатор или последовательный номер задачи"
// @Param asset formData file true "Файл для загрузки"
// @Success 200 {file} binary "ZIP-файл со всеми вложениями"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/issues/{issueIdOrSeq}/issue-attachments/all/ [get]
func (s *Services) downloadIssueAttachments(c echo.Context) error {
	project := c.(IssueContext).Project
	issueId := c.(IssueContext).Issue.ID

	var attachments []dao.IssueAttachment
	if err := s.db.
		Joins("Asset").
		Where("issue_attachments.project_id = ?", project.ID).
		Where("issue_attachments.issue_id = ?", issueId).
		Order("issue_attachments.created_at").
		Find(&attachments).Error; err != nil {
		return EError(c, err)
	}

	var sumSize int
	for _, attachment := range attachments {
		sumSize += attachment.Asset.FileSize
	}

	if sumSize > apierrors.AttachmentsZipMaxSizeMB*1024*1024 {
		return EErrorDefined(c, apierrors.ErrTooHeavyAttachmentsZip)
	}

	c.Response().Header().Set(echo.HeaderContentType, "application/zip")
	c.Response().Header().Set(echo.HeaderContentDisposition, fmt.Sprintf("attachment; filename=\"%s_attachments.zip\"", c.(IssueContext).Issue.GetString()))
	c.Response().WriteHeader(http.StatusOK)

	w := zip.NewWriter(c.Response())

	for _, attachment := range attachments {
		attachR, err := s.storage.LoadReader(attachment.AssetId)
		if err != nil {
			attachR.Close()
			return EError(c, err)
		}
		attachW, err := w.CreateHeader(&zip.FileHeader{
			Name:     attachment.Asset.Name,
			Modified: attachment.CreatedAt,
			Comment:  "Created by AIPlan. https://plan.aisa.ru",
		})
		if err != nil {
			attachR.Close()
			return EError(c, err)
		}

		if _, err := io.Copy(attachW, attachR); err != nil {
			attachR.Close()
			return EError(c, err)
		}
		attachR.Close()
		c.Response().Flush()
	}
	w.Close()

	return nil
}

// deleteIssueAttachment godoc
// @id deleteIssueAttachment
// @Summary Задачи (вложения): удаление вложения из задачи
// @Description Удаляет указанное вложение, прикрепленное к задаче
// @Tags Issues
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param issueIdOrSeq path string true "Идентификатор или последовательный номер задачи"
// @Param attachmentId path string true "ID вложения"
// @Success 200 "Вложение успешно удалено"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/issues/{issueIdOrSeq}/issue-attachments/{attachmentId} [delete]
func (s *Services) deleteIssueAttachment(c echo.Context) error {
	user := *c.(IssueContext).User
	project := c.(IssueContext).Project
	issueId := c.(IssueContext).Issue.ID
	attachmentId := c.Param("attachmentId")

	var attachment dao.IssueAttachment
	if err := s.db.
		Preload("Project").
		Preload("Asset").
		Where("project_id = ?", project.ID).
		Where("issue_id = ?", issueId).
		Where("issue_attachments.id = ?", attachmentId).
		Find(&attachment).Error; err != nil {
		return EError(c, err)
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		err := tracker.TrackActivity[dao.IssueAttachment, dao.IssueActivity](s.tracker, actField.EntityDeleteActivity, nil, nil, attachment, &user)
		if err != nil {
			errStack.GetError(c, err)
			return err
		}

		return s.db.Omit(clause.Associations).Delete(&attachment).Error
	}); err != nil {
		return EError(c, err)
	}

	return c.NoContent(http.StatusOK)
}

// getIssueLinkedIssueList godoc
// @id getIssueLinkedIssueList
// @Summary Задачи: получение связанных задач
// @Description Возвращает список задач, связанных с указанной задачей
// @Tags Issues
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param issueIdOrSeq path string true "Идентификатор или последовательный номер задачи"
// @Success 200 {array} dto.Issue "Список связанных задач"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/issues/{issueIdOrSeq}/linked-issues [get]
func (s *Services) getIssueLinkedIssueList(c echo.Context) error {
	issue := c.(IssueContext).Issue

	var issues []dao.Issue
	if err := s.db.Where("project_id = ?", issue.ProjectId).
		Preload(clause.Associations).
		Where("id in (?)", issue.LinkedIssuesIDs).
		Order("sequence_id").Find(&issues).Error; err != nil {
		return EError(c, err)
	}
	return c.JSON(
		http.StatusOK,
		utils.SliceToSlice(&issues, func(il *dao.Issue) dto.Issue { return *il.ToDTO() }),
	)
}

// addIssueLinkedIssueList godoc
// @id addIssueLinkedIssueList
// @Summary Задачи: установка связанных задач
// @Description Устанавливает список связанных задач для указанной задачи
// @Tags Issues
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param issueIdOrSeq path string true "Идентификатор или последовательный номер задачи"
// @Param data body LinkedIssuesIds true "Идентификаторы связанных задач"
// @Success 200 {array} dto.IssueLight "Обновленный список связанных задач"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/issues/{issueIdOrSeq}/linked-issues [post]
func (s *Services) addIssueLinkedIssueList(c echo.Context) error {
	issue := c.(IssueContext).Issue
	//project := c.(IssueContext).Project
	user := c.(IssueContext).User

	var param LinkedIssuesIds
	if err := c.Bind(&param); err != nil {
		return EError(c, err)
	}

	oldIssue := StructToJSONMap(issue)

	var issues []dao.Issue
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		// Clean all links with this issue
		var linkedIssues []dao.LinkedIssues
		var oldIDs []interface{}
		if err := tx.Where("id1 = ? or id2 = ?", issue.ID, issue.ID).Find(&linkedIssues).Error; err != nil {
			return err
		}

		for _, v := range linkedIssues {
			if v.Id1 != issue.ID {
				oldIDs = append(oldIDs, v.Id1)
			}
			if v.Id2 != issue.ID {
				oldIDs = append(oldIDs, v.Id2)
			}
		}

		newIDs := make([]interface{}, len(param.IssueIDs))
		for i, v := range param.IssueIDs {
			newIDs[i] = v
		}

		if err := tx.Where("id1 = ? or id2 = ?", issue.ID, issue.ID).Delete(&dao.LinkedIssues{}).Error; err != nil {
			return err
		}

		for _, newId := range param.IssueIDs {
			if err := issue.AddLinkedIssue(tx, newId); err != nil {
				return err
			}
		}

		if err := issue.FetchLinkedIssues(tx); err != nil {
			return err
		}

		if err := tx.Where("id in (?)", issue.LinkedIssuesIDs).Find(&issues).Error; err != nil {
			return err
		}

		return nil
	}); err != nil {
		return EError(c, err)
	}

	newIssue := StructToJSONMap(issue)

	err := tracker.TrackActivity[dao.Issue, dao.IssueActivity](s.tracker, actField.EntityUpdatedActivity, newIssue, oldIssue, issue, user)
	if err != nil {
		errStack.GetError(c, err)
	}

	return c.JSON(
		http.StatusOK,
		utils.SliceToSlice(&issues, func(i *dao.Issue) dto.IssueLight { return *i.ToLightDTO() }),
	)
}

// getIssuePdf godoc
// @id getIssuePdf
// @Summary Задачи: Получение PDF-файла задачи
// @Description Возвращает PDF-файл в соответствии с заданными параметрами
// @Tags Issues
// @Security ApiKeyAuth
// @Accept json
// @Produce application/pdf
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param issueIdOrSeq path string true "Идентификатор или последовательный номер задачи"
// @Success 200 {file} binary "PDF-файл задачи"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/issues/{issueIdOrSeq}/pdf [get]
func (s *Services) getIssuePdf(c echo.Context) error {
	issue := c.(IssueContext).Issue

	c.Response().Header().Add("Content-Disposition", fmt.Sprintf("attachment; filename=%s.pdf", issue.String()))

	var buf bytes.Buffer
	if err := export.IssueToFPDF(&issue, cfg.WebURL, &buf); err != nil {
		return EError(c, err)
	}

	return c.Blob(http.StatusOK, "application/pdf", buf.Bytes())
}

// issueDescriptionLock godoc
// @id issueDescriptionLock
// @Summary Задачи: Блокировка описания задачи за пользователем
// @Description Блокирует описание задачи за вызвавшим пользователем, так что только он сможет сохранять описание
// @Tags Issues
// @Security ApiKeyAuth
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param issueIdOrSeq path string true "Идентификатор или последовательный номер задачи"
// @Success 200 {object} dto.IssueLockResponse "Описание успешно заблокировано"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 409 {object} dto.IssueLockResponse "Описание заблокировано другим пользователем"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/issues/{issueIdOrSeq}/description-lock [post]
func (s *Services) issueDescriptionLock(c echo.Context) error {
	user := c.(IssueContext).User
	issue := c.(IssueContext).Issue

	return s.db.Transaction(func(tx *gorm.DB) error {
		// Remove old locks for this issue
		if err := tx.Where("issue_id = ? and locked_until < NOW()", issue.ID).Delete(&dao.IssueDescriptionLock{}).Error; err != nil {
			return EError(c, err)
		}

		var lock dao.IssueDescriptionLock
		if err := tx.Where("issue_id = ?", issue.ID).Joins("User").First(&lock).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				// No lock
				lock := dao.IssueDescriptionLock{
					UserId:      user.ID,
					IssueId:     issue.ID,
					LockedUntil: time.Now().Add(descriptionLockTime),
				}

				if err := tx.Create(&lock).Error; err != nil {
					return EError(c, err)
				}

				return c.JSON(http.StatusOK, dto.IssueLockResponse{
					Locked:      true,
					LockedUntil: lock.LockedUntil,
				})
			}
			return EError(c, err)
		}
		// Lock exist
		return c.JSON(http.StatusConflict, dto.IssueLockResponse{
			Locked:      false,
			LockedBy:    lock.User.ToLightDTO(),
			LockedUntil: lock.LockedUntil,
		})
	})
}

// issueDescriptionUnlock godoc
// @id issueDescriptionUnlock
// @Summary Задачи: Разблокировка описания задачи
// @Description Разблокирует описание задачи
// @Tags Issues
// @Security ApiKeyAuth
// @Produce json
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param issueIdOrSeq path string true "Идентификатор или последовательный номер задачи"
// @Success 200 "Описание успешно разблокировано"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/issues/{issueIdOrSeq}/description-unlock [post]
func (s *Services) issueDescriptionUnlock(c echo.Context) error {
	user := c.(IssueContext).User
	issue := c.(IssueContext).Issue

	res := s.db.Where("issue_id = ? and user_id = ?", issue.ID, user.ID).Delete(&dao.IssueDescriptionLock{})
	if res.Error != nil {
		return EError(c, res.Error)
	}

	if res.RowsAffected < 1 {
		return EErrorDefined(c, apierrors.ErrIssueDescriptionNotLockedByUser)
	}

	return c.NoContent(http.StatusOK)
}

// issuePin godoc
// @id issuePin
// @Summary Задачи: Закрепление задачи
// @Tags Issues
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param issueIdOrSeq path string true "Идентификатор или последовательный номер задачи"
// @Success 200 "Задача успешно закреплена"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/issues/{issueIdOrSeq}/pin [post]
func (s *Services) issuePin(c echo.Context) error {
	issue := c.(IssueContext).Issue

	if err := s.db.Model(&dao.Issue{}).Where("id = ?", issue.ID).UpdateColumn("pinned", true).Error; err != nil {
		return EError(c, err)
	}
	return c.NoContent(http.StatusOK)
}

// issueUnpin godoc
// @id issueUnpin
// @Summary Задачи: Открепление задачи
// @Tags Issues
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param issueIdOrSeq path string true "Идентификатор или последовательный номер задачи"
// @Success 200 "Задача успешно откреплена"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/issues/{issueIdOrSeq}/pin [post]
func (s *Services) issueUnpin(c echo.Context) error {
	issue := c.(IssueContext).Issue

	if err := s.db.Model(&dao.Issue{}).Where("id = ?", issue.ID).UpdateColumn("pinned", false).Error; err != nil {
		return EError(c, err)
	}
	return c.NoContent(http.StatusOK)
}

// ############# Issue Properties methods ###################

// getIssueProperties godoc
// @id getIssueProperties
// @Summary Свойства задачи: получение всех полей
// @Description Возвращает все шаблоны полей проекта с их значениями для задачи.
// Если значение не установлено, возвращается дефолтное значение для типа поля.
// @Tags IssueProperties
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param issueIdOrSeq path string true "Идентификатор или последовательный номер задачи"
// @Success 200 {array} dto.IssueProperty "Список свойств задачи"
// @Failure 403 {object} apierrors.DefinedError "Нет доступа к задаче"
// @Failure 404 {object} apierrors.DefinedError "Задача не найдена"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/issues/{issueIdOrSeq}/properties/ [get]
func (s *Services) getIssueProperties(c echo.Context) error {
	issue := c.(IssueContext).Issue
	projectMember := c.(IssueContext).ProjectMember

	// Получаем все шаблоны полей проекта
	var templates []dao.ProjectPropertyTemplate
	if err := s.db.Where("project_id = ?", issue.ProjectId).
		Where("only_admin = ? OR only_admin = ?", false, projectMember.Role == types.AdminRole).
		Order("sort_order, created_at").
		Find(&templates).Error; err != nil {
		return EError(c, err)
	}

	// Получаем существующие значения для задачи
	var existingProps []dao.IssueProperty
	if err := s.db.Where("issue_id = ?", issue.ID).
		Find(&existingProps).Error; err != nil {
		return EError(c, err)
	}

	// Создаем map для быстрого поиска
	propsMap := make(map[uuid.UUID]dao.IssueProperty)
	for _, p := range existingProps {
		propsMap[p.TemplateId] = p
	}

	// Собираем результат: все шаблоны с значениями или дефолтами
	result := make([]dto.IssueProperty, 0, len(templates))
	for _, tmpl := range templates {
		// Пропускаем OnlyAdmin поля для не-админов
		if tmpl.OnlyAdmin && projectMember.Role < types.AdminRole {
			continue
		}

		prop := dto.IssueProperty{
			TemplateId:  tmpl.Id,
			IssueId:     issue.ID,
			ProjectId:   issue.ProjectId,
			WorkspaceId: issue.WorkspaceId,
			Name:        tmpl.Name,
			Type:        tmpl.Type,
			Value:       getDefaultPropertyValue(tmpl.Type),
		}

		// Добавляем options только для select полей
		if tmpl.Type == "select" {
			prop.Options = tmpl.Options
		}

		if existing, ok := propsMap[tmpl.Id]; ok {
			prop.Id = existing.Id
			prop.Value = parsePropertyValue(tmpl.Type, existing.Value)
		}

		result = append(result, prop)
	}

	return c.JSON(http.StatusOK, result)
}

// setIssueProperty godoc
// @id setIssueProperty
// @Summary Свойства задачи: установка значения
// @Description Устанавливает или обновляет значение кастомного поля для задачи.
// @Tags IssueProperties
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug рабочего пространства"
// @Param projectId path string true "ID проекта"
// @Param issueIdOrSeq path string true "Идентификатор или последовательный номер задачи"
// @Param templateId path string true "ID шаблона поля"
// @Param request body dto.SetIssuePropertyRequest true "Данные свойства"
// @Success 200 {object} dto.IssueProperty "Установленное свойство"
// @Success 201 {object} dto.IssueProperty "Созданное свойство"
// @Failure 400 {object} apierrors.DefinedError "Некорректные данные"
// @Failure 403 {object} apierrors.DefinedError "Нет прав на установку"
// @Failure 404 {object} apierrors.DefinedError "Задача или шаблон не найден"
// @Router /api/auth/workspaces/{workspaceSlug}/projects/{projectId}/issues/{issueIdOrSeq}/properties/{templateId}/ [post]
func (s *Services) setIssueProperty(c echo.Context) error {
	user := c.(IssueContext).User
	issue := c.(IssueContext).Issue
	projectMember := c.(IssueContext).ProjectMember

	templateId := c.Param("templateId")
	templateUUID, err := uuid.FromString(templateId)
	if err != nil {
		return EErrorDefined(c, apierrors.ErrPropertyTemplateNotFound)
	}

	var request dto.SetIssuePropertyRequest
	if err := c.Bind(&request); err != nil {
		return EError(c, err)
	}

	// Проверяем существование шаблона
	var template dao.ProjectPropertyTemplate
	if err := s.db.Where("id = ? AND project_id = ?", templateUUID, issue.ProjectId).First(&template).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return EErrorDefined(c, apierrors.ErrPropertyTemplateNotFound)
		}
		return EError(c, err)
	}

	// Проверяем права на OnlyAdmin поля
	if template.OnlyAdmin && projectMember.Role < types.AdminRole {
		return EErrorDefined(c, apierrors.ErrPropertyOnlyAdminCanSet)
	}

	// Валидируем значение через JSON Schema
	if err := validatePropertyValue(template, request.Value); err != nil {
		return EErrorDefined(c, apierrors.ErrPropertyValueValidationFailed)
	}

	// Сериализуем значение для хранения
	valueStr := serializePropertyValue(request.Value)

	// Проверяем существование значения
	var existingProp dao.IssueProperty
	err = s.db.Where("issue_id = ? AND template_id = ?", issue.ID, templateUUID).First(&existingProp).Error

	status := http.StatusOK
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Создаем новое значение
		existingProp = dao.IssueProperty{
			Id:          dao.GenUUID(),
			IssueId:     issue.ID,
			TemplateId:  templateUUID,
			ProjectId:   issue.ProjectId,
			WorkspaceId: issue.WorkspaceId,
			Value:       valueStr,
			CreatedById: uuid.NullUUID{UUID: user.ID, Valid: true},
			UpdatedById: uuid.NullUUID{UUID: user.ID, Valid: true},
		}

		if err := s.db.Create(&existingProp).Error; err != nil {
			return EError(c, err)
		}
		status = http.StatusCreated
	} else if err != nil {
		return EError(c, err)
	} else {
		// Обновляем существующее значение
		existingProp.Value = valueStr
		existingProp.UpdatedById = uuid.NullUUID{UUID: user.ID, Valid: true}

		if err := s.db.Save(&existingProp).Error; err != nil {
			return EError(c, err)
		}
	}

	// Загружаем шаблон для ответа
	existingProp.Template = &template

	return c.JSON(status, existingProp.ToDTO())
}

// getDefaultPropertyValue возвращает дефолтное значение для типа поля
func getDefaultPropertyValue(propType string) any {
	switch propType {
	case "string":
		return ""
	case "select":
		return nil
	case "boolean":
		return false
	default:
		return nil
	}
}

// parsePropertyValue парсит строковое значение в соответствии с типом
func parsePropertyValue(propType, value string) any {
	switch propType {
	case "boolean":
		return value == "true"
	case "select":
		if value == "" {
			return nil
		}
		return value
	default:
		return value
	}
}

// validatePropertyValue валидирует значение через JSON Schema
func validatePropertyValue(template dao.ProjectPropertyTemplate, value any) error {
	schema := types.GenValueSchema(template.Type, template.Options)

	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("schema.json", schema); err != nil {
		return err
	}

	sch, err := compiler.Compile("schema.json")
	if err != nil {
		return err
	}

	return sch.Validate(value)
}

// serializePropertyValue сериализует значение в строку для хранения в БД
func serializePropertyValue(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

// LinkedIssuesIds представляет собой структуру для передачи связанных задач
type LinkedIssuesIds struct {
	IssueIDs []uuid.UUID `json:"issue_ids"`
}

// SubIssuesIds представляет собой структуру для передачи идентификаторов подзадач
type SubIssuesIds struct {
	SubIssueIDs []uuid.UUID `json:"sub_issue_ids"`
}

type IssueLinkRequest struct {
	Url   string `json:"url"`
	Title string `json:"title"`
}

// formatUserName форматирует имя пользователя для CSV экспорта
func formatUserName(u dto.UserLight) string {
	name := strings.TrimSpace(u.FirstName + " " + u.LastName)
	if name == "" {
		return u.Email
	}
	return name
}

// formatTargetDate форматирует дату для CSV экспорта
func formatTargetDate(d *types.TargetDateTimeZ) string {
	if d == nil {
		return ""
	}
	return d.Time.Format(time.RFC3339)
}

// csvExportHeader возвращает заголовок CSV для экспорта задач
func csvExportHeader() []string {
	return []string{
		"ID",
		"Номер",
		"Название",
		"Приоритет",
		"Статус",
		"Дата начала",
		"Срок исполнения",
		"Дата завершения",
		"Дата создания",
		"Последнее изменение",
		"Автор",
		"Исполнители",
		"Наблюдатели",
		"Теги",
		"Проект",
		"Рабочее пространство",
		"Черновик",
		"Закреплено",
		"Подзадач",
		"Ссылок",
		"Вложений",
		"Связанных задач",
		"Комментариев",
		"Спринты",
	}
}

// issueToCSVRow преобразует задачу в строку CSV
func issueToCSVRow(issue *dto.IssueWithCount) []string {
	assignees := make([]string, 0, len(issue.Assignees))
	for _, a := range issue.Assignees {
		assignees = append(assignees, formatUserName(a))
	}

	watchers := make([]string, 0, len(issue.Watchers))
	for _, w := range issue.Watchers {
		watchers = append(watchers, formatUserName(w))
	}

	labels := make([]string, 0, len(issue.Labels))
	for _, l := range issue.Labels {
		labels = append(labels, l.Name)
	}

	sprints := make([]string, 0, len(issue.Sprints))
	for _, sp := range issue.Sprints {
		sprints = append(sprints, sp.Name)
	}

	stateName := ""
	if issue.State != nil {
		stateName = issue.State.Name
	}

	authorName := ""
	if issue.Author != nil {
		authorName = formatUserName(*issue.Author)
	}

	projectName := ""
	if issue.Project != nil {
		projectName = issue.Project.Name
	}

	workspaceName := ""
	if issue.Workspace != nil {
		workspaceName = issue.Workspace.Name
	}

	priority := ""
	if issue.Priority != nil {
		priority = *issue.Priority
	}

	return []string{
		issue.Id.String(),
		strconv.Itoa(issue.SequenceId),
		issue.Name,
		priority,
		stateName,
		formatTargetDate(issue.StartDate),
		formatTargetDate(issue.TargetDate),
		formatTargetDate(issue.CompletedAt),
		issue.CreatedAt.Format(time.RFC3339),
		issue.UpdatedAt.Format(time.RFC3339),
		authorName,
		strings.Join(assignees, ", "),
		strings.Join(watchers, ", "),
		strings.Join(labels, ", "),
		projectName,
		workspaceName,
		strconv.FormatBool(issue.Draft),
		strconv.FormatBool(issue.Pinned),
		strconv.Itoa(issue.SubIssuesCount),
		strconv.Itoa(issue.LinkCount),
		strconv.Itoa(issue.AttachmentCount),
		strconv.Itoa(issue.LinkedIssuesCount),
		strconv.Itoa(issue.CommentsCount),
		strings.Join(sprints, ", "),
	}
}

// getGroupFileName возвращает имя файла для группы в ZIP архиве
func getGroupFileName(entity any, index int) string {
	var name string
	switch e := entity.(type) {
	case dto.UserLight:
		name = formatUserName(e)
	case *dto.UserLight:
		if e != nil {
			name = formatUserName(*e)
		}
	case dto.StateLight:
		name = e.Name
	case *dto.StateLight:
		if e != nil {
			name = e.Name
		}
	case dto.LabelLight:
		name = e.Name
	case *dto.LabelLight:
		if e != nil {
			name = e.Name
		}
	case dto.ProjectLight:
		name = e.Name
	case *dto.ProjectLight:
		if e != nil {
			name = e.Name
		}
	}

	if name == "" {
		name = fmt.Sprintf("group_%d", index+1)
	}

	// Очищаем имя от недопустимых символов для имени файла
	name = strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' {
			return '_'
		}
		return r
	}, name)

	return name + ".csv"
}
