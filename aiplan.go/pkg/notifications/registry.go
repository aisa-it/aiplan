package notifications

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/dao"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/engine"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/notifications/email"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

// Реестр типов уведомлений, добавляемых движком. Встроенные типы очереди
// (message, deadline_notification, service_message) живут в switch
// CreateNotificationSender; реестр — запасной путь для остальных.

var (
	rendererMu        sync.RWMutex
	rendererFactories = map[string]func() engine.NotificationRenderer{}
)

// RegisterNotificationType регистрирует фабрику представления для типа.
func RegisterNotificationType(name string, factory func() engine.NotificationRenderer) {
	if name == "" || factory == nil {
		return
	}
	rendererMu.Lock()
	defer rendererMu.Unlock()
	rendererFactories[name] = factory
}

// IsNotificationTypeRegistered сообщает, известен ли тип очереди —
// встроенный или зарегистрированный движком.
func IsNotificationTypeRegistered(name string) bool {
	switch name {
	case "message", "deadline_notification", "service_message":
		return true
	}
	rendererMu.RLock()
	defer rendererMu.RUnlock()
	_, ok := rendererFactories[name]
	return ok
}

// registeredSender собирает отправителя зарегистрированного типа и разбирает
// в него payload записи.
func registeredSender(notification *dao.DeferredNotifications) (INotifySend, error) {
	rendererMu.RLock()
	factory, ok := rendererFactories[notification.NotificationType]
	rendererMu.RUnlock()
	if !ok {
		return nil, errors.New("unknown type notify")
	}
	r := factory()
	if len(notification.NotificationPayload) > 0 {
		if err := json.Unmarshal(notification.NotificationPayload, r); err != nil {
			return nil, err
		}
	}
	return &rendererSender{r: r}, nil
}

// rendererSender адаптирует представление движка к отправителю очереди.
type rendererSender struct {
	r   engine.NotificationRenderer
	app *dao.UserAppNotify
}

func (s *rendererSender) getUserNotification() *dao.UserAppNotify { return s.app }

func (s *rendererSender) getAuthor(*gorm.DB) *dao.User { return nil }

func (s *rendererSender) isNotifyApp(_ *gorm.DB, n *dao.DeferredNotifications) bool {
	s.app = s.r.RenderApp(*n)
	if s.app == nil {
		return false
	}
	// Без id запись очереди не найдёт своё уведомление при повторе
	if s.app.ID == uuid.Nil {
		s.app.ID = n.ID
	}
	if s.app.Type == "" {
		s.app.Type = n.NotificationType
	}
	return true
}

func (s *rendererSender) isNotifyTg(*gorm.DB, *dao.DeferredNotifications) bool { return true }

func (s *rendererSender) isNotifyEmail(*gorm.DB, *dao.DeferredNotifications) bool { return true }

func (s *rendererSender) toTelegram(n *dao.DeferredNotifications, _ *dao.User) (int64, string, []any) {
	text, ok := s.r.RenderTelegram(*n)
	if !ok {
		return 0, "", nil
	}
	return *n.User.TelegramId, "%s", []any{text}
}

func (s *rendererSender) toEmail(es *email.EmailService, n *dao.DeferredNotifications, _ *dao.User) bool {
	msg, ok := s.r.RenderEmail(*n)
	if !ok {
		return true
	}
	err := es.MessageNotify(*n, msg.Subject, email.MessageNotifyCtx{
		WebUrl:     msg.ButtonPath,
		TitleMsg:   msg.Title,
		Msg:        msg.Body,
		TextButton: msg.ButtonText,
		TimeSend:   time.Now(),
		Workspace:  n.Workspace,
	})
	return err == nil
}

// EnqueueNotification кладёт уведомление движка в очередь: по записи на канал.
func EnqueueNotification(db *gorm.DB, n engine.Notification) error {
	if n.UserID == uuid.Nil {
		return errors.New("notification recipient is empty")
	}
	if !IsNotificationTypeRegistered(n.Type) {
		return fmt.Errorf("notification type %q is not registered", n.Type)
	}
	payload, err := json.Marshal(n.Payload)
	if err != nil {
		return err
	}
	channels := n.Channels
	if len(channels) == 0 {
		channels = engine.AllChannels
	}
	sendAt := n.SendAt
	if sendAt.IsZero() {
		sendAt = time.Now()
	}
	rows := make([]dao.DeferredNotifications, 0, len(channels))
	for _, ch := range channels {
		rows = append(rows, dao.DeferredNotifications{
			ID:                  dao.GenUUID(),
			UserID:              n.UserID,
			IssueID:             n.IssueID,
			ProjectID:           n.ProjectID,
			WorkspaceID:         n.WorkspaceID,
			NotificationType:    n.Type,
			DeliveryMethod:      string(ch),
			TimeSend:            &sendAt,
			NotificationPayload: payload,
		})
	}
	return db.Create(&rows).Error
}
