package types

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/config"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestUUID(t *testing.T) {
	s := `["5E838708-6D06-4187-AB8D-E6A9073B33F3", ""]`
	var ss FilterUUIDs
	fmt.Println(json.Unmarshal([]byte(s), &ss))
	fmt.Println(ss.Array, ss.IncludeEmpty)

	d, err := json.Marshal(ss)
	fmt.Println(err)
	fmt.Println(string(d))
}

func TestJSONTime(t *testing.T) {
	tt := IssuesListFilters{
		CreatedAtFrom: JSONTime(time.Now()),
	}

	d, err := json.Marshal(tt)
	require.NoError(t, err)
	fmt.Println(string(d))

	var ttUn IssuesListFilters
	require.NoError(t, json.Unmarshal(d, &ttUn))
	fmt.Println(ttUn.CreatedAtFrom.Time())
}

func TestJSONTimeDB(t *testing.T) {
	cfg := config.ReadConfig()
	db, err := gorm.Open(postgres.New(postgres.Config{DSN: cfg.DatabaseDSN}))
	require.NoError(t, err)

	var tt bool
	fmt.Println(db.Raw("select $1 > NOW()", JSONTime(time.Now())).Scan(&tt).Error)
	fmt.Println(tt)
}
