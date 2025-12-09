package notifications

import (
	"bytes"
	"fmt"
	"log/slog"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"gorm.io/gorm"
)

type emailNotifyWorkspace struct {
	service *EmailService
}

func newEmailNotifyWorkspace(es *EmailService) *emailNotifyWorkspace {
	if es == nil {
		return nil
	}
	return &emailNotifyWorkspace{service: es}
}

func (e *emailNotifyWorkspace) Process() {
	e.service.sending = true

	defer func() {
		e.service.sending = false
	}()

	var activities []dao.WorkspaceActivity
	if err := e.service.db.Unscoped().
		Joins("Workspace").
		Joins("Actor").
		Order("workspace_activities.created_at").
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

		sorter := workspaceActivitySorter{
			skipActivities: make([]dao.WorkspaceActivity, 0),
			Workspace:      make(map[string]workspaceActivity),
		}
		for i := range activities {
			sorter.sortEntity(e.service.db, activities[i])
		}

		var mailsToSend []mail

		for _, prAct := range sorter.Workspace {
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
		slog.Error("Error processing WorkspaceActivities", "err", err)
	}

	if err := e.service.db.Transaction(func(tx *gorm.DB) error {
		for _, activity := range activities {
			if err := e.service.db.Model(&dao.WorkspaceActivity{}).
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

type workspaceMember struct {
	User                    dao.User
	WorkspaceOwner          bool
	WorkspaceAdmin          bool
	DocMember               bool
	WorkspaceRole           int
	WorkspaceAuthorSettings types.WorkspaceMemberNS
	WorkspaceMemberSettings types.WorkspaceMemberNS
}

type workspaceActivitySorter struct {
	skipActivities []dao.WorkspaceActivity
	Workspace      map[string]workspaceActivity //map[workspaceId]
}

type workspaceActivity struct {
	workspace  *dao.Workspace
	activities []dao.WorkspaceActivity
	users      map[string]workspaceMember //map[user.Email]
	AllMember  []dao.WorkspaceMember
}

func (as *workspaceActivitySorter) sortEntity(tx *gorm.DB, activity dao.WorkspaceActivity) {
	if activity.WorkspaceId != "" { // TODO check it
		if v, ok := as.Workspace[activity.WorkspaceId]; !ok {
			wa := newWorkspaceActivity(tx, activity.Workspace)
			if wa != nil {
				if !wa.AddActivity(activity) {
					as.skipActivities = append(as.skipActivities, activity)
				}
				as.Workspace[activity.WorkspaceId] = *wa
			}
		} else {
			if !v.AddActivity(activity) {
				as.skipActivities = append(as.skipActivities, activity)
			}
			as.Workspace[activity.WorkspaceId] = v
		}
	}
	return
}

func newWorkspaceActivity(tx *gorm.DB, workspace *dao.Workspace) *workspaceActivity {
	res := workspaceActivity{
		workspace: workspace,
		users:     make(map[string]workspaceMember),
	}

	err := res.workspace.AfterFind(tx)
	if err != nil {
		return nil
	}

	if err := tx.
		Joins("Member").
		Where("workspace_id = ?", workspace.ID).
		Find(&res.AllMember).Error; err != nil {
		return nil
	}

	memberMap := utils.SliceToMap(&res.AllMember, func(v *dao.WorkspaceMember) string {
		return v.MemberId
	})

	{ //add Leader
		if owner, ok := memberMap[workspace.OwnerId]; ok && owner.Member != nil {
			res.users[memberMap[workspace.OwnerId].Member.Email] = workspaceMember{
				User:                    *owner.Member,
				WorkspaceOwner:          true,
				WorkspaceRole:           owner.Role,
				WorkspaceAuthorSettings: owner.NotificationAuthorSettingsEmail,
				WorkspaceMemberSettings: owner.NotificationSettingsEmail,
			}
		}
	}

	for _, member := range memberMap {
		isAdmin := member.Role == types.AdminRole

		if wm, ok := res.users[member.Member.Email]; !ok {
			if member.Member == nil {
				continue
			}
			res.users[member.Member.Email] = workspaceMember{
				User:           *member.Member,
				WorkspaceAdmin: isAdmin,

				WorkspaceRole:           member.Role,
				WorkspaceMemberSettings: member.NotificationSettingsEmail,
				WorkspaceAuthorSettings: member.NotificationAuthorSettingsEmail,
			}
		} else {
			wm.WorkspaceAdmin = isAdmin
			wm.WorkspaceRole = member.Role
			wm.WorkspaceAuthorSettings = member.NotificationAuthorSettingsEmail
			wm.WorkspaceMemberSettings = member.NotificationSettingsEmail
			res.users[member.Member.Email] = wm
		}
	}
	return &res
}

func (wa *workspaceActivity) AddActivity(activity dao.WorkspaceActivity) bool {
	if wa.skip(activity) {
		return false
	}

	wa.activities = append(wa.activities, activity)
	return true
}

func (wa *workspaceActivity) skip(activity dao.WorkspaceActivity) bool {
	if activity.Field != nil && *activity.Field != actField.Doc.Field.String() {
		return true
	}

	return false
}

func (wa *workspaceActivity) getMails(tx *gorm.DB) []mail {
	mails := make([]mail, 0)
	subj := fmt.Sprintf("Обновления для %s", wa.workspace.Slug)
	for _, member := range wa.users {
		if !member.User.CanReceiveNotifications() {
			continue
		}

		if member.User.Settings.EmailNotificationMute {
			continue
		}

		var sendActivities []dao.WorkspaceActivity
		for _, activity := range wa.activities {

			if activity.NewDoc != nil || activity.OldDoc != nil {
				var docId string
				if activity.OldDoc != nil {
					docId = *activity.OldIdentifier
				}
				if activity.NewDoc != nil {
					docId = *activity.NewIdentifier
				}

				var doc dao.Doc
				if err := tx.
					Joins("ParentDoc").
					Joins("Workspace").
					Joins("Author").
					Preload("AccessRules.Member").
					Where("docs.id = ?", docId).First(&doc).Error; err != nil {
					continue
				}
				isWatcher := utils.CheckInSlice([]string{member.User.ID}, doc.WatcherIDs...)
				isReader := utils.CheckInSlice([]string{member.User.ID}, doc.ReaderIDs...)
				isEditor := utils.CheckInSlice([]string{member.User.ID}, doc.EditorsIDs...)

				if isWatcher || isReader || isEditor || doc.CreatedById == member.User.ID {
					if doc.CreatedById == member.User.ID {
						if member.WorkspaceAuthorSettings.IsNotify(activity.Field, "workspace", activity.Verb, member.WorkspaceRole) {
							sendActivities = append(sendActivities, activity)
							continue
						}
						continue
					}
					if member.WorkspaceMemberSettings.IsNotify(activity.Field, "workspace", activity.Verb, member.WorkspaceRole) {
						sendActivities = append(sendActivities, activity)
						continue
					}
				}
				continue
			}
			if activity.Field != nil && *activity.Field == actField.Doc.Field.String() && activity.Verb == actField.VerbDeleted {
				if *activity.ActorId == member.User.ID {
					sendActivities = append(sendActivities, activity)
					continue
				}
			}

			if member.WorkspaceAdmin {
				if activity.Field != nil && *activity.Field == actField.Doc.Field.String() {
					continue
				}
				//sendActivities = append(sendActivities, activity)
				continue
			}
		}

		if len(sendActivities) == 0 {
			continue
		}

		content, textContent, err := getWorkspaceNotificationHTML(tx, sendActivities, &member.User)
		if err != nil {
			slog.Error("Make workspace notification HTML", "err", err)
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

func getWorkspaceNotificationHTML(tx *gorm.DB, activities []dao.WorkspaceActivity, targetUser *dao.User) (string, string, error) {
	result := ""
	//actorsChangesMap := make(map[string]map[string]dao.ProjectActivity)
	//actorsMap := make(map[string]dao.User)

	for _, act := range activities {
		if act.Field != nil && *act.Field == actField.Doc.Field.String() {
			a := dao.DocActivity{
				CreatedAt:     act.CreatedAt,
				Verb:          act.Verb,
				Field:         act.Field,
				OldValue:      act.OldValue,
				NewValue:      act.NewValue,
				Comment:       act.Comment,
				WorkspaceId:   act.WorkspaceId,
				ActorId:       act.ActorId,
				NewIdentifier: act.NewIdentifier,
				OldIdentifier: act.OldIdentifier,
				Notified:      act.Notified,
				TelegramMsgId: act.TelegramMsgId,
				Workspace:     act.Workspace,
				Actor:         act.Actor,
			}
			result += gocGetEmailHtml(tx, targetUser, &a)
		}
	}

	var templateBody dao.Template
	if err := tx.Where("name = ?", "workspace_body").First(&templateBody).Error; err != nil {
		return "", "", err
	}

	workspace := activities[0].Workspace
	workspace.AfterFind(tx)

	var buff bytes.Buffer
	if err := templateBody.ParsedTemplate.Execute(&buff, struct {
		Workspace     *dao.Workspace
		DocBreadcrumb string
		Title         string
		CreatedAt     time.Time
		WorkspaceBody string
	}{
		Workspace:     workspace,
		DocBreadcrumb: fmt.Sprintf("пространство '%s'", workspace.Slug),
		Title:         workspace.Name,
		CreatedAt:     time.Now(), //TODO: timezone
		WorkspaceBody: result,
	}); err != nil {
		return "", "", err
	}

	content := buff.String()
	return content, htmlStripPolicy.Sanitize(content), nil
}
