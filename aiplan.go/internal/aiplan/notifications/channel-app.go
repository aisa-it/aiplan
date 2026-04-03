package notifications

import (
	"fmt"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	member_role "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications/member-role"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type appChannel struct {
	db        *gorm.DB
	wsService *WebsocketNotificationService
}

func newWsChannel(db *gorm.DB, service *WebsocketNotificationService) *appChannel {
	return &appChannel{
		db:        db,
		wsService: service,
	}
}

func (w *appChannel) ChannelType() types.NotifyChannel {
	return types.AppCh
}

func (w *appChannel) NewDelivery(event *dao.ActivityEvent, userIds []uuid.UUID) Delivery {
	if msg := formatWsActivity(event); msg != nil {
		counts, err := GetUnreadNotificationCounts(w.db, userIds, batchSize)
		if err != nil {
			return nil
		}
		return &AppDelivery{
			db:              w.db,
			wsService:       w.wsService,
			event:           event,
			msg:             *msg,
			userCountNotify: counts,
		}
	}
	return nil
}

type AppDelivery struct {
	db        *gorm.DB
	wsService *WebsocketNotificationService

	event *dao.ActivityEvent
	msg   WebsocketMsg

	notifications   []dao.UserAppNotify
	userCountNotify map[uuid.UUID]int
}

func (d *AppDelivery) CanSend(r member_role.MemberNotify) bool {
	if r.GetUser().Settings.AppNotificationMute {
		return false
	}

	return true
}

func (d *AppDelivery) Send(r *member_role.MemberNotify) error {
	id := dao.GenUUID()

	d.notifications = append(d.notifications, dao.UserAppNotify{
		ID:              id,
		UserId:          r.GetUser().ID,
		User:            r.GetUser(),
		Type:            "activity",
		WorkspaceId:     d.event.WorkspaceID,
		Workspace:       d.event.Workspace,
		IssueId:         d.event.IssueID,
		Issue:           d.event.Issue,
		AuthorId:        uuid.NullUUID{UUID: d.event.ActorID, Valid: true},
		Author:          d.event.Actor,
		ActivityEventId: uuid.NullUUID{UUID: d.event.ID, Valid: true},
		ActivityEvent:   d.event,
	})
	var count int
	if v, ok := d.userCountNotify[r.GetUser().ID]; ok {
		count = v
	}

	d.msg.Id = id
	d.msg.CountNotify = count
	d.msg.CreatedAt = time.Now().UTC()

	err := d.wsService.SendMsg(r.GetUser().GetId(), d.msg)
	if err != nil {
		return err
	}
	return nil
}

func (d *AppDelivery) Commit(tx *gorm.DB) error {
	if len(d.notifications) == 0 {
		return nil
	}
	if err := tx.Omit(clause.Associations).CreateInBatches(d.notifications, batchSize).Error; err != nil {
		return fmt.Errorf("failed to insert notifications: %w", err)
	}

	return nil
}

// ----

func formatWsActivity(event *dao.ActivityEvent) *WebsocketMsg {
	var msg WebsocketMsg
	if event.EntityType == types.LayerIssue && event.Verb == actField.VerbDeleted && event.Field != actField.Linked.Field {
		return nil
	}
	msg.Type = "activity"
	msg.Detail = NotificationDetailResponse{
		User:      event.Actor.ToLightDTO(),
		Issue:     event.Issue.ToLightDTO(),
		Project:   event.Project.ToLightDTO(),
		Workspace: event.Workspace.ToLightDTO(),
		Doc:       event.Doc.ToLightDTO(),
		Form:      event.Form.ToLightDTO(),
		Sprint:    event.Sprint.ToLightDTO(),
	}
	msg.Data = event.ToLightDTO()
	return &msg
}

func GetUnreadNotificationCounts(db *gorm.DB, userIDs []uuid.UUID, batchSize int) (map[uuid.UUID]int, error) {
	resultMap := make(map[uuid.UUID]int, len(userIDs))
	for _, userID := range userIDs {
		resultMap[userID] = 0
	}

	for i := 0; i < len(userIDs); i += batchSize {
		end := i + batchSize
		if end > len(userIDs) {
			end = len(userIDs)
		}

		batch := userIDs[i:end]
		var batchResults []struct {
			UserID uuid.UUID
			Count  int
		}

		err := db.Model(&dao.UserAppNotify{}).
			Select("user_id, COUNT(*) as count").
			Where("user_id IN ? AND viewed = ?", batch, false).
			Group("user_id").
			Scan(&batchResults).Error

		if err != nil {
			return nil, err
		}

		for _, result := range batchResults {
			resultMap[result.UserID] = result.Count
		}
	}

	return resultMap, nil
}
