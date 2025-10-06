package store

import (
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/apierrors"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/gofrs/uuid"
	"sync"
	"time"
)

const (
	codeLifeTime  time.Duration = time.Minute * 5
	emailLimitReq               = time.Minute
)

type EmailChangeStore struct {
	mu      sync.RWMutex
	userReq map[uuid.UUID]changeData
}

type changeData struct {
	timer    *time.Timer
	codeList []emailData
	cleanup  chan struct{}
}

type emailData struct {
	newEmail string
	code     string
	expires  time.Time
}

func NewEmailChangeStore() *EmailChangeStore {
	return &EmailChangeStore{
		userReq: make(map[uuid.UUID]changeData),
	}
}

func (es *EmailChangeStore) NewEmailChange(user *dao.User, newEmail, code string) error {
	es.mu.Lock()
	defer es.mu.Unlock()
	id, err := uuid.FromString(user.ID)
	if err != nil {
		return err
	}
	if v, ok := es.userReq[id]; ok {
		lastCodeTime := v.codeList[len(v.codeList)-1].expires.Add(-codeLifeTime)

		if time.Now().Sub(lastCodeTime) < emailLimitReq {
			return apierrors.ErrEmailChangeLimit
		}

		if v.cleanup != nil {
			close(v.cleanup)
		}

		v.timer.Reset(codeLifeTime)
		v.codeList = append(v.codeList, emailData{newEmail: newEmail, code: code, expires: time.Now().Add(codeLifeTime)})
		v.cleanup = make(chan struct{})

		go es.setupTimerCleanup(id, v.timer, v.cleanup)
		es.userReq[id] = v
	} else {
		cleanupCh := make(chan struct{})
		n := changeData{
			timer: time.NewTimer(codeLifeTime),
			codeList: []emailData{
				{newEmail: newEmail, code: code, expires: time.Now().Add(codeLifeTime)},
			},
			cleanup: cleanupCh,
		}

		go es.setupTimerCleanup(id, n.timer, cleanupCh)
		es.userReq[id] = n
	}
	return err
}

func (es *EmailChangeStore) ValidCodeEmail(user *dao.User, newEmail, code string) bool {
	es.mu.RLock()
	defer es.mu.RUnlock()

	id, err := uuid.FromString(user.ID)
	if err != nil {
		return false
	}
	if v, ok := es.userReq[id]; ok {
		for _, v := range v.codeList {
			if v.newEmail == newEmail && v.code == code {
				return true
			}
		}
	}
	return false
}

func (es *EmailChangeStore) CleanupUser(userID uuid.UUID) {
	es.mu.Lock()
	defer es.mu.Unlock()

	if data, ok := es.userReq[userID]; ok {
		if data.cleanup != nil {
			close(data.cleanup)
		}
		if data.timer != nil {
			data.timer.Stop()
		}
		delete(es.userReq, userID)
	}
}

func (es *EmailChangeStore) setupTimerCleanup(userID uuid.UUID, timer *time.Timer, stopCh <-chan struct{}) {
	select {
	case <-timer.C:
		es.CleanupUser(userID)
	case <-stopCh:
		return
	}
}
