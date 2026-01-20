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
		Preload("Sprint.Watchers").
		Joins("Sprint.CreatedBy").
		Preload("Sprint.Issues.Project").
		Joins("Actor").
		Joins("Workspace").
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
		slog.Error("Error processing SprintActivities", "err", err)
	}

	if err := e.service.db.Transaction(func(tx *gorm.DB) error {
		for _, activity := range activities {
			if err := e.service.db.Model(&dao.SprintActivity{}).
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
	}
	return nil
}

type sprintActivitySorter struct {
	skipActivities []dao.SprintActivity
	Sprint         map[uuid.UUID]sprintActivity //map[sprintId]
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
	sprint.SetUrl()

	res := sprintActivity{
		sprint:     sprint,
		activities: make([]dao.SprintActivity, 0),
		users:      make(map[string]sprintMember),
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
	return true
}

// Для пропуска активностей
func (ia *sprintActivity) skip(activity dao.SprintActivity) bool {
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

		content, textContent, err := getSprintNotificationHTML(tx, ia.sprint, sendActivities, &member.User)
		if err != nil {
			slog.Error("Make sprint notification HTML", "err", err)
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

func getSprintNotificationHTML(tx *gorm.DB, sprint *dao.Sprint, activities []dao.SprintActivity, targetUser *dao.User) (string, string, error) {
	result := ""

	actorsChangesMap := make(map[uuid.UUID]map[string]dao.SprintActivity)
	actorsMap := make(map[uuid.UUID]dao.User)
	removeIssuesId := make([]uuid.UUID, 0)
	removeWatchers := make([]dao.User, 0)

	issuesExist := make(map[uuid.UUID]struct{})
	watchersExist := make(map[uuid.UUID]struct{})

	type IssueView struct {
		Issue dao.Issue
		IsNew bool
	}
	type WatchersView struct {
		Watcher dao.User
		IsNew   bool
	}
	for _, activity := range activities {
		changesMap, ok := actorsChangesMap[activity.ActorId.UUID]
		if !ok {
			changesMap = make(map[string]dao.SprintActivity)
		}
		field := *activity.Field

		if field == actField.StartDate.Field.String() || field == actField.EndDate.Field.String() {
			newT, errNew := FormatDate(activity.NewValue, "02.01.2006", &targetUser.UserTimezone)

			if activity.OldValue != nil {
				if oldT, errOld := FormatDate(*activity.OldValue, "02.01.2006", &targetUser.UserTimezone); errOld == nil {
					activity.OldValue = &oldT
				}
			}

			if errNew == nil {
				activity.NewValue = newT
			}
		}
		if field == actField.Issue.Field.String() {
			switch activity.Verb {
			case actField.VerbAdded:
				issuesExist[activity.NewSprintIssue.ID] = struct{}{}
			case actField.VerbRemoved:
				removeIssuesId = append(removeIssuesId, activity.OldSprintIssue.ID)
			}
		}
		if field == actField.Watchers.Field.String() {
			switch activity.Verb {
			case actField.VerbAdded:
				watchersExist[activity.NewSprintWatcher.ID] = struct{}{}
			case actField.VerbRemoved:
				removeWatchers = append(removeWatchers, *activity.OldSprintWatcher)
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

	issues := make([]IssueView, 0, len(activities[0].Sprint.Issues))
	watchers := make([]WatchersView, 0, len(activities[0].Sprint.Watchers))

	for _, issue := range activities[0].Sprint.Issues {
		_, iok := issuesExist[issue.ID]
		issues = append(issues, IssueView{issue, iok})
	}

	for _, w := range activities[0].Sprint.Watchers {
		_, iok := watchersExist[w.ID]
		watchers = append(watchers, WatchersView{w, iok})
	}

	var removeIssues []dao.Issue
	if err := tx.Joins("Project").
		Where("issues.workspace_id = ?", sprint.WorkspaceId).
		Where("issues.id IN (?)", removeIssuesId).
		Find(&removeIssues).Error; err != nil {
		return "", "", err
	}

	var template dao.Template
	if err := tx.Where("name = ?", "sprint_activity").First(&template).Error; err != nil {
		return "", "", err
	}
	activityCount := 0

	for userId, changesMap := range actorsChangesMap {
		context := struct {
			SprintURL      string
			Changes        map[string]dao.SprintActivity
			Issues         []IssueView
			Watchers       []WatchersView
			RemoveIssues   []dao.Issue
			RemoveWatchers []dao.User
			Actor          dao.User
			CreatedAt      time.Time
		}{
			sprint.URL.String(),
			changesMap,
			issues,
			watchers,
			removeIssues,
			removeWatchers,
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
		Sprint           *dao.Sprint
		Title            string
		CreatedAt        time.Time
		SprintBreadcrumb string
		Body             string
		ActivityCount    int
	}{
		Title:            sprint.GetFullName(),
		CreatedAt:        time.Now(), //TODO: timezone
		Body:             result,
		Sprint:           activities[0].Sprint,
		ActivityCount:    activityCount,
		SprintBreadcrumb: fmt.Sprintf("/%s/", activities[0].Workspace.Slug),
	}); err != nil {
		return "", "", err
	}

	content := buff.String()
	return content, htmlStripPolicy.Sanitize(content), nil
}
