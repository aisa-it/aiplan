package server

import (
	"context"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/engine"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/notifications"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/notifications/email"
	"gorm.io/gorm"
)

// coreNotifier — отправка уведомлений движка через сервисы ядра.
type coreNotifier struct {
	db *gorm.DB
	es *email.EmailService
}

func (n *coreNotifier) SendEmail(_ context.Context, msg engine.EmailMessage) error {
	return n.es.Send(email.EmailMessage{
		To:          msg.To,
		Subject:     msg.Subject,
		Content:     msg.HTML,
		TextContent: msg.Text,
	})
}

func (n *coreNotifier) Enqueue(ctx context.Context, msg engine.Notification) error {
	return notifications.EnqueueNotification(n.db.WithContext(ctx), msg)
}

var _ engine.Notifier = (*coreNotifier)(nil)
