package engine

import (
	"context"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/dao"
	"github.com/gofrs/uuid"
)

// Channel — канал доставки уведомления пользователю.
type Channel string

const (
	ChannelEmail    Channel = "email"
	ChannelTelegram Channel = "telegram"
	ChannelApp      Channel = "app"
)

// AllChannels — все каналы ядра в порядке постановки в очередь.
var AllChannels = []Channel{ChannelEmail, ChannelTelegram, ChannelApp}

// EmailMessage — письмо, отправляемое напрямую, минуя очередь и настройки
// пользователя. Для писем не пользователю системы (внешний адресат) или для
// синхронных подтверждений. HTML обязателен, Text — запасной вариант.
type EmailMessage struct {
	To      string
	Subject string
	HTML    string
	Text    string
}

// Notification — уведомление пользователю через очередь ядра.
//
// Очередь уважает настройки пользователя (мьют по каналу, блокировка),
// делает повторы при сбое доставки и умеет отложенную отправку. Type должен
// быть зарегистрирован через Core.RegisterNotificationType, Payload —
// значение того же типа, что возвращает фабрика: он сериализуется в jsonb
// и разбирается обратно при доставке.
type Notification struct {
	Type   string
	UserID uuid.UUID
	// Привязки — для карточки внутреннего уведомления и ссылок в письме.
	IssueID     uuid.NullUUID
	ProjectID   uuid.NullUUID
	WorkspaceID uuid.NullUUID
	// Channels — куда доставлять. Пусто — во все каналы.
	Channels []Channel
	// SendAt — когда отправить. Нулевое время — при ближайшем проходе очереди.
	SendAt  time.Time
	Payload any
}

// EmailRender — письмо из очереди в общем шаблоне ядра: заголовок, текст и
// кнопка. ButtonPath — путь относительно адреса сайта.
type EmailRender struct {
	Subject    string
	Title      string
	Body       string
	ButtonText string
	ButtonPath string
}

// NotificationRenderer — представление уведомления движка в каналах ядра.
// Фабрика типа возвращает указатель на структуру с json-тегами: в него
// разбирается Payload перед вызовом методов.
//
// Каждый метод отвечает и за сам факт доставки в канал: ok == false или
// nil — канал пропускается без ошибки.
type NotificationRenderer interface {
	RenderApp(n dao.DeferredNotifications) *dao.UserAppNotify
	RenderTelegram(n dao.DeferredNotifications) (text string, ok bool)
	RenderEmail(n dao.DeferredNotifications) (msg EmailRender, ok bool)
}

// Notifier — отправка уведомлений из движка средствами ядра.
type Notifier interface {
	// SendEmail ставит письмо в очередь воркеров почты. Ошибка — если почта
	// выключена конфигурацией.
	SendEmail(ctx context.Context, msg EmailMessage) error
	// Enqueue кладёт уведомление в очередь ядра: по записи на каждый канал.
	// Ошибка — незарегистрированный тип, пустой получатель или сбой записи.
	Enqueue(ctx context.Context, n Notification) error
}
