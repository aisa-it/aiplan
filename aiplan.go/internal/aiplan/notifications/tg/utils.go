package tg

import (
	"database/sql"
	"fmt"
	"log/slog"
	"strings"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/go-telegram/bot"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

const (
	targetDateTimeZ = "TargetDateTimeZ"
)

func Stelegramf(format string, a ...any) string {
	for i, v := range a {
		switch tv := v.(type) {
		case string:
			a[i] = bot.EscapeMarkdown(tv)
		}
	}

	return fmt.Sprintf(format, a...)
}

func getUserTg(user dao.User) (userTg, bool) {
	if user.TelegramId == nil {
		return userTg{}, false
	}
	if !user.CanReceiveNotifications() {
		return userTg{}, false
	}
	if user.Settings.TgNotificationMute {
		return userTg{}, false
	}
	return userTg{id: *user.TelegramId, loc: user.UserTimezone}, true
}

func getUserTgIdIssueActivity(tx *gorm.DB, act *dao.IssueActivity) []userTg {
	users := make(map[uuid.UUID]userTg)

	addOriginalCommentAuthor(tx, act, users)
	addDefaultWatchers(tx, act.ProjectId, users)
	addIssueUsers(act.Issue, users)

	var projectMembers []dao.ProjectMember
	if err := tx.
		Where("project_id = ?", act.ProjectId).
		Where("member_id IN (?)", utils.MapToSlice(users, func(k uuid.UUID, t userTg) uuid.UUID { return k })).
		Find(&projectMembers).Error; err != nil {
		return []userTg{}
	}

	return filterEntityTgIdIsNotify(projectMembers, act.Issue.Author.ID, users, act.Field, act.Verb, "issue")
}

func getUserTgProjectActivity(tx *gorm.DB, act *dao.ProjectActivity) []userTg {
	users := make(map[uuid.UUID]userTg)
	addDefaultWatchers(tx, act.ProjectId, users)
	addIssueUsers(act.NewIssue, users)
	addProjectAdmin(tx, act, users)

	var projectMember []dao.ProjectMember
	if err := tx.
		Where("project_id = ?", act.ProjectId).
		Where("member_id IN (?)", utils.MapToSlice(users, func(k uuid.UUID, t userTg) uuid.UUID { return k })).
		Find(&projectMember).Error; err != nil {
		return []userTg{}
	}
	return filterEntityTgIdIsNotify(projectMember, act.ActorId.UUID, users, act.Field, act.Verb, "project")
}

func getUserTgDocActivity(tx *gorm.DB, act *dao.DocActivity) []userTg {
	userIds := append(append(append([]uuid.UUID{act.Doc.CreatedById}, act.Doc.EditorsIDs...), act.Doc.ReaderIDs...), act.Doc.WatcherIDs...)
	var workspaceMembers []dao.WorkspaceMember
	if err := tx.Joins("Member").
		Where("workspace_id = ?", act.WorkspaceId).
		Where("workspace_members.member_id IN (?)", userIds).Find(&workspaceMembers).Error; err != nil {
		return []userTg{}
	}
	users := make(map[uuid.UUID]userTg, len(workspaceMembers))
	for _, member := range workspaceMembers {
		if usr, ok := getUserTg(*member.Member); ok {
			users[member.MemberId] = usr
		}
	}
	return filterDocTgIdIsNotify(workspaceMembers, act.ActorId.UUID, users, act.Field, act.Verb)
}

func filterDocTgIdIsNotify(wm []dao.WorkspaceMember, authorId uuid.UUID, userTgId map[uuid.UUID]userTg, field *string, verb string) []userTg {
	res := make([]userTg, 0)
	for _, member := range wm {
		if member.MemberId == authorId {
			if member.NotificationAuthorSettingsTG.IsNotify(field, "doc", verb, member.Role) {
				if v, ok := userTgId[authorId]; ok {
					res = append(res, v)
					continue
				}
			} else {
				continue
			}
		}

		if member.NotificationSettingsTG.IsNotify(field, "doc", verb, member.Role) {
			if v, ok := userTgId[member.MemberId]; ok {
				res = append(res, v)
				continue
			}
		}
	}
	return res
}

func filterProjectTgIdIsNotify(wm []dao.ProjectMember, authorId uuid.UUID, userTgId map[uuid.UUID]userTg, field *string, verb string, adminMembers map[uuid.UUID]struct{}) []userTg {
	res := make([]userTg, 0)
	for _, member := range wm {
		if member.Role == types.AdminRole && field != nil && *field == "issue" && authorId != member.MemberId {
			if _, ok := adminMembers[member.MemberId]; !ok {
				continue
			}
		}
		if member.MemberId == authorId {
			if member.NotificationAuthorSettingsTG.IsNotify(field, "project", verb, member.Role) {
				if v, ok := userTgId[authorId]; ok {
					res = append(res, v)
					continue
				}
			} else {
				continue
			}
		}

		if member.NotificationSettingsTG.IsNotify(field, "project", verb, member.Role) {
			if v, ok := userTgId[member.MemberId]; ok {
				res = append(res, v)
				continue
			}
		}
	}
	return res
}

func filterEntityTgIdIsNotify(projectMembers []dao.ProjectMember, authorId uuid.UUID, userTgId map[uuid.UUID]userTg, field *string, verb string, entity string) []userTg {
	res := make([]userTg, 0)
	for _, member := range projectMembers {
		if member.MemberId == authorId {
			if member.NotificationAuthorSettingsTG.IsNotify(field, entity, verb, member.Role) {
				if v, ok := userTgId[authorId]; ok {
					res = append(res, v)
					continue
				}
			}
		}

		if member.NotificationSettingsTG.IsNotify(field, entity, verb, member.Role) {
			if v, ok := userTgId[member.MemberId]; ok {
				res = append(res, v)
				continue
			}
		}
	}
	return res
}

func addOriginalCommentAuthor(tx *gorm.DB, act *dao.IssueActivity, users map[uuid.UUID]userTg) {
	if act.NewIssueComment == nil || !act.NewIssueComment.ReplyToCommentId.Valid {
		return
	}

	err := tx.Preload("Actor").
		Where("workspace_id = ? ", act.WorkspaceId).
		Where("project_id = ?", act.ProjectId).
		Where("issue_id = ?", act.IssueId).
		Where("id = ?", act.NewIssueComment.ReplyToCommentId.UUID).
		First(&act.NewIssueComment.OriginalComment).Error

	if err != nil || act.NewIssueComment.OriginalComment.Actor == nil {
		return
	}

	if user, ok := getUserTg(*act.NewIssueComment.OriginalComment.Actor); ok {
		users[act.NewIssueComment.OriginalComment.Actor.ID] = user
	}
}

func addProjectAdmin(tx *gorm.DB, act *dao.ProjectActivity, users map[uuid.UUID]userTg) {
	var member []dao.ProjectMember
	if err := tx.Joins("Member").
		Where("project_id = ?", act.ProjectId).
		Where("project_members.role = ?", types.AdminRole).
		Find(&member).Error; err != nil {
		return
	}

	for _, v := range member {
		if *act.Field == actField.Issue.Field.String() {
			continue
		}
		if _, ok := users[v.MemberId]; ok {
			continue
		} else {
			if usr, ok := getUserTg(*v.Member); ok {
				users[v.MemberId] = usr
			}
		}
	}
}

func addDefaultWatchers(tx *gorm.DB, projectId uuid.UUID, users map[uuid.UUID]userTg) {
	var defaultWatchers []struct {
		ID           uuid.UUID
		TelegramId   int64
		UserTimezone types.TimeZone
		Settings     types.UserSettings
	}

	err := tx.Model(&dao.ProjectMember{}).
		Select("users.id, users.telegram_id, users.user_timezone, users.settings").
		Joins("JOIN users ON users.id = project_members.member_id").
		Where("project_id = ? AND is_default_watcher = true", projectId).
		Scan(&defaultWatchers).Error

	if err != nil {
		slog.Error("Fetch default watchers for activity", "project_id", projectId, "err", err)
		return
	}

	for _, w := range defaultWatchers {
		if _, exists := users[w.ID]; exists {
			continue
		}

		if user, ok := getUserTg(dao.User{
			ID:           w.ID,
			TelegramId:   &w.TelegramId,
			UserTimezone: w.UserTimezone,
			Settings:     w.Settings,
		}); ok {
			users[w.ID] = user
		}
	}
}

func addIssueUsers(issue *dao.Issue, users map[uuid.UUID]userTg) {
	if issue == nil {
		return
	}
	if user, ok := getUserTg(*issue.Author); ok {
		users[issue.Author.ID] = user
	}

	if issue.Assignees != nil {
		for _, assignee := range *issue.Assignees {
			if _, exists := users[assignee.ID]; exists {
				continue
			}
			if user, ok := getUserTg(assignee); ok {
				users[assignee.ID] = user
			}
		}
	}

	if issue.Watchers != nil {
		for _, watcher := range *issue.Watchers {
			if _, exists := users[watcher.ID]; exists {
				continue
			}
			if user, ok := getUserTg(watcher); ok {
				users[watcher.ID] = user
			}
		}
	}
}

func strReplace(in string) string {
	out := strings.Split(in, "_")
	return "$$$" + strings.Join(out, "$$$") + "$$$"
}

func msgReplace(user userTg, msg TgMsg) TgMsg {
	for k, v := range msg.replace {
		key := k
		keys := strings.Split(k, "_")
		if len(keys) > 1 {
			key = keys[0]
		}
		switch key {
		case targetDateTimeZ:
			if strNeW, err := utils.FormatDateStr(v.(sql.NullTime).Time.String(), "02.01.2006 15:04", &user.loc); err == nil {
				msg.body = strings.ReplaceAll(msg.body, strReplace(k), Stelegramf("%s", strNeW))
			} else {
				return NewTgMsg()
			}
		}
	}
	return msg
}

func getUserName(user *dao.User) string {
	if user == nil {
		return "Новый пользователь"
	}
	if user.LastName == "" {
		return fmt.Sprintf("%s", user.Email)
	}
	return fmt.Sprintf("%s %s", user.FirstName, user.LastName)
}

func getExistUser(user ...*dao.User) *dao.User {
	for _, u := range user {
		if u != nil {
			return u
		}
	}
	return nil
}

func (m TgMsg) IsEmpty() bool {
	if m.title == "" && m.body == "" {
		return true
	}
	return false
}
