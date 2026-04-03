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
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

type emailNotifyIssue struct {
	service *EmailService
}

func newEmailNotifyIssue(es *EmailService) *emailNotifyIssue {
	if es == nil {
		return nil
	}
	return &emailNotifyIssue{service: es}
}

func (e *emailNotifyIssue) Process() {
	e.service.sending = true

	defer func() {
		e.service.sending = false
	}()

	var activities []dao.ActivityEvent
	if err := e.service.db.Unscoped().
		Preload("Issue").
		Preload("Actor").
		Preload("Project").
		Preload("Issue.Workspace").
		Preload("Issue.Author").
		Preload("Issue.Assignees").
		Preload("Issue.Watchers").
		Preload("Issue.State").
		Preload("Issue.Parent").
		Preload("Issue.Project").
		Preload("Issue.Project.DefaultWatchersDetails", "is_default_watcher = ?", true).
		Preload("Issue.Project.DefaultWatchersDetails.Member").
		Preload("Issue.Parent.Project").
		Order("activity_events.created_at").
		Where("activity_events.entity_type = ?", types.LayerIssue).
		Where("activity_events.notified = ?", false).
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

		sorter := issueActivitySorter{
			skipActivities: make([]dao.ActivityEvent, 0),
			Issues:         make(map[uuid.UUID]issueActivity),
		}

		for i := range activities {
			sorter.sortEntity(activities[i])
		}

		var mailsToSend []mail

		for _, issAct := range sorter.Issues {
			err := issAct.Finalization(e.service.db)
			if err != nil {
				slog.Error("Issue activity finalization error: ", err)
				continue
			}

			mailsToSend = append(mailsToSend, issAct.getMails(e.service.db)...)
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
			if err := e.service.db.Model(&dao.ActivityEvent{}).
				Unscoped().
				Where("id = ?", activity.ID).
				Update("notified", true).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		slog.Error("Update notified flag in DB", "err", err)
	}
}

type issueActivitySorter struct {
	skipActivities []dao.ActivityEvent
	Issues         map[uuid.UUID]issueActivity //map[issueId]
}

type issueMember struct {
	User                  dao.User
	ProjectRole           int //todo дополнить
	ProjectAuthorSettings types.ProjectMemberNS
	ProjectMemberSettings types.ProjectMemberNS
	IssueAuthor           bool
	Assigner              bool
	Watcher               bool
}

type issueCommentAuthor struct {
	User                  dao.User
	ProjectAuthorSettings types.ProjectMemberNS
	ProjectMemberSettings types.ProjectMemberNS
	activities            []dao.ActivityEvent
	ProjectRole           int //todo дополнить
}

type issueActivity struct {
	issue      *dao.Issue
	activities []dao.ActivityEvent
	users      map[string]issueMember //map[user.Email]

	commentActivityMap  map[uuid.UUID][]dao.ActivityEvent // map[commentId]
	commentActivityUser map[string]issueCommentAuthor     //map[user.Email]
}

func (ia *issueActivity) getMails(tx *gorm.DB) []mail {
	mails := make([]mail, 0)
	subj := fmt.Sprintf("Обновления для %s-%d", ia.issue.Project.Identifier, ia.issue.SequenceId)
	for _, member := range ia.users {
		if !member.User.CanReceiveNotifications() {
			continue
		}

		if member.User.Settings.EmailNotificationMute {
			continue
		}

		var sendActivities []dao.ActivityEvent

		for _, activity := range ia.activities {
			var authorNotify, memberNotify bool
			memberNotify = member.ProjectMemberSettings.IsNotify(activity.Field, types.LayerIssue, activity.Verb, member.ProjectRole)
			if activity.Issue.CreatedById == member.User.ID {
				authorNotify = member.ProjectAuthorSettings.IsNotify(activity.Field, types.LayerIssue, activity.Verb, member.ProjectRole)
			}
			if (member.IssueAuthor && authorNotify) || (!member.IssueAuthor && memberNotify) {
				sendActivities = append(sendActivities, activity)
			}
		}

		if len(sendActivities) == 0 {
			continue
		}

		content, textContent, err := getIssueNotificationHTML(tx, sendActivities, &member.User)
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

	for _, author := range ia.commentActivityUser {
		field := actField.Comment.Field.String()

		if len(author.activities) == 0 {
			continue
		}

		if author.User.CanReceiveNotifications() && !author.User.Settings.EmailNotificationMute && author.ProjectMemberSettings.IsNotify(actField.ActivityField(field), types.LayerIssue, "all", author.ProjectRole) {
			content, textContent, err := getIssueNotificationHTML(tx, author.activities, &author.User)
			if err != nil {
				slog.Error("Make issue notification HTML", "err", err)
				continue
			}
			mails = append(mails, mail{
				To:          author.User.Email,
				Subject:     subj,
				Content:     content,
				TextContent: textContent,
			})
		}
	}

	return mails
}

func (ia *issueActivity) Finalization(tx *gorm.DB) error {
	if err := ia.getCommentNotify(tx); err != nil {
		return err
	}
	if err := ia.getNotifySettings(tx); err != nil {
		return err
	}
	return nil
}

func (ia *issueActivity) getCommentNotify(tx *gorm.DB) error {
	var commentIds []uuid.UUID
	for commentId, _ := range ia.commentActivityMap {
		commentIds = append(commentIds, commentId)
	}
	var comment []dao.IssueComment
	if len(commentIds) > 0 {
		if err := tx.
			Preload("OriginalComment").
			Preload("OriginalComment.Actor").
			Where("issue_id = ? and id IN (?) and reply_to_comment_id is not null", ia.issue.ID, commentIds).
			Find(&comment).
			Error; err != nil {
			return err
		}
	}

	for _, issueComment := range comment {
		if issueComment.OriginalComment == nil || issueComment.OriginalComment.Actor == nil {
			continue
		}
		authorComment := *issueComment.OriginalComment.Actor

		if _, ok := ia.users[authorComment.Email]; !ok {
			if ca, exist := ia.commentActivityUser[authorComment.Email]; !exist {
				ia.commentActivityUser[authorComment.Email] = issueCommentAuthor{
					User:       authorComment,
					activities: ia.commentActivityMap[issueComment.Id],
				}
			} else {
				ca.activities = append(ca.activities, ia.commentActivityMap[issueComment.Id]...)
				ia.commentActivityUser[authorComment.Email] = ca
			}
		}
	}

	return nil
}

func (ia *issueActivity) getNotifySettings(tx *gorm.DB) error {
	userIds := make([]uuid.UUID, 0, len(ia.users))
	for _, member := range ia.users {
		userIds = append(userIds, member.User.ID)
	}

	for _, author := range ia.commentActivityUser {
		userIds = append(userIds, author.User.ID)

	}

	var projectMembers []dao.ProjectMember
	if err := tx.
		Preload("Member").
		Where("project_id = ?", ia.issue.ProjectId).
		Where("member_id IN (?)", userIds).
		Find(&projectMembers).Error; err != nil {
		return err
	}

	for _, member := range projectMembers {
		if v, ok := ia.users[member.Member.Email]; ok {
			v.ProjectAuthorSettings = member.NotificationAuthorSettingsEmail
			v.ProjectMemberSettings = member.NotificationSettingsEmail
			ia.users[member.Member.Email] = v
		}
		if v, ok := ia.commentActivityUser[member.Member.Email]; ok {
			v.ProjectAuthorSettings = member.NotificationAuthorSettingsEmail
			v.ProjectMemberSettings = member.NotificationSettingsEmail
			ia.commentActivityUser[member.Member.Email] = v
		}
	}
	return nil
}

// Для пропуска активностей
func (ia *issueActivity) skip(activity dao.ActivityEvent) bool {
	if activity.Verb == "cloned" {
		return true
	}
	if activity.Issue.Draft {
		return true
	}

	if activity.Field == actField.StartDate.Field {
		return true
	}

	if activity.Field == actField.CompletedAt.Field {
		return true
	}

	if activity.Field == actField.Link.Field && activity.Verb == actField.VerbDeleted {
		return true
	}

	return false
}

func newIssueActivity(issue *dao.Issue) *issueActivity {
	if issue == nil {
		return nil
	}

	res := issueActivity{
		issue:               issue,
		activities:          make([]dao.ActivityEvent, 0),
		users:               make(map[string]issueMember),
		commentActivityMap:  make(map[uuid.UUID][]dao.ActivityEvent),
		commentActivityUser: make(map[string]issueCommentAuthor),
	}

	{ //add author
		res.users[issue.Author.Email] = issueMember{
			User:        *issue.Author,
			IssueAuthor: true,
			Assigner:    false,
			Watcher:     false,
		}
	}

	{ // add assignees
		if res.issue.Assignees != nil {
			for _, assignee := range *res.issue.Assignees {
				if im, ok := res.users[assignee.Email]; !ok {
					res.users[assignee.Email] = issueMember{
						User:        assignee,
						IssueAuthor: false,
						Assigner:    true,
						Watcher:     false,
					}
				} else {
					im.Assigner = true
					res.users[assignee.Email] = im
				}
			}
		}
	}

	{ // add watchers
		if res.issue.Watchers != nil {
			for _, watcher := range *res.issue.Watchers {
				if im, ok := res.users[watcher.Email]; !ok {
					res.users[watcher.Email] = issueMember{
						User:        watcher,
						IssueAuthor: false,
						Assigner:    false,
						Watcher:     true,
					}
				} else {
					im.Watcher = true
					res.users[watcher.Email] = im
				}
			}
		}
	}

	{ // add default watchers
		if res.issue.Project != nil {
			for _, watcher := range res.issue.Project.DefaultWatchersDetails {
				if im, ok := res.users[watcher.Member.Email]; !ok {
					res.users[watcher.Member.Email] = issueMember{
						User:    *watcher.Member,
						Watcher: true,
					}
				} else {
					im.Watcher = true
					res.users[watcher.Member.Email] = im
				}
			}
		}
	}
	return &res
}

func (ia *issueActivity) AddActivity(activity dao.ActivityEvent) bool {
	if ia.skip(activity) {
		return false
	}

	ia.activities = append(ia.activities, activity)

	if activity.Field == actField.Comment.Field && activity.NewIdentifier.Valid {
		if activity.Verb == actField.VerbCreated || activity.Verb == actField.VerbUpdated {
			//TODO
			var arr []dao.ActivityEvent
			if v, ok := ia.commentActivityMap[activity.NewIdentifier.UUID]; !ok {
				arr = append(arr, activity)
			} else {
				arr = append(v, activity)
			}
			ia.commentActivityMap[activity.NewIdentifier.UUID] = arr
		}
	}
	return true
}

func (as *issueActivitySorter) sortEntity(activity dao.ActivityEvent) {
	if v, ok := as.Issues[activity.IssueID.UUID]; !ok {
		ia := newIssueActivity(activity.Issue)
		if ia != nil {
			if !ia.AddActivity(activity) {
				as.skipActivities = append(as.skipActivities, activity)
			}
			as.Issues[activity.IssueID.UUID] = *ia
		}
	} else {
		if !v.AddActivity(activity) {
			as.skipActivities = append(as.skipActivities, activity)
		}
		as.Issues[activity.IssueID.UUID] = v
	}

	return
}

func getIssueNotificationHTML(tx *gorm.DB, activities []dao.ActivityEvent, targetUser *dao.User) (string, string, error) {
	result := ""

	actorsChangesMap := make(map[uuid.UUID]map[string]dao.ActivityEvent)
	actorsMap := make(map[uuid.UUID]dao.User)
	commentCount := 0
	for i, activity := range activities {
		activities[i].Issue.SetUrl()
		// issue deletion
		if activity.Field == actField.Issue.Field && activity.Verb == actField.VerbDeleted {
			var template dao.Template
			if err := tx.Where("name = ?", "issue_activity_delete").First(&template).Error; err != nil {
				return "", "", err
			}

			var buf bytes.Buffer
			if err := template.ParsedTemplate.Execute(&buf, struct {
				Actor     *dao.User
				Issue     dao.Issue
				CreatedAt time.Time
			}{
				activity.Actor,
				*activity.Issue,
				activity.CreatedAt.In((*time.Location)(&targetUser.UserTimezone)),
			}); err != nil {
				return "", "", err
			}

			result += buf.String()
			continue
		}

		// new issue
		//if activity.Field == nil {
		//	var template dao.Template
		//	if err := tx.Where("name = ?", "issue_activity_new").First(&template).Error; err != nil {
		//		return "", "", err
		//	}
		//	var p string
		//	p = types.TranslateMap(types.PriorityTranslation, activity.Issue.Priority)
		//
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

		if activity.Field == actField.IssueTransfer.Field && activity.Verb == "cloned" {
			continue
		}

		if activity.Field == actField.IssueTransfer.Field && (activity.Verb == actField.VerbCopied || activity.Verb == "move") {
			var template dao.Template
			if err := tx.Where("name = ?", "issue_migrate").First(&template).Error; err != nil {
				return "", "", err
			}
			var oldProject dao.Project
			if err := tx.Where("id = ?", activity.OldIdentifier.UUID).First(&oldProject).Error; err != nil {
				return "", "", err
			}

			var p string
			p = types.TranslateMap(types.PriorityTranslation, activity.Issue.Priority)
			activity.Issue.Priority = &p
			description := replaceTablesToText(replaceImageToText(activity.Issue.DescriptionHtml))
			description = policy.ProcessCustomHtmlTag(description)
			description = prepareToMail(prepareHtmlBody(htmlStripPolicy, description))
			description = template.ReplaceTxtToSvg(description)
			var buf bytes.Buffer
			if err := template.ParsedTemplate.Execute(&buf, struct {
				Actor       *dao.User
				Issue       dao.Issue
				OldProject  *dao.Project
				NewProject  *dao.Project
				CreatedAt   time.Time
				Description string
				Move        bool
			}{
				activity.Actor,
				*activity.Issue,
				&oldProject,
				activity.Project,
				activity.CreatedAt.In((*time.Location)(&targetUser.UserTimezone)),
				description,
				activity.Verb == "move",
			}); err != nil {
				return "", "", err
			}

			result += buf.String()
			continue
		}

		// comment
		if activity.Field == actField.Comment.Field {
			var template dao.Template
			if err := tx.Where("name = ?", "issue_activity_comment").First(&template).Error; err != nil {
				return "", "", err
			}
			newComment := false
			deleted := false
			switch activity.Verb {
			case actField.VerbCreated:
				newComment = true
			case actField.VerbDeleted:
				deleted = true
			}

			comment := replaceTablesToText(replaceImageToText(activity.NewValue))
			comment = policy.ProcessCustomHtmlTag(comment)
			comment = prepareToMail(prepareHtmlBody(htmlStripPolicy, comment))

			var buf bytes.Buffer
			if err := template.ParsedTemplate.Execute(&buf, struct {
				Actor     dao.User
				Issue     *dao.Issue
				Comment   string
				CreatedAt time.Time
				New       bool
				Deleted   bool
			}{
				*activity.Actor,
				activity.Issue,
				comment,
				activity.CreatedAt.In((*time.Location)(&targetUser.UserTimezone)),
				newComment,
				deleted,
			}); err != nil {
				return "", "", err
			}

			result += buf.String()
			commentCount++
			continue
		}

		changesMap, ok := actorsChangesMap[activity.ActorID]
		if !ok {
			changesMap = make(map[string]dao.ActivityEvent)
		}
		field := activity.Field

		if field == actField.Priority.Field {
			activity.NewValue = types.TranslateMap(types.PriorityTranslation, utils.ToPtr(activity.NewValue))
			activity.OldValue = utils.ToPtr(types.TranslateMap(types.PriorityTranslation, activity.OldValue))
		}

		if field == actField.TargetDate.Field {
			newT, errNew := FormatDate(activity.NewValue, "02.01.2006 15:04", &targetUser.UserTimezone)

			if activity.OldValue != nil {
				if oldT, errOld := FormatDate(*activity.OldValue, "02.01.2006 15:04", &targetUser.UserTimezone); errOld == nil {
					activity.OldValue = &oldT
				}
			}

			if errNew == nil {
				activity.NewValue = newT
			}

		}

		if field == actField.Description.Field {
			oldValue := replaceTablesToText(replaceImageToText(*activity.OldValue))
			newValue := replaceTablesToText(replaceImageToText(activity.NewValue))
			oldValue = policy.ProcessCustomHtmlTag(oldValue)
			newValue = policy.ProcessCustomHtmlTag(newValue)
			oldValue = prepareToMail(prepareHtmlBody(htmlStripPolicy, oldValue))
			newValue = prepareToMail(prepareHtmlBody(htmlStripPolicy, newValue))
			activity.OldValue = &oldValue
			activity.NewValue = newValue
		}

		if field == actField.LinkTitle.Field || field == actField.LinkUrl.Field {
			field = actField.Link.Field
		}

		changesMap[field.String()] = activity
		actorsMap[activity.ActorID] = *activity.Actor
		actorsChangesMap[activity.ActorID] = changesMap
	}

	var template dao.Template
	if err := tx.Where("name = ?", "issue_activity").First(&template).Error; err != nil {
		return "", "", err
	}
	activityCount := 0

	var subIssues []dao.Issue
	if err := tx.Where("parent_id = ?", activities[0].Issue.ID).Find(&subIssues).Error; err != nil {
		return "", "", err
	}
	for userId, changesMap := range actorsChangesMap {
		context := struct {
			IssueURL  string
			SubIssues []dao.Issue
			Changes   map[string]dao.ActivityEvent
			Actor     dao.User
			CreatedAt time.Time
		}{
			activities[0].Issue.URL.String(),
			subIssues,
			changesMap,
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
	if err := tx.Where("name = ?", "body").First(&templateBody).Error; err != nil {
		return "", "", err
	}

	var buff bytes.Buffer
	if err := templateBody.ParsedTemplate.Execute(&buff, struct {
		Issue         *dao.Issue
		Title         string
		CreatedAt     time.Time
		Body          string
		CommentCount  int
		ActivityCount int
		Project       *dao.Project
	}{
		Title:         activities[0].Issue.Name,
		CreatedAt:     time.Now(), //TODO: timezone
		Body:          result,
		Issue:         activities[0].Issue,
		CommentCount:  commentCount,
		ActivityCount: activityCount,
		Project:       nil,
	}); err != nil {
		return "", "", err
	}

	content := buff.String()
	return content, htmlStripPolicy.Sanitize(content), nil
}
