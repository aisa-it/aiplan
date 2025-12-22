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

var fieldsTranslation map[actField.ActivityField]string = map[actField.ActivityField]string{
	actField.Name.Field:          "Название",
	actField.Parent.Field:        "Родитель",
	actField.Priority.Field:      "Приоритет",
	actField.Status.Field:        "Статус",
	actField.Description.Field:   "Описание",
	actField.TargetDate.Field:    "Срок исполнения",
	actField.StartDate.Field:     "Дата начала",
	actField.CompletedAt.Field:   "Дата завершения",
	actField.Label.Field:         "Теги",
	actField.Assignees.Field:     "Исполнители",
	actField.Blocking.Field:      "Блокирует",
	actField.Blocks.Field:        "Заблокирована",
	actField.EstimatePoint.Field: "Оценки",
	actField.SubIssue.Field:      "Подзадачи",
	actField.Identifier.Field:    "Идентификатор",
	actField.Emoj.Field:          "Emoji",
	actField.Title.Field:         "Название",
}

var priorityTranslation map[string]string = map[string]string{
	"urgent": "Критический",
	"high":   "Высокий",
	"medium": "Средний",
	"low":    "Низкий",
	"":       "Не выбран",
}

const (
	targetDateTimeZ = "TargetDateTimeZ"
)

func translateMap(tMap map[string]string, str *string) string {
	key := "<nil>"
	if str != nil {
		key = *str
	}
	if v, ok := tMap[key]; ok {
		return v
	}
	return ""
}

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

	userIds := make([]uuid.UUID, 0, len(users))
	for k := range users {
		userIds = append(userIds, k)
	}

	var projectMembers []dao.ProjectMember
	if err := tx.Where("project_id = ?", act.ProjectId).Where("member_id IN (?)", userIds).Find(&projectMembers).Error; err != nil {
		return []userTg{}
	}

	return filterIssueTgIdIsNotify(projectMembers, act.Issue.Author.ID, users, act.Field, act.Verb)
}

func filterIssueTgIdIsNotify(projectMembers []dao.ProjectMember, authorId uuid.UUID, userTgId map[uuid.UUID]userTg, field *string, verb string) []userTg {
	res := make([]userTg, 0)
	for _, member := range projectMembers {
		if member.MemberId == authorId {
			if member.NotificationAuthorSettingsTG.IsNotify(field, "issue", verb, member.Role) {
				if v, ok := userTgId[authorId]; ok {
					res = append(res, v)
					continue
				}
			}
		}

		if member.NotificationSettingsTG.IsNotify(field, "issue", verb, member.Role) {
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

func msgReplace(user userTg, msg *TgMsg) TgMsg {
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
	return *msg
}
