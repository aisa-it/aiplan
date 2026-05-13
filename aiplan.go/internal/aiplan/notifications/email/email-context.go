package email

import (
	memNotify "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications/member-role"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/tdewolff/minify/v2"
)

var minifier *minify.M = minify.New()

type emailPlan struct {
	EntityType types.EntityLayer
	AuthorRole memNotify.Role
}

type EmailContext struct {
	Settings memNotify.IsNotifyFunc
	Steps    []memNotify.UsersStep
	Plan     *emailPlan
}
