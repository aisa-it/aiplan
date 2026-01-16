package authprovider

import (
	"testing"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/config"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/stretchr/testify/require"
)

var cfg *config.Config

func TestMain(t *testing.T) {
	cfg = config.ReadConfig()
}

func TestSync(t *testing.T) {
	lp, err := InitLDAP(cfg.LDAPServerURL, cfg.LDAPBindUser, cfg.LDAPBindPassword, cfg.LDAPBaseDN, "(&(uniqueIdentifier={email}))")
	require.NoError(t, err)
	users := []dao.User{
		{
			Email:       "",
			IsActive:    false,
			IsSuperuser: true,
		},
		{
			Email:       "",
			IsActive:    true,
			IsSuperuser: true,
		},
	}
	require.NoError(t, lp.Sync(users))
}
