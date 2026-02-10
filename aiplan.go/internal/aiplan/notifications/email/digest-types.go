package email

import (
	"database/sql"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
)

type DigestView struct {
	Title  string
	IsNew  bool
	IsGone bool
}

type FieldPrerender struct {
	Verb  string
	Value string
	Count int

	Author dao.User

	Start sql.NullTime
	End   sql.NullTime

	Replace map[string]any
}
