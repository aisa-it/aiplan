// Пакет для отправки уведомлений пользователям различными способами (Telegram, Email, App).
//
// Основные возможности:
//   - Отправка уведомлений в Telegram.
//   - Отправка уведомлений по электронной почте.
//   - Отправка уведомлений в мобильное приложение.
package notifications

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications/tg"
	"gorm.io/gorm/clause"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

// Constant for the maximum number of retry attempts
const maxRetryAttempts = 3

// NotificationProcessor is responsible for processing notifications
type NotificationProcessor struct {
	db               *gorm.DB
	telegramService  *tg.TgService
	emailService     *EmailService
	websocketService *WebsocketNotificationService
}

func CreateNotificationSender(notification *dao.DeferredNotifications) (INotifySend, error) {
	var res INotifySend
	switch notification.NotificationType {
	case "message":
		res = &notifyMessage{}
	case "deadline_notification":
		res = &notifyDeadline{}
	case "service_message":
		res = &serviceMessage{}
	default:
		return nil, fmt.Errorf("unknown type notify")
	}
	if err := json.Unmarshal(notification.NotificationPayload, &res); err != nil {
		return nil, err
	}
	return res, nil
}

func NewNotificationProcessor(db *gorm.DB, telegramService *tg.TgService, emailService *EmailService, websocketService *WebsocketNotificationService) *NotificationProcessor {
	return &NotificationProcessor{
		db:               db,
		telegramService:  telegramService,
		emailService:     emailService,
		websocketService: websocketService,
	}
}

// ProcessNotifications processes records from the notifications_log table
func (np *NotificationProcessor) ProcessNotifications() {
	var notifications []dao.DeferredNotifications

	err := np.db.Preload("User").
		Preload("Issue").
		Preload("Issue.Workspace").
		Preload("Issue.Project").
		Preload("Workspace").
		Where("sent_at IS NULL AND time_send < NOW()  AND attempt_count < ?", maxRetryAttempts).
		Find(&notifications).Error
	if err != nil {
		return
	}

	var notifyDel []uuid.UUID
	for _, notification := range notifications {
		if !notification.User.CanReceiveNotifications() {
			notifyDel = append(notifyDel, notification.ID)
			continue
		}
		switch notification.NotificationType {
		case "message":
			if notification.Workspace == nil {
				notifyDel = append(notifyDel, notification.ID)
				continue
			}
		case "deadline_notification":
			if notification.Issue == nil || notification.Issue.Project == nil {
				notifyDel = append(notifyDel, notification.ID)
				continue
			}
		}

		np.handleNotification(&notification)
	}

	if err := np.db.Where("id IN (?)", notifyDel).Delete(&dao.DeferredNotifications{}).Error; err != nil {
		return
	}
}

// handleNotification processes a single notification
func (np *NotificationProcessor) handleNotification(notification *dao.DeferredNotifications) {
	sender, err := CreateNotificationSender(notification)
	if err != nil {
		return
	}
	var success bool
	if notification.User.IsNotify(notification.DeliveryMethod) {
		switch notification.DeliveryMethod {
		case "telegram":
			success = np.sendToTelegram(notification, sender)
		case "email":
			success = np.sendToEmail(notification, sender)
		case "app":
			success = np.sendToApp(notification, sender)
		default:
			return
		}
	} else {
		success = true
	}

	// Update the record depending on the delivery result
	if success {
		now := time.Now()
		notification.SentAt = &now

	} else {
		notification.AttemptCount++
		notification.LastAttemptAt = time.Now()
	}

	if err := np.db.Model(&dao.DeferredNotifications{}).Where("id = ?", notification.ID).Updates(notification).Error; err != nil {
		log.Println("Error updating notification log:", err)
	}
}

func (np *NotificationProcessor) sendToTelegram(notification *dao.DeferredNotifications, sender INotifySend) bool {
	if np.telegramService.Disabled {
		return false
	}
	if !notification.User.CanReceiveNotifications() && !notification.User.Settings.TgNotificationMute {
		return true
	}

	if notification.User.TelegramId == nil {
		return false
	}

	if sender.isNotifyTg(np.db, notification) {
		author := sender.getAuthor(np.db)
		if !np.telegramService.SendMessage(sender.toTelegram(notification, author)) {
			return false
		}
	}
	return true
}

func (np *NotificationProcessor) sendToEmail(notification *dao.DeferredNotifications, sender INotifySend) bool {
	if !notification.User.CanReceiveNotifications() && !notification.User.Settings.EmailNotificationMute {
		return true
	}

	if sender.isNotifyEmail(np.db, notification) {
		author := sender.getAuthor(np.db)
		if !sender.toEmail(np.emailService, notification, author) {
			return false
		}
	}
	return true
}

func (np *NotificationProcessor) sendToApp(notification *dao.DeferredNotifications, sender INotifySend) bool {
	if !notification.User.CanReceiveNotifications() && !notification.User.Settings.AppNotificationMute {
		return true
	}

	if sender.isNotifyApp(np.db, notification) {
		if un, countNotify, _ := np.createUserNotify(notification, sender); un != nil {
			np.websocketService.Send(notification.UserID, un.ID, *un, countNotify)
		}
	}
	return true
}

func (np *NotificationProcessor) createUserNotify(notification *dao.DeferredNotifications, send INotifySend) (*dao.UserNotifications, int, error) {
	un := send.getUserNotification()

	var exist bool
	if err := np.db.Model(&dao.UserNotifications{}).
		Select("EXISTS(?)",
			np.db.Model(&dao.UserNotifications{}).
				Select("1").
				Where("id = ?", un.ID),
		).
		Find(&exist).Error; err != nil {
		return nil, 0, err
	}

	if !exist {
		un.UserId = notification.UserID
		if notification.WorkspaceID.Valid {
			un.WorkspaceId = notification.WorkspaceID
		}
		un.Workspace = notification.Workspace
		if notification.IssueID.Valid {
			un.IssueId = notification.IssueID
		}
		un.Issue = notification.Issue

		if un.AuthorId.Valid {
			if err := np.db.Where("id = ?", un.AuthorId.UUID).First(&un.Author).Error; err != nil {
				return nil, 0, err
			}
		}

		if err := np.db.Omit(clause.Associations).Create(un).Error; err != nil {
			return nil, 0, err
		}
		var count int
		if err := np.db.Select("count(*)").
			Where("viewed = false").
			Where("user_id = ?", notification.UserID).
			Where("deleted_at IS NULL").
			Model(&dao.UserNotifications{}).
			Find(&count).Error; err != nil {
			return nil, 0, err
		}
		return un, count, nil
	}

	return nil, 0, nil
}

type emailNotify struct {
	Subj       string
	Title      string
	Msg        string
	Author     *dao.User
	AddRout    string
	TextButton string
}

type INotifySend interface {
	getUserNotification() *dao.UserNotifications
	getAuthor(tx *gorm.DB) *dao.User
	isNotifyTg(tx *gorm.DB, notification *dao.DeferredNotifications) bool
	isNotifyEmail(tx *gorm.DB, notification *dao.DeferredNotifications) bool
	isNotifyApp(tx *gorm.DB, notification *dao.DeferredNotifications) bool
	toTelegram(notification *dao.DeferredNotifications, author *dao.User) (tgId int64, format string, any []string)
	toEmail(emailService *EmailService, notification *dao.DeferredNotifications, author *dao.User) bool
}

// Workspace message
type notifyMessage struct {
	Id       uuid.UUID `json:"id"`
	Title    string    `json:"title"`
	Msg      string    `json:"msg"`
	AuthorId string    `json:"author_id"`
}

func (nm *notifyMessage) getAuthor(tx *gorm.DB) *dao.User {
	var user dao.User
	if err := tx.Where("id = ?", nm.AuthorId).First(&user).Error; err != nil {
		return nil
	}
	return &user
}

func (nm *notifyMessage) getUserNotification() *dao.UserNotifications {
	authorUUID, _ := uuid.FromString(nm.AuthorId)
	res := dao.UserNotifications{
		ID:       nm.Id,
		Type:     "message",
		Title:    nm.Title,
		Msg:      nm.Msg,
		AuthorId: uuid.NullUUID{UUID: authorUUID, Valid: nm.AuthorId != ""},
		Viewed:   false,
	}
	return &res
}

func (nm *notifyMessage) toTelegram(notification *dao.DeferredNotifications, author *dao.User) (tgId int64, format string, any []string) {
	var firstName, lastName string
	if author == nil {
		firstName = "Администратор"
		lastName = "пространства"
	} else {
		firstName = author.FirstName
		lastName = author.LastName
	}

	message := replaceTablesToText(nm.Msg)
	message = replaceImageToText(message)
	message = prepareHtmlBody(htmlStripPolicy, message)
	formatMsg := "%s %s отправил сообщение пользователям\n[%s](%s)\n*%s*\n```\n%s```"
	var out []string
	out = append(out,
		firstName,
		lastName,
		notification.Workspace.Name,
		notification.Workspace.URL.String(),
		nm.Title,
		substr(replaceImgToEmoj(message), 0, 4000))
	return *notification.User.TelegramId, formatMsg, out
}

func (nm *notifyMessage) toEmail(emailService *EmailService, notification *dao.DeferredNotifications, author *dao.User) bool {
	msg := emailNotify{
		Subj:       "Сообщение для участников рабочего пространства: " + notification.Workspace.Name,
		Title:      nm.Title,
		Msg:        nm.Msg,
		Author:     author,
		AddRout:    fmt.Sprintf("%s/", notification.Workspace.Slug),
		TextButton: "Перейти в рабочее пространство",
	}

	err := emailService.MessageNotify(*notification, msg)
	if err != nil {
		return false
	}
	return true
}

func (nm *notifyMessage) isNotifyTg(tx *gorm.DB, notification *dao.DeferredNotifications) bool {
	return true
}

func (nm *notifyMessage) isNotifyEmail(tx *gorm.DB, notification *dao.DeferredNotifications) bool {
	return true
}

func (nm *notifyMessage) isNotifyApp(tx *gorm.DB, notification *dao.DeferredNotifications) bool {
	return true
}

// notifyDeadline
type notifyDeadline struct {
	Id       uuid.UUID `json:"id"`
	Body     string    `json:"body"`
	Deadline time.Time `json:"deadline"`
}

func (nd *notifyDeadline) getAuthor(tx *gorm.DB) *dao.User {
	return nil
}

func (nd *notifyDeadline) getUserNotification() *dao.UserNotifications {

	res := dao.UserNotifications{
		ID:     nd.Id,
		Type:   "message",
		Title:  "Уведомление об истечении срока выполнения задачи",
		Msg:    nd.Body,
		Viewed: false,
	}
	return &res
}

func (nd *notifyDeadline) toTelegram(notification *dao.DeferredNotifications, author *dao.User) (tgId int64, format string, any []string) {
	formatMsg := "❗Срок выполнения задачи\n[%s](%s)\nистекает *%s*"
	var out []string

	date, err := FormatDate(nd.Deadline.Format("02.01.2006 15:04 MST"), "02.01.2006 15:04 MST", &notification.User.UserTimezone)
	if err != nil {
		return 0, "", nil
	}
	out = append(out,
		notification.Issue.FullIssueName(),
		notification.Issue.URL.String(),
		date,
	)
	return *notification.User.TelegramId, formatMsg, out
}

func (nd *notifyDeadline) toEmail(emailService *EmailService, notification *dao.DeferredNotifications, author *dao.User) bool {
	err := emailService.DeadlineMessageNotify(*notification.User, *notification, *nd)
	if err != nil {
		return false
	}
	return true
}

func (nd *notifyDeadline) isNotifyTg(tx *gorm.DB, notification *dao.DeferredNotifications) bool {
	var projectMember dao.ProjectMember
	err := tx.Where("project_id = ?", notification.Issue.ProjectId).Where("member_id = ?", notification.User.ID).First(&projectMember).Error

	if err != nil {
		log.Println("Error fetching project member:", err)
		return false
	}
	field := "deadline"
	if notification.Issue.CreatedById == projectMember.MemberId {
		if !projectMember.NotificationAuthorSettingsTG.IsNotify(&field, "issue", "all", projectMember.Role) {
			return false
		}
	} else {
		if !projectMember.NotificationSettingsTG.IsNotify(&field, "issue", "all", projectMember.Role) {
			return false
		}
	}

	return true
}

func (nd *notifyDeadline) isNotifyEmail(tx *gorm.DB, notification *dao.DeferredNotifications) bool {
	var projectMember dao.ProjectMember
	err := tx.Where("project_id = ?", notification.Issue.ProjectId).Where("member_id = ?", notification.User.ID).First(&projectMember).Error

	if err != nil {
		log.Println("Error fetching project member:", err)
		return false
	}

	field := "deadline"

	if notification.Issue.CreatedById == projectMember.MemberId {
		if !projectMember.NotificationAuthorSettingsEmail.IsNotify(&field, "issue", "all", projectMember.Role) {
			return false
		}
	} else {
		if !projectMember.NotificationSettingsEmail.IsNotify(&field, "issue", "all", projectMember.Role) {
			return false
		}
	}
	return true
}

func (nd *notifyDeadline) isNotifyApp(tx *gorm.DB, notification *dao.DeferredNotifications) bool {
	var projectMember dao.ProjectMember
	err := tx.Where("project_id = ?", notification.Issue.ProjectId).Where("member_id = ?", notification.User.ID).First(&projectMember).Error

	if err != nil {
		log.Println("Error fetching project member:", err)
		return false
	}
	field := "deadline"
	if notification.Issue.CreatedById == projectMember.MemberId {
		if !projectMember.NotificationAuthorSettingsApp.IsNotify(&field, "issue", "all", projectMember.Role) {
			return false
		}
	} else {
		if !projectMember.NotificationSettingsApp.IsNotify(&field, "issue", "all", projectMember.Role) {
			return false
		}
	}

	return true
}

// service message
type serviceMessage struct {
	Id    uuid.UUID `json:"id"`
	Title string    `json:"title"`
	Msg   string    `json:"msg"`
}

func (s serviceMessage) getUserNotification() *dao.UserNotifications {
	res := dao.UserNotifications{
		ID:       s.Id,
		Type:     "service_message",
		Title:    s.Title,
		Msg:      s.Msg,
		AuthorId: uuid.NullUUID{},
		Viewed:   false,
	}
	return &res
}

func (s serviceMessage) getAuthor(tx *gorm.DB) *dao.User {
	return nil
}

func (s serviceMessage) isNotifyTg(tx *gorm.DB, notification *dao.DeferredNotifications) bool {
	return true
}

func (s serviceMessage) isNotifyEmail(tx *gorm.DB, notification *dao.DeferredNotifications) bool {
	return true
}

func (s serviceMessage) isNotifyApp(tx *gorm.DB, notification *dao.DeferredNotifications) bool {
	return true
}

func (s serviceMessage) toTelegram(notification *dao.DeferredNotifications, author *dao.User) (tgId int64, format string, any []string) {
	formatMsg := "🔹Сервисное уведомление пользователям\n*%s*\n```\n%s```"
	message := replaceTablesToText(s.Msg)
	message = replaceImageToText(message)
	message = prepareHtmlBody(htmlStripPolicy, message)
	var out []string
	out = append(out,
		s.Title,
		substr(replaceImgToEmoj(message), 0, 4000))
	return *notification.User.TelegramId, formatMsg, out
}

func (s serviceMessage) toEmail(emailService *EmailService, notification *dao.DeferredNotifications, author *dao.User) bool {
	msg := emailNotify{
		Subj:       "Сервисное уведомление пользователям",
		Title:      s.Title,
		Msg:        s.Msg,
		TextButton: "Перейти на главную страницу",
	}

	err := emailService.MessageNotify(*notification, msg)
	if err != nil {
		return false
	}
	return true
}
