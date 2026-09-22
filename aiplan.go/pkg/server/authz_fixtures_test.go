package server

import "github.com/gofrs/uuid"

// Общие идентификаторы для таблиц решений о правах (authz_matrix_test.go,
// authz_scopes_test.go). Сами правила зафиксированы эталонами в testdata.
var (
	permUserID   = uuid.Must(uuid.NewV4())
	permOtherID  = uuid.Must(uuid.NewV4())
	permIssueID  = uuid.Must(uuid.NewV4())
	permProjID   = uuid.Must(uuid.NewV4())
	permWsID     = uuid.Must(uuid.NewV4())
	permMemberID = uuid.Must(uuid.NewV4())
)
