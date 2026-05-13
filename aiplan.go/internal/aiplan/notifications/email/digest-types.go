package email

import (
	"database/sql"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
)

type DigestView struct {
	Title  string
	IsNew  bool
	IsGone bool
}

type FieldPrerender struct {
	Verb  string
	Field actField.ActivityField
	Value string
	Count int

	Authors []dao.User

	Start sql.NullTime
	End   sql.NullTime

	Replace map[string]any
}
