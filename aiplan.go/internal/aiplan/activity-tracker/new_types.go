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
	Field      actField.ActivityField
	Verb       string
	OldVal     string
	NewVal     string
	OldID      uuid.NullUUID
	NewID      uuid.NullUUID
	PreserveID bool      // нужно ли сохранять ID для этого поля
	EntityID   uuid.UUID // target entity ID (для linked-событий, иначе основной entity)
	IsLinked   bool      // является ли это linked-событием
}

type ActivityFieldSpec struct {
	Req         string
	Field       string
	Kind        string
	Transform   string
	Table       string
	PreserveID  bool   // по умолчанию true - сохранять ID для этого поля
	LinkedField string // для linked-коллекций: обратное поле (blocking -> blocked)
}
