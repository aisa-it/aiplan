package notifications

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

type TgNotifyDoc struct {
	TelegramService
}

func NewTgNotifyDoc(ts *TelegramService) *TgNotifyDoc {
	if ts == nil {
		return nil
	}
	return &TgNotifyDoc{TelegramService: *ts}
}

func (tnd *TgNotifyDoc) Handle(activity dao.ActivityI) error {
	if a, ok := activity.(dao.DocActivity); ok {
		tnd.LogActivity(a)
	}
	return nil
}

func (tnd *TgNotifyDoc) LogActivity(activity dao.DocActivity) {
	if tnd.disabled {
		return
	}

	go func() {
		msg := tgbotapi.NewMessage(0, "")
		msg.ParseMode = "MarkdownV2"
		var act *tgMsg

		if err := tnd.db.Unscoped().
			Joins("Workspace").
			Joins("Author").
			Preload("Editors").
			Preload("Readers").
			Preload("Watchers").
			Where("docs.id = ?", activity.DocId).
			First(&activity.Doc).Error; err != nil {
			slog.Error("Get doc for activity", "activityId", activity.Id, "err", err)
			return
		}

		activity.Doc.AfterFind(tnd.db)

		act = NewTgActivity("doc")
		act.SetTitleTemplate(&activity)
		if activity.Field == nil {
			return
		}
		switch activity.Verb {
		case "updated":
			switch *activity.Field {
			case "description":
				msg.Text = act.Title("изменил(-а) описание документа")
				msg.Text += Stelegramf("```\n%s```",
					HtmlToTg(activity.NewValue),
				)
			case "title":
				var oldV string
				if activity.OldValue != nil {
					oldV = *activity.OldValue
				}
				msg.Text = act.Title("изменил(-a) в документе")
				msg.Text += Stelegramf("*%s*: ~%s~ %s", fieldsTranslation[*activity.Field], oldV, activity.NewValue)
			case "comment":
				msg.Text = act.Title("изменил(-a) комментарий в документе")
				msg.Text += Stelegramf("```\n%s```",
					HtmlToTg(activity.NewDocComment.CommentHtml.Body),
				)
			case "reader_role", "editor_role":
				msg.Text = act.Title("изменил(-a) роли в документе")
				if *activity.Field == "reader_role" {
					msg.Text += Stelegramf("*%s*: ", "Просмотр раздела")
				}
				if *activity.Field == "editor_role" {
					msg.Text += Stelegramf("*%s*: ", "Редактирование")
				}
				msg.Text += Stelegramf("~%s~ %s", memberRoleStr(fmt.Sprint(*activity.OldValue)), memberRoleStr(activity.NewValue))

			default:
				return
			}
		case "created":
			switch *activity.Field {
			case "attachment":
				msg.Text = act.Title("добавил(-a) вложение в документ")
				msg.Text += Stelegramf("*%s*", activity.NewValue)
				//if activity.NewDocAttachment.Asset != nil {
				//	msg.Text += Stelegramf("\\- %d", activity.NewDocAttachment.Asset.FileSize)
				//}
			case "doc":
				msg.Text = act.Title("создал(-a) в документе")
				msg.Text += Stelegramf("*Вложенный документ*: [%s](%s)", activity.NewValue, activity.NewDoc.URL.String())
			case "comment":
				msg.Text = act.Title("прокомментировал(-a) документ")
				msg.Text += Stelegramf("```\n%s```",
					HtmlToTg(activity.NewDocComment.CommentHtml.Body),
				)
			default:
				return
			}
		case "added":
			switch *activity.Field {
			case "doc":
				msg.Text = act.Title("добавил(-a) в документ")
				msg.Text += Stelegramf("*Вложенный документ*: [%s](%s)", activity.NewValue, activity.NewDoc.URL.String())
			case "readers":
				msg.Text = act.Title("добавил(-a) пользователя в документ")
				msg.Text += Stelegramf("Права *Просмотр*:  %s\n", getUserName(activity.NewDocReader))
			case "editors":
				msg.Text = act.Title("добавил(-a) пользователя в документ")
				msg.Text += Stelegramf("Права *Редактирование*:  %s\n", getUserName(activity.NewDocEditor))
			case "watchers":
				msg.Text = act.Title("добавил(-a) пользователя в документ")
				msg.Text += Stelegramf("*Наблюдатель*:  %s\n", getUserName(activity.NewDocWatcher))
			default:
				return
			}
		case "deleted":
			switch *activity.Field {
			case "attachment":
				msg.Text = act.Title("удалил(-a) вложение из документа")
				if activity.OldValue != nil {
					msg.Text += Stelegramf("~%s~", *activity.OldValue)
				}
			case "comment":
				msg.Text = act.Title("удалил(-a) комментарий из документа")
				if activity.OldValue != nil {
					msg.Text += Stelegramf("```\n%s```",
						HtmlToTg(*activity.OldValue),
					)
				}
			case "doc":
				msg.Text = act.Title("удалил(-a) из документа")
				msg.Text += Stelegramf("*Вложенный документ*:  ~%s~\n", fmt.Sprint(*activity.OldValue))
			default:
				return
			}
		case "move_workspace_to_doc", "move_doc_to_doc", "move_doc_to_workspace":
			if *activity.Field != "doc" {
				return
			}
			if activity.Verb == "move_doc_to_workspace" {
				msg.Text = act.Title("сделал(-a) корневым документ")
				if activity.OldValue != nil {
					msg.Text += Stelegramf("*Из документа*: [%s](%s)", *activity.OldValue, activity.OldDoc.URL.String())
				}
			} else {
				msg.Text = act.Title("переместил(-a) документ")
				if activity.Verb == "move_doc_to_doc" {
					msg.Text += Stelegramf("*Из документа*: [%s](%s)\n", *activity.OldValue, activity.OldDoc.URL.String())
				}
				if activity.Verb == "move_workspace_to_doc" {
					msg.Text += Stelegramf("*Из корневой директории*\n")
				}
				msg.Text += Stelegramf("*В документ*: [%s](%s)", activity.NewValue, activity.NewDoc.URL.String())

			}
		case "removed":
			switch *activity.Field {
			case "doc":
				msg.Text = act.Title("убрал(-a) из документа")
				msg.Text += Stelegramf("*Вложенный документ*: [%s](%s)", *activity.OldValue, activity.OldDoc.URL.String())
			case "readers":
				msg.Text = act.Title("убрал(-a) пользователя из документа")
				msg.Text += Stelegramf("Права *Просмотр*:  ~%s~\n", getUserName(activity.OldDocReader))
			case "editors":
				msg.Text = act.Title("убрал(-a) пользователя из документа")
				msg.Text += Stelegramf("Права *Редактирование*:  ~%s~\n", getUserName(activity.OldDocEditor))
			case "watchers":
				msg.Text = act.Title("убрал(-a) пользователя из документа")
				msg.Text += Stelegramf("*Наблюдатель*:  ~%s~\n", getUserName(activity.OldDocWatcher))
			default:
				return
			}
		default:
			return
		}

		msg.Text = strings.ReplaceAll(msg.Text, "$$$$DOC$$$$", activity.Doc.URL.String())

		if act != nil {
			var msgIds []int64

			usersTelegram := act.GetIdsToSend(tnd.db, &activity)
			for _, ut := range usersTelegram {
				if activity.ActivitySender.SenderTg == ut.id {
					continue
				}
				msg.ChatID = ut.id
				msg.DisableWebPagePreview = true
				r, err := tnd.bot.Send(msg)
				if err != nil && err.Error() != "Bad Request: chat not found" {
					slog.Error("Telegram send error", "docActivities", err, "activityId", activity.Id)
				}
				msgIds = append(msgIds, int64(r.MessageID))
			}

			if err := tnd.db.Model(&activity).Select("telegram_msg_ids").Update("telegram_msg_ids", pq.Int64Array(msgIds)).Error; err != nil {
				slog.Error("Update activity tg msg ids", "err", err)
			}
		}
	}()

}

func getUserTgIdDocActivity(tx *gorm.DB, activity interface{}) []userTg {
	var act *dao.DocActivity
	if v, ok := activity.(*dao.DocActivity); ok {
		act = v
	} else {
		return []userTg{}
	}

	doc := act.Doc

	authorId := doc.CreatedById
	readerIds := doc.ReaderIDs
	editorsIds := doc.EditorsIDs
	watcherIds := doc.WatcherIDs

	userIds := append(append(append([]string{authorId}, editorsIds...), readerIds...), watcherIds...)

	var workspaceMembers []dao.WorkspaceMember
	if err := tx.Joins("Member").
		Where("workspace_id = ?", act.WorkspaceId).
		Where("workspace_members.member_id IN (?)", userIds).Find(&workspaceMembers).Error; err != nil {
		return []userTg{}
	}

	memberMap := make(map[string]userTg)
	for _, member := range workspaceMembers {
		if member.Member.TelegramId != nil && member.Member.CanReceiveNotifications() && !member.Member.Settings.TgNotificationMute {
			memberMap[member.MemberId] = userTg{
				id:  *member.Member.TelegramId,
				loc: member.Member.UserTimezone,
			}
		}
	}

	return filterDocTgIdIsNotify(workspaceMembers, *act.ActorId, memberMap, act.Field, act.Verb)
}

func filterDocTgIdIsNotify(wm []dao.WorkspaceMember, authorId string, userTgId map[string]userTg, field *string, verb string) []userTg {
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
