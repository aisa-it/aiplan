// Package tokenscache реализует кратковременный кеш для JWT токенов,
// решающий проблему параллельных запросов на обновление токена (token refresh race condition).
//
// Когда несколько вкладок браузера или параллельных запросов одновременно
// пытаются обновить токен по одному и тому же refresh token, первый запрос
// создаёт новую пару токенов и сохраняет её в кеш. Последующие запросы
// с тем же старым refresh token в течение 15 секунд получат ту же пару токенов
// из кеша вместо ошибки о недействительном токене.
package tokenscache

import (
	"sync"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
)

type TokenInfo struct {
	AccessToken  string
	RefreshToken string
	User         *dao.User
	CreatedAt    time.Time
}

type TokensCache struct {
	m  map[string]TokenInfo
	mu sync.Mutex
}

func NewTokensCache() *TokensCache {
	return &TokensCache{
		m: make(map[string]TokenInfo),
	}
}

func (c *TokensCache) GetTokens(oldRefreshToken string) *TokenInfo {
	c.mu.Lock()
	defer c.mu.Unlock()
	t, ok := c.m[oldRefreshToken]
	if !ok {
		return nil
	}
	if time.Now().After(t.CreatedAt.Add(time.Second * 15)) {
		delete(c.m, oldRefreshToken)
		return nil
	}
	return &t
}

func (c *TokensCache) StoreTokens(oldRefreshToken, accessToken, refreshToken string, user *dao.User) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[oldRefreshToken] = TokenInfo{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		CreatedAt:    time.Now(),
		User:         user,
	}
}
