// Пакет для импорта данных из Jira.
// Содержит функции для аутентификации в Jira, получения информации о проектах, и запуска процесса импорта.
// Основные возможности:
//   - Аутентификация в Jira через Basic Auth.
//   - Получение списка проектов, типов ссылок и приоритетов Jira.
//   - Запуск процесса импорта Jira проекта, включая взаимодействие с базой данных и отправку уведомлений.
package issues_import

import (
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/aisa-it/aiplan/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/internal/aiplan/issues-import/context"
	"github.com/aisa-it/aiplan/internal/aiplan/issues-import/entity"
	importErrors "github.com/aisa-it/aiplan/internal/aiplan/issues-import/errors"
	"github.com/aisa-it/aiplan/internal/aiplan/issues-import/steps"
	"github.com/aisa-it/aiplan/internal/aiplan/issues-import/steps/db"
	"github.com/aisa-it/aiplan/internal/aiplan/issues-import/utils"
	jira "github.com/andygrunwald/go-jira"

	"github.com/hashicorp/go-retryablehttp"
)

func init() {
	// RunningImports clean loop
	go func() {
		for {
			time.Sleep(time.Minute * 20)
			for _, context := range RunningImports.Array() {
				if context.Finished && time.Since(context.EndAt) > time.Hour {
					RunningImports.Delete(context.ProjectKey)
				}
			}
		}
	}()
}

func newJiraClient(login string, token string, host string) (*jira.Client, error) {
	tp := &jira.BasicAuthTransport{
		Username: login,
		Password: token,
	}

	cl := retryablehttp.NewClient()
	cl.RetryMax = 5
	cl.RetryWaitMin = time.Second * 5
	cl.HTTPClient.Transport = tp
	cl.Logger = slog.Default()

	client, err := jira.NewClient(cl.StandardClient(), host)
	if err != nil {
		return nil, err
	}

	_, resp, err := client.User.GetSelf()
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusForbidden {
			return nil, importErrors.ErrJiraUnauthorized
		}
		return nil, err
	}

	return client, nil
}

// GetJiraInfo - получение первоначальной информации о сущностях жиры для дальнейшего мапинга
func (is *ImportService) GetJiraInfo(login string, token string, host string) (*entity.JiraInfo, error) {
	client, err := newJiraClient(login, token, host)
	if err != nil {
		return nil, err
	}

	projects, _, err := client.Project.GetList()
	if err != nil {
		return nil, err
	}

	linkTypes, err := utils.GetLinkTypes(client)
	if err != nil {
		return nil, err
	}

	priorities, _, err := client.Priority.GetList()
	if err != nil {
		return nil, err
	}

	return &entity.JiraInfo{Projects: *projects, LinkTypes: linkTypes, Priorities: priorities}, nil
}

func (is *ImportService) StartJiraProjectImport(webURL *url.URL, user dao.User, login string, token string, host string, projectKey string, targetWorkspaceId string, blockLinkID string, relatesLinkIds entity.LinkMapper, prioritiesMapping entity.PrioritiesMapping) (*context.ImportContext, error) {
	client, err := newJiraClient(login, token, host)
	if err != nil {
		return nil, err
	}

	context, err := context.NewImportContext(
		webURL,
		client,
		is.db,
		is.notifyService,
		is.storage,
		user,
		projectKey,
		targetWorkspaceId,
		blockLinkID,
		relatesLinkIds,
		prioritiesMapping,
	)
	if err != nil {
		return nil, err
	}

	go func() {
		steps.RunImportSteps(context)

		// If import canceled
		if context.Finished {
			is.memDB.Model(&Import{ID: context.ID}).UpdateColumn("finished", true)
			return
		}

		err := db.RunDBSteps(context, is.db)

		// If import canceled
		if context.Finished {
			is.memDB.Model(&Import{ID: context.ID}).UpdateColumn("finished", true)
			return
		}

		context.Log.Info("Jira import finished", "project", context.ProjectKey)
		context.EndAt = time.Now()
		context.Finished = true
		is.memDB.Model(&Import{ID: context.ID}).UpdateColumn("finished", true)

		// If db save returns error
		if err != nil {
			context.Log.Error("Save jira project to DB", "err", err)
			context.Error = importErrors.ErrSaveDB

			context.Log.Info("Clean minio assets", "count", len(context.AssetIds))
			for _, id := range context.AssetIds {
				if err := is.storage.Delete(id); err != nil {
					context.Log.Error("Delete minio asset", "id", id, "err", err)
				}
			}
			return
		}

		// Email notification with password
		if context.NotifyNewMembers {
			for _, user := range context.UsersToCreate {
				if !user.IsActive {
					continue
				}
				if err := is.notifyService.NewUserPasswordNotify(user, user.Password); err != nil {
					context.Log.Error("Send Jira user password", "err", err)
				}
			}
		}

		if err := is.db.Save(&dao.ImportedProject{
			Id:                dao.GenUUID(),
			Type:              "jira",
			ProjectKey:        context.ProjectKey,
			StartAt:           context.StartAt,
			EndAt:             context.EndAt,
			TotalIssues:       context.Counters.TotalIssues,
			TotalAttachments:  context.Counters.TotalAttachments,
			NewUsers:          len(context.UsersToCreate),
			TargetWorkspaceId: context.TargetWorkspaceID,
			TargetProjectId:   context.Project.ID,
		}).Error; err != nil {
			context.Log.Error("Save ImportedProject", "err", err)
		}

		// Fetch URL
		context.Project.AfterFind(is.db)

		if err := is.notifyService.JiraImportEndNotify(user, &context.Project,
			context.Counters.TotalIssues,
			context.Counters.TotalAttachments,
			context.Users.Len()); err != nil {
			context.Log.Error("Send jira import finish email notification", "err", err)
		}
	}()

	if err := is.memDB.Save(&Import{
		ID:                context.ID,
		TargetWorkspaceID: context.TargetWorkspaceID,
		ActorID:           user.ID,
		ProjectKey:        context.ProjectKey,
		Context:           context,
		StartAt:           time.Now(),
	}).Error; err != nil {
		return nil, err
	}

	return context, nil
}
