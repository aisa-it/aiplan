package cache

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dto"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

const (
	usersNotifyChannel = "users_light_changes"
)

var UsersCache usersCache

type usersCache struct {
	db *gorm.DB
	rw sync.RWMutex
	m  map[uuid.UUID]dto.UserLight
}

func InitUsersCache(db *gorm.DB) {
	UsersCache = usersCache{m: make(map[uuid.UUID]dto.UserLight), db: db}
	dao.NotifiSubscription.Subscribe(usersNotifyChannel, UsersCache.notifyHandler)
}

func (c *usersCache) notifyHandler(payload string) {
	var user dto.UserLight
	if err := json.Unmarshal([]byte(payload), &user); err != nil {
		slog.Error("USERS CACHE unmarshal payload", "payload", payload, "err", err)
		return
	}

	c.rw.Lock()
	defer c.rw.Unlock()
	c.m[user.ID] = user
}

func (c *usersCache) Load(id uuid.UUID) (*dto.UserLight, bool) {
	c.rw.RLock()
	user, ok := c.m[id]
	c.rw.RUnlock()
	if ok {
		return &user, ok
	}
	var u dao.User
	if err := c.db.Where("id = ?", id).First(&u).Error; err != nil {
		slog.Error("USERS CACHE get user", "id", id, "err", err)
		return nil, false
	}
	user = *u.ToLightDTO()
	c.rw.Lock()
	c.m[u.ID] = user
	c.rw.Unlock()
	return &user, true
}

func (c *usersCache) LoadAsDAO(id uuid.UUID) (*dao.User, bool) {
	light, ok := c.Load(id)
	if !ok {
		return nil, false
	}
	return userLightToDAO(light), true
}

func userLightToDAO(u *dto.UserLight) *dao.User {
	if u == nil {
		return nil
	}
	d := &dao.User{
		ID:            u.ID,
		Username:      u.Username,
		Email:         u.Email,
		FirstName:     u.FirstName,
		LastName:      u.LastName,
		Avatar:        u.Avatar,
		AvatarId:      u.AvatarId,
		UserTimezone:  u.UserTimezone,
		LastActive:    u.LastActive,
		TelegramId:    u.TelegramId,
		CreatedAt:     u.CreatedAt,
		IsSuperuser:   u.IsSuperuser,
		IsActive:      u.IsActive,
		IsOnboarded:   u.IsOnboarded,
		IsBot:         u.IsBot,
		IsIntegration: u.IsIntegration,
	}
	if u.StatusEmoji != nil {
		d.StatusEmoji = sql.NullString{String: *u.StatusEmoji, Valid: true}
	}
	if u.Status != nil {
		d.Status = sql.NullString{String: *u.Status, Valid: true}
	}
	if u.StatusEndDate != nil {
		d.StatusEndDate = sql.NullTime{Time: *u.StatusEndDate, Valid: true}
	}
	if u.BlockedUntil != nil {
		d.BlockedUntil = sql.NullTime{Time: *u.BlockedUntil, Valid: true}
	}
	return d
}
