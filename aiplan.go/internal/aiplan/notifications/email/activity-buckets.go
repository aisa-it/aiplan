package email

import (
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications/member-role"
	"github.com/gofrs/uuid"
)

type ActivityBuckets[A dao.ActivityI, E dao.IDaoAct] map[uuid.UUID]*ActivityBucket[A, E]

type ActivityBucket[A dao.ActivityI, E dao.IDaoAct] struct {
	Entity     E
	Activities []A

	MemberNotify []member_role.MemberNotify
	Prepared     map[string]fieldPrerender

	FirstAt time.Time
	LastAt  time.Time
}
