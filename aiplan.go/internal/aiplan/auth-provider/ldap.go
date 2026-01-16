package authprovider

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
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

func (lp *LdapProvider) Sync(users []dao.User) error {
	l, err := ldap.DialURL(lp.serverAdr.String())
	if err != nil {
		slog.Error("Dial LDAP", "err", err)
		return err
	}
	defer l.Close()

	err = l.StartTLS(&tls.Config{InsecureSkipVerify: true})
	if err != nil {
		slog.Debug("Start LDAP TLS", "err", err)
	}

	if err := l.Bind(lp.adminUsr, lp.adminPwd); err != nil {
		slog.Error("LDAP bind admin user", "err", err)
		return err
	}

	userMap := make(map[string]int, len(users))
	var filterStr strings.Builder
	for i, user := range users {
		filterStr.WriteString(fmt.Sprintf("(uniqueIdentifier=%s)", ldap.EscapeFilter(user.Email)))
		userMap[user.Email] = i
	}

	searchRequest := ldap.NewSearchRequest(
		lp.baseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		fmt.Sprintf("(|%s)", filterStr.String()),
		[]string{"mail", "aiplan", "aiplanadmin"},
		nil,
	)

	sr, err := l.SearchWithPaging(searchRequest, 10)
	if err != nil {
		slog.Error("LDAP search", "filter", searchRequest.Filter, "err", err)
		return err
	}

	if len(sr.Entries) == 0 {
		return err
	}

	for _, entry := range sr.Entries {
		attributes := make(map[string][]string, len(entry.Attributes))
		for _, attr := range entry.Attributes {
			attributes[attr.Name] = attr.Values
		}

		idx := userMap[attributes["mail"][0]]
		users[idx].IsActive = strings.ToLower(attributes["aiplan"][0]) == "true"
		users[idx].IsSuperuser = strings.ToLower(attributes["aiplanadmin"][0]) == "true"
	}
	return nil
}
