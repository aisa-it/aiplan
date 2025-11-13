// Пакет содержит шаги импорта данных из Jira в систему.
//
// Основные возможности:
//   - Загрузка пользователей, проектов и связанных данных.
//   - Импорт задач, ссылок и аватаров пользователей.
//   - Обработка ошибок и логирование действий.
//   - Поддержка параллельной обработки шагов для ускорения импорта.
package steps

import (
	"log/slog"
	"sync"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	filestorage "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/file-storage"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/issues-import/context"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/issues-import/entity"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/issues-import/errors"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

// Количество горутин импорта задач
const importGoroutines = 10

var importSteps []func(*context.ImportContext) error = []func(*context.ImportContext) error{
	GetUsersStep,
	GetProjectStep,
	EmailNotifyStep,
	GetIssuesStep,
	DownloadAttachmentsStep,
	MapLinksStep,
	PrepareMembershipsStep,
	AvatarsStep,
}

func RunImportSteps(context *context.ImportContext) {
	for _, fn := range importSteps {
		if context.Finished {
			return
		}

		if err := fn(context); err != nil {
			context.EndAt = time.Now()
			context.Finished = true
			return
		}
	}
}

func GetUsersStep(context *context.ImportContext) error {
	context.Log.Info("Get users from project")
	if err := context.FetchProjectUsers(); err != nil {
		context.Log.Error("Get users from project", "err", err)
		return err
	}
	context.Log.Info("Fetch users done", "total", context.Users.Len())
	return nil
}

func GetProjectStep(context *context.ImportContext) error {
	context.Log.Info("Get project")
	return context.GetProject(context.ProjectKey)
}

func EmailNotifyStep(context *context.ImportContext) error {
	if err := context.Es.JiraImportStartNotify(context.ImportAuthor, &context.Project); err != nil {
		context.Log.Error("Send start jira import email notification", "err", err)
	}
	return nil
}

func GetIssuesStep(context *context.ImportContext) error {
	context.Log.Info("Get issues from project", "total", context.Counters.TotalIssues)
	if err := context.GetProjectIssues(context.ProjectKey); err != nil && err != errors.ErrCanceled {
		context.Log.Error("Get issues from project", "err", err)
		context.Error = errors.ErrGetProjectInfo
		return err
	}

	return nil
}

func DownloadAttachmentsStep(c *context.ImportContext) error {
	if c.IgnoreAttachments {
		return nil
	}

	c.Log.Info("Copy attachments to aiplan storage", "count", c.Attachments.Len())
	c.Stage = "attachments"
	c.Counters.TotalAttachments = c.Attachments.Len()
	var wg sync.WaitGroup
	attachmentChan := make(chan *entity.Attachment)

	for range importGoroutines {
		wg.Add(1)
		go func(ch <-chan *entity.Attachment) {
			defer wg.Done()
			for v := range ch {
				var downloadURL string
				if v.JiraAttachment != nil {
					downloadURL = v.JiraAttachment.Content
				} else if v.FullURL != nil {
					downloadURL = v.FullURL.String()
				} else {
					continue
				}

				urlLogAttr := slog.String("attachmentURL", downloadURL)

				success := false
				for i := range 5 {
					req, _ := c.Client.NewRequest("GET", downloadURL, nil)
					resp, err := c.Client.Do(req, nil)
					if err != nil {
						c.Log.Error("Download attachment", "try", i+1, urlLogAttr, "err", err)
						if resp != nil {
							resp.Body.Close()
						}
						time.Sleep(time.Second * 30)
						continue
					}

					var metadata filestorage.Metadata

					// Issue attachment
					if v.IssueAttachment != nil {
						metadata.WorkspaceId = v.IssueAttachment.WorkspaceId
						metadata.ProjectId = v.IssueAttachment.ProjectId
						metadata.IssueId = v.IssueAttachment.IssueId
					} else if v.InlineAsset != nil {
						// Inline asset
						metadata.WorkspaceId = *v.InlineAsset.WorkspaceId
						metadata.IssueId = v.InlineAsset.IssueId.UUID.String()
					}

					contentType := resp.Header.Get("content-type")
					fileSize := resp.ContentLength

					if err := c.Storage.SaveReaderWithBuf(
						resp.Body,
						fileSize,
						v.DstAssetID,
						contentType,
						&metadata,
						nil,
					); err != nil {
						c.Log.Error("Save attachment to minio", "try", i+1, urlLogAttr, "err", err)
						resp.Body.Close()
						time.Sleep(time.Second * 30)
						continue
					}

					c.AssetIds = append(c.AssetIds, v.DstAssetID)

					resp.Body.Close()
					break
				}
				c.Counters.ImportedAttachments.Add(1)

				var asset dao.FileAsset
				if v.InlineAsset != nil {
					asset = *v.InlineAsset
				} else if v.JiraAttachment != nil {
					asset = dao.FileAsset{
						Id:          v.DstAssetID,
						CreatedAt:   time.Now(),
						WorkspaceId: &v.IssueAttachment.WorkspaceId,
						Name:        v.JiraAttachment.Filename,
						FileSize:    v.JiraAttachment.Size,
					}
				}

				c.FileAssets.Append(asset)
				success = true

				if !success {
					c.Log.Error("Can't download jira attachment", urlLogAttr)
					c.BadAttachments.Append(v)
				}
			}
		}(attachmentChan)
	}

	c.Attachments.Range(func(k string, v *entity.Attachment) {
		attachmentChan <- v
	})

	close(attachmentChan)
	wg.Wait()

	c.BadAttachments.Range(func(i int, s *entity.Attachment) {
		c.Attachments.Delete(s.JiraAttachment.ID)
	})

	return nil
}

func MapLinksStep(context *context.ImportContext) error {
	// Blocks
	context.Blocks.Range(func(key string, blocks *dao.IssueBlocker) {
		i := context.Issues.GetLight(key)
		if i.ID.IsNil() {
			return
		}
		blocks.BlockId = i.ID
		context.Blockers.Put(blocks)
	})

	// Blocked by
	context.Blocked.Range(func(key string, blocks *dao.IssueBlocker) {
		i, _ := context.Issues.Get(key)
		blocks.BlockedById = i.ID
		context.Blockers.Put(blocks)
	})

	// Relates
	context.Linked.Range(func(key string, v entity.RawLinkedIssues) {
		i1, _ := context.Issues.Get(v.Key1)
		i2, _ := context.Issues.Get(v.Key2)
		context.LinkedIssues = append(context.LinkedIssues, dao.GetIssuesLink(i1.ID, i2.ID))
	})

	return nil
}

func PrepareMembershipsStep(context *context.ImportContext) error {
	// Workspace/project memberships
	context.Stage = "users"
	context.Counters.TotalUsers = context.Users.Len()
	context.Users.Range(func(k string, v dao.User) {
		if v.ID == "" {
			context.Log.Warn("Empty user", "key", k)
			return
		}

		if v.Username == nil {
			context.Log.Warn("Empty username", "email", v.Email)
		}

		var user dao.User
		if err := context.DB.Where("email = ?", v.Email).Or("username = ?", v.Username).First(&user).Error; err == gorm.ErrRecordNotFound {
			user = v
			// Add new user to create list
			context.UsersToCreate = append(context.UsersToCreate, user)
		}

		if !dao.IsWorkspaceMember(context.DB, user.ID, context.TargetWorkspaceID) {
			context.WorkspaceMembers = append(context.WorkspaceMembers,
				dao.WorkspaceMember{
					ID:          dao.GenID(),
					Role:        10,
					MemberId:    user.ID,
					WorkspaceId: context.TargetWorkspaceID,
				})
		}

		context.ProjectMembers = append(context.ProjectMembers,
			dao.ProjectMember{
				ID:          dao.GenID(),
				Role:        10,
				MemberId:    user.ID,
				ProjectId:   context.Project.ID,
				WorkspaceId: context.TargetWorkspaceID,
			})
	})

	// Set actual users count
	context.Counters.TotalUsers = len(context.UsersToCreate)

	return nil
}

func AvatarsStep(context *context.ImportContext) error {
	if context.IgnoreAttachments {
		return nil
	}

	context.Log.Info("Copy avatars to aiplan storage", "count", len(context.UsersToCreate))
	for i, user := range context.UsersToCreate {
		req, _ := context.Client.NewRawRequest("GET", user.Avatar, nil)

		resp, err := context.Client.Do(req, nil)
		if err != nil {
			context.Log.Error("Download avatar", "url", user.Avatar, "err", err)
		}

		fileAsset := dao.FileAsset{
			Id:        dao.GenUUID(),
			CreatedAt: time.Now(),
			Name:      *user.Username + ".png",
			FileSize:  int(resp.ContentLength),
		}

		if err := context.Storage.SaveReader(resp.Body, resp.ContentLength, fileAsset.Id, "image/png", nil, nil); err != nil {
			context.Log.Error("Save user avatar", "url", user.Avatar, "err", err)
			continue
		}
		context.AssetIds = append(context.AssetIds, fileAsset.Id)

		//context.FileAssets.Append(fileAsset)
		context.AvatarAssets = append(context.AvatarAssets, fileAsset)

		u := context.UsersToCreate[i]
		u.AvatarId = uuid.NullUUID{Valid: true, UUID: fileAsset.Id}
		context.UsersToCreate[i] = u

		context.Counters.ImportedUsers.Add(1)
	}

	return nil
}
