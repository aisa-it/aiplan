package tracker

import (
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
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
	PreserveID bool              // нужно ли сохранять ID для этого поля
	EntityID   uuid.UUID         // target entity ID (для linked-событий, иначе основной entity)
	IsLinked   bool              // является ли это linked-событием
	Layer      types.EntityLayer // для linked-коллекций: слой на котором нужно создать активность
}

type ActivityFieldSpec struct {
	Field       string
	Kind        string
	PreserveID  bool   // по умолчанию true - сохранять ID для этого поля
	LinkedField string // для linked-коллекций: обратное поле (blocking -> blocked)
	LinkedLayer string // для linked-коллекций: слой на котором нужно создать активность
	Secret      bool   // для скрытия чувствительных данных
	Verb        string // кастомный verb для collection активностей (если отсутствует - дублирует слой от основной активности)
}

type TrackOption func(*trackParams)

type trackParams struct {
	field    actField.ActivityField
	oldVal   string
	newVal   string
	oldID    uuid.NullUUID
	newID    uuid.NullUUID
	tgSender int64
}

func WithField(f actField.ActivityField) TrackOption {
	return func(tp *trackParams) {
		tp.field = f
	}
}

func WithOldVal(v string) TrackOption {
	return func(tp *trackParams) {
		tp.oldVal = v
	}
}

func WithNewVal(v string) TrackOption {
	return func(tp *trackParams) {
		tp.newVal = v
	}
}

func WithOldID(id uuid.UUID) TrackOption {
	return func(tp *trackParams) {
		tp.oldID = uuid.NullUUID{UUID: id, Valid: true}
	}
}

func WithNewID(id uuid.UUID) TrackOption {
	return func(tp *trackParams) {
		tp.newID = uuid.NullUUID{UUID: id, Valid: true}
	}
}

func WithTgSender(id int64) TrackOption {
	return func(tp *trackParams) {
		tp.tgSender = id
	}
}
