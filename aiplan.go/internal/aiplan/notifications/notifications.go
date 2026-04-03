package notifications

import (
	"fmt"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/business"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/config"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications/tg"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

type Notification struct {
	Ws *WebsocketNotificationService
	Tg *tg.TgService
	Db *gorm.DB
	//IssueActivityHandler(activity *dao.IssueActivity)
}

func NewNotificationService(cfg *config.Config, db *gorm.DB, bl *business.Business) *Notification {
	//NewTelegramService(db, cfg, tracker, bl),
	return &Notification{
		Ws: NewWebsocketNotificationService(),
		Tg: tg.New(db, cfg, bl),
		Db: db,
	}
}

func getWorkspaceMembers(tx *gorm.DB, workspaceId uuid.UUID) (members []dao.WorkspaceMember, err error) {
	var wm []dao.WorkspaceMember
	if err := tx.Joins("Member").
		Where("workspace_id = ?", workspaceId).
		Find(&wm).Error; err != nil {
		return nil, fmt.Errorf("get workspace members err: %v", err)
	}
	return wm, nil
}
