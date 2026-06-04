package email

import (
	"log/slog"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	memNotify "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications/member-role"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
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

func BuildRecipientsFromActivities(
	tx *gorm.DB, acts []dao.ActivityEvent, ctx *EmailContext,
) ([]memNotify.MemberNotify, EmailContext) {

	users := make(memNotify.UserRegistry)

	// добавляем пользователей в зависимости от потребностей слоя
	for _, step := range ctx.Steps {
		if err := step(tx, users); err != nil {
			slog.Error("step", "err", err)
		}
	}

	// добавляем по каждой активности, автора события
	for _, act := range acts {
		if act.Actor != nil {
			users.AddUser(act.Actor, nil, memNotify.ActionAuthor)
		}
		// если заданны кастомные роли для групп событий
		for _, f := range ctx.CustomRoleFunc {
			for _, customRoleFunc := range f(act) {
				if err := customRoleFunc(tx, users); err != nil {
					slog.Error("customRoleFunc step", "err", err)
				}
			}
		}

	}

	switch ctx.Plan.EntityType {
	case types.LayerIssue, types.LayerProject:
		err := memNotify.LoadProjectSettings(tx, acts[0].ProjectID.UUID, users)
		if err != nil {
			return []memNotify.MemberNotify{}, *ctx
		}
	case types.LayerRoot:
		return []memNotify.MemberNotify{}, *ctx

	default:
		err := memNotify.LoadWorkspaceSettings(tx, acts[0].WorkspaceID.UUID, users)
		if err != nil {
			return []memNotify.MemberNotify{}, *ctx
		}
	}

	return utils.MapToSlice(users, func(k uuid.UUID, t *memNotify.MemberNotify) memNotify.MemberNotify {
		return *t
	}), *ctx
}
