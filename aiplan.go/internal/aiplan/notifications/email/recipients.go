package email

import (
	"log/slog"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	memNotify "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications/member-role"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

type Recipient struct {
	Email        string
	MemberNotify *memNotify.MemberNotify
}

func buildRecipient(m *memNotify.MemberNotify) (*Recipient, bool) {
	user := m.GetUser()
	if user == nil {
		return nil, false
	}
	if user.Email == "" {
		return nil, false
	}
	if user.Settings.EmailNotificationMute {
		return nil, false
	}

	return &Recipient{
		Email:        user.Email,
		MemberNotify: m,
	}, true
}

func BuildRecipientsFromActivities[A dao.ActivityI](
	tx *gorm.DB, acts []A, actor func(A) *dao.User, plan *emailPlan,
) []memNotify.MemberNotify {

	users := make(memNotify.UserRegistry)

	// добавляем пользователей в зависимости от потребностей слоя
	for _, step := range plan.Steps {
		if err := step(tx, users); err != nil {
			slog.Error("step", "err", err)
		}
	}

	// добавляем по каждой активности, автора события
	for _, act := range acts {
		if u := actor(act); u != nil {
			users.AddUser(u, memNotify.ActionAuthor)
		}
	}

	if err := plan.settings.Load(tx, plan.settings.EntityID, plan.AuthorRole, users, memNotify.EmailSettings); err != nil {
		slog.Error("Get user emailActivity LoadSettings", "entityId", plan.settings.EntityID, "err", err)
		return []memNotify.MemberNotify{}
	}

	return utils.MapToSlice(users, func(k uuid.UUID, t *memNotify.MemberNotify) memNotify.MemberNotify {
		return *t
	})
}
