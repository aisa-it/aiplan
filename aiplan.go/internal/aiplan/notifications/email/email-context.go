package email

import (
	"sync"

	memNotify "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications/member-role"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/html"
)

var emailMinifier *minify.M
var emailMinifierOnce sync.Once

func getEmailMinifier() *minify.M {
	emailMinifierOnce.Do(func() {
		m := minify.New()
		m.Add("text/html", &html.Minifier{
			KeepDocumentTags:    true,
			KeepEndTags:         true,
			KeepSpecialComments: true,
			KeepDefaultAttrVals: true,
			KeepWhitespace:      false,
		})
		emailMinifier = m
	})
	return emailMinifier
}

type emailPlan struct {
	EntityType types.EntityLayer
	AuthorRole memNotify.Role
}

type EmailContext struct {
	Settings memNotify.IsNotifyFunc
	Steps    []memNotify.UsersStep
	Plan     *emailPlan
}
