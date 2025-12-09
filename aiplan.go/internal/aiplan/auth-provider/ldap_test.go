package authprovider

import (
	"fmt"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAuth(t *testing.T) {
	// The username and password we want to check
	username := ""
	password := ""

	bindusername := ""
	bindpassword := ""

	ldapURL, _ := url.Parse("")
	baseDn := ""

	lp, err := InitLDAP(ldapURL, bindusername, bindpassword, baseDn, "(&(uniqueIdentifier={email}))")
	require.NoError(t, err)

	fmt.Println(lp.AuthUser(username, password))
}
