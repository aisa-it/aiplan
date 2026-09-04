package deferred

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications/member-role"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

type Resolver interface {
	Preload(db *gorm.DB, s *dao.NotificationSchedule) error
	Steps(s *dao.NotificationSchedule) []member_role.UsersStep
	LoadSettings(db *gorm.DB, s *dao.NotificationSchedule, users member_role.UserRegistry) error
	NotifyRoles() []member_role.Role
	SettingsFunc() member_role.IsNotifyFunc
	ActivityEvent(s *dao.NotificationSchedule) *dao.ActivityEvent
	CheckTime(user *dao.User, s *dao.NotificationSchedule) bool
}

var errIssueCompletedOrCancelled = errors.New("issue is completed or cancelled")

var resolverFactories = map[string]func() Resolver{
	dao.NotificationTypeDeadline:         func() Resolver { return &deadlineResolver{} },
	dao.NotificationTypeWorkspaceMessage: func() Resolver { return &workspaceMessageResolver{} },
	dao.NotificationTypeServiceMessage:   func() Resolver { return &serviceMessageResolver{} },
}

// resolvePipeline — общий пайплайн для всех типов уведомлений.
func (np *NotificationProcessor) resolvePipeline(r Resolver, s *dao.NotificationSchedule) ([]ResolvedRecipient, error) {
	if err := r.Preload(np.db, s); err != nil {
		return nil, err
	}

	users := make(member_role.UserRegistry)
	for _, step := range r.Steps(s) {
		if err := step(np.db, users); err != nil {
			return nil, err
		}
	}

	if err := r.LoadSettings(np.db, s, users); err != nil {
		return nil, err
	}

	notifyRoles := r.NotifyRoles()
	settingsFunc := r.SettingsFunc()
	event := r.ActivityEvent(s)

	var result []ResolvedRecipient
	for _, m := range users {
		if notifyRoles != nil && !slices.ContainsFunc(notifyRoles, m.Has) {
			continue
		}

		user := m.GetUser()
		if !user.CanReceiveNotifications() {
			continue
		}

		channels := collectChannels(m, user, settingsFunc, event)
		if len(channels) == 0 {
			continue
		}

		if !r.CheckTime(user, s) {
			continue
		}

		result = append(result, ResolvedRecipient{User: user, Channels: channels})
	}
	return result, nil
}

// resolveRecipients — resolve получателей с учётом текущего delivery.
func (np *NotificationProcessor) resolveRecipients(s *dao.NotificationSchedule) ([]ResolvedRecipient, error) {
	delivery := loadDeliveryMap(s.Payload)

	factory, ok := resolverFactories[s.NotificationType]
	if !ok {
		return nil, fmt.Errorf("unknown notification type: %s", s.NotificationType)
	}

	recipients, err := np.resolvePipeline(factory(), s)
	if err != nil {
		return nil, err
	}

	// Фильтруем: отдаём только тех, у кого есть недоставленные каналы
	filtered := make([]ResolvedRecipient, 0, len(recipients))
	for _, r := range recipients {
		userDelivery, exists := delivery[r.User.ID.String()]
		if !exists {
			userDelivery = make(map[string]dao.DeliveryStatus)
		}

		pendingChannels := pendingChannelsFor(r.Channels, userDelivery)

		if len(pendingChannels) > 0 {
			filtered = append(filtered, ResolvedRecipient{
				User:     r.User,
				Channels: pendingChannels,
			})
		}
	}

	return filtered, nil
}

// ---------- deadline ----------

type deadlineResolver struct {
	issue dao.Issue
}

func (r *deadlineResolver) Preload(db *gorm.DB, s *dao.NotificationSchedule) error {
	if !s.IssueID.Valid || !s.ProjectID.Valid {
		return nil
	}
	if err := db.
		Joins("Author").
		Preload("Assignees").
		Preload("Watchers").
		Joins("State").
		Where(`"issues"."id" = ?`, s.IssueID.UUID).
		First(&r.issue).Error; err != nil {
		return err
	}
	// Завершённые и отменённые задачи — расписание-сирота, отменяем (не retry)
	if r.issue.CompletedAt != nil || (r.issue.State != nil && r.issue.State.Group == "cancelled") {
		return errIssueCompletedOrCancelled
	}
	return nil
}

func (r *deadlineResolver) Steps(s *dao.NotificationSchedule) []member_role.UsersStep {
	return []member_role.UsersStep{
		member_role.AddIssueUsers(&r.issue),
		member_role.AddDefaultWatchers(s.ProjectID.UUID),
	}
}

func (r *deadlineResolver) LoadSettings(db *gorm.DB, _ *dao.NotificationSchedule, users member_role.UserRegistry) error {
	return member_role.LoadProjectSettings(db, r.issue.ProjectId, users)
}

func (r *deadlineResolver) NotifyRoles() []member_role.Role {
	return []member_role.Role{
		member_role.IssueAuthor,
		member_role.ProjectDefaultWatcher,
		member_role.ProjectDefaultAssigner,
		member_role.IssueWatcher,
		member_role.IssueAssigner,
	}
}

func (r *deadlineResolver) SettingsFunc() member_role.IsNotifyFunc {
	return member_role.FromProject()
}

func (r *deadlineResolver) ActivityEvent(_ *dao.NotificationSchedule) *dao.ActivityEvent {
	return &dao.ActivityEvent{
		EntityType: types.LayerIssue,
		Field:      actField.Deadline.Field,
	}
}

func (r *deadlineResolver) CheckTime(user *dao.User, s *dao.NotificationSchedule) bool {
	advance := time.Duration(user.Settings.DeadlineNotification)
	return time.Now().After(s.ScheduledAt.Add(-advance))
}

// ---------- workspace_message ----------

type workspaceMessageResolver struct{}

func (workspaceMessageResolver) Preload(_ *gorm.DB, _ *dao.NotificationSchedule) error { return nil }

func (workspaceMessageResolver) Steps(s *dao.NotificationSchedule) []member_role.UsersStep {
	return []member_role.UsersStep{
		func(tx *gorm.DB, users member_role.UserRegistry) error {
			if !s.WorkspaceID.Valid {
				return nil
			}

			var memberIDs []string
			if len(s.Payload) > 0 {
				var p workspaceMessagePayload
				if err := json.Unmarshal(s.Payload, &p); err == nil {
					memberIDs = p.MemberIDs
				}
			}

			query := fmt.Sprintf(`
				SELECT DISTINCT ON (u.id)
					%s
				FROM workspace_members wm
				JOIN users u ON u.id = wm.member_id
				WHERE wm.workspace_id = ?
				  AND u.is_active = true
			`, recipientSelectColumns("u."))
			args := []interface{}{s.WorkspaceID.UUID}
			if len(memberIDs) > 0 {
				query += ` AND wm.id IN ?`
				args = append(args, memberIDs)
			}

			return addRecipientUsers(tx, query, args, users, "resolve workspace message")
		},
	}
}

func (workspaceMessageResolver) LoadSettings(db *gorm.DB, s *dao.NotificationSchedule, users member_role.UserRegistry) error {
	if !s.WorkspaceID.Valid {
		return nil
	}
	return member_role.LoadWorkspaceSettings(db, s.WorkspaceID.UUID, users)
}

func (workspaceMessageResolver) NotifyRoles() []member_role.Role { return nil }

func (workspaceMessageResolver) SettingsFunc() member_role.IsNotifyFunc { return nil }

func (workspaceMessageResolver) ActivityEvent(_ *dao.NotificationSchedule) *dao.ActivityEvent {
	return &dao.ActivityEvent{
		EntityType: types.LayerWorkspace,
	}
}

func (workspaceMessageResolver) CheckTime(_ *dao.User, s *dao.NotificationSchedule) bool {
	return !s.ScheduledAt.After(time.Now())
}

// ---------- service_message ----------

type serviceMessageResolver struct{}

func (serviceMessageResolver) Preload(_ *gorm.DB, _ *dao.NotificationSchedule) error { return nil }

func (serviceMessageResolver) Steps(s *dao.NotificationSchedule) []member_role.UsersStep {
	return []member_role.UsersStep{
		func(tx *gorm.DB, users member_role.UserRegistry) error {
			var userIDs []string
			if len(s.Payload) > 0 {
				var p serviceMessagePayload
				if err := json.Unmarshal(s.Payload, &p); err == nil {
					userIDs = p.UserIDs
				}
			}

			query := fmt.Sprintf(`
				SELECT
					%s
				FROM users
				WHERE is_active = true
			`, recipientSelectColumns(""))
			args := []interface{}{}
			if len(userIDs) > 0 {
				query += ` AND id IN ?`
				args = append(args, userIDs)
			}

			return addRecipientUsers(tx, query, args, users, "resolve service message")
		},
	}
}

func (serviceMessageResolver) LoadSettings(_ *gorm.DB, _ *dao.NotificationSchedule, _ member_role.UserRegistry) error {
	return nil
}

func (serviceMessageResolver) NotifyRoles() []member_role.Role        { return nil }
func (serviceMessageResolver) SettingsFunc() member_role.IsNotifyFunc { return nil }

func (serviceMessageResolver) ActivityEvent(_ *dao.NotificationSchedule) *dao.ActivityEvent {
	return nil
}

func (serviceMessageResolver) CheckTime(_ *dao.User, s *dao.NotificationSchedule) bool {
	return !s.ScheduledAt.After(time.Now())
}

// ---------- helpers ----------

// Ключи в users.settings — должны совпадать с полями types.UserSettings.
// Вынесены в константы: используются в SQL-запросах Steps, и рассинхрон
// со структурой настроек происходит молча (ложный mute/unmute).
const (
	settingTgMute    = "telegram_notification_mute"
	settingEmailMute = "email_notification_mute"
	settingAppMute   = "app_notification_mute"
)

// recipientSelectColumns — колонки SELECT для genericRecipientRow.
// p — префикс таблицы ("" или "u."): workspace-запрос алиасит users как u.
// Алиасы колонок соответствуют snake_case полям genericRecipientRow.
func recipientSelectColumns(p string) string {
	col := func(name string) string { return p + name }

	cols := []string{
		col("id"),
		col("telegram_id"),
		col("email"),
		col("user_timezone"),
	}
	for _, ch := range []struct {
		key   string // ключ в users.settings
		alias string // алиас колонки (поле genericRecipientRow)
	}{
		{settingTgMute, "tg_notification_mute"},
		{settingEmailMute, "email_notification_mute"},
		{settingAppMute, "app_notification_mute"},
	} {
		cols = append(cols, fmt.Sprintf("COALESCE((%ssettings->>'%s')::bool, false) AS %s", p, ch.key, ch.alias))
	}
	cols = append(cols, col("is_active"), col("is_bot"), col("is_integration"))

	return strings.Join(cols, ",\n\t")
}

// genericRecipientRow — строка результата SQL-запроса пользователей с настройками.
// GORM-теги не нужны: имена полей выбраны так, что их snake_case совпадает с
// алиасами колонок (см. recipientSelectColumns). При переименовании поля менять алиас.
type genericRecipientRow struct {
	ID                    uuid.UUID
	TelegramID            *int64
	Email                 string
	UserTimezone          string
	TgNotificationMute    bool
	EmailNotificationMute bool
	AppNotificationMute   bool
	IsActive              bool
	IsBot                 bool
	IsIntegration         bool
}

// addRecipientUsers выполняет SQL-запрос пользователей и наполняет UserRegistry
// ролью WorkspaceMemberRole. errCtx добавляется к ошибке запроса для диагностики.
func addRecipientUsers(tx *gorm.DB, query string, args []interface{}, users member_role.UserRegistry, errCtx string) error {
	var rows []genericRecipientRow
	if err := tx.Raw(query, args...).Scan(&rows).Error; err != nil {
		return fmt.Errorf("%s: %w", errCtx, err)
	}

	userList := make([]dao.User, len(rows))
	for i, r := range rows {
		userList[i] = *recipientRowToUser(r)
	}
	return member_role.AddUsers(userList, member_role.WorkspaceMemberRole)(tx, users)
}

// recipientRowToUser конвертирует genericRecipientRow в *dao.User для member_role.UserRegistry.
func recipientRowToUser(r genericRecipientRow) *dao.User {
	user := &dao.User{
		ID:            r.ID,
		TelegramId:    r.TelegramID,
		Email:         r.Email,
		IsActive:      r.IsActive,
		IsBot:         r.IsBot,
		IsIntegration: r.IsIntegration,
	}
	user.Settings.TgNotificationMute = r.TgNotificationMute
	user.Settings.EmailNotificationMute = r.EmailNotificationMute
	user.Settings.AppNotificationMute = r.AppNotificationMute
	if r.UserTimezone == "" {
		user.UserTimezone = types.TimeZone(*time.UTC)
	} else if loc, err := time.LoadLocation(r.UserTimezone); err != nil {
		slog.Warn("invalid user timezone", "user_id", r.ID, "timezone", r.UserTimezone, "err", err)
		user.UserTimezone = types.TimeZone(*time.UTC)
	} else {
		user.UserTimezone = types.TimeZone(*loc)
	}
	return user
}

// collectChannels собирает каналы, разрешённые для пользователя.
// Приоритет: project/workspace настройки → глобальный mute.
func collectChannels(
	m *member_role.MemberNotify,
	user *dao.User,
	settingsFunc member_role.IsNotifyFunc,
	event *dao.ActivityEvent,
) []string {
	channels := make([]string, 0, 3)

	for _, ch := range []struct {
		nCh     types.NotifyChannel
		channel string
		muted   bool
	}{
		{types.TgCh, dao.ChannelTG, user.Settings.TgNotificationMute},
		{types.EmailCh, dao.ChannelEmail, user.Settings.EmailNotificationMute},
		{types.AppCh, dao.ChannelApp, user.Settings.AppNotificationMute},
	} {
		if settingsFunc != nil && event != nil {
			if !settingsFunc(*m, event, m.Has(member_role.IssueAuthor), ch.nCh) {
				continue
			}
		}
		if ch.muted {
			continue
		}
		if ch.channel == dao.ChannelTG && user.TelegramId == nil {
			continue
		}
		if ch.channel == dao.ChannelEmail && user.Email == "" {
			continue
		}

		channels = append(channels, ch.channel)
	}
	return channels
}
