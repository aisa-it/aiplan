// Пакет содержит контекст для импорта задач Jira в AIplan.  Он отслеживает статус процесса импорта, хранит данные о пользователях, проектах и задачах, а также предоставляет функции для взаимодействия с Jira API и базой данных.
//
// Основные возможности:
//   - Загрузка и обработка задач Jira.
//   - Синхронизация данных о пользователях и проектах.
//   - Отслеживание статуса импорта и обработка ошибок.
//   - Интеграция с базой данных и внешними сервисами (email, файловое хранилище).
package context

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	filestorage "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/file-storage"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/issues-import/atomic"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/issues-import/counters"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/issues-import/entity"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/issues-import/errors"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/andygrunwald/go-jira"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

// ImportContext - контекст с данными для импорта задач жиры в аиплан. Содержит поля статусов для отслеживания процесса импорта.
type ImportContext struct {
	WebURL *url.URL

	Client  *jira.Client
	DB      *gorm.DB
	Es      *notifications.EmailService
	Storage filestorage.FileStorage

	ImportAuthor dao.User

	IgnoreAttachments bool

	ID      uuid.UUID
	StartAt time.Time
	EndAt   time.Time

	Stage string

	rawStatuses []jira.Status

	Counters counters.ImportCounters

	// ID типа связи заблокированных задач. Используется для маппинга блокировок из жиры
	BlockLinkID string

	// ID типа связи связанных задач. Используется для маппинга связей из жиры
	RelateLinkMapper entity.LinkMapper

	// Маппинг приоритетов
	PrioritiesMapping entity.PrioritiesMapping

	TargetWorkspaceID string

	Project          dao.Project
	ProjectKey       string
	NotifyNewMembers bool

	RawIssues atomic.ConvertMap[*jira.Issue]

	States atomic.ImportMap[dao.State]

	// Map email to accountID
	EmailsMap     map[string]string
	EmailMapMutex sync.RWMutex

	Users         atomic.ImportMap[dao.User]
	UsersToCreate []dao.User
	AvatarAssets  []dao.FileAsset

	WorkspaceMembers []dao.WorkspaceMember
	ProjectMembers   []dao.ProjectMember

	Issues         atomic.ImportMap[dao.Issue]
	Attachments    atomic.ImportMap[*entity.Attachment]
	BadAttachments atomic.AtomicArray[*entity.Attachment]
	Labels         atomic.ImportMap[dao.Label]

	ProjectMemberships atomic.AtomicArray[dao.ProjectMember]

	IssueLinks atomic.AtomicArray[dao.IssueLink]

	// Jira coomentId as key
	IssueComments atomic.SyncMap[string, dao.IssueComment]
	IssueLabels   atomic.AtomicArray[dao.IssueLabel]

	IssueAssignees atomic.ConvertMap[dao.IssueAssignee]
	IssueWatchers  atomic.ConvertMap[dao.IssueWatcher]

	LinkedIssues []dao.LinkedIssues

	Blockers atomic.ConvertMap[*dao.IssueBlocker]

	// Jira issue(key) blocked by aiplan issue
	Blocks atomic.SyncMap[string, *dao.IssueBlocker]

	// Jira issue(key) blocks aiplan issue
	Blocked atomic.SyncMap[string, *dao.IssueBlocker]

	// Jira issue(key) related to jira issue
	Linked atomic.ConvertMap[entity.RawLinkedIssues]

	// Jira release related to aiplan tag
	ReleasesTags atomic.SyncMap[string, dao.Label]

	FileAssets atomic.AtomicArray[dao.FileAsset]
	AssetIds   []uuid.UUID // Array for cleaning on fail

	SortOrder atomic.SortOrderCounter

	// Флаг окончания импорта
	Finished bool
	// Последняя ошибка импорта. nil если все хорошо
	Error error

	Log *slog.Logger
}

func NewImportContext(
	webUrl *url.URL,
	client *jira.Client,
	DB *gorm.DB,
	es *notifications.EmailService,
	storage filestorage.FileStorage,
	user dao.User,
	projectKey string,
	targetWorkspaceId string,
	blockLinkID string,
	relatesLinkIds entity.LinkMapper,
	prioritiesMapping entity.PrioritiesMapping,
) (*ImportContext, error) {
	context := &ImportContext{
		WebURL:  webUrl,
		Client:  client,
		DB:      DB,
		Es:      es,
		Storage: storage,

		ImportAuthor: user,

		//IgnoreAttachments: true, // For tests

		ID: uuid.Must(uuid.NewV4()),

		StartAt: time.Now(),

		TargetWorkspaceID: targetWorkspaceId,

		ProjectKey: projectKey,

		BlockLinkID:      blockLinkID,
		RelateLinkMapper: relatesLinkIds,

		PrioritiesMapping: prioritiesMapping,

		RawIssues: atomic.NewConvertMap(func(i *jira.Issue) string {
			return i.Key
		}),

		EmailsMap: make(map[string]string),

		Blocks:  atomic.NewSyncMap[string, *dao.IssueBlocker](),
		Blocked: atomic.NewSyncMap[string, *dao.IssueBlocker](),
		Linked: atomic.NewConvertMap(func(rli entity.RawLinkedIssues) string {
			return rli.String()
		}),

		IssueAssignees: atomic.NewConvertMap(func(ia dao.IssueAssignee) string {
			return ia.IssueId.String() + ia.AssigneeId.String()
		}),
		IssueWatchers: atomic.NewConvertMap(func(iw dao.IssueWatcher) string {
			return iw.IssueId.String() + iw.WatcherId.String()
		}),

		Blockers: atomic.NewConvertMap(func(block *dao.IssueBlocker) string {
			return fmt.Sprintf("%s:%s", block.BlockedById.String(), block.BlockId.String())
		}),

		ReleasesTags: atomic.NewSyncMap[string, dao.Label](),

		SortOrder: atomic.NewSortOrderCounter(),

		IssueComments: atomic.NewSyncMap[string, dao.IssueComment](),

		Log: slog.With(slog.Group("importInfo",
			slog.String("projectKey", projectKey),
			slog.String("actorId", user.String()),
		)),
	}
	context.Users = atomic.NewImportMap(context.getUser)
	context.Issues = atomic.NewImportMap(context.GetIssue)
	context.States = atomic.NewImportMap(context.getState)
	context.Attachments = atomic.NewImportMap[*entity.Attachment](nil)
	context.Labels = atomic.NewImportMap[dao.Label](nil)

	// Get all statuses
	statuses, _, err := client.Status.GetAllStatuses()
	if err != nil {
		return nil, err
	}
	context.rawStatuses = statuses

	// Count issues
	_, resp, err := client.Issue.Search("project="+projectKey, &jira.SearchOptions{MaxResults: 0})
	if err != nil {
		return nil, err
	}

	context.Counters.TotalIssues = resp.Total

	return context, nil
}

func (context *ImportContext) Cancel() {
	context.Log.Info("Import canceled")
	context.EndAt = time.Now()
	context.Finished = true
	context.Error = errors.ErrCanceled

	context.Log.Info("Clean minio assets", "count", len(context.AssetIds))
	for _, id := range context.AssetIds {
		if err := context.Storage.Delete(id); err != nil {
			context.Log.Error("Delete minio asset", "id", id, "err", err)
		}
	}
}

func (c *ImportContext) GetIssue(key string) (dao.Issue, error) {
	i := c.RawIssues.Get(key)
	if i == nil {
		c.Log.Warn("Not found in buffer", "key", key)
		var err error
		i, _, err = c.Client.Issue.Get(key, &jira.GetQueryOptions{Expand: "renderedFields", Fields: "*all"})
		if err != nil {
			return dao.Issue{}, err
		}
	}

	issue, err := c.mapJiraIssue(i)
	if err != nil {
		return dao.Issue{}, err
	}

	return *issue, nil
}

func (c *ImportContext) getUser(accountId string) (dao.User, error) {
	c.Log.Debug("User not found in buffer", "accountId", accountId)

	val := make(url.Values)
	val.Add("username", accountId)
	val.Add("accountId", accountId)

	reqUrl := fmt.Sprintf("/rest/api/2/user?%s", val.Encode())
	req, _ := c.Client.NewRequest("GET", reqUrl, nil)

	var user jira.User
	_, err := c.Client.Do(req, &user)
	if err != nil {
		return dao.User{}, fmt.Errorf("get %s user: %w", accountId, err)
	}

	if user.EmailAddress != "" {
		c.EmailMapMutex.RLock()
		existId, ok := c.EmailsMap[user.EmailAddress]
		c.EmailMapMutex.RUnlock()

		if ok {
			slog.Warn("Duplicated user email, merge", "mapped", existId, "new", accountId)
			return c.Users.GetNoLock(existId)
		} else {
			c.EmailMapMutex.Lock()
			c.EmailsMap[user.EmailAddress] = accountId
			c.EmailMapMutex.Unlock()
		}
	}

	query := c.DB.Where("username = ?", accountId)
	if user.EmailAddress != "" {
		query = query.Or("email = ?", user.EmailAddress)
	}

	var DBUser dao.User
	if err := query.First(&DBUser).Error; err != nil {
		return c.translateUser(user), nil
	}
	//fmt.Println(user.AccountID, user.EmailAddress, user.Name, "===", DBUser.Email, *DBUser.Username)

	return DBUser, nil
}

func (c *ImportContext) getState(id string) (dao.State, error) {
	for _, status := range c.rawStatuses {
		if status.ID == id {
			return dao.State{
				ID:          dao.GenUUID(),
				Name:        status.Name,
				Description: status.Description,
				Group:       mapJiraStatusCat(status.StatusCategory),
				Color:       "#26b5ce",
				ProjectId:   c.Project.ID,
				WorkspaceId: c.Project.WorkspaceId,
			}, nil
		}
	}
	return dao.State{}, nil
}

func (c *ImportContext) GetProject(key string) error {
	project, _, err := c.Client.Project.Get(key)
	if err != nil {
		return err
	}

	// Workaround for translateUser(gets project id)
	projectId := dao.GenUUID()
	c.Project.ID = projectId

	lead, err := c.Users.Get(c.getJiraUserUsername(project.Lead))
	if err != nil {
		return err
	}

	c.Project = dao.Project{
		ID:            projectId,
		Name:          project.Name,
		Identifier:    project.Key,
		ProjectLeadId: lead.ID,
		CreatedById:   c.ImportAuthor.ID,
		WorkspaceId:   uuid.FromStringOrNil(c.TargetWorkspaceID),
	}
	return nil
}

func (c *ImportContext) GetProjectIssues(key string) error {
	c.Stage = "fetch"
	if err := c.Client.Issue.SearchPages("project="+key, &jira.SearchOptions{Expand: "renderedFields", Fields: []string{"*all"}, MaxResults: 1000},
		func(i jira.Issue) error {
			c.RawIssues.Put(&i)
			c.Counters.FetchedIssues.Add(1)
			return nil
		}); err != nil {
		return err
	}

	if c.Finished {
		return errors.ErrCanceled
	}

	c.Stage = "issues"

	c.RawIssues.Range(func(s string, i *jira.Issue) {
		if c.Finished {
			return
		}
		issue, err := c.mapJiraIssue(i)
		if err != nil {
			c.Log.Error("Map jira issue", "err", err)
		} else if issue != nil {
			c.Issues.Put(i.Key, *issue)
		}
		c.Counters.MappedIssues.Add(1)
	})

	if c.Finished {
		return errors.ErrCanceled
	}

	return nil
}

func (c *ImportContext) translateUser(user jira.User) dao.User {
	nameArr := strings.Split(user.DisplayName, " ")

	tz, err := time.LoadLocation(user.TimeZone)
	if err != nil {
		c.Log.Error("Parse Jira user timezone", "timezone", user.TimeZone, "err", err)
		tz = time.Local
	}

	if user.Name == "" && user.AccountID != "" {
		user.Name = user.AccountID
	}

	u := dao.User{
		ID:           dao.GenUUID(),
		Username:     &user.Name,
		Email:        user.EmailAddress,
		IsActive:     user.Active,
		UserTimezone: types.TimeZone(*tz),
		Avatar:       user.AvatarUrls.Four8X48,
	}
	if *u.Username == "" {
		username := user.AccountID
		u.Username = &username
	}

	if len(nameArr) == 1 {
		u.FirstName = nameArr[0]
	} else if len(nameArr) >= 2 {
		u.FirstName = nameArr[1]
		u.LastName = nameArr[0]
	}

	return u
}

func (c *ImportContext) mapJiraComment(comment *jira.Comment, issueID string) (*dao.IssueComment, error) {
	author, err := c.Users.Get(c.getJiraUserUsername(comment.Author))
	if err != nil {
		return nil, err
	}

	created, err := time.Parse("2006-01-02T15:04:05-0700", comment.Created)
	if err != nil {
		return nil, err
	}

	actorId := uuid.NullUUID{UUID: author.ID, Valid: true}
	issueUUID, _ := uuid.FromString(issueID)
	return &dao.IssueComment{
		Id:          dao.GenUUID(),
		CommentHtml: types.RedactorHTML{Body: comment.Body},
		ActorId:     actorId,
		CreatedAt:   created,
		IssueId:     issueUUID,
		ProjectId:   c.Project.ID,
		WorkspaceId: c.Project.WorkspaceId,
		OriginalId:  sql.NullString{String: comment.ID, Valid: true},
	}, nil
}

func (c *ImportContext) mapJiraPriority(priority jira.Priority) *string {
	res := "low"
	switch strings.ToLower(priority.ID) {
	case c.PrioritiesMapping.UrgentID:
		res = "urgent"
	case c.PrioritiesMapping.HighID:
		res = "high"
	case c.PrioritiesMapping.MediumID:
		res = "medium"
	case c.PrioritiesMapping.LowID:
		res = "low"
	default:
		return nil
	}
	return &res
}

func mapJiraStatusCat(category jira.StatusCategory) string {
	switch category.Key {
	case "new":
		return "backlog"
	case "indeterminate":
		return "started"
	case "done":
		return "completed"
	}
	return "backlog"
}

func (c *ImportContext) getJiraUserUsername(user interface{}) string {
	switch v := user.(type) {
	case jira.User:
		if v.Name != "" {
			return v.Name
		}
		return v.AccountID
	case jira.Watcher:
		if v.Name != "" {
			return v.Name
		}
		return v.AccountID
	}
	return ""
}

func (c *ImportContext) FetchProjectUsers() error {
	startAt := 0
	maxResults := 100
	for {
		req, _ := c.Client.NewRequest("GET", fmt.Sprintf("/rest/api/2/user/assignable/search?project=%s&startAt=%d&maxResults=%d", c.ProjectKey, startAt, maxResults), nil)
		var users []jira.User
		_, err := c.Client.Do(req, &users)
		if err != nil {
			return err
		}

		for _, user := range users {
			if user.EmailAddress != "" {
				existId, ok := c.EmailsMap[user.EmailAddress]
				accountId := c.getJiraUserUsername(user)

				if ok {
					slog.Warn("Duplicated user email, merge", "mapped", existId, "new", accountId)
					c.Users.Put(accountId, c.Users.GetLight(existId))
					continue
				} else {
					c.EmailsMap[user.EmailAddress] = accountId
				}
			}

			query := c.DB.Where("username = ?", user.AccountID).Or("username = ?", user.Name)
			if user.EmailAddress != "" {
				query = query.Or("email = ?", user.EmailAddress)
			}

			var DBUser dao.User
			if err := query.First(&DBUser).Error; err == gorm.ErrRecordNotFound {
				DBUser = c.translateUser(user)
			} else if err != nil {
				return err
			}

			c.Users.Put(c.getJiraUserUsername(user), DBUser)
		}

		if startAt+maxResults >= len(users) {
			return nil
		}

		startAt += maxResults
	}
}
