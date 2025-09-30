package business

import (
	"fmt"

	"gorm.io/gorm"
	"sheff.online/aiplan/internal/aiplan/dao"
)

var (
	userFKs []userFK

	deletedServiceUser *dao.User
)

type userFK struct {
	Table string
	Field string
}

func (b *Business) ReplaceUser(tx *gorm.DB, origUserId string, newUserId string) error {
	return tx.Transaction(func(tx *gorm.DB) error {
		for _, fk := range userFKs {
			tx.SavePoint("preUpdate")
			if err := tx.Table(fk.Table).
				Where(fk.Field+"=?", origUserId).
				Update(fk.Field, newUserId).Error; err != nil {
				if err == gorm.ErrDuplicatedKey {
					tx.RollbackTo("preUpdate")
				} else {
					return err
				}
			}

			if err := tx.Exec(fmt.Sprintf("delete from %s where %s=?", fk.Table, fk.Field), origUserId).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (b *Business) DeleteUser(userId string) error {
	return b.db.Transaction(func(tx *gorm.DB) error {
		// Replace all users records to deleted user
		if err := b.ReplaceUser(tx, userId, deletedServiceUser.ID); err != nil {
			return err
		}

		// Hard delete user
		return tx.Unscoped().Where("id = ?", userId).Delete(&dao.User{}).Error
	})
}

func (b *Business) PopulateUserFKs() error {
	return b.db.Raw(`SELECT
    tc.table_name as Table,
    kcu.column_name as Field
FROM
    information_schema.table_constraints AS tc
    JOIN information_schema.key_column_usage AS kcu
        ON tc.constraint_name = kcu.constraint_name
    JOIN information_schema.constraint_column_usage AS ccu
        ON ccu.constraint_name = tc.constraint_name
WHERE
    tc.constraint_type = 'FOREIGN KEY'
    AND ccu.table_name = 'users'
    AND ccu.column_name = 'id'`).Find(&userFKs).Error
}
