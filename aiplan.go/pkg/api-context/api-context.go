package apicontext

import (
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/token"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/types"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/apierrors"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/dao"
	"github.com/gofrs/uuid"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

const (
	entityExpireTime = time.Second * 15
	EchoKey          = "api-context"
)

type ExpireEntity[T any] struct {
	Entity T
	Loaded time.Time
}

type APIContext struct {
	echo.Context
	db *gorm.DB

	userMeta *UserMeta

	workspace       *dao.Workspace
	workspaceMember *dao.WorkspaceMember

	project       *dao.Project
	projectMember *dao.ProjectMember

	issue Issue

	sprint Sprint

	form Form

	doc Doc

	searchFilter *dao.SearchFilter
	releaseNote  *dao.ReleaseNote
	dictionary   *dao.Dictionary

	// prefilled — сущности подставлены снаружи, геттеры не ходят в БД
	prefilled bool

	error error
}

// Prefilled — набор заранее загруженных сущностей для NewPrefilled.
type Prefilled struct {
	// DB — соединение для правил, которым нужен запрос (проверка членства
	// по задаче). Необязательно: остальные геттеры в БД не ходят.
	DB              *gorm.DB
	User            *dao.User
	Workspace       *dao.Workspace
	WorkspaceMember *dao.WorkspaceMember
	Project         *dao.Project
	ProjectMember   *dao.ProjectMember
	Issue           *dao.Issue
	Sprint          *dao.Sprint
	Doc             *dao.Doc
	Form            *dao.Form
}

// NewPrefilled создаёт APIContext с уже загруженными сущностями: геттеры отдают
// их напрямую, без обращений к БД. Нужен там, где сущности получены вне
// HTTP-цепочки middleware (внешние интеграции, тесты).
func NewPrefilled(c echo.Context, p Prefilled) *APIContext {
	a := &APIContext{
		Context:         c,
		db:              p.DB,
		prefilled:       true,
		workspace:       p.Workspace,
		workspaceMember: p.WorkspaceMember,
		project:         p.Project,
		projectMember:   p.ProjectMember,
	}
	if p.User != nil {
		a.userMeta = &UserMeta{User: p.User}
	}
	a.issue.Issue = p.Issue
	a.sprint.Sprint = p.Sprint
	a.doc.Doc = p.Doc
	a.form.Form = p.Form
	return a
}

// NewSubject собирает контекст без HTTP-запроса — для вызывающих вне
// обработчиков: MCP-инструментов и хуков загрузки файлов. Сущности должны
// быть подставлены целиком: обращений к БД и к echo.Context не происходит.
func NewSubject(p Prefilled) *APIContext {
	return NewPrefilled(nil, p)
}

// SetPrefilledContext кладёт prefilled-контекст в echo.Context под общим ключом.
func SetPrefilledContext(c echo.Context, p Prefilled) *APIContext {
	a := NewPrefilled(c, p)
	c.Set(EchoKey, a)
	return a
}

type UserMeta struct {
	User         *dao.User
	AccessToken  *token.Token
	RefreshToken *token.Token
	TokenAuth    bool
}

type Issue struct {
	Issue       *dao.Issue
	LastOptions IssueFetchOptions
}

type Sprint struct {
	Sprint      *dao.Sprint
	LastOptions SprintFetchOptions
}

type Form struct {
	Form        *dao.Form
	LastOptions FormFetchOptions
}

type Doc struct {
	Doc         *dao.Doc
	LastOptions DocFetchOptions
}

func SetContext(c echo.Context, db *gorm.DB, userMeta *UserMeta) {
	c.Set(EchoKey, &APIContext{Context: c, db: db, userMeta: userMeta})
}

func GetContext(c echo.Context) *APIContext {
	raw := c.Get(EchoKey)
	if raw == nil {
		return nil
	}
	return raw.(*APIContext)
}

func (a *APIContext) Error() error {
	return a.error
}

func (a *APIContext) GetUser() *dao.User {
	if a.userMeta == nil {
		return nil
	}
	return a.userMeta.User
}

func (a *APIContext) GetAuthInfo() *UserMeta {
	return a.userMeta
}

func (a *APIContext) GetAccessToken() *token.Token {
	if a.userMeta == nil {
		return nil
	}
	return a.userMeta.AccessToken
}

func (a *APIContext) GetRefreshToken() *token.Token {
	if a.userMeta == nil {
		return nil
	}
	return a.userMeta.RefreshToken
}

func (a *APIContext) IsTokenAuth() bool {
	if a.userMeta == nil {
		return false
	}
	return a.userMeta.TokenAuth
}

func (a *APIContext) GetWorkspace() *dao.Workspace {
	if a.prefilled {
		return a.workspace
	}
	if a.workspace != nil {
		return a.workspace
	}

	slugOrId := a.Param("workspaceSlug")

	workspaceQuery := a.db.
		Joins("Owner").
		Joins("LogoAsset")

	if id, err := uuid.FromString(slugOrId); err == nil {
		workspaceQuery = workspaceQuery.Where("workspaces.id = ?", id)
	} else {
		workspaceQuery = workspaceQuery.Where("slug = ?", slugOrId)
	}

	if err := workspaceQuery.First(&a.workspace).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			a.error = apierrors.ErrWorkspaceNotFound
			return nil
		}
		a.error = err
		return nil
	}
	return a.workspace
}

func (a *APIContext) GetWorkspaceMember() *dao.WorkspaceMember {
	if a.prefilled {
		return a.workspaceMember
	}
	if a.workspaceMember != nil {
		return a.workspaceMember
	}
	workspace := a.GetWorkspace()
	user := a.GetUser()
	if workspace == nil || user == nil {
		return nil
	}

	if err := a.db.Where("workspace_id = ?", workspace.ID).Where("member_id = ?", user.ID).First(&a.workspaceMember).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			a.error = apierrors.ErrWorkspaceNotFound
			return nil
		}
		a.error = err
		return nil
	}
	return a.workspaceMember
}

func (a *APIContext) CleanProject() *APIContext {
	a.project = nil
	return a
}

func (a *APIContext) GetProject() *dao.Project {
	if a.prefilled {
		return a.project
	}
	if a.project != nil {
		return a.project
	}
	projectId := a.Param("projectId")
	if projectId == "" {
		return nil
	}

	workspace := a.GetWorkspace()
	if workspace == nil {
		return nil
	}

	projectQuery := a.db.
		Joins("ProjectLead").
		Where("projects.workspace_id = ?", workspace.ID).
		Preload("DefaultAssigneesDetails", "is_default_assignee = ?", true).
		Preload("DefaultWatchersDetails", "is_default_watcher = ?", true)

	// Search by id or identifier
	if id, err := uuid.FromString(projectId); err == nil {
		projectQuery = projectQuery.Where("projects.id = ?", id)
	} else {
		projectQuery = projectQuery.Where("projects.identifier = ?", projectId)
	}

	if err := projectQuery.First(&a.project).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			a.error = apierrors.ErrProjectNotFound
			return nil
		}
		a.error = err
		return nil
	}

	return a.project
}

func (a *APIContext) GetProjectMember() *dao.ProjectMember {
	if a.prefilled {
		return a.projectMember
	}
	if a.projectMember != nil {
		return a.projectMember
	}
	project := a.GetProject()
	user := a.GetUser()
	if project == nil || user == nil {
		return nil
	}

	if err := a.db.Where("project_id = ?", project.ID).
		Where("member_id = ?", user.ID).
		First(&a.projectMember).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			a.error = apierrors.ErrProjectMemberNotFound
			return nil
		}
		a.error = err
		return nil
	}

	if err := a.db.Model(&dao.ProjectFavorites{}).
		Select("EXISTS(?)",
			a.db.Model(&dao.ProjectFavorites{}).
				Select("1").
				Where("user_id = ?", user.ID).
				Where("project_id = ?", project.ID),
		).
		Find(&a.project.IsFavorite).Error; err != nil {
		a.error = err
		return nil
	}

	return a.projectMember
}

func (a *APIContext) CleanIssue() *APIContext {
	a.issue.Issue = nil
	return a
}

func (a *APIContext) GetIssue(options ...FetchOption) *dao.Issue {
	if a.prefilled {
		return a.issue.Issue
	}
	fetchOptions := &IssueFetchOptions{query: a.db.Session(&gorm.Session{}), loaded: make(map[string]struct{}, 11)}

	for _, option := range options {
		option(fetchOptions)
	}

	if a.issue.Issue != nil && (a.issue.LastOptions.GetID() == fetchOptions.GetID() || len(options) == 0) {
		return a.issue.Issue
	}

	a.fetchIssue(fetchOptions.query)

	if a.error != nil {
		return nil
	}

	if err := a.applyIssueFetchPostLoad(fetchOptions); err != nil {
		a.error = err
		return nil
	}

	a.issue.LastOptions = *fetchOptions
	return a.issue.Issue
}

func (a *APIContext) applyIssueFetchPostLoad(fetchOptions *IssueFetchOptions) error {
	if _, ok := fetchOptions.loaded["Author"]; ok {
		if err := a.issue.Issue.Author.AfterFind(a.db); err != nil {
			return err
		}
	}
	if _, ok := fetchOptions.loaded["Blockers"]; ok {
		if err := a.fetchIssueBlockers(); err != nil {
			return err
		}
	}
	if _, ok := fetchOptions.loaded["LinkedIssues"]; ok {
		if err := a.issue.Issue.FetchLinkedIssues(a.db); err != nil {
			return err
		}
	}
	return nil
}

func (a *APIContext) fetchIssueBlockers() error {
	if err := a.db.
		Where("block_id = ?", a.issue.Issue.ID).
		Preload("BlockedBy.Project").
		Joins("BlockedBy").
		Find(&a.issue.Issue.BlockerIssuesIDs).Error; err != nil {
		return err
	}
	return a.db.
		Where("blocked_by_id = ?", a.issue.Issue.ID).
		Joins("Block").
		Find(&a.issue.Issue.BlockedIssuesIDs).Error
}

func (a *APIContext) fetchIssue(query *gorm.DB) {
	workspace := a.GetWorkspace()
	project := a.GetProject()
	if project == nil {
		return
	}

	issueIdOrSeq := a.Param("issueIdOrSeq")

	if issueIdOrSeq == "" {
		a.error = apierrors.ErrIssueNotFound
		return
	}

	query = query.Where("issues.project_id = ?", project.ID)

	var issue dao.Issue
	issue.Project = project
	issue.Workspace = workspace
	if _, err := uuid.FromString(issueIdOrSeq); err == nil {
		// uuid id of issue
		query = query.Where("issues.id = ?", issueIdOrSeq)
	} else if sequenceId, err := strconv.Atoi(issueIdOrSeq); err == nil {
		// sequence id of issue
		query = query.Where("issues.sequence_id = ?", sequenceId)
	} else {
		a.error = apierrors.ErrIssueNotFound
		return
	}

	if err := query.First(&issue).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			a.error = apierrors.ErrIssueNotFound
			return
		}
		a.error = err
		return
	}

	a.issue.Issue = &issue
}

func (a *APIContext) GetSprint(options ...SprintFetchOption) *dao.Sprint {
	if a.prefilled {
		return a.sprint.Sprint
	}
	fetchOptions := &SprintFetchOptions{query: a.db.Session(&gorm.Session{}), loaded: make(map[string]struct{}, 6)}

	for _, option := range options {
		option(fetchOptions)
	}

	if a.sprint.Sprint != nil && (a.sprint.LastOptions.GetID() == fetchOptions.GetID() || len(options) == 0) {
		return a.sprint.Sprint
	}

	a.fetchSprint(fetchOptions)

	if a.error != nil {
		return nil
	}

	if _, ok := fetchOptions.loaded["Issues"]; ok {
		a.sprint.Sprint.UpdateStats()
	}

	a.sprint.LastOptions = *fetchOptions
	return a.sprint.Sprint
}

func (a *APIContext) CleanForm() *APIContext {
	a.form.Form = nil
	return a
}

func (a *APIContext) GetForm(options ...FormFetchOption) *dao.Form {
	if a.prefilled {
		return a.form.Form
	}
	fetchOptions := &FormFetchOptions{query: a.db.Session(&gorm.Session{}), loaded: make(map[string]struct{}, 5)}

	for _, option := range options {
		option(fetchOptions)
	}

	if a.form.Form != nil && (a.form.LastOptions.GetID() == fetchOptions.GetID() || len(options) == 0) {
		return a.form.Form
	}

	a.fetchForm(fetchOptions)

	if a.error != nil {
		return nil
	}

	a.form.LastOptions = *fetchOptions
	return a.form.Form
}

func (a *APIContext) CleanDoc() *APIContext {
	a.doc.Doc = nil
	return a
}

func (a *APIContext) GetDoc(options ...DocFetchOption) *dao.Doc {
	if a.prefilled {
		return a.doc.Doc
	}
	fetchOptions := &DocFetchOptions{query: a.db.Session(&gorm.Session{}), loaded: make(map[string]struct{}, 7)}

	for _, option := range options {
		option(fetchOptions)
	}

	if a.doc.Doc != nil && (a.doc.LastOptions.GetID() == fetchOptions.GetID() || len(options) == 0) {
		return a.doc.Doc
	}

	a.fetchDoc(fetchOptions)

	if a.error != nil {
		return nil
	}

	a.doc.LastOptions = *fetchOptions
	return a.doc.Doc
}

func (a *APIContext) GetSearchFilter() *dao.SearchFilter {
	if a.searchFilter != nil {
		return a.searchFilter
	}

	id, err := uuid.FromString(a.Param("filterId"))
	if err != nil {
		a.error = apierrors.ErrSearchFilterNotFound
		return nil
	}

	var filter dao.SearchFilter
	if err := a.db.Where("id = ?", id).First(&filter).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			a.error = apierrors.ErrSearchFilterNotFound
			return nil
		}
		a.error = err
		return nil
	}

	a.searchFilter = &filter
	return a.searchFilter
}

// GetDictionary лениво загружает справочник проекта по параметру :dictionaryId
// с проверкой принадлежности текущему проекту
func (a *APIContext) GetDictionary() *dao.Dictionary {
	if a.dictionary != nil {
		return a.dictionary
	}

	id, err := uuid.FromString(a.Param("dictionaryId"))
	if err != nil {
		a.error = apierrors.ErrDictionaryNotFound
		return nil
	}

	project := a.GetProject()
	if a.error != nil {
		return nil
	}

	var dictionary dao.Dictionary
	if err := a.db.Where("id = ? AND project_id = ?", id, project.ID).First(&dictionary).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			a.error = apierrors.ErrDictionaryNotFound
			return nil
		}
		a.error = err
		return nil
	}

	a.dictionary = &dictionary
	return a.dictionary
}

func (a *APIContext) GetReleaseNote() *dao.ReleaseNote {
	if a.releaseNote != nil {
		return a.releaseNote
	}

	noteId := a.Param("noteId")
	query := a.db.Session(&gorm.Session{})
	if id, err := uuid.FromString(noteId); err == nil {
		query = query.Where("id = ?", id)
	} else {
		query = query.Where("tag_name = ?", noteId)
	}

	var note dao.ReleaseNote
	if err := query.First(&note).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			a.error = apierrors.ErrReleaseNoteNotFound
			return nil
		}
		a.error = err
		return nil
	}

	a.releaseNote = &note
	return a.releaseNote
}

func (a *APIContext) fetchForm(fetchOptions *FormFetchOptions) {
	formSlug := a.Param("formSlug")
	if formSlug == "" {
		a.error = apierrors.ErrFormNotFound
		return
	}

	query := fetchOptions.query

	if _, ok := fetchOptions.loaded["CurrentMember"]; ok {
		if user := a.GetUser(); user != nil {
			query = query.Set("userId", user.ID)
		}
	}

	var form dao.Form
	if err := query.Where("forms.slug = ?", formSlug).First(&form).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			a.error = apierrors.ErrFormNotFound
			return
		}
		a.error = err
		return
	}

	a.form.Form = &form
}

func (a *APIContext) fetchDoc(fetchOptions *DocFetchOptions) {
	workspace := a.GetWorkspace()
	workspaceMember := a.GetWorkspaceMember()
	if a.error != nil {
		return
	}

	// Как у задач: UUID — поиск по id, иначе по адресу (слагу);
	// «/» внутри адреса приходит экранированным.
	docRef := a.Param("docId")
	if docRef == "" {
		a.error = apierrors.ErrDocNotFound
		return
	}
	var byRef func(*gorm.DB) *gorm.DB
	if id, err := uuid.FromString(docRef); err == nil {
		byRef = func(q *gorm.DB) *gorm.DB { return q.Where("docs.id = ?", id) }
	} else {
		slug, err := url.PathUnescape(docRef)
		if err != nil {
			a.error = apierrors.ErrDocNotFound
			return
		}
		byRef = func(q *gorm.DB) *gorm.DB { return q.Where("docs.slug = ?", strings.Trim(slug, "/")) }
	}

	query := fetchOptions.query.
		Set("member_id", workspaceMember.MemberId).
		Set("member_role", workspaceMember.Role)

	if _, ok := fetchOptions.loaded["Breadcrumbs"]; ok {
		query = query.Set("breadcrumbs", true)
	}

	if workspaceMember.Role != types.AdminRole {
		query = query.Where("docs.reader_role <= ? OR docs.editor_role <= ? OR EXISTS (SELECT 1 FROM doc_access_rules dar WHERE dar.doc_id = docs.id AND dar.member_id = ?) OR docs.created_by_id = ?",
			workspaceMember.Role, workspaceMember.Role, workspaceMember.MemberId, workspaceMember.MemberId)
	}

	var doc dao.Doc
	if err := byRef(query.Where("docs.workspace_id = ?", workspace.ID)).
		First(&doc).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			a.error = apierrors.ErrDocNotFound
			return
		}
		a.error = err
		return
	}

	a.doc.Doc = &doc
}

func (a *APIContext) fetchSprint(fetchOptions *SprintFetchOptions) {
	workspace := a.GetWorkspace()
	if workspace == nil {
		return
	}

	user := a.GetUser()

	sprintId := a.Param("sprintId")
	if sprintId == "" {
		a.error = apierrors.ErrSprintNotFound
		return
	}

	query := fetchOptions.query.Where("sprints.workspace_id = ?", workspace.ID)

	if _, ok := fetchOptions.loaded["Issues"]; ok && user != nil {
		query = query.Set("issueProgress", true).Set("userId", user.ID)
	}

	if val, err := uuid.FromString(sprintId); err == nil {
		query = query.Where("sprints.id = ?", val)
	} else {
		query = query.Where("sprints.sequence_id = ?", sprintId)
	}

	var sprint dao.Sprint
	if err := query.First(&sprint).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			a.error = apierrors.ErrSprintNotFound
			return
		}
		a.error = err
		return
	}

	a.sprint.Sprint = &sprint
}
