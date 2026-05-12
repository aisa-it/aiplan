package cache

import (
	"crypto/md5"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dto"
	"github.com/gofrs/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const spaceMembersTTL = time.Hour

var (
	WorkspaceMembersCache workspaceMembersCache = workspaceMembersCache{m: make(map[uuid.UUID]workspaceMembersEntry)}

	workspaceMembersLoadDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Subsystem: "cache",
		Name:      "workspace_members_load_duration_seconds",
		Help:      "Duration of WorkspaceMembersCache.Load including hash recompute and user cache lookups, in seconds.",
		Buckets:   []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1},
	})
)

type workspaceMembersCache struct {
	rw sync.RWMutex
	m  map[uuid.UUID]workspaceMembersEntry
}

type workspaceMembersEntry struct {
	Hash   []byte
	expire time.Time

	Members []dto.WorkspaceMemberLight
}

func (c *workspaceMembersCache) Load(workspaceId uuid.UUID) (*workspaceMembersEntry, bool) {
	c.rw.RLock()
	defer c.rw.RUnlock()
	s := time.Now()
	entry, ok := c.m[workspaceId]
	if !ok || time.Now().After(entry.expire) {
		return nil, false
	}

	members := slices.Clone(entry.Members)

	h := md5.New()
	for i, member := range members {
		fmt.Fprintf(h, "%s:%d", member.MemberId.String(), member.Role)
		u, ok := UsersCache.Load(member.MemberId)
		if ok {
			members[i].Member = u
			fmt.Fprintf(h, "%x", u.Hash())
		}
	}
	entry.Members = members
	entry.Hash = h.Sum(nil)

	workspaceMembersLoadDuration.Observe(time.Since(s).Seconds())

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
	entry.Members = new
	c.m[workspaceId] = entry
}
