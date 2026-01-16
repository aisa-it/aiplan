package authprovider

import (
	"crypto/tls"
	"log/slog"
	"net/url"
	"strings"

	"github.com/go-ldap/ldap/v3"
)

type LdapProvider struct {
	serverAdr *url.URL
	adminUsr  string
	adminPwd  string

	baseDN string
	filter string
}

func InitLDAP(
	serverAdr *url.URL,
	adminUsr string,
	adminPwd string,
	baseDN string,
	filter string) (*LdapProvider, error) {
	lp := &LdapProvider{
		serverAdr: serverAdr,
		adminUsr:  adminUsr,
		adminPwd:  adminPwd,
		baseDN:    baseDN,
		filter:    filter,
	}
	return lp, lp.check()
}

func (lp *LdapProvider) check() error {
	l, err := ldap.DialURL(lp.serverAdr.String())
	if err != nil {
		return err
	}
	defer l.Close()

	if err := l.StartTLS(&tls.Config{InsecureSkipVerify: true}); err != nil {
		slog.Debug("Start LDAP TLS", "err", err)
	}

	return l.Bind(lp.adminUsr, lp.adminPwd)
}

func (lp *LdapProvider) AuthUser(email string, password string) bool {
	l, err := ldap.DialURL(lp.serverAdr.String())
	if err != nil {
		slog.Error("Dial LDAP", "err", err)
		return false
	}
	defer l.Close()

	err = l.StartTLS(&tls.Config{InsecureSkipVerify: true})
	if err != nil {
		slog.Debug("Start LDAP TLS", "err", err)
	}

	if err := l.Bind(lp.adminUsr, lp.adminPwd); err != nil {
		slog.Error("LDAP bind admin user", "err", err)
		return false
	}

	filterStr := strings.ReplaceAll(lp.filter, "{email}", ldap.EscapeFilter(email))

	searchRequest := ldap.NewSearchRequest(
		lp.baseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		filterStr,
		[]string{"dn"},
		nil,
	)

	sr, err := l.Search(searchRequest)
	if err != nil {
		slog.Error("LDAP search", "filter", searchRequest.Filter, "err", err)
		return false
	}

	if len(sr.Entries) == 0 {
		return false
	}

	if err := l.Bind(sr.Entries[0].DN, password); err != nil {
		if ldapErr, ok := err.(*ldap.Error); ok && ldapErr.ResultCode == 49 {
			return false
		}
		slog.Error("LDAP bind", "err", err)
	}

	return true
}
