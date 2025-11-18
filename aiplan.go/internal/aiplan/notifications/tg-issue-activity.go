package notifications

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

type TgNotifyIssue struct {
	TelegramService
}

func NewTgNotifyIssue(ts *TelegramService) *TgNotifyIssue {
	if ts == nil {
		return nil
	}
	return &TgNotifyIssue{TelegramService: *ts}
}

func (tni *TgNotifyIssue) Handle(activity dao.ActivityI) error {
	if a, ok := activity.(dao.IssueActivity); ok {
		tni.LogActivity(a)
	}
	return nil
}

func (tni *TgNotifyIssue) LogActivity(activity dao.IssueActivity) {
	if tni.disabled {
		return
	}

	go func() {
		msg := tgbotapi.NewMessage(0, "")
		msg.ParseMode = "MarkdownV2"
		var act *tgMsg

		if err := tni.db.Unscoped().
			Joins("Author").
			Joins("Workspace").
			Joins("Project").
			Joins("Parent").
			//Preload("Author").
			Preload("Assignees").
			Preload("Watchers").
			//Preload("Workspace").
			//Preload("Project").
			//Preload("Parent").
			Preload("Parent.Project").
			Where("issues.id = ?", activity.IssueId).
			First(&activity.Issue).Error; err != nil {
			slog.Error("Get issue for activity", "activityId", activity.Id, "err", err)
			return
		}

		act = NewTgActivity("issue")
		act.SetTitleTemplate(&activity)

		switch activity.Verb {
		case "created":
			if activity.Field == nil { // todo move to project
				msg.Text = act.Title("создал(-a)")

				if activity.Issue.Parent != nil {
					parentURL := activity.Issue.Parent.URL.String()
					msg.Text += Stelegramf("*%s*: [%s](%s)\n",
						fieldsTranslation["parent"],
						activity.Issue.Parent.String(),
						parentURL,
					)
				}

				if activity.Issue.Priority != nil {
					msg.Text += Stelegramf("*%s*: %s\n",
						fieldsTranslation["priority"],
						priorityTranslation[*activity.Issue.Priority],
					)
				}

				if activity.Issue.Assignees != nil && len(*activity.Issue.Assignees) > 0 {
					assignees := []string{}
					for _, assignee := range *activity.Issue.Assignees {
						if assignee.LastName == "" {
							assignees = append(assignees, Stelegramf("%s", assignee.Email))
						} else {
							assignees = append(assignees, Stelegramf("%s %s", assignee.FirstName, assignee.LastName))
						}
					}
					msg.Text += fmt.Sprintf("Исполнители: *%s*", strings.Join(assignees, "*, *"))
				}
			} else {
				switch fmt.Sprint(*activity.Field) {
				case "comment":
					if activity.NewIssueComment == nil {
						return
					}
					msg.Text = act.Title("прокомментировал(-a)")
					msg.Text += Stelegramf("```\n%s```",
						HtmlToTg(activity.NewIssueComment.CommentHtml.Body),
					)

				case "link":
					msg.Text = act.Title("добавил(-a) ссылку в")
					msg.Text += Stelegramf("[%s](%s)",
						activity.NewLink.Title,
						activity.NewLink.Url,
					)
				case "attachment":
					msg.Text = act.Title("добавил(-a) вложение в")
				case "linked":
					msg.Text = act.Title("добавил(-a) связь к задаче")
					msg.Text += Stelegramf("%s",
						activity.NewValue,
					)
				}
			}
		case "added":
			switch *activity.Field {
			case "labels":
				msg.Text = act.Title("добавил(-a) тег в")
				msg.Text += Stelegramf("%s",
					activity.NewLabel.Name,
				)
			case "sub_issue":
				activity.NewSubIssue.Project = activity.Issue.Project
				msg.Text = act.Title("изменил(-a)")
				msg.Text += Stelegramf("*Подзадача*: [%s](%s)",
					activity.NewSubIssue.FullIssueName(),
					activity.NewSubIssue.URL,
				)
			case "assignees":
				msg.Text = act.Title("добавил(-a) нового исполнителя в")
				msg.Text += Stelegramf("%s %s",
					activity.NewAssignee.FirstName,
					activity.NewAssignee.LastName,
				)
			case "watchers":
				msg.Text = act.Title("добавил(-a) нового наблюдателя в")
				msg.Text += Stelegramf("%s %s",
					activity.NewWatcher.FirstName,
					activity.NewWatcher.LastName,
				)
			}

		case "removed":
			switch *activity.Field {
			case "labels":
				msg.Text = act.Title("убрал(-a) тег из")
				msg.Text += Stelegramf("%s",
					activity.OldLabel.Name,
				)
			case "sub_issue":
				activity.OldSubIssue.Project = activity.Issue.Project
				msg.Text = act.Title("изменил(-a)")
				msg.Text += Stelegramf("*Подзадача*: ~[%s](%s)~",
					activity.OldSubIssue.FullIssueName(),
					activity.OldSubIssue.URL,
				)
			case "assignees":
				msg.Text = act.Title("убрал(-а) исполнителя из")
				msg.Text += Stelegramf("%s %s",
					activity.OldAssignee.FirstName,
					activity.OldAssignee.LastName,
				)
			case "watchers":
				msg.Text = act.Title("убрал(-а) наблюдателя из")
				msg.Text += Stelegramf("%s %s",
					activity.OldWatcher.FirstName,
					activity.OldWatcher.LastName,
				)
			}

		case "updated":
			switch *activity.Field {
			case "description":
				msg.Text = act.Title("изменил(-а) описание")
				msg.Text += Stelegramf("```\n%s```",
					HtmlToTg(activity.NewValue),
				)
			case "comment":
				msg.Text = act.Title("изменил(-a) комментарий")
				msg.Text += Stelegramf("```\n%s```",
					HtmlToTg(activity.NewIssueComment.CommentHtml.Body),
				)
			case "link":
				var old string
				if activity.OldValue != nil {
					old = *activity.OldValue
				}
				msg.Text = act.Title("изменил(-a) ссылку")
				msg.Text += Stelegramf("~%s~ [%s](%s)",
					old,
					activity.NewLink.Title,
					activity.NewLink.Url,
				)
			case "link_title":
				var old string
				if activity.OldValue != nil {
					old = *activity.OldValue
				}
				msg.Text = act.Title("изменил(-a) название ссылки")
				msg.Text += Stelegramf("~%s~ [%s](%s)",
					old,
					activity.NewLink.Title,
					activity.NewLink.Url,
				)

			case "link_url":
				var old string
				if activity.OldValue != nil {
					old = *activity.OldValue
				}
				msg.Text = act.Title("изменил(-a) url ссылки")
				msg.Text += Stelegramf("~%s~ [%s](%s)",
					old,
					activity.NewLink.Url,
					activity.NewLink.Url,
				)

			case "linked":
				var targetIssue dao.Issue

				if activity.OldIdentifier == nil && activity.NewIdentifier != nil {
					msg.Text = act.Title("добавил(-а) связь к ")
					targetIssue = *activity.NewIssueLinked
				}
				if activity.NewIdentifier == nil && activity.OldIdentifier != nil {
					msg.Text = act.Title("убрал(-а) связь из")
					targetIssue = *activity.OldIssueLinked
				}
				targetIssue.Project = activity.Issue.Project

				msg.Text += Stelegramf("*Задача*: [%s](%s)",
					targetIssue.FullIssueName(),
					targetIssue.URL,
				)
			case "target_date":
				oldValue := ""
				newValue := activity.NewValue
				if activity.OldValue != nil && *activity.OldValue != "<nil>" {
					oldValue = *activity.OldValue
				}

				act.newValTime = utils.FormatDateToSqlNullTime(newValue)
				act.oldValTime = utils.FormatDateToSqlNullTime(oldValue)

				if newValue == "<nil>" {
					newValue = ""
				} else if act.newValTime.Valid {
					newValue = "$$$$TargetDateTimeZ$$$$new$$$$"
				}

				if act.oldValTime.Valid {
					oldValue = "$$$$TargetDateTimeZ$$$$old$$$$"
				}

				if oldValue != "" {
					msg.Text = act.Title("изменил(-a)")
					msg.Text += Stelegramf("*%s*: ~%s~ %s",
						fieldsTranslation[*activity.Field],
						oldValue,
						newValue,
					)
				} else {
					msg.Text = act.Title("изменил(-a)")
					msg.Text += Stelegramf("*%s*: %s",
						fieldsTranslation[*activity.Field],
						newValue,
					)
				}

			case "parent":
				msg.Text = act.Title("изменил(-a)")
				var newName, newUrl, oldName, oldUrl string
				if activity.NewParentIssue != nil {
					activity.NewParentIssue.SetUrl()
					activity.NewParentIssue.Workspace = activity.Issue.Workspace
					activity.NewParentIssue.Project = activity.Issue.Project

					newName = activity.NewParentIssue.FullIssueName()
					newUrl = activity.NewParentIssue.URL.String()
				}

				if activity.OldParentIssue != nil {
					activity.OldParentIssue.SetUrl()
					activity.OldParentIssue.Workspace = activity.Issue.Workspace
					activity.OldParentIssue.Project = activity.Issue.Project

					oldName = activity.OldParentIssue.FullIssueName()
					oldUrl = activity.OldParentIssue.URL.String()
				}

				if newName != "" && oldName != "" {
					msg.Text += Stelegramf("*Родитель*: ~[%s](%s)~ [%s](%s)",
						oldName, oldUrl,
						newName, newUrl,
					)
				}
				if newName == "" && oldName != "" {
					msg.Text += Stelegramf("*Родитель*: ~[%s](%s)~ ",
						oldName, oldUrl,
					)
				}
				if newName != "" && oldName == "" {
					msg.Text += Stelegramf("*Родитель*: [%s](%s)",
						newName, newUrl,
					)
				}

				//}

			default:
				oldValue := ""
				newValue := activity.NewValue
				if activity.OldValue != nil && *activity.OldValue != "<nil>" {
					oldValue = *activity.OldValue
				}

				if oldValue == "<p></p>" {
					oldValue = ""
				}

				if activity.Field != nil {
					if *activity.Field == "priority" {
						oldValue = translateMap(priorityTranslation, activity.OldValue)
						newValue = translateMap(priorityTranslation, &activity.NewValue)
						if newValue == "" {
							newValue = priorityTranslation["<nil>"]
						}
						oldValue = capitalizeFirst(oldValue)
						newValue = capitalizeFirst(newValue)

					} else if *activity.Field == "start_date" || *activity.Field == "completed_at" {
						newT, err := FormatDate(newValue, "02.01.2006 15:04 MST", nil)
						oldValue, _ = FormatDate(oldValue, "02.01.2006 15:04 MST", nil)
						if newValue == "<nil>" {
							newValue = ""
						}
						if err == nil {
							newValue = newT
						}
					}
				}

				if oldValue != "" {
					msg.Text = act.Title("изменил(-a)")
					msg.Text += Stelegramf("*%s*: ~%s~ %s",
						fieldsTranslation[*activity.Field],
						oldValue,
						newValue,
					)
				} else {
					msg.Text = act.Title("изменил(-a)")
					msg.Text += Stelegramf("*%s*: %s",
						fieldsTranslation[*activity.Field],
						newValue,
					)
				}
			}
		case "deleted":
			switch *activity.Field {
			case "issue": // todo move to project
				msg.Text = Stelegramf("*%s %s* удалил(-a) %s",
					activity.Actor.FirstName,
					activity.Actor.LastName,
					activity.Issue.FullIssueName(),
				)
			case "link":
				msg.Text = act.Title("удалил(-a) ссылку из")
			case "attachment":
				msg.Text = act.Title("удалил(-a) вложение из")
			case "comment":
				msg.Text = act.Title("удалил(-a) комментарий из")
			case "linked":
				msg.Text = act.Title("удалил(-a) связь из")
				msg.Text += Stelegramf("%s",
					fmt.Sprint(*activity.OldValue),
				)
			}

		case "move":
			msg.Text = act.Title("перенес(-лa)")
			msg.Text += Stelegramf("из ~%s~ в %s ",
				fmt.Sprint(*activity.OldValue),
				activity.NewValue,
			)

		case "copied":
			msg.Text = act.Title("создал(-a) копию")
			msg.Text += Stelegramf("из %s в %s",
				fmt.Sprint(*activity.OldValue),
				activity.NewValue,
			)
		default:
			return
		}
		msg.Text = strings.ReplaceAll(msg.Text, "$$$$ISSUE$$$$", activity.Issue.URL.String())
		//}

		// TODO: make domain switch
		//activity.Issue.URL.Scheme = activity.Issue.Author.Domain.URL.Scheme
		//activity.Issue.URL.Host = activity.Issue.Author.Domain.URL.Host

		if act != nil {
			var msgIds []int64

			usersTelegram := act.GetIdsToSend(tni.db, &activity)

			for _, ut := range usersTelegram {
				if activity.ActivitySender.SenderTg == ut.id {
					continue
				}

				r, err := tni.bot.Send(MsgWithUser(msg, ut, act))
				if err != nil && err.Error() != "Bad Request: chat not found" {
					slog.Error("Telegram send error", "issueActivities", err, "activityId", activity.Id)

				}
				msgIds = append(msgIds, int64(r.MessageID))
			}

			if err := tni.db.Model(&activity).Select("telegram_msg_ids").Update("telegram_msg_ids", pq.Int64Array(msgIds)).Error; err != nil {
				slog.Error("Update activity tg msg ids", "err", err)
			}
		}
	}()
}

func getUserTgIdIssueActivity(tx *gorm.DB, activity interface{}) []userTg {
	var act *dao.IssueActivity
	if v, ok := activity.(*dao.IssueActivity); ok {
		act = v
	} else {
		return []userTg{}
	}

	issueUserTgId := GetUserTgIdFromIssue(act.Issue)
	authorId := act.Issue.Author.ID

	if act.NewIssueComment != nil && act.NewIssueComment.ReplyToCommentId.Valid {
		if err := tx.Preload("Actor").
			Where("workspace_id = ? ", act.WorkspaceId).
			Where("project_id = ?", act.ProjectId).
			Where("issue_id = ?", act.IssueId).
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

	defaultWatcherUserTgId := GetUserTgIgDefaultWatchers(tx, act.ProjectId)
	resMap := utils.MergeMaps(defaultWatcherUserTgId, issueUserTgId)

	userIds := make([]string, 0, len(resMap))
	for k := range resMap {
		userIds = append(userIds, k)
	}

	var projectMembers []dao.ProjectMember
	if err := tx.Where("project_id = ?", act.ProjectId).Where("member_id IN (?)", userIds).Find(&projectMembers).Error; err != nil {
		return []userTg{}
	}

	return filterIssueTgIdIsNotify(projectMembers, authorId, resMap, act.Field, act.Verb)
}

func filterIssueTgIdIsNotify(projectMembers []dao.ProjectMember, authorId string, userTgId map[string]userTg, field *string, verb string) []userTg {
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
