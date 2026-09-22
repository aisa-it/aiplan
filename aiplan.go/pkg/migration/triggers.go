package migration

import (
	_ "embed"
	"log/slog"

	"gorm.io/gorm"
)

//go:embed triggers.sql
var triggersSQL string

// CreateTriggers создаёт триггеры БД из встроенного SQL-скрипта.
// Скрипт лежит рядом с пакетом миграций, а не в cmd, чтобы встраивающий
// модуль получал триггеры ядра вместе с библиотекой.
func CreateTriggers(db *gorm.DB) error {
	slog.Info("Create DB triggers")
	return db.Exec(triggersSQL).Error
}
