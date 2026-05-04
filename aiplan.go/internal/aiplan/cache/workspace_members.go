package cache

import (
	"crypto/md5"
	"fmt"
	"sync"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dto"
	"github.com/gofrs/uuid"
)

const spaceMembersTTL = time.Hour

var WorkspaceMembersCache workspaceMembersCache = workspaceMembersCache{m: make(map[uuid.UUID]workspaceMembersEntry)}

type workspaceMembersCache struct {
	rw sync.RWMutex
	m  map[uuid.UUID]workspaceMembersEntry
}

type workspaceMembersEntry struct {
	Hash   []byte
	expire time.Time

	Members []dto.WorkspaceMemberLight
}

func (e *workspaceMembersEntry) calcHash() {
	h := md5.New()
	for _, member := range e.Members {
		fmt.Fprintf(h, "%s:%d", member.MemberId.String(), member.Role)
	}
	e.Hash = h.Sum(nil)
}

func (c *workspaceMembersCache) Load(workspaceId uuid.UUID) (*workspaceMembersEntry, bool) {
	c.rw.RLock()
	defer c.rw.RUnlock()
	entry, ok := c.m[workspaceId]
	if !ok || time.Now().After(entry.expire) {
		return nil, false
	}
	for i, e := range entry.Members {
		u, ok := UsersCache.Load(e.MemberId)
		if ok {
			entry.Members[i].Member = u
		}
	}
	return &entry, true
}

func (c *workspaceMembersCache) Store(workspaceId uuid.UUID, members []dto.WorkspaceMemberLight) {
	c.rw.Lock()
	defer c.rw.Unlock()
	entry := workspaceMembersEntry{
		Members: members,
		expire:  time.Now().Add(spaceMembersTTL),
	}
	for i := range entry.Members {
		entry.Members[i].Member = nil
	}
	entry.calcHash()
	c.m[workspaceId] = entry
}

func (c *workspaceMembersCache) Expire(workspaceId uuid.UUID) {
	c.rw.Lock()
	defer c.rw.Unlock()
	delete(c.m, workspaceId)
}

func (c *workspaceMembersCache) Update(workspaceId uuid.UUID, updMember dao.WorkspaceMember) {
	c.rw.Lock()
	defer c.rw.Unlock()
	entry, ok := c.m[workspaceId]
	if !ok {
		return
	}
	for i, member := range entry.Members {
		if updMember.ID == member.ID {
			entry.Members[i] = *updMember.ToLightDTO()
		}
	}
}

func (c *workspaceMembersCache) Delete(workspaceId uuid.UUID, dltMember dao.WorkspaceMember) {
	c.rw.Lock()
	defer c.rw.Unlock()
	entry, ok := c.m[workspaceId]
	if !ok {
		return
	}
	new := make([]dto.WorkspaceMemberLight, 0, len(entry.Members)-1)
	for _, member := range entry.Members {
		if dltMember.ID != member.ID {
			new = append(new, member)
		}
	}
}
