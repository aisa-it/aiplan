package utils

import (
	"database/sql"
	"time"
)

func SqlNullTimeToPointerTime(val sql.NullTime) *time.Time {
	if val.Valid {
		return &val.Time
	} else {
		return nil
	}
}
