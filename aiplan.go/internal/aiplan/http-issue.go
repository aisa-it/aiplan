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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"sheff.online/aiplan/internal/aiplan/apierrors"
	errStack "sheff.online/aiplan/internal/aiplan/stack-error"
	"sheff.online/aiplan/internal/aiplan/utils"

	"sheff.online/aiplan/internal/aiplan/export"
	"sheff.online/aiplan/internal/aiplan/types"

	"sheff.online/aiplan/internal/aiplan/dto"

	"sheff.online/aiplan/internal/aiplan/notifications"

	"github.com/gofrs/uuid"
	"github.com/labstack/echo/v4"
	tusd "github.com/tus/tusd/v2/pkg/handler"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	tracker "sheff.online/aiplan/internal/aiplan/activity-tracker"
	"sheff.online/aiplan/internal/aiplan/dao"
	filestorage "sheff.online/aiplan/internal/aiplan/file-storage"
	"sheff.online/aiplan/internal/aiplan/rules"
)

const (
	commentsCooldown = time.Second * 5

	descriptionLockTime = time.Minute * 5
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

	g.Any("attachments/tus/*", s.storage.GetTUSHandler(cfg, "/api/auth/attachments/tus/", s.attachmentsUploadValidator, s.attachmentsPostUploadHook))
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

		issueAttachment := dao.IssueAttachment{
			Id:          dao.GenID(),
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
			CreatedById: &userId,
			UpdatedById: &userId,
			AssetId:     assetName,
			IssueId:     issueId,
			ProjectId:   issue.ProjectId,
			WorkspaceId: issue.WorkspaceId,
		}

		fa := dao.FileAsset{
			Id:          assetName,
			CreatedById: &userId,
			WorkspaceId: &issue.WorkspaceId,
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

		if err := tracker.TrackActivity[dao.IssueAttachment, dao.IssueActivity](s.tracker, tracker.ENTITY_CREATE_ACTIVITY, data, nil, issueAttachment, &user); err != nil {
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

		docAttachment := dao.DocAttachment{
			Id:          dao.GenID(),
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
			CreatedById: &userId,
			UpdatedById: &userId,
			AssetId:     assetName,
			DocId:       doc.ID.String(),
			WorkspaceId: doc.WorkspaceId,
		}

		fa := dao.FileAsset{
			Id:          assetName,
			CreatedById: &userId,
			WorkspaceId: &doc.WorkspaceId,
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
		if err := tracker.TrackActivity[dao.DocAttachment, dao.DocActivity](s.tracker, tracker.ENTITY_CREATE_ACTIVITY, data, nil, docAttachment, &user); err != nil {
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
			Joins("Workspace").
			Joins("State").
			Joins("Project").
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

var issueSortFields = []string{"id", "created_at", "updated_at", "name", "priority", "target_date", "sequence_id", "state", "labels", "sub_issues_count", "link_count", "attachment_count", "linked_issues_count", "assignees", "watchers", "author"}
var issueGroupFields = []string{"priority", "author", "state", "labels", "assignees", "watchers"}

// getIssueList godoc
// @id getIssueList
// @Summary Задачи: поиск задач
// @Description Выполняет поиск задач с использованием фильтров и сортировки
// @Tags Issues
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param show_sub_issues query bool false "Включать подзадачи" default(true)
// @Param order_by query string false "Поле для сортировки" default("sequence_id") enum(id, created_at, updated_at, name, priority, target_date, sequence_id, state, labels, sub_issues_count, link_count, attachment_count, linked_issues_count, assignees, watchers, author)
// @Param group_by query string false "Поле для группировки результатов" default("") enum(priority, author, state, labels, assignees, watchers)
// @Param offset query int false "Смещение для пагинации" default(-1)
// @Param limit query int false "Лимит записей" default(100)
// @Param desc query bool false "Сортировка по убыванию" default(true)
// @Param only_count query bool false "Вернуть только количество" default(false)
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
	if context, ok := c.(ProjectContext); ok {
		projectMember = context.ProjectMember
		user = *context.User
	}
	if context, ok := c.(AuthContext); ok {
		user = *context.User
		globalSearch = true
	}

	showSubIssues := true
	draft := true
	orderByParam := "sequence_id"
	groupByParam := ""
	onlyCount := false
	offset := -1
	limit := 100
	desc := true
	lightSearch := false

	if err := echo.QueryParamsBinder(c).
		Bool("show_sub_issues", &showSubIssues).
		Bool("draft", &draft).
		String("order_by", &orderByParam).
		String("group_by", &groupByParam).
		Int("offset", &offset).
		Int("limit", &limit).
		Bool("desc", &desc).
		Bool("only_count", &onlyCount).
		Bool("light", &lightSearch).
		BindError(); err != nil {
		return EError(c, err)
	}

	var filters types.IssuesListFilters
	if err := c.Bind(&filters); err != nil {
		return EError(c, err)
	}

	if limit > 100 {
		return EErrorDefined(c, apierrors.ErrLimitTooHigh)
	}

	// Validate grouped by
	if groupByParam != "" && !slices.Contains(issueGroupFields, groupByParam) {
		return EErrorDefined(c, apierrors.ErrUnsupportedGroup)
	}

	orderByParam = strings.TrimPrefix(orderByParam, "-")

	sortValid := false
	for _, f := range issueSortFields {
		if f == orderByParam {
			sortValid = true
		}
	}
	if !sortValid {
		return EErrorDefined(c, apierrors.ErrUnsupportedSortParam.WithFormattedMessage(orderByParam))
	}

	var query *gorm.DB
	if lightSearch {
		query = s.db.Preload("Author").Preload("State").Preload("Project").Preload("Workspace").Preload("Assignees").Preload("Watchers").Preload("Labels")
	} else {
		query = s.db.Preload(clause.Associations)
	}

	// Add membership info to project details on global search
	if globalSearch && !lightSearch {
		query = query.Set("userId", user.ID)
	}

	// Fill filters
	if !globalSearch {
		query = query.
			Where("issues.workspace_id = ?", projectMember.WorkspaceId).
			Where("issues.project_id = ?", projectMember.ProjectId)
	} else if !user.IsSuperuser {
		query = query.
			Where("issues.project_id in (?)", s.db.
				Select("project_id").
				Where("member_id = ?", user.ID).
				Model(&dao.ProjectMember{}),
			)
	}

	// Filters
	if groupByParam == "" {
		if len(filters.AuthorIds) > 0 {
			query = query.Where("issues.created_by_id in (?)", filters.AuthorIds)
		}

		if len(filters.AssigneeIds) > 0 {
			q := s.db.Where("issues.id in (?)",
				s.db.Select("issue_id").
					Where("assignee_id in (?)", filters.AssigneeIds).
					Model(&dao.IssueAssignee{}))
			if slices.Contains(filters.AssigneeIds, "") {
				q = q.Or("issues.id not in (?)", s.db.
					Select("issue_id").
					Model(&dao.IssueAssignee{}))
			}
			query = query.Where(q)
		}

		if len(filters.WatcherIds) > 0 {
			q := s.db.Where("issues.id in (?)",
				s.db.Select("issue_id").
					Where("watcher_id in (?)", filters.WatcherIds).
					Model(&dao.IssueWatcher{}))
			if slices.Contains(filters.WatcherIds, "") {
				q = q.Or("issues.id not in (?)", s.db.
					Select("issue_id").
					Model(&dao.IssueWatcher{}))
			}
			query = query.Where(q)
		}

		if len(filters.StateIds) > 0 {
			query = query.Where("issues.state_id in (?)", filters.StateIds)
		}

		if len(filters.Priorities) > 0 {
			hasNull := false
			var arr []any
			for _, p := range filters.Priorities {
				if p != "" {
					arr = append(arr, p)
				} else {
					hasNull = true
				}
			}
			if hasNull {
				query = query.Where("issues.priority in (?) or issues.priority is null", arr)
			} else {
				query = query.Where("issues.priority in (?)", arr)
			}
		}

		if len(filters.Labels) > 0 {
			q := s.db.Where("issues.id in (?)", s.db.
				Model(&dao.IssueLabel{}).
				Select("issue_id").
				Where("label_id in (?)", filters.Labels))
			if slices.Contains(filters.Labels, "") {
				q = q.Or("issues.id not in (?)", s.db.
					Select("issue_id").
					Model(&dao.IssueLabel{}))
			}
			query = query.Where(q)
		}

		if len(filters.WorkspaceIds) > 0 {
			query = query.Where("issues.workspace_id in (?)",
				s.db.Select("workspace_id").
					Model(&dao.WorkspaceMember{}).
					Where("member_id = ?", user.ID).
					Where("workspace_id in (?)", filters.WorkspaceIds))
		}

		if len(filters.WorkspaceSlugs) > 0 {
			query = query.Where("issues.workspace_id in (?)",
				s.db.Model(&dao.WorkspaceMember{}).
					Select("workspace_id").
					Where("member_id = ?", user.ID).
					Where("workspace_id in (?)", s.db.Model(&dao.Workspace{}).
						Select("id").
						Where("slug in (?)", filters.WorkspaceSlugs)))
		}

		if len(filters.ProjectIds) > 0 {
			query = query.Where("issues.project_id in (?)",
				s.db.Select("project_id").
					Model(&dao.WorkspaceMember{}).
					Where("member_id = ?", user.ID).
					Where("project_id in (?)", filters.ProjectIds))
		}

		// If workspace not specified, use all user workspaces
		if len(filters.WorkspaceIds) == 0 && len(filters.WorkspaceSlugs) == 0 && globalSearch && !user.IsSuperuser {
			query = query.Where("issues.workspace_id in (?)",
				s.db.Select("workspace_id").
					Model(&dao.WorkspaceMember{}).
					Where("member_id = ?", user.ID))
		}

		if filters.AssignedToMe {
			query = query.Where("issues.id in (?)", s.db.Select("issue_id").Model(&dao.IssueAssignee{}).Where("assignee_id = ?", user.ID))
		}

		if filters.WatchedByMe {
			query = query.Where("issues.id in (?)", s.db.Select("issue_id").Model(&dao.IssueWatcher{}).Where("watcher_id = ?", user.ID))
		}

		if filters.AuthoredByMe {
			query = query.Where("issues.created_by_id = ?", user.ID)
		}

		if filters.OnlyActive {
			query = query.Where("issues.state_id in (?)", s.db.Model(&dao.State{}).
				Select("id").
				Where("\"group\" <> ?", "cancelled").
				Where("\"group\" <> ?", "completed"))
		}

		if filters.SearchQuery != "" {
			query = query.Joins("join projects p on p.id = issues.project_id").Where(dao.Issue{}.FullTextSearch(s.db, filters.SearchQuery))
		}
	}

	// Ignore slave issues
	if !showSubIssues {
		query = query.Where("issues.parent_id is null")
	}

	// Ignore draft issues
	if !draft {
		query = query.Where("issues.draft = false or issues.draft is null ")
	}

	if onlyCount {
		var count int64
		if err := query.Model(&dao.Issue{}).Count(&count).Error; err != nil {
			return EError(c, err)
		}

		return c.JSON(http.StatusOK, map[string]any{
			"count": count,
		})
	}

	var selectExprs []string
	var selectInterface []any

	// Fetch counters fo full search
	if !lightSearch {
		selectExprs = []string{
			"issues.*",
			"count(*) over() as all_count",
			"(?) as sub_issues_count",
			"(?) as link_count",
			"(?) as attachment_count",
			"(?) as linked_issues_count",
		}
		selectInterface = []interface{}{
			s.db.Table("issues as \"child\"").Select("count(*)").Where("\"child\".parent_id = issues.id"),
			s.db.Select("count(*)").Where("issue_id = issues.id").Model(&dao.IssueLink{}),
			s.db.Select("count(*)").Where("issue_id = issues.id").Model(&dao.IssueAttachment{}),
			s.db.Select("count(*)").Where("id1 = issues.id OR id2 = issues.id").Model(&dao.LinkedIssues{}),
		}
	} else {
		selectExprs = []string{
			"issues.*",
			"count(*) over() as all_count",
		}
	}

	// Rank count
	if filters.SearchQuery != "" {
		searchSelects := []string{
			"ts_headline('russian', issues.name, plainto_tsquery('russian', ?)) as name_highlighted",
			"ts_headline('russian', issues.description_stripped, plainto_tsquery('russian', ?), 'MaxFragments=10, MaxWords=8, MinWords=3') as desc_highlighted",
			"calc_rank(tokens, p.identifier, issues.sequence_id, ?) as ts_rank",
		}
		searchInterface := []interface{}{
			filters.SearchQuery,
			filters.SearchQuery,
			filters.SearchQuery,
		}

		selectExprs = append(selectExprs, searchSelects...)
		selectInterface = append(selectInterface, searchInterface...)

		query = query.Order("ts_rank desc")
	}

	order := &clause.OrderByColumn{Desc: desc}
	switch orderByParam {
	case "priority":
		order = nil
		sql := "case when priority='urgent' then 5 when priority='high' then 4 when priority='medium' then 3 when priority='low' then 2 when priority is null then 1 end"
		if desc {
			sql += " DESC"
		}
		query = query.Order(sql)
	case "author":
		selectExprs = append(selectExprs, "(?) as author_sort")
		selectInterface = append(selectInterface, s.db.Select("COALESCE(NULLIF(last_name,''), email)").Where("id = issues.created_by_id").Model(&dao.User{}))
		order.Column = clause.Column{Name: "author_sort"}
	case "state":
		selectExprs = append(selectExprs, "(?) as state_sort")
		selectInterface = append(selectInterface, s.db.Select(`concat(case "group" when 'backlog' then 1 when 'unstarted' then 2 when 'started' then 3 when 'completed' then 4 when 'cancelled' then 5 end, name, color)`).Where("id = issues.state_id").Model(&dao.State{}))
		order.Column = clause.Column{Name: "state_sort"}
	case "labels":
		selectExprs = append(selectExprs, "array(?) as labels_sort")
		selectInterface = append(selectInterface, s.db.Select("name").Where("id in (?)", s.db.Select("label_id").Where("issue_id = issues.id").Model(&dao.IssueLabel{})).Model(&dao.Label{}))
		order.Column = clause.Column{Name: "labels_sort"}
	case "sub_issues_count":
		fallthrough
	case "link_count":
		fallthrough
	case "linked_issues_count":
		fallthrough
	case "attachment_count":
		order.Column = clause.Column{Name: orderByParam}
	case "assignees":
		selectExprs = append(selectExprs, "array(?) as assignees_sort")
		selectInterface = append(selectInterface, s.db.Select("COALESCE(NULLIF(last_name,''), email)").Where("users.id in (?)", s.db.Select("assignee_id").Where("issue_id = issues.id").Model(&dao.IssueAssignee{})).Model(&dao.User{}))
		order.Column = clause.Column{Name: orderByParam + "_sort"}
	case "watchers":
		selectExprs = append(selectExprs, "array(?) as watchers_sort")
		selectInterface = append(selectInterface, s.db.Select("COALESCE(NULLIF(last_name,''), email)").Where("users.id in (?)", s.db.Select("watcher_id").Where("issue_id = issues.id").Model(&dao.IssueWatcher{})).Model(&dao.User{}))
		order.Column = clause.Column{Name: orderByParam + "_sort"}
	default:
		order.Column = clause.Column{Table: "issues", Name: orderByParam}
	}

	groupSelectQuery := query.Select(strings.Join(selectExprs, ", "), selectInterface...).Limit(limit).Offset(offset).Session(&gorm.Session{})

	// Get groups
	if groupByParam != "" {
		groupSize, err := dao.GetIssuesGroupsSize(s.db, groupByParam, projectMember.ProjectId)
		if err != nil {
			return EError(c, err)
		}
		groupMap := make(map[string]IssuesGroupResponse, len(groupSize))

		totalCount := 0

		for group, size := range groupSize {
			totalCount += size

			q := groupSelectQuery.Session(&gorm.Session{})

			var entity any
			switch groupByParam {
			case "priority":
				if group != "" {
					q = q.Where("issues.priority = ?", group)
				} else {
					q = q.Where("issues.priority is null")
				}
				entity = group
			case "author":
				q = q.Where("created_by_id = ?", group)
				if size == 0 {
					var user dao.User
					if err := s.db.Where("id = ?", group).First(&user).Error; err != nil {
						return EError(c, err)
					}
					entity = user.ToLightDTO()
				}
			case "state":
				q = q.Where("state_id = ?", group)
				if size == 0 {
					var state dao.State
					if err := s.db.Where("id = ?", group).First(&state).Error; err != nil {
						return EError(c, err)
					}
					entity = state.ToLightDTO()
				}
			case "labels":
				qq := s.db.Where("issues.id in (?)", s.db.
					Model(&dao.IssueLabel{}).
					Select("issue_id").
					Where("label_id = ?", group))
				if group == "" {
					qq = qq.Or("issues.id not in (?)", s.db.
						Select("issue_id").
						Model(&dao.IssueLabel{}))
				}
				q = q.Where(qq)
				if group != "" {
					var label dao.Label
					if err := s.db.Where("id = ?", group).First(&label).Error; err != nil {
						return EError(c, err)
					}
					entity = label.ToLightDTO()
				}
			case "assignees":
				qq := s.db.Where("issues.id in (?)", s.db.
					Model(&dao.IssueAssignee{}).
					Select("issue_id").
					Where("assignee_id = ?", group))
				if group == "" {
					qq = qq.Or("issues.id not in (?)", s.db.
						Select("issue_id").
						Model(&dao.IssueAssignee{}))
				}
				q = q.Where(qq)
				if group != "" {
					var u dao.User
					if err := s.db.Where("id = ?", group).First(&u).Error; err != nil {
						return EError(c, err)
					}
					entity = u.ToLightDTO()
				}
			case "watchers":
				qq := s.db.Where("issues.id in (?)", s.db.
					Model(&dao.IssueWatcher{}).
					Select("issue_id").
					Where("watcher_id = ?", group))
				if group == "" {
					qq = qq.Or("issues.id not in (?)", s.db.
						Select("issue_id").
						Model(&dao.IssueWatcher{}))
				}
				q = q.Where(qq)
				if group != "" {
					var u dao.User
					if err := s.db.Where("id = ?", group).First(&u).Error; err != nil {
						return EError(c, err)
					}
					entity = u.ToLightDTO()
				}
			}

			if size == 0 {
				groupMap[group] = IssuesGroupResponse{
					Entity: entity,
					Count:  size,
				}
				continue
			}

			var issues []dao.IssueWithCount
			if err := q.Find(&issues).Error; err != nil {
				return EError(c, err)
			}

			switch groupByParam {
			case "author":
				entity = issues[0].Author.ToLightDTO()
			case "state":
				entity = issues[0].State.ToLightDTO()
			}

			groupMap[group] = IssuesGroupResponse{
				Entity: entity,
				Count:  size,
				Issues: utils.SliceToSlice(&issues, func(i *dao.IssueWithCount) *dto.IssueWithCount { return i.ToDTO() }),
			}
		}

		return c.JSON(http.StatusOK, IssuesGroupedResponse{
			Count:   totalCount,
			Offset:  offset,
			Limit:   limit,
			GroupBy: groupByParam,
			Issues:  SortIssuesGroups(groupByParam, groupMap),
		})
	}

	if order != nil {
		groupSelectQuery = groupSelectQuery.Order(*order)
	}

	var issues []dao.IssueWithCount
	if err := groupSelectQuery.Find(&issues).Error; err != nil {
		return EError(c, err)
	}

	count := 0
	if len(issues) > 0 {
		count = issues[0].AllCount
	}

	if !lightSearch {
		// Fetch parents
		var parentIds []uuid.NullUUID
		for _, issue := range issues {
			if issue.ParentId.Valid {
				parentIds = append(parentIds, issue.ParentId)
			}
		}
		var parents []dao.Issue
		if err := s.db.Where("id in (?)", parentIds).Find(&parents).Error; err != nil {
			return EError(c, err)
		}
		parentsMap := make(map[string]*dao.Issue)
		for i := range parents {
			parentsMap[parents[i].ID.String()] = &parents[i]
		}
		for i := range issues {
			if issues[i].ParentId.Valid {
				issues[i].Parent = parentsMap[issues[i].ParentId.UUID.String()]
			}
		}
	}

	if lightSearch {
		return c.JSON(http.StatusOK, map[string]any{
			"count":  count,
			"offset": offset,
			"limit":  limit,
			"issues": utils.SliceToSlice(&issues, func(iwc *dao.IssueWithCount) dto.SearchLightweightResponse { return iwc.ToSearchLightDTO() }),
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"count":  count,
		"offset": offset,
		"limit":  limit,
		"issues": utils.SliceToSlice(&issues, func(iwc *dao.IssueWithCount) dto.IssueWithCount { return *iwc.ToDTO() }),
	})
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

		// Tariffication
		if len(form.File["files"]) > 0 && user.Tariffication != nil && !user.Tariffication.AttachmentsAllow {
			return EError(c, apierrors.ErrAssetsNotAllowed)
		}
	}

	if val, ok := data["description_html"]; ok {
		if val == "" {
			delete(data, "description_html")
		} else {
			var locked bool
			if err := s.db.
				Select("count(*) > 0").
				Where("issue_id = ? and user_id <> ? and locked_until > NOW()", issue.ID, user.ID).
				Model(&dao.IssueDescriptionLock{}).
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
			if date, err := notifications.FormatDate(issue.TargetDate.String(), "2006-01-02", nil); err != nil {
				return EErrorDefined(c, apierrors.ErrGeneric)
			} else {
				issueMapOld["target_date_activity_val"] = date
			}
		}
		if val != nil {
			date, err := notifications.FormatDate(val.(string), "2006-01-02", nil)
			if err != nil {
				return EErrorDefined(c, apierrors.ErrGeneric)
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
				var ancestorIDs []string
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

				if slices.Contains(ancestorIDs, issue.ID.String()) {
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
		data["state_id"] = stateId

		if err := s.db.Where("id = ?", stateId).
			Where("project_id = ?", issue.ProjectId).
			First(&newState).Error; err != nil {
			return EError(c, err)
		}

		issueMapOld["state_activity_val"] = issue.State.Name
		data["state_activity_val"] = newState.Name
		if newState.Group == "started" && issue.State.Group != "started" {
			data["start_date"] = &types.TargetDate{Time: time.Now()}
		}

		if issue.State.Group == "started" && (newState.Group == "backlog" || newState.Group == "unstarted") {
			data["start_date"] = nil
		}

		if newState.Group == "completed" && issue.State.Group != "completed" {
			data["completed_at"] = &types.TargetDate{Time: time.Now()}
		} else if newState.Group != "completed" {
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
		issue.StateId = &newState.ID
	}

	blockers, blockersOk := data["blockers_list"].([]interface{}) // задача блокирует [blocker_issues]
	assignees, assigneesOk := data["assignees_list"].([]interface{})
	if assigneesOk {
		assignees = utils.SetToSlice(utils.SliceToSet(assignees))
	}
	watchers, watchersOk := data["watchers_list"].([]interface{})
	if watchersOk {
		watchers = utils.SetToSlice(utils.SliceToSet(watchers))
	}
	labels, labelsOk := data["labels_list"].([]interface{})
	blocks, blocksOk := data["blocks_list"].([]interface{}) // блокируют эту задачу [blocked_issues]
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

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		// Upload new files
		if form != nil {
			// Save issue attachments to
			for _, f := range form.File["files"] {
				fileAsset := dao.FileAsset{
					Id:          dao.GenUUID(),
					CreatedAt:   time.Now(),
					CreatedById: &user.ID,
					Name:        f.Filename,
					FileSize:    int(f.Size),
					WorkspaceId: &issue.WorkspaceId,
					IssueId:     uuid.NullUUID{Valid: true, UUID: issue.ID},
				}

				if err := s.uploadAssetForm(tx, f, &fileAsset,
					filestorage.Metadata{
						WorkspaceId: issue.WorkspaceId,
						ProjectId:   issue.ProjectId,
						IssueId:     issue.ID.String(),
					}); err != nil {
					return err
				}

				issue.InlineAttachments = append(issue.InlineAttachments, fileAsset)
			}
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
					Id:          dao.GenID(),
					BlockedById: blockerUUID,
					BlockId:     issue.ID,
					ProjectId:   issue.ProjectId,
					WorkspaceId: issue.WorkspaceId,
					CreatedById: &user.ID,
					UpdatedById: &user.ID,
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
				newAssignees = append(newAssignees, dao.IssueAssignee{
					Id:          dao.GenID(),
					AssigneeId:  fmt.Sprint(assignee),
					IssueId:     issue.ID.String(),
					ProjectId:   issue.ProjectId,
					WorkspaceId: issue.WorkspaceId,
					CreatedById: &user.ID,
					UpdatedById: &user.ID,
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
				newWatchers = append(newWatchers, dao.IssueWatcher{
					Id:          dao.GenID(),
					WatcherId:   fmt.Sprint(watcher),
					IssueId:     issue.ID.String(),
					ProjectId:   issue.ProjectId,
					WorkspaceId: issue.WorkspaceId,
					CreatedById: &user.ID,
					UpdatedById: &user.ID,
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
					Id:          dao.GenID(),
					LabelId:     fmt.Sprint(label),
					IssueId:     issue.ID.String(),
					ProjectId:   issue.ProjectId,
					WorkspaceId: issue.WorkspaceId,
					CreatedById: &user.ID,
					UpdatedById: &user.ID,
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
					Id:          dao.GenID(),
					BlockId:     blockUUID,
					BlockedById: issue.ID,
					ProjectId:   issue.ProjectId,
					WorkspaceId: issue.WorkspaceId,
					CreatedById: &user.ID,
					UpdatedById: &user.ID,
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
				var notifyUserIds *[]string

				if targetDateOk {
					notifyUserIds = &issue.AssigneeIDs
					*notifyUserIds = append(*notifyUserIds, issue.WatcherIDs...)
				}

				if assigneesOk || watchersOk {
					dateStr, err := notifications.FormatDate(issue.TargetDate.Time.String(), "2006-01-02", nil)
					if err != nil {
						return EErrorDefined(c, apierrors.ErrGeneric)
					}

					if assigneesOk {
						notifyUserIds = &[]string{}
						for _, v := range assignees {
							if str, ok := v.(string); ok {
								*notifyUserIds = append(*notifyUserIds, str)
							}
						}
						*notifyUserIds = append(*notifyUserIds, *watcherIds...)
					}
					if watchersOk {
						notifyUserIds = &[]string{}

						for _, v := range watchers {
							if str, ok := v.(string); ok {
								*notifyUserIds = append(*notifyUserIds, str)
							}
						}
						*notifyUserIds = append(*notifyUserIds, *assigneeIds...)
					}

					targetDate = &dateStr
				}
				*notifyUserIds = append(*notifyUserIds, issue.Author.ID)

				err := notifications.CreateDeadlineNotification(tx, &issue, targetDate, notifyUserIds)
				if err != nil {
					return EErrorDefined(c, apierrors.ErrGeneric)
				}
			}

		}

		issue.UpdatedAt = time.Now()
		issue.UpdatedById = &user.ID

		data["updated_at"] = time.Now()
		data["updated_by_id"] = user.ID

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

	err := tracker.TrackActivity[dao.Issue, dao.IssueActivity](s.tracker, tracker.ENTITY_UPDATED_ACTIVITY, data, issueMapOld, issue, &user)
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

		err := tracker.TrackActivity[dao.Issue, dao.ProjectActivity](s.tracker, tracker.ENTITY_DELETE_ACTIVITY, nil, nil, issue, &user)
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
// @Success 200 {object} ResponseSubIssueList "Список подзадач и распределение состояний"
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

	resp := ResponseSubIssueList{
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
		oldSubIssuesData[i]["parent"] = nil
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
		subIssues[i].UpdatedById = &user.ID
		subIssues[i].SortOrder = i + maxSortOrder + 1
	}

	if err := s.db.Save(&subIssues).Error; err != nil {
		return EError(c, err)
	}

	// Save new data for activity tracking
	newSubIssuesData := make([]map[string]interface{}, len(subIssues))
	for i := range subIssues {
		newSubIssuesData[i] = make(map[string]interface{})
		newSubIssuesData[i]["parent"] = parentId.UUID.String()
	}

	// Activity tracking
	for i := range subIssues {
		err := tracker.TrackActivity[dao.Issue, dao.IssueActivity](s.tracker, tracker.ENTITY_UPDATED_ACTIVITY, newSubIssuesData[i], oldSubIssuesData[i], subIssues[i], user)
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
	issueId := c.(IssueContext).Issue.ID.String()

	var req IssueLinkRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return EError(c, err)
	}

	if req.Url == "" || req.Title == "" {
		return EErrorDefined(c, apierrors.ErrURLAndTitleRequired)
	}

	link := dao.IssueLink{
		Id:          dao.GenID(),
		Title:       req.Title,
		Url:         req.Url,
		CreatedById: &user.ID,
		UpdatedById: &user.ID,
		IssueId:     issueId,
		ProjectId:   project.ID,
		WorkspaceId: project.WorkspaceId,
	}

	if err := s.db.Create(&link).Error; err != nil {
		return EError(c, err)
	}

	//if err := s.tracker.TrackActivity(tracker.LINK_CREATED_ACTIVITY, StructToJSONMap(link), nil, issueId, tracker.ENTITY_TYPE_ISSUE, &project, user); err != nil {
	//	return EError(c, err)
	//}

	err := tracker.TrackActivity[dao.IssueLink, dao.IssueActivity](s.tracker, tracker.ENTITY_CREATE_ACTIVITY, nil, nil, link, &user)
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

	oldLink.UpdatedAt = time.Now()
	oldLink.UpdatedById = &user.ID
	oldLink.Title = newLink.Title
	oldLink.Url = newLink.Url

	if err := s.db.Omit(clause.Associations).Save(&oldLink).Error; err != nil {
		return EError(c, err)
	}
	newMap := StructToJSONMap(oldLink)
	newMap["updateScopeId"] = linkId

	oldMap["updateScope"] = "link"
	oldMap["updateScopeId"] = linkId

	//if err := s.tracker.TrackActivity(tracker.LINK_UPDATED_ACTIVITY, newMap, oldMap, issueId, tracker.ENTITY_TYPE_ISSUE, &project, user); err != nil {
	//	return EError(c, err)
	//}
	err := tracker.TrackActivity[dao.IssueLink, dao.IssueActivity](s.tracker, tracker.ENTITY_UPDATED_ACTIVITY, newMap, oldMap, oldLink, &user)
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
	issueId := c.(IssueContext).Issue.ID.String()
	linkId := c.Param("linkId")

	var link dao.IssueLink

	if err := s.db.Where("project_id = ?", project.ID).
		Where("issue_id = ?", issueId).
		Where("issue_links.id = ?", linkId).First(&link).Error; err != nil {
		return EError(c, err)
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		err := tracker.TrackActivity[dao.IssueLink, dao.IssueActivity](s.tracker, tracker.ENTITY_DELETE_ACTIVITY, nil, nil, link, &user)
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

		// Tariffication
		if len(form.File["files"]) > 0 && user.Tariffication != nil && !user.Tariffication.AttachmentsAllow {
			return EError(c, apierrors.ErrAssetsNotAllowed)
		}
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		comment.Id = dao.GenID()
		comment.ProjectId = project.ID
		comment.Project = &project
		comment.IssueId = issue.ID.String()
		comment.WorkspaceId = project.WorkspaceId
		comment.Workspace = issue.Workspace
		comment.ActorId = &user.ID
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
					CreatedById: &user.ID,
					Name:        f.Filename,
					FileSize:    int(f.Size),
					WorkspaceId: &issue.WorkspaceId,
					CommentId:   &comment.Id,
				}

				if err := s.uploadAssetForm(tx, f, &fileAsset,
					filestorage.Metadata{
						WorkspaceId: issue.WorkspaceId,
						ProjectId:   issue.ProjectId,
						IssueId:     issue.ID.String(),
					}); err != nil {
					return err
				}

				comment.Attachments = append(comment.Attachments, fileAsset)
			}
		}

		var authorOriginalComment *dao.User
		var replyNotMember bool
		if comment.ReplyToCommentId != nil {
			if err := tx.Where(
				"id = (?)", tx.
					Select("actor_id").
					Model(&dao.IssueComment{}).
					Where("id = ?", comment.ReplyToCommentId)).
				First(&authorOriginalComment).Error; err != nil {
				return err
			}
			if authorOriginalComment.ID != issue.CreatedById {
				for _, id := range issue.AssigneeIDs {
					if id == authorOriginalComment.ID {
						replyNotMember = true
						break
					}
				}
				for _, id := range issue.WatcherIDs {
					if id == authorOriginalComment.ID {
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
		for _, user := range users {
			if authorOriginalComment != nil && !replyNotMember {
				if user.ID == authorOriginalComment.ID {
					continue
				}
			}

			s.notificationsService.Tg.UserMentionNotification(user, comment)
			if notify, countNotify, err := notifications.CreateUserNotificationAddComment(tx, user.ID, comment); err == nil {
				s.notificationsService.Ws.Send(user.ID, notify.ID, notifications.Mention{IssueComment: comment}, countNotify)
			}
		}

		return nil
	}); err != nil {
		return EError(c, err)
	}

	//if err := s.tracker.TrackActivity(tracker.COMMENT_CREATED_ACTIVITY, StructToJSONMap(comment), nil, issue.ID.String(), tracker.ENTITY_TYPE_ISSUE, &project, user); err != nil {
	//	return EError(c, err)
	//}

	err := tracker.TrackActivity[dao.IssueComment, dao.IssueActivity](s.tracker, tracker.ENTITY_CREATE_ACTIVITY, nil, nil, comment, &user)
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

	if *commentOld.ActorId != user.ID {
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

		// Tariffication
		if len(form.File["files"]) > 0 && user.Tariffication != nil && !user.Tariffication.AttachmentsAllow {
			return EError(c, apierrors.ErrAssetsNotAllowed)
		}
	}

	if commentOld.CommentHtml.Body == comment.CommentHtml.Body {
		return c.JSON(http.StatusOK, commentOld)
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		commentOld.Attachments = comment.Attachments
		commentOld.CommentHtml = comment.CommentHtml
		commentOld.CommentStripped = comment.CommentStripped
		commentOld.ReplyToCommentId = comment.ReplyToCommentId
		commentOld.UpdatedById = &user.ID

		if err := tx.Omit(clause.Associations).Save(&commentOld).Error; err != nil {
			return err
		}

		if form != nil {
			// Save issue attachments to
			for _, f := range form.File["files"] {
				fileAsset := dao.FileAsset{
					Id:          dao.GenUUID(),
					CreatedAt:   time.Now(),
					CreatedById: &user.ID,
					Name:        f.Filename,
					FileSize:    int(f.Size),
					WorkspaceId: &issue.WorkspaceId,
					CommentId:   &commentOld.Id,
				}

				if err := s.uploadAssetForm(tx, f, &fileAsset,
					filestorage.Metadata{
						WorkspaceId: issue.WorkspaceId,
						ProjectId:   issue.ProjectId,
						IssueId:     issue.ID.String(),
					}); err != nil {
					return err
				}

				commentOld.Attachments = append(commentOld.Attachments, fileAsset)
			}
		}

		var authorOriginalComment *dao.User
		var replyNotMember bool
		if comment.ReplyToCommentId != nil {
			if err := tx.Where(
				"id = (?)", tx.
					Select("actor_id").
					Model(&dao.IssueComment{}).
					Where("id = ?", comment.ReplyToCommentId)).
				First(&authorOriginalComment).Error; err != nil {
				return err
			}
			if authorOriginalComment.ID != issue.CreatedById {
				for _, id := range issue.AssigneeIDs {
					if id == authorOriginalComment.ID {
						replyNotMember = true
						break
					}
				}
				for _, id := range issue.WatcherIDs {
					if id == authorOriginalComment.ID {
						replyNotMember = true
						break
					}
				}
			} else {
				replyNotMember = true
			}
			if !replyNotMember {
				comment.Id = commentId
				comment.IssueId = issue.ID.String()
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
		for _, user := range users {
			if authorOriginalComment != nil && !replyNotMember {
				if user.ID == authorOriginalComment.ID {
					continue
				}
			}
			s.notificationsService.Tg.UserMentionNotification(user, commentOld)
			if notify, countNotify, err := notifications.CreateUserNotificationAddComment(tx, user.ID, commentOld); err == nil {
				s.notificationsService.Ws.Send(user.ID, notify.ID, commentOld, countNotify)
			}
		}

		return nil
	}); err != nil {
		return EError(c, err)
	}

	newMap := StructToJSONMap(commentOld)
	newMap["updateScopeId"] = commentId
	newMap["field_log"] = "comment"

	oldMap["updateScope"] = "comment"
	oldMap["updateScopeId"] = commentId
	//if err := s.tracker.TrackActivity(tracker.COMMENT_UPDATED_ACTIVITY, newMap, oldMap, issue.ID.String(), tracker.ENTITY_TYPE_ISSUE, commentOld.Project, user); err != nil {
	//	return EError(c, err)
	//}

	err := tracker.TrackActivity[dao.IssueComment, dao.IssueActivity](s.tracker, tracker.ENTITY_UPDATED_ACTIVITY, newMap, oldMap, commentOld, &user)
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
	issueId := c.(IssueContext).Issue.ID.String()
	commentId := c.Param("commentId")

	var comment dao.IssueComment
	if err := s.db.Where("project_id = ?", project.ID).
		Where("issue_id = ?", issueId).
		Where("id = ?", commentId).
		Preload("Attachments").
		First(&comment).Error; err != nil {
		return EError(c, err)
	}

	if projectMember.Role != types.AdminRole && *comment.ActorId != user.ID {
		return EErrorDefined(c, apierrors.ErrCommentEditForbidden)
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		err := tracker.TrackActivity[dao.IssueComment, dao.IssueActivity](s.tracker, tracker.ENTITY_DELETE_ACTIVITY, nil, nil, comment, &user)
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
	reaction := dao.CommentReaction{
		Id:        dao.GenID(),
		CreatedAt: time.Now(),
		UserId:    user.ID,
		CommentId: commentId,
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
		query = query.Where("ia.field = ?", field)
		if field == "state" {
			query = query.Select("ia.*, round(extract('epoch' from ia.created_at - (LAG(ia.created_at, 1, \"Issue\".created_at) over (order by ia.created_at))) * 1000) as state_lag")

		} else {
			query = query.Select("ia.*")
		}
	} else {
		query = query.Where("ia.field <> ?", "state")
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

	if user.Tariffication != nil && !user.Tariffication.AttachmentsAllow {
		return EError(c, apierrors.ErrAssetsNotAllowed)
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
			WorkspaceId: issue.WorkspaceId,
			ProjectId:   issue.ProjectId,
			IssueId:     issue.ID.String(),
		},
	); err != nil {
		return EError(c, err)
	}

	issueAttachment := dao.IssueAttachment{
		Id:          dao.GenID(),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		CreatedById: &user.ID,
		UpdatedById: &user.ID,
		Attributes:  attributes,
		AssetId:     assetName,
		IssueId:     issue.ID.String(),
		ProjectId:   project.ID,
		WorkspaceId: issue.WorkspaceId,
	}

	issueAttachment.Asset = &dao.FileAsset{
		Id:          assetName,
		CreatedById: &user.ID,
		WorkspaceId: &issue.WorkspaceId,
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
	err = tracker.TrackActivity[dao.IssueAttachment, dao.IssueActivity](s.tracker, tracker.ENTITY_CREATE_ACTIVITY, nil, nil, issueAttachment, &user)
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
			return EError(c, err)
		}
		attachW, err := w.CreateHeader(&zip.FileHeader{
			Name:     attachment.Asset.Name,
			Modified: attachment.CreatedAt,
			Comment:  "Created by AIPlan. https://plan.aisa.ru",
		})
		if err != nil {
			return EError(c, err)
		}

		if _, err := io.Copy(attachW, attachR); err != nil {
			return EError(c, err)
		}
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
	issueId := c.(IssueContext).Issue.ID.String()
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
		err := tracker.TrackActivity[dao.IssueAttachment, dao.IssueActivity](s.tracker, tracker.ENTITY_DELETE_ACTIVITY, nil, nil, attachment, &user)
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
			if v.Id1.String() != issue.ID.String() {
				oldIDs = append(oldIDs, v.Id1.String())
			}
			if v.Id2.String() != issue.ID.String() {
				oldIDs = append(oldIDs, v.Id2.String())
			}
		}

		newIDs := make([]interface{}, len(param.IssueIDs))
		for i, v := range param.IssueIDs {
			newIDs[i] = v.String()
		}

		if err := tx.Where("id1 = ? or id2 = ?", issue.ID, issue.ID).Delete(&dao.LinkedIssues{}).Error; err != nil {
			return err
		}

		changes, err := utils.CalculateIDChanges(newIDs, oldIDs)
		if err != nil {
			return err
		}

		changes.InvolvedIds = append(changes.InvolvedIds, issue.ID.String())

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

	err := tracker.TrackActivity[dao.Issue, dao.IssueActivity](s.tracker, tracker.ENTITY_UPDATED_ACTIVITY, newIssue, oldIssue, issue, user)
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
// @Success 200 {object} IssueLockResponse "Описание успешно заблокировано"
// @Failure 400 {object} apierrors.DefinedError "Некорректные параметры запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 409 {object} IssueLockResponse "Описание заблокировано другим пользователем"
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
					UserId:      uuid.Must(uuid.FromString(user.ID)),
					IssueId:     issue.ID,
					LockedUntil: time.Now().Add(descriptionLockTime),
				}

				if err := tx.Create(&lock).Error; err != nil {
					return EError(c, err)
				}

				return c.JSON(http.StatusOK, IssueLockResponse{
					Locked:      true,
					LockedUntil: lock.LockedUntil,
				})
			}
			return EError(c, err)
		}
		// Lock exist
		return c.JSON(http.StatusConflict, IssueLockResponse{
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

// SubIssuesIds представляет собой структуру для передачи связанных задач
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

type ResponseSubIssueList struct {
	SubIssues         []dto.Issue    `json:"sub_issues"`
	StateDistribution map[string]int `json:"state_distribution"`
}

type IssueLockResponse struct {
	Locked      bool           `json:"ok"`
	LockedBy    *dto.UserLight `json:"locked_by,omitempty"`
	LockedUntil time.Time      `json:"locked_until"`
}

type IssuesGroupedResponse struct {
	Count   int                   `json:"count"`
	Offset  int                   `json:"offset"`
	Limit   int                   `json:"limit"`
	GroupBy string                `json:"group_by"`
	Issues  []IssuesGroupResponse `json:"issues"`
}

type IssuesGroupResponse struct {
	Entity any                   `json:"entity"`
	Count  int                   `json:"count"`
	Issues []*dto.IssueWithCount `json:"issues"`
}

func SortIssuesGroups(groupByParam string, issuesGroups map[string]IssuesGroupResponse) []IssuesGroupResponse {
	return slices.SortedFunc(maps.Values(issuesGroups), func(e1, e2 IssuesGroupResponse) int {
		switch groupByParam {
		case "priority":
			entity1, _ := e1.Entity.(string) // use _ for nil transform into empty string
			entity2, _ := e2.Entity.(string)
			return utils.PrioritiesSortValues[entity1] - utils.PrioritiesSortValues[entity2]
		case "author":
			entity1 := e1.Entity.(*dto.UserLight)
			entity2 := e2.Entity.(*dto.UserLight)
			return utils.CompareUsers(entity1, entity2)
		case "state":
			entity1, _ := e1.Entity.(*dto.StateLight)
			entity2, _ := e2.Entity.(*dto.StateLight)
			if entity1 == entity2 {
				return 0
			}
			if entity1 == nil || (entity2 != nil && entity1.Name > entity2.Name) {
				return 1
			} else {
				return -1
			}
		case "labels":
			entity1, _ := e1.Entity.(*dto.LabelLight)
			entity2, _ := e2.Entity.(*dto.LabelLight)
			if entity1 == entity2 {
				return 0
			}
			if entity1 == nil || (entity2 != nil && entity1.Name > entity2.Name) {
				return 1
			} else {
				return -1
			}
		case "assignees":
			entity1, _ := e1.Entity.(*dto.UserLight)
			entity2, _ := e2.Entity.(*dto.UserLight)
			return utils.CompareUsers(entity1, entity2)
		case "watchers":
			entity1, _ := e1.Entity.(*dto.UserLight)
			entity2, _ := e2.Entity.(*dto.UserLight)
			return utils.CompareUsers(entity1, entity2)
		}
		return 0
	})
}
