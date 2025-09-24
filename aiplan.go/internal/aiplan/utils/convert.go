package utils

import (
	"database/sql"
	"github.com/gofrs/uuid"
	"time"
)

func SqlNullTimeToPointerTime(val sql.NullTime) *time.Time {
	if val.Valid {
		return &val.Time
	} else {
		return nil
	}
}

func UuidFromId(id string) (uuid.UUID, error) {
	if val, err := uuid.FromString(id); err != nil {
		return uuid.UUID{}, err
	} else {
		return val, err
	}
}
