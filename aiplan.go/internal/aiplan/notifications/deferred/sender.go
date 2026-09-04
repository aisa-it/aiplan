package deferred

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications"
	"github.com/gofrs/uuid"
	"gorm.io/gorm/clause"
)

func (np *NotificationProcessor) dispatchDelivery(s *dao.NotificationSchedule, recipients []ResolvedRecipient) {
	var userNotifies []dao.UserAppNotify
	var appUserIDs []string

	delivery := loadDeliveryMap(s.Payload)
	dctx := &dispatchCtx{}

	for _, r := range recipients {
		entry := delivery.ensureUser(r.User.ID.String())

		for _, ch := range r.Channels {
			if ch == dao.ChannelApp {
				np.collectAppNotify(s, r.User, dctx, &userNotifies, &appUserIDs)
				continue
			}
			entry[ch] = np.attemptChannelSend(r.User, s, dctx, ch, entry[ch])
		}
	}

	if len(userNotifies) > 0 {
		if err := np.saveUserAppNotifies(userNotifies); err != nil {
			slog.Error("v2 save user app notifies", "err", err)
		} else {
			for _, uid := range appUserIDs {
				if entry, ok := delivery[uid]; ok {
					entry[dao.ChannelApp] = dao.DeliverySuccess
				}
			}
		}
	}

	payloadMap := make(map[string]interface{})
	if len(s.Payload) > 0 {
		if err := json.Unmarshal(s.Payload, &payloadMap); err != nil {
			slog.Error("v2 dispatch: failed to unmarshal payload", "id", s.ID, "err", err)
		}
	}
	if payloadMap == nil {
		payloadMap = make(map[string]interface{})
	}
	payloadMap["delivery"] = delivery
	s.Payload, _ = json.Marshal(payloadMap)
}

func (np *NotificationProcessor) attemptChannelSend(user *dao.User, s *dao.NotificationSchedule, dctx *dispatchCtx, channel string, current dao.DeliveryStatus) dao.DeliveryStatus {
	var err error
	switch channel {
	case dao.ChannelTG:
		err = np.sendTgDeferred(user, s, dctx)
	case dao.ChannelEmail:
		err = np.sendEmailDeferred(user, s, dctx)
	default:
		return current
	}
	return nextDeliveryStatus(current, err)
}

func (np *NotificationProcessor) collectAppNotify(s *dao.NotificationSchedule, user *dao.User, dctx *dispatchCtx, userNotifies *[]dao.UserAppNotify, appUserIDs *[]string) {
	title, msg := np.formatAppContent(user, s, dctx)
	un := dao.UserAppNotify{
		ID:          dao.GenUUID(),
		UserId:      user.ID,
		Type:        typeForDeferredApp(s.NotificationType),
		WorkspaceId: s.WorkspaceID,
		IssueId:     s.IssueID,
		Title:       title,
		Msg:         msg,
		Viewed:      false,
	}
	if s.AuthorID.Valid {
		un.AuthorId = s.AuthorID
	}
	*userNotifies = append(*userNotifies, un)
	*appUserIDs = append(*appUserIDs, user.ID.String())
}

// ---------- Telegram ----------

func (np *NotificationProcessor) sendTgDeferred(user *dao.User, s *dao.NotificationSchedule, dctx *dispatchCtx) error {
	if np.tgService.Disabled {
		return errors.New("tg service disabled")
	}
	if user.TelegramId == nil {
		return errors.New("no telegram id")
	}

	var tgID int64
	var format string
	var args []interface{}

	switch s.NotificationType {
	case dao.NotificationTypeDeadline:
		tgID, format, args = np.formatDeadlineTg(user, s, dctx)
	case dao.NotificationTypeWorkspaceMessage:
		tgID, format, args = np.formatWorkspaceMsgTg(user, s, dctx)
	case dao.NotificationTypeServiceMessage:
		_, format, args = np.formatServiceMsgTg(s)
		tgID = *user.TelegramId
	default:
		return fmt.Errorf("unknown notification type: %s", s.NotificationType)
	}

	if format == "" || tgID == 0 {
		return errors.New("empty format or tgid")
	}

	if !np.tgService.SendMessage(tgID, format, args) {
		return errors.New("tg send failed")
	}
	return nil
}

// ---------- App ----------

func (np *NotificationProcessor) saveUserAppNotifies(notifies []dao.UserAppNotify) error {
	if err := np.db.Omit(clause.Associations).CreateInBatches(notifies, 50).Error; err != nil {
		return fmt.Errorf("create user app notifies: %w", err)
	}

	userIDs := make([]uuid.UUID, 0, len(notifies))
	for _, un := range notifies {
		userIDs = append(userIDs, un.UserId)
	}
	counts, _ := notifications.GetUnreadNotificationCounts(np.db, userIDs, 50)

	for _, un := range notifies {
		count := 0
		if c, ok := counts[un.UserId]; ok {
			count = c
		}

		msg := notifications.WebsocketMsg{
			NotificationResponse: notifications.NotificationResponse{
				Id:        un.ID,
				Type:      un.Type,
				Data:      notifications.NotificationResponseMessage{Title: un.Title, Msg: un.Msg},
				CreatedAt: time.Now().UTC(),
			},
			CountNotify: count,
		}
		np.wsService.SendMsg(un.UserId, msg)
	}
	return nil
}
