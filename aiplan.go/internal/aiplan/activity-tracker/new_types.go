package tracker

import (
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/gofrs/uuid"
)

type EntityRef struct {
	ID        uuid.UUID
	NameValue string
	NameField string
}

type FieldChange struct {
	Field  actField.ActivityField
	Verb   string
	OldVal string
	NewVal string
	OldID  uuid.NullUUID
	NewID  uuid.NullUUID
}

type ActivityFieldSpec struct {
	Req       string
	Field     string
	Kind      string
	Transform string
	Table     string
}
