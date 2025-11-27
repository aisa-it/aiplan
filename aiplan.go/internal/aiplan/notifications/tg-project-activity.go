package notifications

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

type TgNotifyProject struct {
	TelegramService
}

func NewTgNotifyProject(ts *TelegramService) *TgNotifyProject {
	if ts == nil {
		return nil
	}
	return &TgNotifyProject{TelegramService: *ts}
}

func (tnp *TgNotifyProject) Handle(activity dao.ActivityI) error {
	if a, ok := activity.(dao.ProjectActivity); ok {
		tnp.LogActivity(a)
	}
	return nil
}

func (tnp *TgNotifyProject) LogActivity(activity dao.ProjectActivity) {
	if tnp.disabled {
		return
	}

	go func() {
		msg := tgbotapi.NewMessage(0, "")
		msg.ParseMode = "MarkdownV2"
		var act *tgMsg

		if err := tnp.db.Unscoped().
			Joins("Workspace").
			Joins("ProjectLead").
			Where("projects.id = ?", activity.ProjectId).
			First(&activity.Project).Error; err != nil {
			slog.Error("Get project for activity", "activityId", activity.Id, "err", err)
			return
		}

		act = NewTgActivity("project")
		act.SetTitleTemplate(&activity)
		if activity.Field == nil {
			return
		}
		switch activity.Verb {
		case "created", "copied", "added":
			switch actField.ActivityField(*activity.Field) {
			case actField.FieldIssue:
				if err := tnp.db.Unscoped().
					Joins("Author").
					Joins("Workspace").
					Joins("Project").
					Joins("Parent").
					Preload("Assignees").
					Preload("Watchers").
					Where("issues.id = ?", activity.NewIssue.ID).
					First(&activity.NewIssue).Error; err != nil {
					slog.Error("Get issue for activity", "activityId", activity.Id, "err", err)
					return
				}

				switch activity.Verb {
				case "created":
					msg.Text = act.Title("создал(-a) задачу в проекте")
				case "copied":
					msg.Text = act.Title("создал(-a) копию задачи в проекте")
				case "added":
					msg.Text = act.Title("добавил(-a) задачу в проекте")
				}
				msg.Text += Stelegramf("[%s](%s)\n",
					activity.NewIssue.FullIssueName(),
					activity.NewIssue.URL.String(),
				)
				if activity.NewIssue.Parent != nil {
					activity.NewIssue.Parent.SetUrl()

					parentURL := activity.NewIssue.Parent.URL.String()
					msg.Text += Stelegramf("*%s*: [%s](%s)\n",
						fieldsTranslation["parent"],
						activity.NewIssue.Parent.String(),
						parentURL,
					)
				}

				if activity.NewIssue.Priority != nil {
					msg.Text += Stelegramf("*%s*: %s\n",
						fieldsTranslation["priority"],
						priorityTranslation[*activity.NewIssue.Priority],
					)
				}

				if activity.NewIssue.Assignees != nil && len(*activity.NewIssue.Assignees) > 0 {
					assignees := []string{}
					for _, assignee := range *activity.NewIssue.Assignees {
						if assignee.LastName == "" {
							assignees = append(assignees, Stelegramf("%s", assignee.Email))
						} else {
							assignees = append(assignees, Stelegramf("%s %s", assignee.FirstName, assignee.LastName))
						}
					}
					msg.Text += fmt.Sprintf("Исполнители: *%s*", strings.Join(assignees, "*, *"))
				}
			case actField.FieldTemplate:
				msg.Text = act.Title("создал(-a) шаблон задачи в")
				msg.Text += Stelegramf("*Название*: %s\n", activity.NewIssueTemplate.Name)
				msg.Text += Stelegramf("```\n%s```",
					HtmlToTg(activity.NewIssueTemplate.Template.String()),
				)
			case actField.FieldStatus:
				msg.Text = act.Title("создал(-a) статус в")
				msg.Text += Stelegramf("*Название*: %s\n*Группа*: %s", activity.NewState.Name, stateTranslate(activity.NewState.Group))
			case actField.FieldLabel:
				msg.Text = act.Title("создал(-a) тег в")
				msg.Text += Stelegramf("*Название*: %s", activity.NewLabel.Name)
			case actField.FieldMember:
				if *activity.Field != "added" {
					return
				}
				msg.Text = act.Title("добавил(-a) участника в")
				msg.Text += Stelegramf("%s\n", getUserName(activity.NewMember))
				msg.Text += Stelegramf("*Роль:* %s", memberRoleStr(activity.NewValue))
			case actField.FieldDefaultWatchers, actField.FieldDefaultAssignees:
				if *activity.Field != "added" {
					return
				}
				if *activity.Field == actField.FieldDefaultWatchers.String() {
					msg.Text = act.Title("добавил(-a) наблюдателя по умолчанию в")
					msg.Text += Stelegramf("%s\n", getUserName(activity.NewDefaultWatcher))
				}
				if *activity.Field == actField.FieldDefaultAssignees.String() {
					msg.Text = act.Title("добавил(-a) исполнителя по умолчанию в")
					msg.Text += Stelegramf("%s\n", getUserName(activity.NewDefaultAssignee))
				}
			default:
				return
			}
		case "updated":
			switch actField.ActivityField(*activity.Field) {
			case actField.FieldIdentifier, actField.FieldName:
				var oldV string
				if activity.OldValue != nil {
					oldV = *activity.OldValue
				}
				msg.Text = act.Title("изменил(-a) в")
				msg.Text += Stelegramf("*%s*: ~%s~ %s", fieldsTranslation[*activity.Field], oldV, activity.NewValue)

			case actField.FieldLogo:
				msg.Text = act.Title("изменил(-a) в проекте")
				msg.Text += Stelegramf("*Логотип проекта*")
			case actField.FieldPublic:
				if activity.NewValue == "true" {
					msg.Text = act.Title("сделал(-a) публичным")
				} else {
					msg.Text = act.Title("сделал(-a) приватным")
				}

			case actField.FieldLabelName, actField.FieldLabelColor:
				action := strings.Split(*activity.Field, "_")[1]
				msg.Text = act.Title("изменил(-a) в")
				switch action {
				case "color":
					msg.Text += Stelegramf("*Тег '%s'*: изменен цвет", activity.NewLabel.Name)

				case "name":
					msg.Text += Stelegramf("*Название Тега*: ~%s~ %s", fmt.Sprint(*activity.OldValue), activity.NewValue)
				}

			case actField.FieldStatusColor, actField.FieldStatusGroup, actField.FieldStatusDescription, actField.FieldStatusName:
				action := strings.Split(*activity.Field, "_")[1]
				msg.Text = act.Title("изменил(-a) в")
				switch action {
				case "color":
					msg.Text += Stelegramf("*Статус '%s'*: изменен цвет", activity.NewState.Name)
				case "group":
					msg.Text += Stelegramf("*Группу Статуса*: %s\n", activity.NewState.Name)
					msg.Text += Stelegramf("~%s~ %s", stateTranslate(fmt.Sprint(*activity.OldValue)), stateTranslate(activity.NewValue))
				case "description":
					msg.Text += Stelegramf("*Описание Статуса*: %s", activity.NewState.Name)
					msg.Text += Stelegramf("```\n%s```",
						HtmlToTg(activity.NewValue),
					)
				case "name":
					msg.Text += Stelegramf("*Название Статуса*: ~%s~ %s", fmt.Sprint(*activity.OldValue), activity.NewValue)
				}

			case actField.FieldTemplateName, actField.FieldTemplateTemplate:
				action := strings.Split(*activity.Field, "_")[1]
				msg.Text = act.Title("изменил(-a) в проекте")
				switch action {
				case "name":
					msg.Text += Stelegramf("*Название шаблона задачи*: \n~%s~ %s", fmt.Sprint(*activity.OldValue), activity.NewValue)
				case "template":
					msg.Text += Stelegramf("*Шаблон задачи '%s'*:", activity.NewIssueTemplate.Name)
					msg.Text += Stelegramf("```\n%s```",
						HtmlToTg(activity.NewValue),
					)
				}
			case actField.FieldStatusDefault:
				msg.Text = act.Title("изменил(-a) статус по умолчанию в")
				msg.Text += Stelegramf("~%s~ %s", activity.OldState.Name, activity.NewState.Name)
			case actField.FieldRole:
				msg.Text = act.Title("изменил(-a) роль пользователя в")
				msg.Text += Stelegramf("%s\n", getUserName(activity.NewRole))
				msg.Text += Stelegramf("*Роль*: ~%s~ %s", memberRoleStr(fmt.Sprint(*activity.OldValue)), memberRoleStr(activity.NewValue))
			case actField.FieldProjectLead:
				msg.Text = act.Title("изменил(-a) лидера проекта в")
				msg.Text += Stelegramf("~%s~ %s", getUserName(activity.OldProjectLead), getUserName(activity.NewProjectLead))
			default:
				return
			}
		case "removed":
			switch actField.ActivityField(*activity.Field) {
			case actField.FieldIssue:
				msg.Text = act.Title("убрал(-a) задачу из")
				msg.Text += Stelegramf("*Задача:* %s", fmt.Sprint(*activity.OldValue))
			case actField.FieldMember:
				msg.Text = act.Title("убрал(-a) участника из")
				msg.Text += Stelegramf("%s", getUserName(activity.OldMember))
			case actField.FieldDefaultWatchers, actField.FieldDefaultAssignees:
				if *activity.Field == "default_watchers" {
					msg.Text = act.Title("убрал(-a) наблюдателя по умолчанию в")
					msg.Text += Stelegramf("%s\n", getUserName(activity.OldDefaultWatcher))
				}
				if *activity.Field == actField.FieldDefaultAssignees.String() {
					msg.Text = act.Title("убрал(-a) исполнителя по умолчанию в")
					msg.Text += Stelegramf("%s\n", getUserName(activity.OldDefaultAssignee))
				}

			default:
				return
			}
		case "deleted":
			msg.Text = act.Title("удалил(-a) из")
			switch actField.ActivityField(*activity.Field) {
			case actField.FieldLabel:
				msg.Text += Stelegramf("*Тег*: ~%s~", fmt.Sprint(*activity.OldValue))
			case actField.FieldStatus:
				msg.Text += Stelegramf("*Статус*: ~%s~", fmt.Sprint(*activity.OldValue))
			case actField.FieldTemplate:
				msg.Text += Stelegramf("*Шаблон*: ~%s~", fmt.Sprint(*activity.OldValue))
			default:
				return
			}
		default:
			return
		}

		msg.Text = strings.ReplaceAll(msg.Text, "$$$$PROJECT$$$$", activity.Project.URL.String())
		//}

		// TODO: make domain switch
		//activity.Issue.URL.Scheme = activity.Issue.Author.Domain.URL.Scheme
		//activity.Issue.URL.Host = activity.Issue.Author.Domain.URL.Host

		if act != nil {
			var msgIds []int64

			usersTelegram := act.GetIdsToSend(tnp.db, &activity)
			for _, ut := range usersTelegram {
				msg.ChatID = ut.id
				msg.DisableWebPagePreview = true
				r, err := tnp.bot.Send(msg)
				if err != nil && err.Error() != "Bad Request: chat not found" {
					slog.Error("Telegram send error", "projectActivities", err, "activityId", activity.Id)
				}
				msgIds = append(msgIds, int64(r.MessageID))
			}

			if err := tnp.db.Model(&activity).Select("telegram_msg_ids").Update("telegram_msg_ids", pq.Int64Array(msgIds)).Error; err != nil {
				slog.Error("Update activity tg msg ids", "err", err)
			}
		}
	}()

}

func getUserTgIdProjectActivity(tx *gorm.DB, activity interface{}) []userTg {
	var act *dao.ProjectActivity
	if v, ok := activity.(*dao.ProjectActivity); ok {
		act = v
	} else {
		return []userTg{}
	}
	var issueUserTgId, defaultWatcherUserTgId map[string]userTg

	query := tx.Joins("Member").
		Where("project_id = ?", act.ProjectId)

	if act.NewIssue != nil {
		act.NewIssue.Author = act.Actor
		act.NewIssue.Workspace = act.Workspace
		issueUserTgId = GetUserTgIdFromIssue(act.NewIssue)
		defaultWatcherUserTgId = GetUserTgIgDefaultWatchers(tx, act.ProjectId)
		userMap := utils.MergeMaps(issueUserTgId, defaultWatcherUserTgId)
		ids := make([]string, 0, len(userMap))
		for id, _ := range userMap {
			ids = append(ids, id)
		}
		query = query.Where("member_id in (?)", ids)
	} else {
		query = query.Where("project_members.role = ?", types.AdminRole)

	}

	var projectMembers []dao.ProjectMember

	if err := query.Find(&projectMembers).Error; err != nil {
		return []userTg{}
	}

	iMember := utils.MergeMaps(defaultWatcherUserTgId, issueUserTgId)
	adminMemberIssue := make(map[string]struct{})

	projMap := make(map[string]userTg)
	for _, member := range projectMembers {
		if _, ok := iMember[member.MemberId]; ok {
			adminMemberIssue[member.MemberId] = struct{}{}
		}
		if member.Member.TelegramId != nil && member.Member.CanReceiveNotifications() && !member.Member.Settings.TgNotificationMute {
			projMap[member.MemberId] = userTg{
				id:  *member.Member.TelegramId,
				loc: member.Member.UserTimezone,
			}
		}
	}

	resMap := utils.MergeMaps(defaultWatcherUserTgId, issueUserTgId, projMap)

	return filterProjectTgIdIsNotify(projectMembers, *act.ActorId, resMap, act.Field, act.Verb, adminMemberIssue)
}

func filterProjectTgIdIsNotify(wm []dao.ProjectMember, authorId string, userTgId map[string]userTg, field *string, verb string, adminMembers map[string]struct{}) []userTg {
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

func stateTranslate(str string) string {
	switch str {
	case "backlog":
		return "Создано"
	case "unstarted":
		return "Не начато"
	case "cancelled":
		return "Отменено"
	case "completed":
		return "Завершено"
	case "started":
		return "Начато"
	}
	return str
}

func memberRoleStr(str string) string {
	switch str {
	case "5":
		return "Гость"
	case "10":
		return "Участник"
	case "15":
		return "Администратор"
	}
	return str
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
