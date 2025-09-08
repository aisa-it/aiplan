// Управление сессиями пользователей с использованием BoltDB.
//
// Основные возможности:
//   - Хранение и отслеживание сессий пользователей.
//   - Блокировка токенов сессий на определенное время (blacklist).
//   - Автоматическая очистка устаревших сессий.
package sessions

import (
	"encoding/binary"
	"log/slog"
	"os"
	"time"

	"github.com/boltdb/bolt"
	"sheff.online/aiplan/internal/aiplan/config"
)

type SessionsManager struct {
	db  *bolt.DB
	ttl time.Duration
}

const (
	sessionsBucketName = "sessions"

	tokenBlacklistFreeze = time.Minute
)

func NewSessionsManager(cfg *config.Config, sessionTTL time.Duration) *SessionsManager {
	if cfg.SessionsDBPath == "" {
		cfg.SessionsDBPath = "sessions.db"
	}

	db, err := bolt.Open(cfg.SessionsDBPath, 0644, nil)
	if err != nil {
		slog.Error("Open sessions db", "err", err)
		os.Exit(1)
	}

	if err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(sessionsBucketName))
		return err
	}); err != nil {
		slog.Error("Create sessions bucket", "err", err)
		os.Exit(1)
	}

	sm := &SessionsManager{db, sessionTTL}

	go sm.cleanLoop()

	return sm
}

func (sm *SessionsManager) BlacklistToken(signature []byte) error {
	return sm.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(sessionsBucketName))

		tm := make([]byte, 8)
		binary.LittleEndian.PutUint64(tm, uint64(time.Now().Add(tokenBlacklistFreeze).Unix()))

		if err := b.Put(signature, tm); err != nil {
			return err
		}

		return nil
	})
}

func (sm *SessionsManager) IsTokenBlacklisted(signature []byte) (bool, error) {
	var blacklisted bool
	err := sm.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(sessionsBucketName))

		timeRaw := b.Get(signature)
		if timeRaw == nil {
			return nil
		}

		blacklisted = time.Now().After(time.Unix(int64(binary.LittleEndian.Uint64(timeRaw)), 0))

		return nil
	})
	return blacklisted, err
}

func (sm *SessionsManager) Close() {
	sm.db.Close()
}

func (sm *SessionsManager) cleanLoop() {
	for {
		keysToRemove := []string{}
		sm.db.View(func(tx *bolt.Tx) error {
			b := tx.Bucket([]byte(sessionsBucketName))

			c := b.Cursor()
			for k, v := c.First(); k != nil; k, v = c.Next() {
				tm := time.Unix(int64(binary.LittleEndian.Uint64(v)), 0)

				if time.Since(tm) > sm.ttl {
					keysToRemove = append(keysToRemove, string(k))
				}
			}
			return nil
		})

		if len(keysToRemove) > 0 {
			sm.db.Update(func(tx *bolt.Tx) error {
				b := tx.Bucket([]byte(sessionsBucketName))

				for _, key := range keysToRemove {
					b.Delete([]byte(key))
				}

				return nil
			})
		}

		time.Sleep(time.Minute)
	}
}
