package notifications

import (
	"bytes"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	policy "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/redactor-policy"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"gorm.io/gorm"
)

type emailNotifyProject struct {
	service *EmailService
}

func newEmailNotifyProject(es *EmailService) *emailNotifyProject {
	if es == nil {
		return nil
	}
	return &emailNotifyProject{service: es}
}

func (e *emailNotifyProject) Process() {
	e.service.sending = true

	defer func() {
		e.service.sending = false
	}()

	var activities []dao.ProjectActivity
	if err := e.service.db.Unscoped().
		Joins("Project").
		Joins("Workspace").
		Joins("Actor").
		Order("project_activities.created_at").
		Where("notified = ?", false).
		Limit(100).
		Find(&activities).Error; err != nil {
		slog.Error("Get activities", slog.Int("interval", e.service.cfg.NotificationsSleep), "err", err)
		return
	}

	resultChan := make(chan []mail, 1)
	errorChan := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				errorChan <- fmt.Errorf("panic in process: %v", r)
			}
			close(resultChan)
			close(errorChan)
		}()

		sorter := projectActivitySorter{
			skipActivities: make([]dao.ProjectActivity, 0),
			Project:        make(map[string]projectActivity),
		}
		for i := range activities {
			sorter.sortEntity(e.service.db, activities[i])
		}

		var mailsToSend []mail

		for _, prAct := range sorter.Project {
			//err := prAct.Finalization(e.service.db)
			//if err != nil {
			//	slog.Error("issue activity finalization error: ", err)
			//	continue
			//}

			mailsToSend = append(mailsToSend, prAct.getMails(e.service.db)...)
		}
		resultChan <- mailsToSend
	}()

	select {
	case mailsToSend := <-resultChan:
		for _, m := range mailsToSend {
			if err := e.service.Send(m); err != nil {
				slog.Error("Send email notification", "mail", m.To, "err", err, "subj", m.Subject)
			}
		}
	case err := <-errorChan:
		slog.Error("Error processing ProjectActivities", "err", err)
	}

	if err := e.service.db.Transaction(func(tx *gorm.DB) error {
		for _, activity := range activities {
			if err := e.service.db.Model(&dao.ProjectActivity{}).
				Unscoped().
				Where("id = ?", activity.Id).
				Update("notified", true).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		slog.Error("Update notified flag in DB", "err", err)
	}
}

type projectMember struct {
	User                  dao.User
	ProjectLeader         bool
	ProjectAdmin          bool
	DefaultWatcher        bool
	DefaultAssigner       bool
	IssueMember           bool
	ProjectRole           int
	ProjectAuthorSettings types.ProjectMemberNS
	ProjectMemberSettings types.ProjectMemberNS
}

type projectActivitySorter struct {
	skipActivities []dao.ProjectActivity
	Project        map[string]projectActivity //map[issueId]
}

type projectActivity struct {
	Project    *dao.Project
	activities []dao.ProjectActivity
	users      map[string]projectMember //map[user.Email]
	AllMember  []dao.ProjectMember
	//commentActivityMap  map[string][]dao.IssueActivity // map[commentId]
	//commentActivityUser map[string]issueCommentAuthor  //map[user.Email]
}

func (as *projectActivitySorter) sortEntity(tx *gorm.DB, activity dao.ProjectActivity) {
	if activity.ProjectId != "" { // TODO check it
		activity.Project.Workspace = activity.Workspace
		if v, ok := as.Project[activity.ProjectId]; !ok {
			pa := newProjectActivity(tx, activity.Project)
			if pa != nil {
				if !pa.AddActivity(activity) {
					as.skipActivities = append(as.skipActivities, activity)
				}
				as.Project[activity.ProjectId] = *pa
			}
		} else {
			if !v.AddActivity(activity) {
				as.skipActivities = append(as.skipActivities, activity)
			}
			as.Project[activity.ProjectId] = v
		}
	}
	return
}

func newProjectActivity(tx *gorm.DB, project *dao.Project) *projectActivity {
	res := projectActivity{
		Project: project,
		users:   make(map[string]projectMember),
	}

	err := res.Project.AfterFind(tx)
	if err != nil {
		return nil
	}

	if err := tx.
		Joins("Member").
		Where("project_id = ?", project.ID).
		Find(&res.AllMember).Error; err != nil {
		return nil
	}

	memberMap := utils.SliceToMap(&res.AllMember, func(v *dao.ProjectMember) string {
		return v.MemberId
	})

	{ //add Leader
		if lead, ok := memberMap[project.ProjectLeadId]; ok && lead.Member != nil {
			res.users[memberMap[project.ProjectLeadId].Member.Email] = projectMember{
				User:                  *lead.Member,
				ProjectLeader:         true,
				ProjectRole:           lead.Role,
				ProjectMemberSettings: lead.NotificationSettingsEmail,
				ProjectAuthorSettings: lead.NotificationAuthorSettingsEmail,
			}
		}
	}

	for _, member := range memberMap {
		isAdmin := member.Role == types.AdminRole

		if pm, ok := res.users[member.Member.Email]; !ok {
			if member.Member == nil {
				continue
			}
			res.users[member.Member.Email] = projectMember{
				User:                  *member.Member,
				ProjectAdmin:          isAdmin,
				DefaultAssigner:       member.IsDefaultAssignee,
				DefaultWatcher:        member.IsDefaultWatcher,
				ProjectRole:           member.Role,
				ProjectMemberSettings: member.NotificationSettingsEmail,
				ProjectAuthorSettings: member.NotificationAuthorSettingsEmail,
			}
		} else {
			pm.ProjectAdmin = isAdmin
			pm.DefaultAssigner = member.IsDefaultAssignee
			pm.DefaultWatcher = member.IsDefaultWatcher
			pm.ProjectRole = member.Role
			pm.ProjectAuthorSettings = member.NotificationAuthorSettingsEmail
			pm.ProjectMemberSettings = member.NotificationSettingsEmail
			res.users[member.Member.Email] = pm
		}
	}
	return &res
}

func (pa *projectActivity) AddActivity(activity dao.ProjectActivity) bool {
	if pa.skip(activity) {
		return false
	}

	pa.activities = append(pa.activities, activity)
	return true
}

func (pa *projectActivity) skip(activity dao.ProjectActivity) bool {
	if activity.Verb != actField.VerbCreated {
		return true
	}
	if activity.Field != nil && *activity.Field != actField.Issue.String() {
		return true
	}
	return false
}

func (pa *projectActivity) getMails(tx *gorm.DB) []mail {
	mails := make([]mail, 0)
	subj := fmt.Sprintf("Обновления для %s/%s", pa.Project.Workspace.Slug, pa.Project.Identifier)
	for _, member := range pa.users {
		if !member.User.CanReceiveNotifications() {
			continue
		}

		if member.User.Settings.EmailNotificationMute {
			continue
		}

		var sendActivities []dao.ProjectActivity
		for _, activity := range pa.activities {

			if activity.NewIssue != nil {
				var issue dao.Issue
				if err := tx.
					Joins("State").
					Joins("Parent").
					Joins("Project").
					Joins("Workspace").
					Joins("Author").
					Preload("Assignees").
					Preload("Watchers").
					Where("issues.id = ?", activity.NewIssue.ID).First(&issue).Error; err != nil {
					continue
				}
				isWatcher := slices.Contains(issue.WatcherIDs, member.User.ID) || member.DefaultWatcher
				isAssignee := slices.Contains(issue.AssigneeIDs, member.User.ID) || member.DefaultAssigner

				if isWatcher || isAssignee || issue.CreatedById == member.User.ID {
					if issue.CreatedById == member.User.ID {
						if member.ProjectAuthorSettings.IsNotify(activity.Field, "project", activity.Verb, member.ProjectRole) {
							sendActivities = append(sendActivities, activity)
							continue
						}
						continue
					}
					if member.ProjectMemberSettings.IsNotify(activity.Field, "project", activity.Verb, member.ProjectRole) {
						sendActivities = append(sendActivities, activity)
						continue
					}
				}
				continue
			}

			if member.ProjectAdmin {
				if activity.Field != nil && *activity.Field == actField.Issue.String() {
					continue
				}
				sendActivities = append(sendActivities, activity)
				continue
			}
		}

		if len(sendActivities) == 0 {
			continue
		}

		content, textContent, err := getProjectNotificationHTML(tx, sendActivities, &member.User)
		if err != nil {
			slog.Error("Make project notification HTML", "err", err)
			continue
		}

		mails = append(mails, mail{
			To:          member.User.Email,
			Subject:     subj,
			Content:     content,
			TextContent: textContent,
		})
	}

	return mails
}

func getProjectNotificationHTML(tx *gorm.DB, activities []dao.ProjectActivity, targetUser *dao.User) (string, string, error) {
	result := ""
	//actorsChangesMap := make(map[string]map[string]dao.ProjectActivity)
	//actorsMap := make(map[string]dao.User)

	for _, act := range activities {
		result += newIssue(tx, targetUser, &act)
	}

	var templateBody dao.Template
	if err := tx.Where("name = ?", "body").First(&templateBody).Error; err != nil {
		return "", "", err
	}

	proj := activities[0].Project
	proj.Workspace = activities[0].Workspace
	proj.AfterFind(tx)

	var buff bytes.Buffer
	if err := templateBody.ParsedTemplate.Execute(&buff, struct {
		Issue         *dao.Issue
		Project       *dao.Project
		Title         string
		CreatedAt     time.Time
		Body          string
		CommentCount  int
		ActivityCount int
	}{
		Title:     proj.Name,
		CreatedAt: time.Now(), //TODO: timezone
		Body:      result,
		Project:   proj,
		Issue:     nil,
	}); err != nil {
		return "", "", err
	}

	content := buff.String()
	return content, htmlStripPolicy.Sanitize(content), nil
}

func newIssue(tx *gorm.DB, user *dao.User, act *dao.ProjectActivity) string {
	if act.Field != nil && *act.Field == actField.Issue.String() && act.Verb == actField.VerbCreated {
		var template dao.Template
		if err := tx.Where("name = ?", "issue_activity_new").First(&template).Error; err != nil {
			return ""
		}

		if act.NewIssue == nil {
			return ""
		}

		var issue dao.Issue

		if err := tx.
			Joins("State").
			Joins("Parent").
			Joins("Project").
			Joins("Workspace").
			Joins("Author").
			Preload("Assignees").
			Preload("Watchers").
			Where("issues.id = ?", act.NewIssue.ID).First(&issue).Error; err != nil {
			return ""
		}

		var p string
		if issue.Priority == nil {
			p = priorityTranslation["<nil>"]
		} else {
			p = priorityTranslation[*issue.Priority]
		}
		issue.Priority = &p
		description := replaceTablesToText(replaceImageToText(issue.DescriptionHtml))
		description = policy.ProcessCustomHtmlTag(description)
		description = prepareToMail(prepareHtmlBody(htmlStripPolicy, description))
		description = template.ReplaceTxtToSvg(description)
		var buf bytes.Buffer
		if err := template.ParsedTemplate.Execute(&buf, struct {
			Actor       *dao.User
			Issue       dao.Issue
			CreatedAt   time.Time
			Description string
		}{
			act.Actor,
			issue,
			act.CreatedAt.In((*time.Location)(&user.UserTimezone)),
			description,
		}); err != nil {
			slog.Error("Make project notification HTML", "err", err)
			return ""
		}

		return buf.String()
	}
	return ""
}
