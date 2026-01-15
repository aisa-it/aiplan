// Пакет `integrations` содержит логику для интеграции с Github. Он обрабатывает события push и создает комментарии к задачам в системе на основе информации из коммитов Github.
//
// Основные возможности:
//   - Регистрация webhook для получения событий push из Github.
//   - Обработка событий push и извлечение информации о коммитах.
//   - Поиск соответствующей задачи в системе и создание комментария с ссылкой на коммит.
//   - Интеграция с Telegram для логирования активности.
package integrations

import (
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	tracker "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/activity-tracker"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/business"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications/tg"
	"github.com/gofrs/uuid"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	filestorage "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/file-storage"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

var logGithub *slog.Logger = slog.With(slog.String("integration", "github"))

type GithubIntegration struct {
	Integration
}

type githubUser struct {
	Date     string  `json:"date"`
	Email    *string `json:"email"`
	Name     string  `json:"name"`
	Username string  `json:"username"`
}

type githubCommit struct {
	Added     []string   `json:"added"`
	Author    githubUser `json:"author"`
	Committer githubUser `json:"committer"`
	Distinct  bool       `json:"distinct"`
	Id        string     `json:"id"`
	Message   string     `json:"message"`
	Modified  []string   `json:"modified"`
	Removed   []string   `json:"removed"`
	Timestamp string     `json:"timestamp"`
	TreeId    string     `json:"treeId"`
	Url       string     `json:"url"`
}

type GithubPushEvent struct {
	After   string         `json:"after"`
	BaseRef *string        `json:"base_ref"`
	Before  string         `json:"before"`
	Commits []githubCommit `json:"commits"`

	Compare    string        `json:"compare"`
	Created    bool          `json:"created"`
	Deleted    bool          `json:"deleted"`
	Forced     bool          `json:"forced"`
	HeadCommit *githubCommit `json:"headCommit"`
	Pusher     githubUser    `json:"pusher"`
	Ref        string        `json:"ref"`
	Repository struct {
		Url      string `json:"url"`
		FullName string `json:"full_name"`
	} `json:"repository"`
	Sender struct {
	} `json:"sender"`
}

func NewGithubIntegration(db *gorm.DB, tS *tg.TgService, fs filestorage.FileStorage, tr *tracker.ActivitiesTracker, bl *business.Business) *GithubIntegration {
	username := "github_integration"
	i := &GithubIntegration{
		Integration: Integration{
			Name:        "Github",
			Description: "Интеграция с Github. Создает комментарий на основе коммита в подключенном репозитории по номеру задачи(пример commit msg: FAB-2 fix). После подключения интеграции необходимо добавить webhook в репозиторий с url /api/integrations/webhooks/github/{{token}}/  где token - токен доступа из настроек пространства.",
			User: &dao.User{
				Email:         "github@aiplan.ru",
				Password:      "",
				FirstName:     "Github",
				Username:      &username,
				Theme:         types.Theme{},
				IsActive:      true,
				IsIntegration: true,
			},
			AvatarSVG: `<svg width="98" height="96" xmlns="http://www.w3.org/2000/svg"><path fill-rule="evenodd" clip-rule="evenodd" d="M48.854 0C21.839 0 0 22 0 49.217c0 21.756 13.993 40.172 33.405 46.69 2.427.49 3.316-1.059 3.316-2.362 0-1.141-.08-5.052-.08-9.127-13.59 2.934-16.42-5.867-16.42-5.867-2.184-5.704-5.42-7.17-5.42-7.17-4.448-3.015.324-3.015.324-3.015 4.934.326 7.523 5.052 7.523 5.052 4.367 7.496 11.404 5.378 14.235 4.074.404-3.178 1.699-5.378 3.074-6.6-10.839-1.141-22.243-5.378-22.243-24.283 0-5.378 1.94-9.778 5.014-13.2-.485-1.222-2.184-6.275.486-13.038 0 0 4.125-1.304 13.426 5.052a46.97 46.97 0 0 1 12.214-1.63c4.125 0 8.33.571 12.213 1.63 9.302-6.356 13.427-5.052 13.427-5.052 2.67 6.763.97 11.816.485 13.038 3.155 3.422 5.015 7.822 5.015 13.2 0 18.905-11.404 23.06-22.324 24.283 1.78 1.548 3.316 4.481 3.316 9.126 0 6.6-.08 11.897-.08 13.526 0 1.304.89 2.853 3.316 2.364 19.412-6.52 33.405-24.935 33.405-46.691C97.707 22 75.788 0 48.854 0z" fill="#24292f"/></svg>`,

			db:              db,
			telegramService: tS,
			fileStorage:     fs,
			tracker:         tr,
			bl:              bl,
		},
	}

	if err := i.FetchUser(); err != nil {
		logGithub.Error("Fetch user", "err", err)
		return nil
	}

	return i
}

func (ghi *GithubIntegration) RegisterWebhook(g *echo.Group) {
	g.POST("github/:token/", func(c echo.Context) error {
		event := c.Request().Header.Get("X-GitHub-Event")
		token := c.Param("token")
		var workspace dao.Workspace
		if err := ghi.db.Where("integration_token = ?", token).First(&workspace).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return c.String(http.StatusNotFound, "workspace not found")
			}
			logGithub.Error("Get workspace", "err", err)
			return c.NoContent(http.StatusInternalServerError)
		}
		switch event {
		case "push":
			var eventData GithubPushEvent
			if err := c.Bind(&eventData); err != nil {
				logGithub.Error("Bind gitlab webhook request", "err", err)
				return c.NoContent(http.StatusBadRequest)
			}
			go ghi.GithubPushEvent(eventData, workspace)
		default:
			return c.String(http.StatusBadRequest, "unsupported event type")
		}
		return c.NoContent(http.StatusOK)
	})
}

func (ghi *GithubIntegration) GithubPushEvent(event GithubPushEvent, workspace dao.Workspace) {
	for _, commit := range event.Commits {
		var exist bool
		if err := ghi.db.Model(&dao.IssueComment{}).
			Select("EXISTS(?)",
				ghi.db.Model(&dao.IssueComment{}).
					Select("1").
					Where("actor_id = ?", ghi.User.ID).
					Where("integration_meta = ?", commit.Id),
			).
			Find(&exist).Error; err != nil {
			logGithub.Error("Check commit comment exists")
			continue
		}
		if exist {
			continue
		}

		userFound := true
		var user dao.User

		if err := ghi.db.Where("email = ?", commit.Author.Email).First(&user).Error; err != nil {
			if err != gorm.ErrRecordNotFound {
				logGithub.Error("Find user by email", "err", err)
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
		if err := ghi.db.Where("project_id = (?)",
			ghi.db.
				Select("id").
				Model(dao.Project{}).
				Where("workspace_id = ?", workspace.ID).
				Where("identifier = ?", foundIssue[0])).
			Where("sequence_id = ?", foundIssue[1]).
			Joins("Workspace").
			First(&issue).Error; err != nil {
			logGithub.Error("Find issue from gitlab event", "issueID", strings.Join(foundIssue, "-"), "err", err)
			continue
		}
		if _, ok := dao.IsProjectMember(ghi.db, ghi.User.ID, issue.ProjectId); !ok {
			continue
		}

		userName := user.GetName()
		if !userFound {
			userName = commit.Author.Name
		}
		msg := fmt.Sprintf("<p><strong>%s</strong> упомянул эту задачу в <a target=\"_blank\" rel=\"nofollow\" href=\"%s\">коммите</a> проекта <a target=\"_blank\" rel=\"nofollow\" href=\"%s\">%s</a>:</p><pre><code>%s</code></pre>",
			userName,
			commit.Url,
			event.Repository.Url,
			event.Repository.FullName,
			commit.Message,
		)

		err := ghi.bl.CreateIssueComment(issue, *ghi.User, msg, uuid.Nil, false, commit.Id)
		if err != nil {
			logGithub.Error("Create issue comment from webhook", "err", err)
			continue
		}

		link := dao.IssueLink{
			Id:          dao.GenUUID(),
			CreatedById: uuid.NullUUID{UUID: ghi.User.ID, Valid: true},
			IssueId:     issue.ID,
			ProjectId:   issue.ProjectId,
			WorkspaceId: issue.WorkspaceId,
			Url:         commit.Url,
			Title:       commit.Message,
		}

		if err := ghi.db.Create(&link).Error; err != nil {
			logGithub.Error("Create issue link from webhook", "err", err)
		}
	}
}
