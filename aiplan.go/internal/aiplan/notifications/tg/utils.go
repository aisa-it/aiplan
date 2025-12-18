package tg

import (
	"fmt"
	"maps"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/go-telegram/bot"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
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

	issueUserTgId := getUserTgIdFromIssue(act.Issue)

	if act.NewIssueComment != nil && act.NewIssueComment.ReplyToCommentId.Valid {
		if err := tx.Preload("Actor").
			Where("workspace_id = ? ", act.WorkspaceId.String()).
			Where("project_id = ?", act.ProjectId.String()).
			Where("issue_id = ?", act.IssueId.String()).
			Where("id = ?", act.NewIssueComment.ReplyToCommentId.UUID).
			First(&act.NewIssueComment.OriginalComment).Error; err == nil {
			if act.NewIssueComment.OriginalComment.Actor.TelegramId != nil &&
				act.NewIssueComment.OriginalComment.Actor.CanReceiveNotifications() &&
				!act.NewIssueComment.OriginalComment.Actor.Settings.TgNotificationMute {
				issueUserTgId[act.NewIssueComment.OriginalComment.Actor.ID] = userTg{
					id:  *act.NewIssueComment.OriginalComment.Actor.TelegramId,
					loc: act.NewIssueComment.OriginalComment.Actor.UserTimezone,
				}
			}
		}
	}

	resMap := make(map[uuid.UUID]userTg)

	maps.Copy(resMap, GetUserTgIgDefaultWatchers(tx, act.ProjectId.String()))
	maps.Copy(resMap, issueUserTgId)

	userIds := make([]uuid.UUID, 0, len(resMap))
	for k := range resMap {
		userIds = append(userIds, k)
	}

	var projectMembers []dao.ProjectMember
	if err := tx.Where("project_id = ?", act.ProjectId).Where("member_id IN (?)", userIds).Find(&projectMembers).Error; err != nil {
		return []userTg{}
	}

	return filterIssueTgIdIsNotify(projectMembers, act.Issue.Author.ID, resMap, act.Field, act.Verb)
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
