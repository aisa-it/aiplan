package cache

import (
	"crypto/md5"
	"fmt"
	"log/slog"
	"slices"
	"strings"
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
		if !ok || u == nil {
			slog.Warn("USERS CACHE miss member", "id", member.MemberId, "u", u)
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

	// Deep copy array without members
	cleanMembers := make([]dto.WorkspaceMemberLight, 0, len(members))
	for _, m := range members {
		cleanMembers = append(cleanMembers, dto.WorkspaceMemberLight{
			ID:              m.ID,
			Role:            m.Role,
			EditableByAdmin: m.EditableByAdmin,
			MemberId:        m.MemberId,
			WorkspaceId:     m.WorkspaceId,
		})
	}

	entry := workspaceMembersEntry{
		Members: cleanMembers,
		expire:  time.Now().Add(spaceMembersTTL),
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

func SortWorkspaceMembers(members []dto.WorkspaceMemberLight, offset, limit int, orderBy string, desc bool) []dto.WorkspaceMemberLight {
	slices.SortFunc(members, func(a dto.WorkspaceMemberLight, b dto.WorkspaceMemberLight) int {
		var res int
		switch orderBy {
		case "email":
			res = strings.Compare(strings.ToLower(a.Member.Email), strings.ToLower(b.Member.Email))
		case "role":
			res = a.Role - b.Role
		default:
			res = strings.Compare(strings.ToLower(a.Member.LastName), strings.ToLower(b.Member.LastName))
		}

		if desc {
			res = res * -1
		}

		return res
	})
	return members[max(offset, 0):min(offset+limit, len(members))]
}
