// Кастомный PostgreSQL диалектор для GORM, который использует нативный тип uuid вместо bytea для uuid.UUID полей.
package utils

import (
	"fmt"
	"log/slog"
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
	if fieldType == reflect.TypeOf(uuid.UUID{}) || fieldType == reflect.TypeOf(uuid.NullUUID{}) {
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
	// Получаем информацию о поле
	stmt := &gorm.Statement{DB: m.DB}
	if err := stmt.Parse(value); err != nil {
		return err
	}

	if schemaField := stmt.Schema.LookUpField(field); schemaField != nil {
		// Проверяем текущий тип в базе данных
		columnTypes, err := m.DB.Migrator().ColumnTypes(value)
		if err != nil {
			return err
		}

		for _, columnType := range columnTypes {
			if columnType.Name() == schemaField.DBName {
				// Если колонка уже имеет тип uuid, не изменяем её
				databaseType := columnType.DatabaseTypeName()

				// Debug logging
				slog.Info("AlterColumn check",
					"table", stmt.Table,
					"column", schemaField.DBName,
					"dbType", databaseType,
					"wantType", m.DataTypeOf(schemaField))

				if strings.EqualFold(databaseType, "uuid") {
					fieldType := schemaField.FieldType
					if fieldType.Kind() == reflect.Pointer {
						fieldType = fieldType.Elem()
					}
					if fieldType == reflect.TypeOf(uuid.UUID{}) || fieldType == reflect.TypeOf(uuid.NullUUID{}) {
						// Колонка уже uuid и поле тоже uuid - пропускаем изменение
						slog.Info("Skipping AlterColumn (already uuid)",
							"table", stmt.Table,
							"column", schemaField.DBName)
						return nil
					}
				}
			}
		}

		// Используем стандартную логику для остальных случаев
		return m.DB.Exec(
			"ALTER TABLE ? ALTER COLUMN ? TYPE ?",
			clause.Table{Name: stmt.Table}, clause.Column{Name: schemaField.DBName},
			clause.Expr{SQL: m.DataTypeOf(schemaField)},
		).Error
	}

	return fmt.Errorf("failed to look up field with name: %s", field)
}
