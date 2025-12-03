package migration

import (
	"fmt"
	"log/slog"

	"gorm.io/gorm"
)

type Migration struct {
	db *gorm.DB

	sources []IMigration
}

type IMigration interface {
	CheckMigrate() (bool, error)
	Name() string
	Execute() error
}

func New(db *gorm.DB) *Migration {
	return &Migration{
		db: db,
		sources: []IMigration{
			NewMigrateDocAccessRule(db),
			NewMigrateActivityFieldsUpdate(db),
		},
	}
}

func (m *Migration) Run() {
	for _, source := range m.sources {
		ok, err := source.CheckMigrate()
		if err != nil {
			slog.Error("Migration", "error", err.Error())
		}
		if ok {
			slog.Info("Run migration", "name", source.Name())
			err := source.Execute()
			if err != nil {
				slog.Error("Migration", "error", err.Error())
			}
		}
	}
}

type MigratePlan struct {
	delete  []string
	migrate []string
}

func CheckRow(db *gorm.DB, table string) (bool, error) {
	var hasRecords bool
	if err := db.Raw("SELECT EXISTS(SELECT 1 FROM " + table + " LIMIT 1)").Scan(&hasRecords).Error; err != nil {
		return false, err
	}
	return hasRecords, nil
}

func checkTables(db *gorm.DB, tables ...string) (*MigratePlan, error) {
	res := MigratePlan{
		migrate: []string{},
		delete:  []string{},
	}
	var existTable []string
	for _, table := range tables {
		if !db.Migrator().HasTable(table) {
			continue
		}
		existTable = append(existTable, table)
	}

	for _, table := range existTable {
		ok, err := CheckRow(db, table)
		if err != nil {
			return nil, fmt.Errorf("check table %s: %w", table, err)
		}
		if ok {
			res.migrate = append(res.migrate, table)
		} else {
			res.delete = append(res.delete, table)
		}
	}

	return &res, nil
}
