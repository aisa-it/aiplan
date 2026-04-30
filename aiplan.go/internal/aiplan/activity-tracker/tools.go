package tracker

import (
	"database/sql"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
)

func normalizeDate(data map[string]interface{}, key string) {
	if v, exists := data[key]; exists {
		data[key] = normalizeDateValue(v)
	}
}

func normalizeDateValue(v interface{}) interface{} {
	switch t := v.(type) {
	case types.TimeValuer:
		return t.GetTime().Format(time.RFC3339)
	case time.Time:
		return t.Format(time.RFC3339)
	case sql.NullTime:
		return t.Time.Format(time.RFC3339)
	case map[string]interface{}:
		if v, ok := t["Time"]; ok {
			return v.(time.Time).Format(time.RFC3339)
		}
		return nil
	case nil:
		return nil
	default:
		return v
	}
}
