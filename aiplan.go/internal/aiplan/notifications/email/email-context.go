package email

import (
	memNotify "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications/member-role"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/tdewolff/minify/v2"
)

var minifier *minify.M = minify.New()

type emailPlan struct {
	Entity     actField.ActivityField
	AuthorRole memNotify.Role
}

type EmailContext struct {
	Settings memNotify.MemberSettings
	Steps    []memNotify.UsersStep
	Plan     *emailPlan
}
