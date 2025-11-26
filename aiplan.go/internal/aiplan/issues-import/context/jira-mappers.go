// Пакет предоставляет контекст для преобразования данных из Jira в формат, используемый в системе AIPlan.
//
// Основные возможности:
//   - Преобразование информации об issue, включая поля, ссылки, вложения, комментарии, метки, назначенных пользователей и наблюдателей.
//   - Интеграция с сервисами Jira и AIPlan для получения и сохранения данных.
//   - Обработка различных типов ссылок между issue.
package context

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/issues-import/entity"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/issues-import/utils"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/andygrunwald/go-jira"
	"github.com/gofrs/uuid"
	"golang.org/x/sync/errgroup"
)

type MapperContext struct {
	c         *ImportContext
	origIssue *jira.Issue
	issue     *dao.Issue

	g *errgroup.Group
}

func (c *ImportContext) mapJiraIssue(issue *jira.Issue) (*dao.Issue, error) {
	if c.Issues.Contains(issue.Key) {
		return nil, nil
	}

	st := time.Now()

	sequenceId, _ := strconv.Atoi(strings.Split(issue.Key, "-")[1])

	state, err := c.States.Get(issue.Fields.Status.ID)
	if err != nil {
		return nil, err
	}

	// Reporter is the current author of issue. Creator field is who created issue. Jira allow to change author in web
	author, err := c.Users.Get(c.getJiraUserUsername(*issue.Fields.Reporter))
	if err != nil {
		return nil, err
	}

	i := &dao.Issue{
		ID:              dao.GenUUID(),
		Name:            issue.Fields.Summary,
		DescriptionHtml: issue.RenderedFields.Description,
		DescriptionType: 1,
		StateId:         &state.ID,
		Priority:        c.mapJiraPriority(*issue.Fields.Priority),
		SequenceId:      sequenceId,
		CreatedAt:       time.Time(issue.Fields.Created),
		UpdatedAt:       time.Time(issue.Fields.Updated),
		CreatedById:     author.ID,
		ProjectId:       c.Project.ID,
		WorkspaceId:     c.Project.WorkspaceId,
	}

	mapperContext := MapperContext{
		c:         c,
		issue:     i,
		origIssue: issue,

		g: new(errgroup.Group),
	}

	mapperContext.MapParent()

	mapperContext.g.Go(mapperContext.MapLinks)

	mapperContext.g.Go(mapperContext.MapAttachments)

	mapperContext.g.Go(mapperContext.MapComments)

	mapperContext.g.Go(mapperContext.MapLabels)

	mapperContext.g.Go(mapperContext.MapAssignees)

	mapperContext.g.Go(mapperContext.MapWatchers)

	mapperContext.g.Go(mapperContext.MapReleases)

	if err := mapperContext.g.Wait(); err != nil {
		c.Log.Error("Mapper failed", "err", err)
		return nil, err
	}

	c.Issues.Put(issue.Key, *i)

	if time.Since(st) > time.Second*4 {
		c.Log.Debug("Slow mapping", "time", time.Since(st))
	}

	return i, nil
}

func (mc *MapperContext) MapParent() error {
	if mc.origIssue.Fields.Parent != nil {
		parent, err := mc.c.Issues.GetNoLock(mc.origIssue.Fields.Parent.Key)
		if err != nil {
			return err
		}
		mc.issue.ParentId = uuid.NullUUID{UUID: parent.ID, Valid: true}
		mc.issue.SortOrder = mc.c.SortOrder.GetNext(mc.issue.ParentId)
	}
	return nil
}

func (mc *MapperContext) MapLinks() error {
	if len(mc.origIssue.Fields.IssueLinks) > 0 {
		for _, link := range mc.origIssue.Fields.IssueLinks {
			if mc.c.BlockLinkID != "" && link.Type.ID == mc.c.BlockLinkID {
				// Outward - текущая задача воздействует на внешнюю
				// Inward - текущая задача под влиянием внешней
				if link.OutwardIssue != nil {
					if prj, _ := utils.ParseKey(*link.OutwardIssue); prj != mc.c.ProjectKey {
						mc.c.Log.Debug("Issue from another project", "key", link.OutwardIssue.Key)

						mc.c.IssueLinks.Append(mc.getAIPlanIssueLink(link))
					} else {
						mc.c.Blocks.Put(link.OutwardIssue.Key, &dao.IssueBlocker{
							Id:          dao.GenUUID(),
							BlockedById: mc.issue.ID,
							ProjectId:   mc.c.Project.ID,
							WorkspaceId: mc.c.Project.WorkspaceId,
						})
					}
				} else if link.InwardIssue != nil {
					if prj, _ := utils.ParseKey(*link.InwardIssue); prj != mc.c.ProjectKey {
						mc.c.Log.Debug("Issue from another project", "key", link.InwardIssue.Key)

						mc.c.IssueLinks.Append(mc.getAIPlanIssueLink(link))
					} else {
						mc.c.Blocked.Put(link.InwardIssue.Key, &dao.IssueBlocker{
							Id:          dao.GenUUID(),
							BlockId:     mc.issue.ID,
							ProjectId:   mc.c.Project.ID,
							WorkspaceId: mc.c.Project.WorkspaceId,
						})
					}
				}
				continue
			}

			// Linked issues
			if mc.c.RelateLinkMapper.Match(link.Type.ID) {
				outIssue := utils.GetNotNil(link.OutwardIssue, link.InwardIssue)
				if outIssue != nil {
					if prj, _ := utils.ParseKey(*outIssue); prj != mc.c.ProjectKey {
						mc.c.Log.Debug("Issue from another project", "key", outIssue.Key)

						mc.c.IssueLinks.Append(mc.getAIPlanIssueLink(link))
					} else {
						if mc.origIssue.Key < outIssue.Key {
							mc.c.Linked.Put(entity.RawLinkedIssues{Key1: mc.origIssue.Key, Key2: outIssue.Key})
						} else {
							mc.c.Linked.Put(entity.RawLinkedIssues{Key1: outIssue.Key, Key2: mc.origIssue.Key})
						}
					}
				}
				continue
			}

			// Unmapped links to IssueLink
			mc.c.IssueLinks.Append(mc.getAIPlanIssueLink(link))
		}
	}

	// Link for original issue
	mc.c.IssueLinks.Append(dao.IssueLink{
		Id:          dao.GenUUID(),
		IssueId:     mc.issue.ID,
		Title:       mc.origIssue.Key,
		Url:         utils.GetJiraIssueURL(mc.origIssue).String(),
		ProjectId:   mc.c.Project.ID,
		WorkspaceId: mc.c.Project.WorkspaceId,
	})

	return nil
}

func (mc *MapperContext) MapAttachments() error {
	for _, attachment := range mc.origIssue.Fields.Attachments {
		attributes := make(map[string]interface{})
		attributes["name"] = attachment.Filename
		attributes["size"] = attachment.Size

		dstID := dao.GenUUID()
		mc.c.Attachments.Put(attachment.ID, &entity.Attachment{
			JiraKey:    mc.origIssue.Key,
			DstAssetID: dstID,
			IssueAttachment: &dao.IssueAttachment{
				Id:          dao.GenUUID(),
				AssetId:     dstID,
				Attributes:  attributes,
				IssueId:     mc.issue.ID,
				ProjectId:   mc.c.Project.ID,
				WorkspaceId: mc.c.Project.WorkspaceId,
			},
			JiraAttachment: attachment,
		})
	}

	desc, err := mc.c.replaceAttachments(mc.origIssue.RenderedFields.Description, mc.issue, nil)
	if err == nil {
		mc.issue.DescriptionHtml = desc
	} else {
		mc.c.Log.Error("Replace attachments", "issue", mc.origIssue.Key, "err", err)
	}

	return nil
}

func (mc *MapperContext) MapComments() error {
	if mc.origIssue.Fields.Comments != nil {
		for _, comment := range mc.origIssue.Fields.Comments.Comments {
			com, err := mc.c.mapJiraComment(comment, mc.issue.ID.String())
			if err != nil {
				mc.c.Log.Error("Parse jira comment", "key", mc.origIssue.Key, "commentID", comment.ID, "err", err)
				continue
			}

			mc.c.IssueComments.Put(com.OriginalId.String, *com)
		}

		var wg sync.WaitGroup
		// Find rendered html
		for _, rendered := range mc.origIssue.RenderedFields.Comments.Comments {
			wg.Add(1)
			go func(rendered *jira.Comment) {
				defer wg.Done()
				com := mc.c.IssueComments.Get(rendered.ID)
				body, err := mc.c.replaceAttachments(rendered.Body, nil, &com)
				if err != nil {
					mc.c.Log.Error("Replace attachments in comment", "commentID", com.OriginalId, "err", err)
					return
				}
				com.CommentHtml = types.RedactorHTML{Body: body}

				mc.c.IssueComments.Put(com.OriginalId.String, com)
			}(rendered)
		}
		wg.Wait()
	}
	return nil
}

func (mc *MapperContext) MapLabels() error {
	for _, label := range mc.origIssue.Fields.Labels {
		issueLabel, _ := mc.c.Labels.Get(label)
		if issueLabel.ID.IsNil() {
			issueLabel = dao.Label{
				ID:          dao.GenUUID(),
				Name:        label,
				ProjectId:   mc.c.Project.ID,
				WorkspaceId: mc.c.Project.WorkspaceId,
			}
			mc.c.Labels.Put(label, issueLabel)
		}

		mc.c.IssueLabels.Append(dao.IssueLabel{
			Id:          dao.GenUUID(),
			IssueId:     mc.issue.ID.String(),
			LabelId:     issueLabel.ID,
			ProjectId:   mc.c.Project.ID,
			WorkspaceId: mc.c.Project.WorkspaceId,
		})
	}

	return nil
}

func (mc *MapperContext) MapAssignees() error {
	if mc.origIssue.Fields.Assignee != nil {
		assignee, err := mc.c.Users.Get(mc.c.getJiraUserUsername(*mc.origIssue.Fields.Assignee))
		if err != nil {
			return err
		}
		mc.c.IssueAssignees.Put(dao.IssueAssignee{
			Id:          dao.GenUUID(),
			CreatedAt:   time.Now(),
			AssigneeId:  assignee.ID,
			IssueId:     mc.issue.ID,
			ProjectId:   mc.issue.ProjectId,
			WorkspaceId: mc.issue.WorkspaceId,
		})
	}
	return nil
}

func (mc *MapperContext) MapWatchers() error {
	watchesAPIEndpoint := fmt.Sprintf("rest/api/2/issue/%s/watchers", mc.origIssue.ID)

	req, err := mc.c.Client.NewRequest("GET", watchesAPIEndpoint, nil)
	if err != nil {
		return err
	}

	watchers := new(jira.Watches)
	_, err = mc.c.Client.Do(req, watchers)
	if err != nil {
		return err
	}

	for _, jiraWatcher := range watchers.Watchers {
		watcher, err := mc.c.Users.Get(mc.c.getJiraUserUsername(*jiraWatcher))
		if err != nil {
			return err
		}
		mc.c.IssueWatchers.Put(dao.IssueWatcher{
			Id:          dao.GenUUID(),
			CreatedAt:   time.Now(),
			WatcherId:   watcher.ID,
			IssueId:     mc.issue.ID,
			ProjectId:   mc.issue.ProjectId,
			WorkspaceId: mc.issue.WorkspaceId,
		})
	}
	return nil
}

func (mc *MapperContext) MapReleases() error {
	for _, version := range mc.origIssue.Fields.FixVersions {
		if !mc.c.ReleasesTags.Contains(version.ID) {
			mc.c.ReleasesTags.Put(version.ID, dao.Label{
				ID:          dao.GenUUID(),
				CreatedAt:   time.Now(),
				Name:        version.Name,
				Description: version.Description,
				ProjectId:   mc.issue.ProjectId,
				WorkspaceId: mc.issue.WorkspaceId,
			})
		}
		tag := mc.c.ReleasesTags.Get(version.ID)

		mc.c.IssueLabels.Append(dao.IssueLabel{
			Id:          dao.GenUUID(),
			IssueId:     mc.issue.ID.String(),
			LabelId:     tag.ID,
			ProjectId:   mc.issue.ProjectId,
			WorkspaceId: mc.issue.WorkspaceId,
		})
	}

	return nil
}

func (mc *MapperContext) getAIPlanIssueLink(link *jira.IssueLink) dao.IssueLink {
	var linkType string

	if link.OutwardIssue != nil {
		linkType = link.Type.Outward
	} else if link.InwardIssue != nil {
		linkType = link.Type.Inward
	}
	outIssue := utils.GetNotNil(link.OutwardIssue, link.InwardIssue)
	outerIssueKey := outIssue.Key
	u := utils.GetJiraIssueURL(outIssue)

	project, is := utils.ParseRawKey(outerIssueKey)
	if project == mc.c.ProjectKey {
		// Change URL to internal issue
		u.Path = fmt.Sprintf("/%s/projects/%s/issues/%s", mc.c.TargetWorkspaceID, mc.c.Project.ID, is)
		u.Host = ""
		u.Scheme = ""
	}
	linkUrl := mc.c.WebURL.ResolveReference(u)

	return dao.IssueLink{
		Id:          dao.GenUUID(),
		IssueId:     mc.issue.ID,
		Title:       fmt.Sprintf("%s %s", linkType, outerIssueKey),
		Url:         linkUrl.String(),
		ProjectId:   mc.c.Project.ID,
		WorkspaceId: mc.c.Project.WorkspaceId,
	}
}
