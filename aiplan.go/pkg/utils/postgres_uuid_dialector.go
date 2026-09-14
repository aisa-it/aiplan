// Кастомный PostgreSQL диалектор для GORM, который использует нативный тип uuid вместо bytea для uuid.UUID полей.
package utils

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/gofrs/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/migrator"
	"gorm.io/gorm/schema"
)

// PostgresUUIDDialector оборачивает стандартный postgres.Dialector и переопределяет DataTypeOf для UUID типов.
type PostgresUUIDDialector struct {
	*postgres.Dialector
}

// NewPostgresUUIDDialector создает новый диалектор с поддержкой нативного uuid типа PostgreSQL.
func NewPostgresUUIDDialector(config postgres.Config) gorm.Dialector {
	return &PostgresUUIDDialector{
		Dialector: postgres.New(config).(*postgres.Dialector),
	}
}

// Migrator возвращает кастомный migrator с правильной обработкой UUID типов.
func (d *PostgresUUIDDialector) Migrator(db *gorm.DB) gorm.Migrator {
	return &PostgresUUIDMigrator{
		Migrator: postgres.Migrator{
			Migrator: migrator.Migrator{
				Config: migrator.Config{
					DB:                          db,
					Dialector:                   d,
					CreateIndexAfterCreateTable: true,
				},
			},
		},
	}
}

// PostgresUUIDMigrator кастомный migrator который правильно обрабатывает UUID типы.
type PostgresUUIDMigrator struct {
	postgres.Migrator
}

// DataTypeOf переопределяет метод для возврата правильного типа данных для UUID полей.
func (m *PostgresUUIDMigrator) DataTypeOf(field *schema.Field) string {
	// Проверяем тип поля
	fieldType := field.FieldType
	if fieldType.Kind() == reflect.Pointer {
		fieldType = fieldType.Elem()
	}

	// Если это uuid.UUID или uuid.NullUUID - используем нативный uuid тип PostgreSQL
	switch fieldType {
	case reflect.TypeFor[uuid.UUID](), reflect.TypeFor[uuid.NullUUID]():
		return "uuid"
	}

	// Для всех остальных типов используем стандартную логику
	return m.Migrator.DataTypeOf(field)
}

// ColumnTypes переопределяет метод для правильного определения типов колонок.
func (m *PostgresUUIDMigrator) ColumnTypes(value interface{}) ([]gorm.ColumnType, error) {
	columnTypes, err := m.Migrator.ColumnTypes(value)
	if err != nil {
		return nil, err
	}
	return columnTypes, nil
}

// AlterColumn переопределяет метод для предотвращения ненужных изменений UUID колонок.
func (m *PostgresUUIDMigrator) AlterColumn(value interface{}, field string) error {
	stmt := &gorm.Statement{DB: m.DB}
	if err := stmt.Parse(value); err != nil {
		return err
	}

	schemaField := stmt.Schema.LookUpField(field)
	if schemaField == nil {
		return fmt.Errorf("failed to look up field with name: %s", field)
	}

	columnTypes, err := m.DB.Migrator().ColumnTypes(value)
	if err != nil {
		return err
	}

	targetType := m.DataTypeOf(schemaField)
	if columnAlreadyHasType(columnTypes, schemaField, targetType) {
		return nil
	}

	return m.DB.Exec(
		"ALTER TABLE ? ALTER COLUMN ? TYPE ?",
		clause.Table{Name: stmt.Table}, clause.Column{Name: schemaField.DBName},
		clause.Expr{SQL: targetType},
	).Error
}

// columnAlreadyHasType возвращает true, если колонка в БД уже имеет тип, эквивалентный целевому.
// Покрывает два кейса: (1) UUID-колонки с uuid.UUID/uuid.NullUUID полем, (2) любой тип, нормализованно
// совпадающий с targetType. Нужно чтобы пропускать ALTER COLUMN TYPE — он падает на колонках,
// упомянутых в триггерах UPDATE OF (например, users.user_timezone в users_light_notify).
func columnAlreadyHasType(columnTypes []gorm.ColumnType, schemaField *schema.Field, targetType string) bool {
	for _, columnType := range columnTypes {
		if columnType.Name() != schemaField.DBName {
			continue
		}

		databaseType := columnType.DatabaseTypeName()

		if strings.EqualFold(databaseType, "uuid") {
			fieldType := schemaField.FieldType
			if fieldType.Kind() == reflect.Pointer {
				fieldType = fieldType.Elem()
			}
			switch fieldType {
			case reflect.TypeFor[uuid.UUID](), reflect.TypeFor[uuid.NullUUID]():
				return true
			}
		}

		return normalizeColumnType(databaseType) == normalizeColumnType(targetType)
	}
	return false
}

// normalizeColumnType приводит SQL-тип к каноничной форме для сравнения:
// нижний регистр, без размера/precision/timezone-суффиксов и без обёртывающих пробелов.
// Используется чтобы пропустить ALTER COLUMN TYPE, когда фактический тип в БД
// уже совпадает с целевым (например, "TEXT" vs "text" или "varchar(255)" vs "varchar").
// Это критично для колонок с триггерами UPDATE OF — Postgres запрещает на них ALTER TYPE.
func normalizeColumnType(t string) string {
	s := strings.ToLower(strings.TrimSpace(t))
	if i := strings.Index(s, "("); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	switch s {
	case "character varying":
		return "varchar"
	case "character":
		return "char"
	case "timestamp without time zone":
		return "timestamp"
	case "timestamp with time zone":
		return "timestamptz"
	case "time without time zone":
		return "time"
	case "time with time zone":
		return "timetz"
	}
	return s
}
