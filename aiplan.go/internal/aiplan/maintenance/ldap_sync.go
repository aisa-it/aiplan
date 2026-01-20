package maintenance

import (
	"log/slog"

	authprovider "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/auth-provider"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"gorm.io/gorm"
)

type LdapSynchronizer struct {
	db           *gorm.DB
	ldapProvider *authprovider.LdapProvider
}

func NewLdapSynchronizer(db *gorm.DB, ldapProvider *authprovider.LdapProvider) *LdapSynchronizer {
	return &LdapSynchronizer{db, ldapProvider}
}

func (ls *LdapSynchronizer) SyncJob() {
	slog.Info("Sync LDAP users params")
	var users []dao.User
	if err := ls.db.Where("auth_provider = ?", "ldap").FindInBatches(&users, 20, func(tx *gorm.DB, batch int) error {
		if err := ls.ldapProvider.Sync(users); err != nil {
			return err
		}

		for _, user := range users {
			if err := tx.Model(&user).Select("is_active", "is_superuser").Updates(&user).Error; err != nil {
				return err
			}
		}

		return nil
	}).Error; err != nil {
		slog.Error("Update users params from LDAP", "err", err)
	}
}
