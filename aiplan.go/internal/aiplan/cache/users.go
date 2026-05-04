package cache

import (
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
