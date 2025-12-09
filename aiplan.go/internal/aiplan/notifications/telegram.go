// Пакет notifications предоставляет функциональность для отправки уведомлений пользователям Telegram.  Он включает в себя обработку различных событий (создание задач, обновление статусов, добавление комментариев и т.д.) и отправку соответствующих сообщений пользователям, а также поддержку различных типов уведомлений и форматирования сообщений для Telegram.  Пакет также предоставляет возможность настройки уведомлений для отдельных пользователей и рабочих пространств.
package notifications

import (
	"database/sql"
	"fmt"
	"html"
	"log"
	"log/slog"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode"

	tracker "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/activity-tracker"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/business"
	policy "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/redactor-policy"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/gofrs/uuid"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/config"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"gorm.io/gorm"
)

var fieldsTranslation map[string]string = map[string]string{
	actField.Name.Field.String():          "Название",
	actField.Parent.Field.String():        "Родитель",
	actField.Priority.Field.String():      "Приоритет",
	actField.Status.Field.String():        "Статус",
	actField.Description.Field.String():   "Описание",
	actField.TargetDate.Field.String():    "Срок исполнения",
	actField.StartDate.Field.String():     "Дата начала",
	actField.CompletedAt.Field.String():   "Дата завершения",
	actField.Label.Field.String():         "Теги",
	actField.Assignees.Field.String():     "Исполнители",
	actField.Blocking.Field.String():      "Блокирует",
	actField.Blocks.Field.String():        "Заблокирована",
	actField.EstimatePoint.Field.String(): "Оценки",
	actField.SubIssue.Field.String():      "Подзадачи",
	actField.Identifier.Field.String():    "Идентификатор",
	actField.Emoj.Field.String():          "Emoji",
	actField.Title.Field.String():         "Название",
}

var priorityTranslation map[string]string = map[string]string{
	"urgent": "Критический",
	"high":   "Высокий",
	"medium": "Средний",
	"low":    "Низкий",
	"<nil>":  "Не выбран",
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

const (
	helpMSG = `Привет, я бот АИплан, вот список моих основных команд:
/i - создание задачи
/domain - получение текущего адреса для уведомлений
/domain set %domain - установка нового адреса для уведомлений
/cancel - отмена операции
`
	IMG_TEXT = "image:  (alt: image)"
)

type TelegramService struct {
	db             *gorm.DB
	bot            *tgbotapi.BotAPI
	cfg            *config.Config
	sessionHandler *SessionHandler
	tracker        *tracker.ActivitiesTracker
	disabled       bool
	bl             *business.Business
}

func NewTelegramService(db *gorm.DB, cfg *config.Config, tracker *tracker.ActivitiesTracker, bl *business.Business) *TelegramService {
	if cfg.TelegramBotToken == "" {
		slog.Info("Telegram notifications disabled")
		return &TelegramService{disabled: true}
	}

	tgbotapi.SetLogger(slog.NewLogLogger(slog.Default().Handler(), slog.LevelError))

	bot, err := tgbotapi.NewBotAPI(cfg.TelegramBotToken)
	if err != nil {
		slog.Error("Connect to TG bot", "err", err)
		os.Exit(1)
	}

	slog.Info("TG bot connected", "username", bot.Self.UserName)

	ts := &TelegramService{db: db, bot: bot, cfg: cfg, sessionHandler: NewSessionHandler(db, bot), tracker: tracker, disabled: false, bl: bl}
	//tracker.AddActivityLogHandler(ts.LogActivity)
	//tracker.RegisterHandler(NewTgNotifyIssue(*ts))

	if !cfg.TelegramCommandsDisable {
		u := tgbotapi.NewUpdate(0)
		u.Timeout = 60

		updates := bot.GetUpdatesChan(u)

		go func() {
			for update := range updates {
				if update.Message != nil && update.Message.Chat.Type == "private" {
					var user dao.User
					if err := db.Where("telegram_id = ?", update.Message.Chat.ID).First(&user).Error; err != nil {
						//slog.Info("Not found tg msg", "chat_id", update.Message.Chat.ID, "username", update.Message.Chat.UserName)
						// not connected users
						if err == gorm.ErrRecordNotFound {
							msg := tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Ваш telegram id *%d*. Внесите его в настройках профиля для получения уведомлений.", update.Message.Chat.ID))
							msg.ParseMode = "markdown"
							bot.Send(msg)
							continue
						}
						//slog.Error("Get user by telegram id", "id", update.Message.Chat.ID, "err", err)
						continue
					}

					if ts.sessionHandler.IsBusy(user) {
						if update.Message.Command() == "cancel" {
							ts.sessionHandler.Abort(user)
							bot.Send(tgbotapi.NewMessage(*user.TelegramId, "Операция прервана"))
							continue
						}

						if err := ts.sessionHandler.Answer(user, update); err != nil {
							bot.Send(tgbotapi.NewMessage(*user.TelegramId, fmt.Sprintf("Произошла ошибка: %s\nВы можете попробовать еще раз или отменить операцию /cancel", err.Error())))
							continue
						}

						step, end := ts.sessionHandler.Next(user)
						if end {
							ts.sessionHandler.Abort(user)
							continue
						}
						step.Question.ParseMode = "markdown"
						bot.Send(step.Question)
						continue
					}

					if update.Message.ReplyToMessage != nil {
						reply := update.Message.ReplyToMessage

						//var activity dao.IssueActivity

						var issue dao.IssueActivity
						issue.UnionCustomFields = "'issue' AS entity_type"
						var doc dao.DocActivity
						doc.UnionCustomFields = "'doc' AS entity_type"
						unionTable := dao.BuildUnionSubquery(db, "ua", dao.FullActivity{}, issue, doc)

						var act dao.FullActivity

						if err := unionTable.Unscoped().
							Joins("Issue").
							Joins("Doc").
							Where("? = any (telegram_msg_ids)", reply.MessageID).Find(&act).Error; err != nil {
							slog.Error("Find activity by telegram msg id", "err", err)
							continue
						}

						//	if err := db.Preload("Issue").Unscoped().
						//  Where("? = any (telegram_msg_ids)", reply.MessageID).Find(&activity).Error; err != nil {
						//  slog.Error("Find activity by telegram msg id", "err", err)
						//  continue
						//}

						// TODO проверить / пофиксить
						//if activity.DeletedAt.Valid || activity.Issue == nil {
						//	bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "Задача была удалена"))
						//	continue
						//}

						if act.Issue != nil {
							var identifier uuid.UUID
							if act.Field != nil && *act.Field == actField.Comment.Field.String() && act.NewIdentifier != nil {
								identifier = uuid.FromStringOrNil(*act.NewIdentifier)
							}
							err := bl.CreateIssueComment(*act.Issue, user, update.Message.Text, identifier, true)
							if err != nil {
								if err.Error() == "create comment forbidden" {
									bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "У вас нет прав оставлять комментарии в данном проекте"))
									continue
								}
								slog.Error("Create comment from tg reply", "err", err)
								continue
							}
							bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Комментарий к задаче '%s'\nотправлен", act.Issue.Name)))
						}

						if act.Doc != nil {
							var identifier uuid.NullUUID
							if act.Field != nil && *act.Field == actField.Comment.Field.String() && act.NewIdentifier != nil {
								if v, err := uuid.FromString(*act.NewIdentifier); err == nil {
									identifier = uuid.NullUUID{UUID: v, Valid: true}
								}
							}

							err := bl.CreateDocComment(*act.Doc, user, update.Message.Text, identifier, true)
							if err != nil {
								if err.Error() == "create comment forbidden" {
									bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "У вас нет прав оставлять комментарии в данном пространстве"))
									continue
								}
								slog.Error("Create comment from tg reply", "err", err)
								continue
							}
							bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Комментарий в документ '%s'\nотправлен", act.Doc.Title)))
						}

						continue
					}

					switch update.Message.Command() {
					case "start":
						bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "Вы подключены к уведомлениям."))
						continue
					/*case "users":
					  	if strings.Contains(update.Message.CommandArguments(), "super") {
					  		if !user.IsSuperuser {
					  			bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "У вас нет прав суперпользователя для данной команды."))
					  			continue
					  		}

					  		email := strings.TrimSpace(strings.ReplaceAll(update.Message.CommandArguments(), "super", ""))

					  		var exist bool
					  		db.Select("count(*) > 0").Where("email = ?", email).Model(&dao.User{}).Find(&exist)
					  		if !exist {
					  			bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "Пользователь несуществует"))
					  			continue
					  		}

					  		if err := db.Model(&dao.User{}).Where("email = ?", email).Update("is_superuser", true).Error; err != nil {
					  			slog.Error("Promote user to superuser", "err", err)
					  			bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "Произошла ошибка при назначении прав"))
					  			continue
					  		}
					  		bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "Пользователю успешно назначены права суперпользователя"))
					  		continue
					  	}
					  	var users []dao.User
					  	query := db.Order("first_name")
					  	if !user.IsSuperuser {
					  		query.Where("id in (?)",
					  			db.Model(&dao.WorkspaceMember{}).Select("member_id").Where("workspace_id in (?)",
					  				db.Model(&dao.WorkspaceMember{}).Select("workspace_id").Where("member_id = ?", user.ID)))
					  	}
					  	if err := query.Find(&users).Error; err != nil {
					  		slog.Error("Get all users", "err", err)
					  		continue
					  	}
					  	msg := tgbotapi.NewMessage(update.Message.Chat.ID, getUsersList(users))
					  	msg.ParseMode = "MarkdownV2"
					  	bot.Send(msg)
					  	continue
					  case "spaces":
					  	var workspaces []dao.Workspace
					  	var err error
					  	if !user.IsSuperuser {
					  		err = db.Where("id in (?)", db.Model(&dao.WorkspaceMember{}).Select("workspace_id").Where("member_id = ?", user.ID)).Find(&workspaces).Error
					  	} else {
					  		err = db.Find(&workspaces).Error
					  	}
					  	if err != nil {
					  		slog.Error("Get all workspaces", "err", err)
					  		continue
					  	}
					  	msg := tgbotapi.NewMessage(update.Message.Chat.ID, getWorkspacesList(workspaces))
					  	msg.ParseMode = "MarkdownV2"
					  	bot.Send(msg)
					  	continue
					*/
					case "i":
						step, _ := ts.sessionHandler.StartIssueCreationFlow(user, ts.createIssue)
						bot.Send(step.Question)
						continue
					case "domain":
						args := strings.Split(update.Message.CommandArguments(), " ")
						if len(args) > 0 && args[0] == "set" {
							u, err := url.ParseRequestURI(args[1])
							if err != nil || u.Host == "" {
								u, err = url.ParseRequestURI("https://" + args[1])
								if err != nil {
									fmt.Println(err)
									msg := tgbotapi.NewMessage(update.Message.Chat.ID, Stelegramf("Неверный формат URL *%s*", args[1]))
									msg.ParseMode = "MarkdownV2"
									bot.Send(msg)
									continue
								}
							}

							domain := types.NullDomain{URL: u, Valid: true}
							if err := db.Model(&user).Update("domain", domain).Error; err != nil {
								slog.Error("Update user domain", "user", user, "domain", domain, "err", err)

								bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "Произошла ошибка, попробуйте позже"))
							} else {
								msg := tgbotapi.NewMessage(update.Message.Chat.ID, Stelegramf("Новый домен для ваших уведомлений: *%s*", domain.String()))
								msg.ParseMode = "MarkdownV2"
								bot.Send(msg)
							}
						} else {
							msg := tgbotapi.NewMessage(update.Message.Chat.ID, Stelegramf("Текущий домен для ваших уведомлений: *%s*", user.Domain.String()))
							msg.ParseMode = "MarkdownV2"
							bot.Send(msg)
						}
						continue
					case "help":
						msg := tgbotapi.NewMessage(update.Message.Chat.ID, helpMSG)
						bot.Send(msg)
						continue
					}
				} else if update.CallbackQuery != nil {
					callback := tgbotapi.NewCallback(update.CallbackQuery.ID, update.CallbackQuery.Data)
					callback.ShowAlert = false
					callback.Text = ""

					var user dao.User
					if err := db.Where("telegram_id = ?", update.CallbackQuery.Message.Chat.ID).First(&user).Error; err != nil {
						slog.Info("Not found callback user", "chat_id", update.CallbackQuery.Message.Chat.ID)
						// not connected users
						if err == gorm.ErrRecordNotFound {
							msg := tgbotapi.NewMessage(update.CallbackQuery.Message.Chat.ID, fmt.Sprintf("Ваш telegram id *%d*. Внесите его в настройках профиля для получения уведомлений.", update.CallbackQuery.Message.Chat.ID))
							msg.ParseMode = "markdown"
							bot.Send(msg)
							bot.Request(callback)
							continue
						}
						slog.Error("Get user by telegram id", "id", update.CallbackQuery.Message.Chat.ID, "err", err)
						bot.Request(callback)
						continue
					}

					if ts.sessionHandler.IsBusy(user) {
						if err := ts.sessionHandler.Answer(user, update); err != nil {
							bot.Send(tgbotapi.NewMessage(*user.TelegramId, fmt.Sprintf("Произошла ошибка: %s\nВы можете попробовать еще раз или отменить операцию /cancel", err.Error())))
							bot.Request(callback)
							continue
						}

						step, end := ts.sessionHandler.Next(user)
						if end {
							ts.sessionHandler.Abort(user)
							bot.Request(callback)
							continue
						}
						bot.Request(callback)
						step.Question.ParseMode = "markdown"
						bot.Send(step.Question)
						continue
					}
				}
			}
		}()
	} else {
		slog.Warn("Telegram commands disabled")
	}

	return ts
}

type tgMsg struct {
	titleTemplate string
	titleFunc     func(activity interface{}) string
	getIdFunc     func(tx *gorm.DB, activity interface{}) []userTg

	oldValTime sql.NullTime
	newValTime sql.NullTime
}

type userTg struct {
	id  int64
	loc types.TimeZone
}

func (tg *tgMsg) SetTitleTemplate(activity interface{}) {
	tg.titleTemplate = tg.titleFunc(activity)
}

func (tg *tgMsg) Title(title string) string {
	return fmt.Sprintf(tg.titleTemplate, escapeCharacters(title))
}

func (tg *tgMsg) GetIdsToSend(tx *gorm.DB, activity interface{}) []userTg {
	return tg.getIdFunc(tx, activity)
}

func NewTgActivity(entity string) *tgMsg {
	var res tgMsg
	switch entity {
	case "workspace":
		res.titleFunc = func(activity interface{}) string {
			if v, ok := activity.(*dao.WorkspaceActivity); ok {
				return Stelegramf("*%s %s* %s [%s](%s)\n", v.Actor.FirstName, v.Actor.LastName, "%s", v.Workspace.Name, "$$$$WORKSPACE$$$$")
			}
			return ""
		}
		res.getIdFunc = getUserTgIdWorkspaceActivity

	case "issue":
		res.titleFunc = func(activity interface{}) string {
			if v, ok := activity.(*dao.IssueActivity); ok {
				return Stelegramf("*%s %s* %s [%s](%s)\n", v.Actor.FirstName, v.Actor.LastName, "%s", v.Issue.FullIssueName(), "$$$$ISSUE$$$$")
			}
			return ""
		}
		res.getIdFunc = getUserTgIdIssueActivity

	case "project":
		res.titleFunc = func(activity interface{}) string {
			if v, ok := activity.(*dao.ProjectActivity); ok {
				return Stelegramf("*%s %s* %s [%s](%s)\n", v.Actor.FirstName, v.Actor.LastName, "%s", fmt.Sprintf("%s/%s", v.Project.Workspace.Slug, v.Project.Identifier), "$$$$PROJECT$$$$") //todo <---
			}
			return ""
		}
		res.getIdFunc = getUserTgIdProjectActivity

	case "doc":
		res.titleFunc = func(activity interface{}) string {
			if v, ok := activity.(*dao.DocActivity); ok {
				docTitle := v.Doc.Title
				if v.Doc.ParentDocID.Valid {
					docTitle = Stelegramf("...%s", v.Doc.Title)
				}
				return Stelegramf("*%s %s* %s [%s](%s)\n", v.Actor.FirstName, v.Actor.LastName, "%s", fmt.Sprintf("%s/%s", v.Doc.Workspace.Slug, docTitle), "$$$$DOC$$$$") //todo <---
			}
			return ""
		}
		res.getIdFunc = getUserTgIdDocActivity

	}
	return &res
}

func GetUserTgIdFromIssue(issue *dao.Issue) map[string]userTg {

	userTgId := make(map[string]userTg)
	authorId := issue.Author.ID
	if issue.Author.TelegramId != nil && issue.Author.CanReceiveNotifications() && !issue.Author.Settings.TgNotificationMute {
		userTgId[authorId] = userTg{
			id:  *issue.Author.TelegramId,
			loc: issue.Author.UserTimezone,
		}
	}

	if issue.Assignees != nil {
		for _, assignee := range *issue.Assignees {
			if _, ok := userTgId[assignee.ID]; ok {
				continue
			}
			if assignee.TelegramId != nil && assignee.CanReceiveNotifications() && !assignee.Settings.TgNotificationMute {
				userTgId[assignee.ID] = userTg{
					id:  *assignee.TelegramId,
					loc: assignee.UserTimezone,
				}
			}
		}
	}

	if issue.Watchers != nil {
		for _, watcher := range *issue.Watchers {
			if _, ok := userTgId[watcher.ID]; ok {
				continue
			}
			if watcher.TelegramId != nil && watcher.CanReceiveNotifications() && !watcher.Settings.TgNotificationMute {
				userTgId[watcher.ID] = userTg{
					id:  *watcher.TelegramId,
					loc: watcher.UserTimezone,
				}
			}
		}
	}
	return userTgId
}

func GetUserTgIgDefaultWatchers(tx *gorm.DB, projectId string) map[string]userTg {
	userTgId := make(map[string]userTg)
	rows, err := tx.Select("users.id, users.telegram_id").
		Model(dao.ProjectMember{}).
		Joins("JOIN users on users.id = project_members.member_id").
		Where("project_id = ?", projectId).
		Where("is_default_watcher = true").
		Rows()
	if err != nil {
		slog.Error("Fetch default watchers for activity", "err", err)
	} else {
		for rows.Next() {
			var res struct {
				Id           string
				TelegramId   int64
				UserTimezone types.TimeZone
				Settings     types.UserSettings
			}
			if err := tx.ScanRows(rows, &res); err != nil {
				slog.Error("Scan default watchers row", "err", err)
				break
			}
			if res.TelegramId != 0 && !res.Settings.TgNotificationMute {
				userTgId[res.Id] = userTg{
					id:  res.TelegramId,
					loc: res.UserTimezone,
				}
			}
		}
		rows.Close()
	}
	return userTgId
}

func (ts *TelegramService) UserPasswordResetNotify(user dao.User, password string) {
	if user.TelegramId == nil || ts.disabled {
		return
	}

	msg := tgbotapi.NewMessage(*user.TelegramId, fmt.Sprintf("Ваш пароль был сброшен. Новый пароль: `%s`\nЕсли это были не вы - свяжитесь с администраторами.", password))
	msg.ParseMode = "markdown"
	msg.DisableWebPagePreview = true
	ts.bot.Send(msg)
}

func (ts *TelegramService) WorkspaceInvitation(workspaceMember dao.WorkspaceMember) {
	if ts.disabled {
		return
	}

	if workspaceMember.Member.TelegramId != nil {
		msg := tgbotapi.NewMessage(*workspaceMember.Member.TelegramId, Stelegramf("Вас добавили в пространство [%s](%s)",
			workspaceMember.Workspace.Slug,
			workspaceMember.Workspace.URL.String()))
		msg.ParseMode = "MarkdownV2"
		msg.DisableWebPagePreview = true
		ts.bot.Send(msg)
	}

	if workspaceMember.CreatedBy.TelegramId != nil {
		msg := tgbotapi.NewMessage(*workspaceMember.CreatedBy.TelegramId, Stelegramf("Вы добавили пользователя *%s* в пространство [%s](%s)",
			workspaceMember.Member.GetName(),
			workspaceMember.Workspace.Slug,
			workspaceMember.Workspace.URL.String()))
		msg.ParseMode = "MarkdownV2"
		msg.DisableWebPagePreview = true
		ts.bot.Send(msg)
	}
}

func (ts *TelegramService) UserMentionNotification(user dao.User, comment dao.IssueComment) {
	if user.TelegramId == nil || ts.disabled {
		return
	}
	tmpComment := replaceTablesToText(comment.CommentHtml.Body)
	tmpComment = replaceImageToText(tmpComment)
	tmpComment = prepareHtmlBody(htmlStripPolicy, tmpComment)
	msg := tgbotapi.NewMessage(*user.TelegramId, fmt.Sprintf("%s %s упомянул(-а) вас в комментарии [%s](%s):\n```\n%s```",
		comment.Actor.FirstName,
		comment.Actor.LastName,
		comment.Issue.FullIssueName(),
		comment.Issue.URL.String(),
		replaceImgToEmoj(tmpComment),
	))
	msg.ParseMode = "markdown"
	msg.DisableWebPagePreview = true
	ts.bot.Send(msg)
}

func (ts *TelegramService) UserBlockedUntil(user dao.User, until time.Time) {
	if user.TelegramId == nil || ts.disabled {
		return
	}

	msg := tgbotapi.NewMessage(*user.TelegramId, fmt.Sprintf("❗️ Ваша учетная запись была заблокирована из-за подозрительной активности до *%s*", until.Format("02.01.2006 15:04")))
	msg.ParseMode = "markdown"
	msg.DisableWebPagePreview = true
	ts.bot.Send(msg)
}

func (ts *TelegramService) SendMessage(tgId int64, format string, anyStr []string) bool {
	msg := tgbotapi.NewMessage(tgId, "")
	msg.ParseMode = "MarkdownV2"
	msg.ChatID = tgId
	anySlice := make([]any, len(anyStr))
	for i, v := range anyStr {
		anySlice[i] = v
	}
	msg.Text = Stelegramf(format, anySlice...)
	msg.DisableWebPagePreview = true
	_, err := ts.bot.Send(msg)
	if err != nil {
		log.Println("Error sending message to Telegram:", err)
		return false
	}
	return true
}

func escapeCharacters(data string) string {
	data = html.UnescapeString(data)
	res := strings.ReplaceAll(data, "\\", "")
	res = strings.ReplaceAll(res, "_", "\\_")
	res = strings.ReplaceAll(res, "*", "\\*")
	res = strings.ReplaceAll(res, "[", "\\[")
	res = strings.ReplaceAll(res, "]", "\\]")
	res = strings.ReplaceAll(res, "(", "\\(")
	res = strings.ReplaceAll(res, ")", "\\)")
	res = strings.ReplaceAll(res, "~", "\\~")
	res = strings.ReplaceAll(res, "`", "\\`")
	res = strings.ReplaceAll(res, ">", "\\>")
	res = strings.ReplaceAll(res, "#", "\\#")
	res = strings.ReplaceAll(res, "+", "\\+")
	res = strings.ReplaceAll(res, "-", "\\-")
	res = strings.ReplaceAll(res, "=", "\\=")
	res = strings.ReplaceAll(res, "|", "\\|")
	res = strings.ReplaceAll(res, "{", "\\{")
	res = strings.ReplaceAll(res, "}", "\\}")
	res = strings.ReplaceAll(res, ".", "\\.")
	res = strings.ReplaceAll(res, "!", "\\!")
	res = strings.ReplaceAll(res, "&lt;", "\\<")
	res = strings.ReplaceAll(res, "&gt;", "\\>")
	res = strings.ReplaceAll(res, "&amp;", "\\&")
	res = strings.ReplaceAll(res, "&#39;", "'")
	return res
}

func substr(input string, start int, length int) string {
	asRunes := []rune(input)

	if start >= len(asRunes) {
		return ""
	}

	if start+length > len(asRunes) {
		length = len(asRunes) - start
	}

	return string(asRunes[start : start+length])
}

func (ts *TelegramService) GetBotLink() string {
	if ts.disabled {
		return ""
	}
	return "https://t.me/" + ts.bot.Self.UserName
}

func getUsersList(users []dao.User) string {
	res := "*Список пользователей:*\n"
	for _, user := range users {
		res += fmt.Sprintf("*%s %s* \\(%s\\) ", escapeCharacters(user.FirstName), escapeCharacters(user.LastName), escapeCharacters(user.Email))
		if user.IsSuperuser {
			res += "👑"
		}
		if !user.IsActive {
			res += "⛔️"
		}
		res += "\n"
	}
	return res
}

func getWorkspacesList(workspaces []dao.Workspace) string {
	res := "*Список рабочих пространств:*\n"
	for _, workspace := range workspaces {
		res += fmt.Sprintf("*%s* [%s](%s)\n", escapeCharacters(workspace.Name), escapeCharacters(workspace.Slug), escapeCharacters(workspace.URL.String()))
	}
	return res
}

// Stelegramf - SprintF с инкапсуляцией строк для телеграммовского MarkdownV2
func Stelegramf(format string, a ...any) string {
	for i, v := range a {
		switch tv := v.(type) {
		case string:
			a[i] = escapeCharacters(tv)
		}
	}

	return fmt.Sprintf(format, a...)
}

func replaceImgToEmoj(body string) string {
	imgRegex := regexp.MustCompile(`image:\s+\(alt:\s*([^)]*)\)`)
	tableRegex := regexp.MustCompile(`table\s*\(size:\s*(\d+)x(\d+)\)`)

	body = imgRegex.ReplaceAllStringFunc(body, func(imgTag string) string {
		matches := imgRegex.FindStringSubmatch(imgTag)
		altText := "image"
		if len(matches) > 1 {
			altText = matches[1]
		}
		return fmt.Sprintf("%s(%s)", "🖼", altText)
	})

	body = tableRegex.ReplaceAllStringFunc(body, func(tableTag string) string {
		matches := tableRegex.FindStringSubmatch(tableTag)
		if len(matches) == 3 {
			rows := matches[1]
			cols := matches[2]
			return fmt.Sprintf("%s(%sx%s)", "📊", rows, cols)
		}
		return tableTag
	})
	return strings.ReplaceAll(body, "&#34;", "\"")
}

func capitalizeFirst(s string) string {
	if len(s) == 0 {
		return s
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

func HtmlToTg(text string) string {
	res := replaceTablesToText(text)
	res = replaceImageToText(res)
	res = policy.ProcessCustomHtmlTag(res)
	res = prepareHtmlBody(htmlStripPolicy, res)
	return substr(replaceImgToEmoj(res), 0, 4000)
}
