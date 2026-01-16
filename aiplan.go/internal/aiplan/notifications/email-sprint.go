package notifications

import (
	"bytes"
	"fmt"
	"log/slog"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	policy "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/redactor-policy"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

type emailNotifySprint struct {
	service *EmailService
}

func newEmailNotifySprint(es *EmailService) *emailNotifySprint {
	if es == nil {
		return nil
	}
	return &emailNotifySprint{service: es}
}

func (e *emailNotifySprint) Process() {
	e.service.sending = true

	defer func() {
		e.service.sending = false
	}()

	var activities []dao.SprintActivity
	if err := e.service.db.Unscoped().
		Preload("Issue").
		Preload("Watchers").
		Joins("CreatedBy").
		Joins("Workspace").
		//Preload("Issue.Author").
		//Preload("Issue.Assignees").
		//Preload("Issue.Watchers").
		//Preload("Issue.State").
		//Preload("Issue.Parent").
		//Preload("Issue.Project").
		//Preload("Issue.Project.DefaultWatchersDetails", "is_default_watcher = ?", true).
		//Preload("Issue.Project.DefaultWatchersDetails.Member").
		//Preload("Issue.Parent.Project").
		Order("created_at").
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

		sorter := sprintActivitySorter{
			skipActivities: make([]dao.SprintActivity, 0),
			Sprint:         make(map[uuid.UUID]sprintActivity),
		}

		for i := range activities {
			sorter.sortEntity(activities[i])
		}

		var mailsToSend []mail

		for _, sprintAct := range sorter.Sprint {
			err := sprintAct.Finalization(e.service.db)
			if err != nil {
				slog.Error("Issue activity finalization error: ", err)
				continue
			}

			mailsToSend = append(mailsToSend, sprintAct.getMails(e.service.db)...)
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
		slog.Error("Error processing IssueActivities", "err", err)
	}

	if err := e.service.db.Transaction(func(tx *gorm.DB) error {
		for _, activity := range activities {
			if err := e.service.db.Model(&dao.IssueActivity{}).
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

func (ia *sprintActivity) Finalization(tx *gorm.DB) error {
	//if err := ia.getCommentNotify(tx); err != nil {
	//  return err
	//}
	if err := ia.getNotifySettings(tx); err != nil {
		return err
	}
	return nil
}

func (ia *sprintActivity) getNotifySettings(tx *gorm.DB) error {
	userIds := make([]uuid.UUID, 0, len(ia.users))
	for _, member := range ia.users {
		userIds = append(userIds, member.User.ID)
	}

	var wm []dao.WorkspaceMember
	if err := tx.
		Preload("Member").
		Where("workspace_id = ?", ia.sprint.WorkspaceId).
		Where("member_id IN (?)", userIds).
		Find(&wm).Error; err != nil {
		return err
	}

	for _, member := range wm {
		if v, ok := ia.users[member.Member.Email]; ok {
			v.WorkspaceAuthorSettings = member.NotificationAuthorSettingsEmail
			v.WorkspaceMemberSettings = member.NotificationSettingsEmail
			ia.users[member.Member.Email] = v
		}
		//if v, ok := ia.commentActivityUser[member.Member.Email]; ok {
		//  v.ProjectAuthorSettings = member.NotificationAuthorSettingsEmail
		//  v.ProjectMemberSettings = member.NotificationSettingsEmail
		//  ia.commentActivityUser[member.Member.Email] = v
		//}
	}
	return nil
}

type sprintActivitySorter struct {
	skipActivities []dao.SprintActivity
	Sprint         map[uuid.UUID]sprintActivity //map[issueId]
}

type sprintMember struct {
	User                    dao.User
	WorkspaceRole           int
	WorkspaceAuthorSettings types.WorkspaceMemberNS
	WorkspaceMemberSettings types.WorkspaceMemberNS
	SprintAuthor            bool
	Watcher                 bool
}

type sprintActivity struct {
	sprint     *dao.Sprint
	activities []dao.SprintActivity
	users      map[string]sprintMember //map[user.Email]

	//commentActivityMap  map[uuid.UUID][]dao.IssueActivity // map[commentId]
	//commentActivityUser map[string]issueCommentAuthor     //map[user.Email]
}

func (as *sprintActivitySorter) sortEntity(activity dao.SprintActivity) {
	if activity.SprintId != uuid.Nil { // TODO check it
		if v, ok := as.Sprint[activity.SprintId]; !ok {
			ia := newSprintActivity(activity.Sprint)
			if ia != nil {
				if !ia.AddActivity(activity) {
					as.skipActivities = append(as.skipActivities, activity)
				}
				as.Sprint[activity.SprintId] = *ia
			}
		} else {
			if !v.AddActivity(activity) {
				as.skipActivities = append(as.skipActivities, activity)
			}
			as.Sprint[activity.SprintId] = v
		}
	}
	return
}

func newSprintActivity(sprint *dao.Sprint) *sprintActivity {
	if sprint == nil {
		return nil
	}

	res := sprintActivity{
		sprint:     sprint,
		activities: make([]dao.SprintActivity, 0),
		users:      make(map[string]sprintMember),
		//commentActivityMap:  make(map[uuid.UUID][]dao.IssueActivity),
		//commentActivityUser: make(map[string]issueCommentAuthor),
	}

	{ //add author
		res.users[sprint.CreatedBy.Email] = sprintMember{
			User:         sprint.CreatedBy,
			SprintAuthor: true,
			Watcher:      false,
		}
	}

	{ // add watchers
		if res.sprint.Watchers != nil {
			for _, watcher := range res.sprint.Watchers {
				if im, ok := res.users[watcher.Email]; !ok {
					res.users[watcher.Email] = sprintMember{
						User:         watcher,
						SprintAuthor: false,
						Watcher:      true,
					}
				} else {
					im.Watcher = true
					res.users[watcher.Email] = im
				}
			}
		}
	}

	return &res
}

func (ia *sprintActivity) AddActivity(activity dao.SprintActivity) bool {
	if ia.skip(activity) {
		return false
	}

	ia.activities = append(ia.activities, activity)

	//if activity.Field != nil && *activity.Field == actField.Comment.Field.String() && activity.NewIdentifier.Valid {
	//  if activity.Verb == "created" || activity.Verb == "updated" {
	//    //TODO
	//    var arr []dao.IssueActivity
	//    if v, ok := ia.commentActivityMap[activity.NewIdentifier.UUID]; !ok {
	//      arr = append(arr, activity)
	//    } else {
	//      arr = append(v, activity)
	//    }
	//    ia.commentActivityMap[activity.NewIdentifier.UUID] = arr
	//  }
	//}
	return true
}

// Для пропуска активностей
func (ia *sprintActivity) skip(activity dao.SprintActivity) bool {
	//if activity.Verb == "cloned" {
	//  return true
	//}
	//if activity.Issue.Draft {
	//  return true
	//}
	//
	//if activity.Field != nil && *activity.Field == actField.StartDate.Field.String() {
	//  return true
	//}
	//
	//if activity.Field != nil && *activity.Field == actField.CompletedAt.Field.String() {
	//  return true
	//}
	//
	//if activity.Field != nil && *activity.Field == actField.Link.Field.String() && activity.Verb == "deleted" {
	//  return true
	//}

	return false
}

func (ia *sprintActivity) getMails(tx *gorm.DB) []mail {
	mails := make([]mail, 0)
	subj := fmt.Sprintf("Обновления для %s", ia.sprint.Name)
	for _, member := range ia.users {
		if !member.User.CanReceiveNotifications() {
			continue
		}

		if member.User.Settings.EmailNotificationMute {
			continue
		}

		var sendActivities []dao.SprintActivity

		for _, activity := range ia.activities {
			var authorNotify, memberNotify bool
			memberNotify = member.WorkspaceMemberSettings.IsNotify(activity.Field, actField.Sprint.Field, activity.Verb, member.WorkspaceRole)
			if activity.Sprint.CreatedById == member.User.ID {
				authorNotify = member.WorkspaceAuthorSettings.IsNotify(activity.Field, actField.Sprint.Field, activity.Verb, member.WorkspaceRole)
			}
			if (member.SprintAuthor && authorNotify) || (!member.SprintAuthor && memberNotify) {
				sendActivities = append(sendActivities, activity)
			}
		}

		if len(sendActivities) == 0 {
			continue
		}

		content, textContent, err := getSprintNotificationHTML(tx, sendActivities, &member.User)
		if err != nil {
			slog.Error("Make issue notification HTML", "err", err)
			continue
		}

		mails = append(mails, mail{
			To:          member.User.Email,
			Subject:     subj,
			Content:     content,
			TextContent: textContent,
		})
	}

	//for _, author := range ia.commentActivityUser {
	//	field := actField.Comment.Field.String()
	//
	//	if len(author.activities) == 0 {
	//		continue
	//	}
	//
	//	if author.User.CanReceiveNotifications() && !author.User.Settings.EmailNotificationMute && author.ProjectMemberSettings.IsNotify(&field, "issue", "all", author.ProjectRole) {
	//		content, textContent, err := getIssueNotificationHTML(tx, author.activities, &author.User)
	//		if err != nil {
	//			slog.Error("Make issue notification HTML", "err", err)
	//			continue
	//		}
	//		mails = append(mails, mail{
	//			To:          author.User.Email,
	//			Subject:     subj,
	//			Content:     content,
	//			TextContent: textContent,
	//		})
	//	}
	//}

	return mails
}

func getSprintNotificationHTML(tx *gorm.DB, activities []dao.SprintActivity, targetUser *dao.User) (string, string, error) {
	result := ""

	actorsChangesMap := make(map[uuid.UUID]map[string]dao.SprintActivity)
	actorsMap := make(map[uuid.UUID]dao.User)
	removeIssues := make([]dao.Issue, 0)
	addIssues := make(map[uuid.UUID]struct{})

	type IssueView struct {
		Issue dao.Issue
		IsNew bool
	}
	for _, activity := range activities {
		// sprint deletion
		//if activity.Field != nil && *activity.Field == actField.Sprint.Field.String() && activity.Verb == "deleted" {
		//	var template dao.Template
		//	if err := tx.Where("name = ?", "issue_activity_delete").First(&template).Error; err != nil {
		//		return "", "", err
		//	}
		//
		//	var buf bytes.Buffer
		//	if err := template.ParsedTemplate.Execute(&buf, struct {
		//		Actor     *dao.User
		//		Issue     dao.Issue
		//		CreatedAt time.Time
		//	}{
		//		activity.Actor,
		//		*activity.Issue,
		//		activity.CreatedAt.In((*time.Location)(&targetUser.UserTimezone)),
		//	}); err != nil {
		//		return "", "", err
		//	}
		//
		//	result += buf.String()
		//	continue
		//}

		// new issue
		//if activity.Field == nil {
		//	var template dao.Template
		//	if err := tx.Where("name = ?", "issue_activity_new").First(&template).Error; err != nil {
		//		return "", "", err
		//	}
		//	var p string
		//	if activity.Issue.Priority == nil {
		//		p = priorityTranslation["<nil>"]
		//	} else {
		//		p = priorityTranslation[*activity.Issue.Priority]
		//	}
		//	activity.Issue.Priority = &p
		//	description := replaceTablesToText(replaceImageToText(activity.Issue.DescriptionHtml))
		//	description = policy.ProcessCustomHtmlTag(description)
		//	description = prepareToMail(prepareHtmlBody(htmlStripPolicy, description))
		//	description = template.ReplaceTxtToSvg(description)
		//	var buf bytes.Buffer
		//	if err := template.ParsedTemplate.Execute(&buf, struct {
		//		Actor       *dao.User
		//		Issue       dao.Issue
		//		CreatedAt   time.Time
		//		Description string
		//	}{
		//		activity.Actor,
		//		*activity.Issue,
		//		activity.CreatedAt.In((*time.Location)(&targetUser.UserTimezone)),
		//		description,
		//	}); err != nil {
		//		return "", "", err
		//	}
		//
		//	result += buf.String()
		//	continue
		//}

		// comment

		changesMap, ok := actorsChangesMap[activity.ActorId.UUID]
		if !ok {
			changesMap = make(map[string]dao.SprintActivity)
		}
		field := *activity.Field

		//if field == actField.Priority.Field.String() {
		//	activity.NewValue = priorityTranslation[activity.NewValue]
		//	if activity.OldValue != nil {
		//		p := priorityTranslation[*activity.OldValue]
		//		activity.OldValue = &p
		//	} else {
		//		p := priorityTranslation["<nil>"]
		//		activity.OldValue = &p
		//	}
		//}
		//
		//if field == actField.TargetDate.Field.String() {
		//	newT, errNew := FormatDate(activity.NewValue, "02.01.2006 15:04", &targetUser.UserTimezone)
		//
		//	if activity.OldValue != nil {
		//		if oldT, errOld := FormatDate(*activity.OldValue, "02.01.2006 15:04", &targetUser.UserTimezone); errOld == nil {
		//			activity.OldValue = &oldT
		//		}
		//	}
		//
		//	if errNew == nil {
		//		activity.NewValue = newT
		//	}
		//}
		if field == actField.Issue.Field.String() {
			switch activity.Verb {
			case actField.VerbAdded:
				//addIssues = append(addIssues, *activity.NewSprintIssue)
			case actField.VerbRemoved:
				removeIssues = append(removeIssues, *activity.OldSprintIssue)
			}
		}

		if field == actField.Description.Field.String() {
			oldValue := replaceTablesToText(replaceImageToText(*activity.OldValue))
			newValue := replaceTablesToText(replaceImageToText(activity.NewValue))
			oldValue = policy.ProcessCustomHtmlTag(oldValue)
			newValue = policy.ProcessCustomHtmlTag(newValue)
			oldValue = prepareToMail(prepareHtmlBody(htmlStripPolicy, oldValue))
			newValue = prepareToMail(prepareHtmlBody(htmlStripPolicy, newValue))
			activity.OldValue = &oldValue
			activity.NewValue = newValue
		}

		changesMap[field] = activity
		actorsMap[activity.ActorId.UUID] = *activity.Actor
		actorsChangesMap[activity.ActorId.UUID] = changesMap
	}

	rrr := make([]IssueView, 0, len(activities[0].Sprint.Issues))

	for _, issue := range activities[0].Sprint.Issues {
		_, iok := addIssues[issue.ID]
		rrr = append(rrr, IssueView{issue, iok})
	}

	var template dao.Template
	if err := tx.Where("name = ?", "sprint_activity").First(&template).Error; err != nil {
		return "", "", err
	}
	activityCount := 0

	for userId, changesMap := range actorsChangesMap {
		context := struct {
			SprintURL    string
			Issues       []dao.Issue
			Changes      map[string]dao.SprintActivity
			AddIssues    []IssueView
			RemoveIssues []dao.Issue
			Actor        dao.User
			CreatedAt    time.Time
		}{
			activities[0].Sprint.URL.String(),
			activities[0].Sprint.Issues,
			changesMap,
			rrr,
			removeIssues,
			actorsMap[userId],
			time.Now().In((*time.Location)(&targetUser.UserTimezone)),
		}

		var buf bytes.Buffer
		if err := template.ParsedTemplate.Execute(&buf, context); err != nil {
			return "", "", err
		}
		result += buf.String()
		activityCount += len(changesMap)
	}
	result = template.ReplaceTxtToSvg(result)
	var templateBody dao.Template
	if err := tx.Where("name = ?", "sprint_body").First(&templateBody).Error; err != nil {
		return "", "", err
	}

	var buff bytes.Buffer
	if err := templateBody.ParsedTemplate.Execute(&buff, struct {
		Sprint        *dao.Sprint
		Title         string
		CreatedAt     time.Time
		Body          string
		ActivityCount int
	}{
		Title:         activities[0].Sprint.Name,
		CreatedAt:     time.Now(), //TODO: timezone
		Body:          result,
		Sprint:        activities[0].Sprint,
		ActivityCount: activityCount,
	}); err != nil {
		return "", "", err
	}

	content := buff.String()
	return content, htmlStripPolicy.Sanitize(content), nil
}
