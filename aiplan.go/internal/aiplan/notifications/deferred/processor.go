package deferred

import (
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications/email"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications/tg"
	"gorm.io/gorm"
)

const (
	defaultBatchSize = 50
)

// ResolvedRecipient — пользователь и какие каналы ему нужно отправить.
type ResolvedRecipient struct {
	User     *dao.User
	Channels []string
}

type NotificationProcessor struct {
	db           *gorm.DB
	tgService    *tg.TgService
	emailService *email.EmailService
	wsService    *notifications.WebsocketNotificationService
	mu           sync.Mutex

	batchSize int
}

func NewNotificationProcessor(
	db *gorm.DB,
	tgService *tg.TgService,
	emailService *email.EmailService,
	wsService *notifications.WebsocketNotificationService,
) *NotificationProcessor {
	return &NotificationProcessor{
		db:           db,
		tgService:    tgService,
		emailService: emailService,
		wsService:    wsService,
		batchSize:    defaultBatchSize,
	}
}

func (np *NotificationProcessor) ProcessScheduled() {
	if !np.mu.TryLock() {
		slog.Info("NotificationProcessor: previous cycle still running, skip")
		return
	}
	defer np.mu.Unlock()

	np.processSchedules()
}

// ---------- internal ----------

func (np *NotificationProcessor) processSchedules() {
	var schedules []dao.NotificationSchedule

	// Фильтр по send_window_start (absolute time): deadline-записи «спят» до начала окна
	// упреждения (scheduled_at - MaxDeadlineAdvance), workspace/service выбираются в момент отправки.
	// timestamptz и NOW() сравниваются по UTC, часовой пояс сервера/пользователей не влияет.
	if err := np.db.Raw(`
		SELECT *
		FROM notification_schedules
		WHERE status = 'pending'
		  AND send_window_start <= NOW()
		ORDER BY send_window_start
		LIMIT ?
	`, np.batchSize).Scan(&schedules).Error; err != nil {
		slog.Error("NotificationProcessorV2: poll select", "err", err)
		return
	}
	if len(schedules) == 0 {
		return
	}

	for i := range schedules {
		np.processSchedule(&schedules[i])
	}
}

func (np *NotificationProcessor) processSchedule(s *dao.NotificationSchedule) {
	recipients, err := np.resolveRecipients(s)
	if err != nil {
		if errors.Is(err, errIssueCompletedOrCancelled) || errors.Is(err, gorm.ErrRecordNotFound) {
			np.markCancelled(s)
		} else {
			slog.Error("processSchedule: resolve recipients failed", "id", s.ID, "err", err)
		}
		return
	}

	if len(recipients) == 0 {
		if shouldAwaitAssignees(s) {
			return
		}
		np.markCompleted(s)
		return
	}

	np.dispatchDelivery(s, recipients)
	np.updateDelivery(s)

	if loadDeliveryMap(s.Payload).hasPending() {
		return
	}
	if shouldAwaitAssignees(s) {
		return
	}
	np.markCompleted(s)
}

func shouldAwaitAssignees(s *dao.NotificationSchedule) bool {
	return s.NotificationType == dao.NotificationTypeDeadline && s.ScheduledAt.After(time.Now())
}

func (np *NotificationProcessor) ReactivateDeadlineSchedules() {
	var schedules []dao.NotificationSchedule
	if err := np.db.Raw(`
		SELECT *
		FROM notification_schedules
		WHERE notification_type = ?
		  AND status IN (?, ?)
		  AND scheduled_at > NOW()
		LIMIT ?
	`, dao.NotificationTypeDeadline, dao.StatusCompleted, dao.StatusCancelled, np.batchSize).Scan(&schedules).Error; err != nil {
		slog.Error("ReactivateDeadlineSchedules: select", "err", err)
		return
	}

	for i := range schedules {
		s := &schedules[i]
		if !s.IssueID.Valid {
			continue
		}

		var row struct {
			CompletedAt *time.Time
			StateGroup  string
		}
		if err := np.db.Raw(`
			SELECT i.completed_at, COALESCE(s."group", '') AS state_group
			FROM issues i
			LEFT JOIN states s ON s.id = i.state_id
			WHERE i.id = ?
		`, s.IssueID.UUID).Scan(&row).Error; err != nil {
			continue
		}
		if row.CompletedAt != nil || row.StateGroup == "cancelled" {
			continue
		}

		now := time.Now().UTC()
		if err := np.db.Model(&dao.NotificationSchedule{}).
			Where("id = ?", s.ID).
			Updates(map[string]interface{}{
				"status":       dao.StatusPending,
				"processed_at": nil,
				"updated_at":   now,
			}).Error; err != nil {
			slog.Error("ReactivateDeadlineSchedules: update", "id", s.ID, "err", err)
		}
	}
}
