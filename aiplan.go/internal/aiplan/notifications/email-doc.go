package notifications

import (
	"bytes"
	"fmt"
	"log/slog"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	policy "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/redactor-policy"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"gorm.io/gorm"
)

type emailNotifyDoc struct {
	service *EmailService
}

func newEmailNotifyDoc(es *EmailService) *emailNotifyDoc {
	if es == nil {
		return nil
	}
	return &emailNotifyDoc{service: es}
}

func (e *emailNotifyDoc) Process() {
	e.service.sending = true

	defer func() {
		e.service.sending = false
	}()

	var activities []dao.DocActivity
	if err := e.service.db.Unscoped().
		Joins("Doc").
		Joins("Actor").
		Joins("Workspace").
		Preload("Doc.ParentDoc").
		Preload("Doc.Author").
		Preload("Doc.AccessRules").
		Order("doc_activities.created_at").
		Where("doc_activities.notified = ?", false).
		Limit(100).
		Find(&activities).Error; err != nil {
		slog.Error("Get activities", slog.Int("interval", e.service.cfg.NotificationsSleep), "err", err)
		return
	}

	resultChan := make(chan []mail, 1)
	errorChan := make(chan error, 1)
	go func() {
		defer func() {
			//if r := recover(); r != nil {
			//	errorChan <- fmt.Errorf("panic in process: %v", r)
			//}
			close(resultChan)
			close(errorChan)
		}()

		sorter := docActivitySorter{
			skipActivities: make([]dao.DocActivity, 0),
			Docs:           make(map[string]docActivity),
		}

		for i := range activities {
			sorter.sortEntity(e.service.db, activities[i])
		}

		var mailsToSend []mail

		for _, docAct := range sorter.Docs {
			err := docAct.Finalization(e.service.db)
			if err != nil {
				slog.Error("Doc activity finalization", "error", err)
				continue
			}

			mailsToSend = append(mailsToSend, docAct.getMails(e.service.db)...)
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
		slog.Error("Error processing DocActivities", "err", err)
	}

	if err := e.service.db.Transaction(func(tx *gorm.DB) error {
		for _, activity := range activities {
			if err := e.service.db.Model(&dao.DocActivity{}).
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

type docActivitySorter struct {
	skipActivities []dao.DocActivity
	Docs           map[string]docActivity //map[docId]
}

type docMember struct {
	User              dao.User
	DocAuthorSettings types.WorkspaceMemberNS
	DocMemberSettings types.WorkspaceMemberNS
	WorkspaceRole     int //todo дотянуть
	DocAuthor         bool
	Editor            bool
	Reader            bool
	Watcher           bool
}

type docCommentAuthor struct {
	User                    dao.User
	WorkspaceAuthorSettings types.WorkspaceMemberNS
	WorkspaceMemberSettings types.WorkspaceMemberNS
	WorkspaceRole           int //todo дотянуть
	activities              []dao.DocActivity
}

type docActivity struct {
	doc                 *dao.Doc
	activities          []dao.DocActivity
	users               map[string]docMember         //map[user.Email]
	commentActivityMap  map[string][]dao.DocActivity // map[commentId]
	commentActivityUser map[string]docCommentAuthor  //map[user.Email]
}

func (da *docActivity) getMails(tx *gorm.DB) []mail {
	mails := make([]mail, 0)
	nameDoc := da.doc.Title
	if da.doc.ParentDoc != nil {
		nameDoc = fmt.Sprintf(".../%s", da.doc.ParentDoc.Title)
	}
	subj := fmt.Sprintf("Обновления для документа %s/%s", da.doc.Workspace.Slug, nameDoc)
	for _, member := range da.users {
		if !member.User.CanReceiveNotifications() {
			continue
		}

		if member.User.Settings.EmailNotificationMute {
			continue
		}

		var sendActivities []dao.DocActivity

		for _, activity := range da.activities {
			var authorNotify, memberNotify bool
			memberNotify = member.DocMemberSettings.IsNotify(activity.Field, "doc", activity.Verb, member.WorkspaceRole)
			if activity.Doc.CreatedById == member.User.ID {
				authorNotify = member.DocAuthorSettings.IsNotify(activity.Field, "doc", activity.Verb, member.WorkspaceRole)
			}
			if (member.DocAuthor && authorNotify) || (!member.DocAuthor && memberNotify) {
				sendActivities = append(sendActivities, activity)
			}
		}

		if len(sendActivities) == 0 {
			continue
		}

		content, textContent, err := getDocNotificationHTML(tx, sendActivities, &member.User, da.doc)
		if err != nil {
			slog.Error("Make doc notification HTML", "err", err)
			continue
		}

		mails = append(mails, mail{
			To:          member.User.Email,
			Subject:     subj,
			Content:     content,
			TextContent: textContent,
		})
	}

	for _, author := range da.commentActivityUser {
		field := "comment"

		if len(author.activities) == 0 {
			continue
		}

		if author.User.CanReceiveNotifications() && !author.User.Settings.EmailNotificationMute && author.WorkspaceMemberSettings.IsNotify(&field, "doc", "all", author.WorkspaceRole) {
			content, textContent, err := getDocNotificationHTML(tx, author.activities, &author.User, da.doc)
			if err != nil {
				slog.Error("Make doc notification HTML", "err", err)
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

func (ia *docActivity) Finalization(tx *gorm.DB) error {
	if err := ia.getCommentNotify(tx); err != nil {
		return err
	}
	//if err := ia.getNotifySettings(tx); err != nil {
	//  return err
	//}
	return nil
}

func (ia *docActivity) getCommentNotify(tx *gorm.DB) error {
	var commentIds []string
	for commentId, _ := range ia.commentActivityMap {
		commentIds = append(commentIds, commentId)
	}
	var comment []dao.DocComment
	if len(commentIds) > 0 {
		if err := tx.
			Preload("OriginalComment").
			Preload("OriginalComment.Actor").
			Where("doc_id = ? and id IN (?) and reply_to_comment_id is not null", ia.doc.ID, commentIds).
			Find(&comment).
			Error; err != nil {
			return err
		}
	}

	for _, docComment := range comment {
		if docComment.OriginalComment == nil || docComment.OriginalComment.Actor == nil {
			continue
		}
		authorComment := *docComment.OriginalComment.Actor

		if _, ok := ia.users[authorComment.Email]; !ok {
			if ca, exist := ia.commentActivityUser[authorComment.Email]; !exist {
				ia.commentActivityUser[authorComment.Email] = docCommentAuthor{
					User:       authorComment,
					activities: ia.commentActivityMap[docComment.Id.String()],
				}
			} else {
				ca.activities = append(ca.activities, ia.commentActivityMap[docComment.Id.String()]...)
				ia.commentActivityUser[authorComment.Email] = ca
			}
		}
	}

	var ids []string
	for _, author := range ia.commentActivityUser {
		ids = append(ids, author.User.ID)
	}

	var members []dao.WorkspaceMember
	if err := tx.Joins("Member").Where("workspace_members.workspace_id = ?", ia.doc.WorkspaceId).Where("member_id in (?)", ids).Find(&members).Error; err != nil {
	}
	for _, member := range members {
		if v, ok := ia.commentActivityUser[member.Member.Email]; ok {
			v.WorkspaceAuthorSettings = member.NotificationAuthorSettingsEmail
			v.WorkspaceMemberSettings = member.NotificationSettingsEmail
			ia.commentActivityUser[member.Member.Email] = v
		}
	}

	return nil
}

// Для пропуска активностей
func (ia *docActivity) skip(activity dao.DocActivity) bool {
	if activity.Field != nil && *activity.Field == "doc" && activity.Verb == "created" && activity.NewDoc == nil {
		return true
	}
	return false
}

func newDocActivity(tx *gorm.DB, doc, newDoc *dao.Doc) *docActivity {
	if doc == nil {
		return nil
	}

	doc.SetUrl()

	res := docActivity{
		doc:                 doc,
		activities:          make([]dao.DocActivity, 0),
		users:               make(map[string]docMember),
		commentActivityMap:  make(map[string][]dao.DocActivity),
		commentActivityUser: make(map[string]docCommentAuthor),
	}

	checkId := func(users *[]dao.User, id string) bool {
		if users == nil {
			return false
		}
		for _, user := range *users {
			if user.ID == id {
				return true
			}
		}
		return false
	}

	var workspaceMembers []dao.WorkspaceMember
	if err := tx.Joins("Member").Where("workspace_id = ?", doc.WorkspaceId).Find(&workspaceMembers).Error; err != nil {
		return nil
	}

	for _, member := range workspaceMembers {
		if member.Member == nil {
			continue
		}
		isAuthor := member.MemberId == doc.Author.ID
		isWatcher := checkId(doc.Watchers, member.MemberId)
		isEditor := checkId(doc.Editors, member.MemberId)
		isReader := checkId(doc.Readers, member.MemberId)
		var isAuthorNewDoc, isWatcherNewDoc, isEditorNewDoc, isReaderNewDoc bool
		if newDoc != nil {
			newDoc.SetUrl()
			isAuthorNewDoc = member.MemberId == doc.Author.ID
			isWatcherNewDoc = checkId(newDoc.Watchers, member.MemberId)
			isEditorNewDoc = checkId(newDoc.Editors, member.MemberId)
			isReaderNewDoc = checkId(newDoc.Readers, member.MemberId)
		}

		if isReader || isEditor || isAuthor || isWatcher || isAuthorNewDoc || isWatcherNewDoc || isEditorNewDoc || isReaderNewDoc {
			res.users[member.Member.Email] = docMember{
				User:              *member.Member,
				DocAuthorSettings: member.NotificationAuthorSettingsEmail,
				DocMemberSettings: member.NotificationSettingsEmail,
				DocAuthor:         isAuthor || isAuthorNewDoc,
				Editor:            isEditor || isEditorNewDoc,
				Reader:            isReader || isReaderNewDoc,
				Watcher:           isWatcher || isWatcherNewDoc,
			}
		}
	}

	return &res
}

func (ia *docActivity) AddActivity(activity dao.DocActivity) bool {
	if ia.skip(activity) {
		return false
	}

	ia.activities = append(ia.activities, activity)

	if activity.Field != nil && *activity.Field == "comment" && activity.NewIdentifier != nil {
		if activity.Verb == "created" || activity.Verb == "updated" {
			//TODO
			var arr []dao.DocActivity
			if v, ok := ia.commentActivityMap[*activity.NewIdentifier]; !ok {
				arr = append(arr, activity)
			} else {
				arr = append(v, activity)
			}
			ia.commentActivityMap[*activity.NewIdentifier] = arr
		}
	}
	return true
}

func (as *docActivitySorter) sortEntity(tx *gorm.DB, activity dao.DocActivity) {
	var newDocCreate *dao.Doc
	if activity.Field != nil && *activity.Field == "doc" && activity.Verb == "created" && activity.NewDoc != nil {
		if tx.
			Joins("Author").
			Joins("Workspace").
			Joins("ParentDoc").
			Preload("AccessRules.Member").
			Where("docs.id = ?", activity.NewDoc.ID).First(&newDocCreate).Error != nil {
		}
	}
	if activity.DocId != "" { //
		if v, ok := as.Docs[activity.DocId]; !ok {
			activity.Doc.Workspace = activity.Workspace
			da := newDocActivity(tx, activity.Doc, newDocCreate)
			if da != nil {
				if !da.AddActivity(activity) {
					as.skipActivities = append(as.skipActivities, activity)
				}
				as.Docs[activity.DocId] = *da
			}
		} else {
			if !v.AddActivity(activity) {
				as.skipActivities = append(as.skipActivities, activity)
			}
			as.Docs[activity.DocId] = v
		}
	}
	return
}

func getDocNotificationHTML(tx *gorm.DB, activities []dao.DocActivity, targetUser *dao.User, doc *dao.Doc) (string, string, error) {
	result := ""
	//
	actorsChangesMap := make(map[string]map[string]dao.DocActivity)
	actorsMap := make(map[string]dao.User)
	commentCount := 0
	for _, activity := range activities {
		// doc deletion
		activity.Doc.AfterFind(tx)
		doc.AfterFind(tx)

		if activity.Field != nil && *activity.Field == "doc" && activity.Verb == "deleted" {
			var template dao.Template
			if err := tx.Where("name = ?", "doc_activity_delete").First(&template).Error; err != nil {
				return "", "", err
			}

			if activity.OldValue == nil {
				continue
			}

			var buf bytes.Buffer
			if err := template.ParsedTemplate.Execute(&buf, struct {
				Actor     *dao.User
				Title     string
				CreatedAt time.Time
			}{
				activity.Actor,
				*activity.OldValue,
				activity.CreatedAt.In((*time.Location)(&targetUser.UserTimezone)),
			}); err != nil {
				return "", "", err
			}

			result += buf.String()
			continue
		}

		// new doc
		if activity.Verb == "created" {
			result += gocGetEmailHtml(tx, targetUser, &activity)
		}

		// comment
		if *activity.Field == "comment" {
			var template dao.Template
			if err := tx.Where("name = ?", "issue_activity_comment").First(&template).Error; err != nil {
				return "", "", err
			}
			newComment := false
			deleted := false
			switch activity.Verb {
			case "created":
				newComment = true
			case "deleted":
				deleted = true
			}

			comment := replaceTablesToText(replaceImageToText(activity.NewValue))
			comment = policy.ProcessCustomHtmlTag(comment)
			comment = prepareToMail(prepareHtmlBody(htmlStripPolicy, comment))

			var buf bytes.Buffer
			if err := template.ParsedTemplate.Execute(&buf, struct {
				Actor     dao.User
				Comment   string
				CreatedAt time.Time
				New       bool
				Deleted   bool
			}{
				*activity.Actor,
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

		changesMap, ok := actorsChangesMap[*activity.ActorId]
		if !ok {
			changesMap = make(map[string]dao.DocActivity)
		}
		field := *activity.Field

		if field == "description" {
			oldValue := replaceTablesToText(replaceImageToText(*activity.OldValue))
			newValue := replaceTablesToText(replaceImageToText(activity.NewValue))
			oldValue = policy.ProcessCustomHtmlTag(oldValue)
			newValue = policy.ProcessCustomHtmlTag(newValue)
			oldValue = prepareToMail(prepareHtmlBody(htmlStripPolicy, oldValue))
			newValue = prepareToMail(prepareHtmlBody(htmlStripPolicy, newValue))
			activity.OldValue = &oldValue
			activity.NewValue = newValue
		}
		if field == "doc" && activity.Verb == "created" {
			continue
		}

		changesMap[field] = activity
		actorsMap[*activity.ActorId] = *activity.Actor
		actorsChangesMap[*activity.ActorId] = changesMap
	}

	var template dao.Template

	if err := tx.Where("name = ?", "doc_activity").First(&template).Error; err != nil {
		return "", "", err
	}
	activityCount := 0

	var attachments []dao.DocAttachment
	if err := tx.
		Joins("Asset").
		Where("doc_attachments.workspace_id = ?", activities[0].WorkspaceId).
		Where("doc_attachments.doc_id = ?", activities[0].DocId).
		Order("doc_attachments.created_at").
		Find(&attachments).Error; err != nil {
		return "", "", err
	}

	for userId, changesMap := range actorsChangesMap {
		context := struct {
			Doc         dao.Doc
			IssueURL    string
			Changes     map[string]dao.DocActivity
			Actor       dao.User
			CreatedAt   time.Time
			Attachments []dao.DocAttachment
		}{
			*doc,
			doc.URL.String(),
			changesMap,
			actorsMap[userId],
			time.Now().In((*time.Location)(&targetUser.UserTimezone)),
			attachments,
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
	if err := tx.Where("name = ?", "doc_body").First(&templateBody).Error; err != nil {
		return "", "", err
	}

	docBreadcrumb := doc.Workspace.Slug
	if doc.ParentDoc == nil {
		docBreadcrumb += fmt.Sprintf("/ документ '%s'", doc.Title)
	} else {
		docBreadcrumb += fmt.Sprintf("/.../ документ '%s'", doc.Title)
	}

	var buff bytes.Buffer
	if err := templateBody.ParsedTemplate.Execute(&buff, struct {
		Doc           *dao.Doc
		DocBreadcrumb string
		Title         string
		CreatedAt     time.Time
		DocBody       string
		CommentCount  int
		ActivityCount int
		Project       *dao.Project
	}{
		Title:         doc.Title,
		DocBreadcrumb: docBreadcrumb,
		CreatedAt:     time.Now(), //TODO: timezone
		DocBody:       result,
		Doc:           doc,
		CommentCount:  commentCount,
		ActivityCount: activityCount,
		Project:       nil,
	}); err != nil {
		return "", "", err
	}

	content := buff.String()
	return content, htmlStripPolicy.Sanitize(content), nil
}

func gocGetEmailHtml(tx *gorm.DB, user *dao.User, act *dao.DocActivity) string {
	if act.Field != nil && *act.Field != "doc" {
		return ""
	}

	if act.Verb == "deleted" {
		var template dao.Template
		if err := tx.Where("name = ?", "doc_activity_delete").First(&template).Error; err != nil {
			return ""
		}

		if act.OldValue == nil {
			return ""
		}

		var buf bytes.Buffer
		if err := template.ParsedTemplate.Execute(&buf, struct {
			Actor     *dao.User
			Title     string
			CreatedAt time.Time
		}{
			act.Actor,
			*act.OldValue,
			act.CreatedAt.In((*time.Location)(&user.UserTimezone)),
		}); err != nil {
			return ""
		}

		return buf.String()
	}

	if act.Verb != "deleted" {
		var template dao.Template
		if err := tx.Where("name = ?", "doc_activity_new").First(&template).Error; err != nil {
			return ""
		}
		var docId string
		if act.NewIdentifier != nil {
			docId = *act.NewIdentifier
		}

		if act.OldIdentifier != nil {
			docId = *act.OldIdentifier
		}

		var newDoc dao.Doc
		if err := tx.Unscoped().
			Joins("Author").
			Joins("Workspace").
			Joins("ParentDoc").
			Preload("AccessRules").
			Where("docs.id = ?", docId).
			First(&newDoc).Error; err != nil {
			return ""
		}

		if act.NewDoc == nil {
			act.NewDoc = &newDoc
		}
		var description, oldVal string

		if act.Verb != "removed" {
			description = replaceTablesToText(replaceImageToText(act.NewDoc.Content.Body))
			description = policy.ProcessCustomHtmlTag(description)
			description = prepareToMail(prepareHtmlBody(htmlStripPolicy, description))
			description = template.ReplaceTxtToSvg(description)
		}

		if act.OldIdentifier != nil {
			oldVal = *act.OldIdentifier
		}

		var buf bytes.Buffer
		if err := template.ParsedTemplate.Execute(&buf, struct {
			Actor       *dao.User
			Doc         dao.Doc
			Verb        string
			Parent      *dao.Doc
			CreatedAt   time.Time
			Description string
			OldVal      string
		}{
			act.Actor,
			newDoc,
			act.Verb,
			newDoc.ParentDoc,
			act.CreatedAt.In((*time.Location)(&user.UserTimezone)),
			description,
			oldVal,
		}); err != nil {
			return ""
		}

		return buf.String()
	}
	return ""
}
