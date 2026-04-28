package apicontext

import (
	"strconv"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/token"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/apierrors"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
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

	error error
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

// todo
func (a *APIContext) GetUser() *dao.User {
	return a.userMeta.User
}

func (a *APIContext) GetWorkspace() *dao.Workspace {
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

func (a *APIContext) GetProject() *dao.Project {
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

func (a *APIContext) GetIssue(options ...FetchOption) *dao.Issue {
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

	if _, ok := fetchOptions.loaded["Author"]; ok {
		// Fetch Author details
		if err := a.issue.Issue.Author.AfterFind(a.db); err != nil {
			a.error = err
			return nil
		}
	}

	return a.issue.Issue
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

	return a.sprint.Sprint
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
