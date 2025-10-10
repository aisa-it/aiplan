package notifications

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

type TgNotifyWorkspace struct {
	TelegramService
}

func NewTgNotifyWorkspace(ts *TelegramService) *TgNotifyWorkspace {
	if ts == nil {
		return nil
	}
	return &TgNotifyWorkspace{TelegramService: *ts}
}

func (tnw *TgNotifyWorkspace) Handle(activity dao.ActivityI) error {
	if a, ok := activity.(dao.WorkspaceActivity); ok {
		tnw.LogActivity(a)
	}
	return nil
}

func (tnw *TgNotifyWorkspace) LogActivity(activity dao.WorkspaceActivity) {
	if tnw.disabled {
		return
	}

	go func() {
		msg := tgbotapi.NewMessage(0, "")
		msg.ParseMode = "MarkdownV2"
		var act *tgMsg

		if err := tnw.db.Unscoped().
			Joins("Owner").
			Where("workspaces.id = ?", activity.WorkspaceId).
			First(&activity.Workspace).Error; err != nil {
			slog.Error("Get workspace for activity", "activityId", activity.Id, "err", err)
			return
		}

		act = NewTgActivity("workspace")
		act.SetTitleTemplate(&activity)
		if activity.Field == nil {
			return
		}
		switch activity.Verb {
		case "created":
			switch *activity.Field {
			case "project":
				msg.Text = act.Title("создал(-a) проект в пространстве")
				msg.Text += Stelegramf("[%s](%s)", activity.NewProject.Name, activity.NewProject.URL.String())
			case "doc":
				msg.Text = act.Title("создал(-a) корневой документ в пространстве")
				msg.Text += Stelegramf("[%s](%s)", activity.NewDoc.Title, activity.NewDoc.URL.String())
			case "form":
				msg.Text = act.Title("создал(-a) форму в пространстве")
				msg.Text += Stelegramf("[%s](%s)", activity.NewForm.Title, activity.NewForm.URL.String())

			default:
				return
			}
		case "updated":
			switch *activity.Field {
			case "description":
				msg.Text = act.Title("изменил(-a) в пространстве")
				msg.Text += Stelegramf("*%s*:", fieldsTranslation[*activity.Field])
				msg.Text += Stelegramf("```\n%s```",
					HtmlToTg(activity.NewValue),
				)
			case "integration_token":
				msg.Text = act.Title("изменил(-a) в пространстве")
				msg.Text += Stelegramf("*Токен для работы интеграций*")
			case "owner":
				msg.Text = act.Title("изменил(-a) владельца пространства")
				msg.Text += Stelegramf("~%s~ %s", getUserName(activity.OldOwner), getUserName(activity.NewOwner))
			case "name":
				var oldV string
				if activity.OldValue != nil {
					oldV = *activity.OldValue
				}
				msg.Text = act.Title("изменил(-a) в пространстве")
				msg.Text += Stelegramf("*Имя пространства*: ~%s~ %s", oldV, activity.NewValue)
			case "logo":
				msg.Text = act.Title("изменил(-a) в пространстве")
				msg.Text += Stelegramf("*Логотип пространства*")
			case "role":
				msg.Text = act.Title("изменил(-a) роль пользователя в пространстве")
				msg.Text += Stelegramf("%s\n", getUserName(activity.NewRole))
				msg.Text += Stelegramf("*Роль*: ~%s~ %s", memberRoleStr(fmt.Sprint(*activity.OldValue)), memberRoleStr(activity.NewValue))
			default:
				return
			}
		case "added":
			switch *activity.Field {
			case "member":
				msg.Text = act.Title("добавил(-a) участника в пространство")
				msg.Text += Stelegramf("%s\n", getUserName(activity.NewMember))
				msg.Text += Stelegramf("*Роль:* %s", memberRoleStr(activity.NewValue))
			case "integration":
				msg.Text = act.Title("добавил(-a) интеграцию в пространство")
				msg.Text += Stelegramf("%s\n", activity.NewValue)
			default:
				return
			}
		case "removed":
			switch *activity.Field {
			case "member":
				msg.Text = act.Title("убрал(-a) участника из пространства")
				msg.Text += Stelegramf("%s", getUserName(activity.OldMember))
			case "integration":
				msg.Text = act.Title("убрал(-a) интеграцию из пространства")
				if activity.OldValue != nil {
					msg.Text += Stelegramf("%s\n", *activity.OldValue)
				}
			default:
				return
			}
		case "deleted":
			msg.Text = act.Title("удалил(-a) из пространства")

			switch *activity.Field {
			case "form":
				msg.Text += Stelegramf("*Форму*: ~%s~", fmt.Sprint(*activity.OldValue))
			case "doc":
				msg.Text += Stelegramf("*Корневой документ*: ~%s~", fmt.Sprint(*activity.OldValue))
			case "project":
				msg.Text += Stelegramf("*Проект*: ~%s~", fmt.Sprint(*activity.OldValue))
			default:
				return
			}
		default:
			return
		}

		msg.Text = strings.ReplaceAll(msg.Text, "$$$$WORKSPACE$$$$", activity.Workspace.URL.String())
		//}

		// TODO: make domain switch
		//activity.Issue.URL.Scheme = activity.Issue.Author.Domain.URL.Scheme
		//activity.Issue.URL.Host = activity.Issue.Author.Domain.URL.Host

		if act != nil {
			var msgIds []int64

			usersTelegram := act.GetIdsToSend(tnw.db, &activity)
			for _, ut := range usersTelegram {
				msg.ChatID = ut.id
				msg.DisableWebPagePreview = true
				r, err := tnw.bot.Send(msg)
				if err != nil && err.Error() != "Bad Request: chat not found" {
					slog.Error("Telegram send error", "workspaceActivities", err, "activityId", activity.Id)
				}
				msgIds = append(msgIds, int64(r.MessageID))
			}

			if err := tnw.db.Model(&activity).Select("telegram_msg_ids").Update("telegram_msg_ids", pq.Int64Array(msgIds)).Error; err != nil {
				slog.Error("Update activity tg msg ids", "err", err)
			}
		}
	}()

}

func getUserTgIdWorkspaceActivity(tx *gorm.DB, activity interface{}) []userTg {
	var act *dao.WorkspaceActivity
	if v, ok := activity.(*dao.WorkspaceActivity); ok {
		act = v
	} else {
		return []userTg{}
	}

	var wm []dao.WorkspaceMember
	if err := tx.Joins("Member").
		Where("workspace_id = ?", act.WorkspaceId).
		Where("workspace_members.role = ?", types.AdminRole).Find(&wm).Error; err != nil {
		return []userTg{}
	}

	workspaceAdminMap := make(map[string]userTg)
	for _, member := range wm {
		if member.Member.TelegramId != nil && member.Member.CanReceiveNotifications() && !member.Member.Settings.TgNotificationMute {
			workspaceAdminMap[member.MemberId] = userTg{
				id:  *member.Member.TelegramId,
				loc: member.Member.UserTimezone,
			}
		}
	}

	return filterWorkspaceTgIdIsNotify(wm, *act.ActorId, workspaceAdminMap, act.Field, act.Verb)
}

func filterWorkspaceTgIdIsNotify(wm []dao.WorkspaceMember, authorId string, userTgId map[string]userTg, field *string, verb string) []userTg {
	res := make([]userTg, 0)
	for _, member := range wm {
		if member.MemberId == authorId {
			if member.NotificationAuthorSettingsTG.IsNotify(field, "workspace", verb, member.Role) {
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
