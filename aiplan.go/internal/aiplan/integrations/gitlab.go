// Пакет `integrations` содержит реализации интеграций с внешними сервисами, в частности с Gitlab.
// Основная задача - обработка событий push-вебхуков Gitlab и создание комментариев к соответствующим задачам в системе.
//   - Обработка push-вебхуков Gitlab для создания комментариев к задачам.
//   - Поддержка авторизации через токен доступа Gitlab.
//   - Поиск соответствующей задачи в системе по идентификатору коммита.
//   - Создание комментариев с ссылкой на коммит в репозитории Gitlab.
//   - Логирование активности в Telegram.
package integrations

import (
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	tracker "github.com/aisa-it/aiplan/internal/aiplan/activity-tracker"
	"github.com/aisa-it/aiplan/internal/aiplan/business"

	filestorage "github.com/aisa-it/aiplan/internal/aiplan/file-storage"
	"github.com/aisa-it/aiplan/internal/aiplan/types"

	"github.com/aisa-it/aiplan/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/internal/aiplan/notifications"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

var log *slog.Logger = slog.With(slog.String("integration", "gitlab"))

type GitlabIntegration struct {
	Integration
}

type GitlabPushEvent struct {
	UserName  string `json:"user_name"`
	Username  string `json:"user_username"`
	UserEmail string `json:"user_email"`
	Project   struct {
		ID                int    `json:"id"`
		Name              string `json:"name"`
		WebUrl            string `json:"web_url"`
		Namespace         string `json:"namespace"`
		PathWithNamespace string `json:"path_with_namespace"`
	} `json:"project"`
	Commits []struct {
		ID        string `json:"id"`
		Message   string `json:"message"`
		Title     string `json:"title"`
		Timestamp string `json:"timestamp"`
		URL       string `json:"url"`
		Author    struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"author"`
		Added    []string `json:"added"`
		Modified []string `json:"modified"`
		Removed  []string `json:"removed"`
	} `json:"commits"`
	TotalCommitsCount int `json:"total_commits_count"`
}

func NewGitlabIntegration(db *gorm.DB, tS *notifications.TelegramService, fs filestorage.FileStorage, tr *tracker.ActivitiesTracker, bl *business.Business) *GitlabIntegration {
	username := "gitlab_integration"
	i := &GitlabIntegration{
		Integration: Integration{
			Name:        "Gitlab",
			Description: "Интеграция с Gitlab. Создает комментарий на основе коммита в подключенном репозитории по номеру задачи(пример commit msg: FAB-2 fix). После подключения интеграции необходимо добавить webhook в репозиторий с url /api/integrations/webhooks/gitlab/ и токеном доступа из настроек пространства.",
			User: &dao.User{
				Email:         "gitlab@aiplan.ru",
				Password:      "",
				FirstName:     "GitLab",
				Username:      &username,
				Theme:         types.Theme{},
				IsActive:      true,
				IsIntegration: true,
			},
			AvatarSVG: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 380 380"><defs><style>.cls-1{fill:#e24329;}.cls-2{fill:#fc6d26;}.cls-3{fill:#fca326;}</style></defs><g id="LOGO"><path class="cls-1" d="M282.83,170.73l-.27-.69-26.14-68.22a6.81,6.81,0,0,0-2.69-3.24,7,7,0,0,0-8,.43,7,7,0,0,0-2.32,3.52l-17.65,54H154.29l-17.65-54A6.86,6.86,0,0,0,134.32,99a7,7,0,0,0-8-.43,6.87,6.87,0,0,0-2.69,3.24L97.44,170l-.26.69a48.54,48.54,0,0,0,16.1,56.1l.09.07.24.17,39.82,29.82,19.7,14.91,12,9.06a8.07,8.07,0,0,0,9.76,0l12-9.06,19.7-14.91,40.06-30,.1-.08A48.56,48.56,0,0,0,282.83,170.73Z"/><path class="cls-2" d="M282.83,170.73l-.27-.69a88.3,88.3,0,0,0-35.15,15.8L190,229.25c19.55,14.79,36.57,27.64,36.57,27.64l40.06-30,.1-.08A48.56,48.56,0,0,0,282.83,170.73Z"/><path class="cls-3" d="M153.43,256.89l19.7,14.91,12,9.06a8.07,8.07,0,0,0,9.76,0l12-9.06,19.7-14.91S209.55,244,190,229.25C170.45,244,153.43,256.89,153.43,256.89Z"/><path class="cls-2" d="M132.58,185.84A88.19,88.19,0,0,0,97.44,170l-.26.69a48.54,48.54,0,0,0,16.1,56.1l.09.07.24.17,39.82,29.82s17-12.85,36.57-27.64Z"/></g></svg>`,

			db:              db,
			telegramService: tS,
			fileStorage:     fs,
			tracker:         tr,
			bl:              bl,
		},
	}

	if err := i.FetchUser(); err != nil {
		log.Error("Fetch user", "err", err)
		return nil
	}

	return i
}

func (gi GitlabIntegration) RegisterWebhook(g *echo.Group) {
	g.POST("gitlab/", func(c echo.Context) error {
		token := c.Request().Header.Get("X-Gitlab-Token")
		event := c.Request().Header.Get("X-Gitlab-Event")

		var workspace dao.Workspace
		if err := gi.db.Where("integration_token = ?", token).First(&workspace).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return c.String(http.StatusNotFound, "workspace not found")
			}
			log.Error("Get workspace", "err", err)
			return c.NoContent(http.StatusInternalServerError)
		}

		if event != "Push Hook" {
			return c.String(http.StatusBadRequest, "unsupported event type")
		}

		var eventData GitlabPushEvent
		if err := c.Bind(&eventData); err != nil {
			log.Error("Bind gitlab webhook request", "err", err)
			return c.NoContent(http.StatusBadRequest)
		}

		go gi.GitlabPushEvent(eventData, workspace)

		return c.NoContent(http.StatusOK)
	})
}

func (gi *GitlabIntegration) GitlabPushEvent(event GitlabPushEvent, workspace dao.Workspace) {
	for _, commit := range event.Commits {
		var exist bool
		if err := gi.db.Select("count(*) > 0").Model(&dao.IssueComment{}).Where("actor_id = ?", gi.User.ID).Where("integration_meta = ?", commit.ID).Find(&exist).Error; err != nil {
			log.Error("Check commit comment exists")
			continue
		}
		if exist {
			continue
		}

		userFound := true
		var user dao.User
		if err := gi.db.Where("email = ?", commit.Author.Email).First(&user).Error; err != nil {
			if err != gorm.ErrRecordNotFound {
				log.Error("Find user by email", "err", err)
			} else {
				userFound = false
			}
		}

		issueReg := regexp.MustCompile(`[A-Z0-9]+-\d+`)
		foundIssue := strings.Split(issueReg.FindString(commit.Message), "-")
		if len(foundIssue) != 2 {
			continue
		}

		var issue dao.Issue
		if err := gi.db.Where("project_id = (?)",
			gi.db.
				Select("id").
				Model(dao.Project{}).
				Where("workspace_id = ?", workspace.ID).
				Where("identifier = ?", foundIssue[0])).
			Where("sequence_id = ?", foundIssue[1]).
			Joins("Workspace").
			First(&issue).Error; err != nil {
			log.Error("Find issue from gitlab event", "issueID", strings.Join(foundIssue, "-"), "err", err)
			continue
		}

		if _, ok := dao.IsProjectMember(gi.db, gi.User.ID, issue.ProjectId); !ok {
			continue
		}

		userName := user.GetName()
		if !userFound {
			userName = commit.Author.Name
		}

		msg := fmt.Sprintf("<p><strong>%s</strong> упомянул эту задачу в <a target=\"_blank\" rel=\"nofollow\" href=\"%s\">коммите</a> проекта <a target=\"_blank\" rel=\"nofollow\" href=\"%s\">%s</a>:</p><pre><code>%s</code></pre>",
			userName,
			commit.URL,
			event.Project.WebUrl,
			event.Project.PathWithNamespace,
			commit.Message,
		)

		err := gi.bl.CreateIssueComment(issue, *gi.User, msg, nil, false, commit.ID)
		if err != nil {
			log.Error("Create issue comment from webhook", "err", err)
			continue
		}

		// BAK-268
		/*link := dao.IssueLink{
		  	Id:          dao.GenID(),
		  	CreatedById: &gi.User.ID,
		  	IssueId:     issue.ID.String(),
		  	ProjectId:   issue.ProjectId,
		  	WorkspaceId: issue.WorkspaceId,
		  	Url:         commit.URL,
		  	Title:       commit.Title,
		  }

		  if err := gi.db.Create(&link).Error; err != nil {
		  	log.Error("Create issue link from webhook", "err", err)
		  }*/
	}
}
