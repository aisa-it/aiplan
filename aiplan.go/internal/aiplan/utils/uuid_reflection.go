// Утилиты для автоматического извлечения UUID полей из моделей DAO через reflection.
// Используется для автоматической генерации списка колонок для миграции text → uuid.
package utils

import (
	"fmt"
	"reflect"
	"strings"
	"unicode"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// ColumnInfo содержит информацию о колонке таблицы для миграции
type ColumnInfo struct {
	Table       string // имя таблицы
	Column      string // имя колонки
	CurrentType string // текущий тип колонки в БД (text, bytea, etc.)
}

// ColumnTypeInfo содержит информацию о текущем типе колонки в базе данных
type ColumnTypeInfo struct {
	DataType string // тип данных (например, "text", "uuid", "bytea")
}

// GetUUIDColumnsFromModels извлекает все UUID поля из переданных моделей DAO.
// Функция использует reflection для обхода всех полей моделей, включая embedded структуры.
//
// Параметры:
//   - models: слайс моделей DAO (обычно содержит &dao.User{}, &dao.Issue{}, и т.д.)
//
// Возвращает:
//   - []ColumnInfo: список пар {table, column} для всех UUID полей с дедупликацией
//
// Пример:
//
//	models := []any{&dao.User{}, &dao.Issue{}, &dao.Project{}}
//	columns := GetUUIDColumnsFromModels(models)
//	// columns = []ColumnInfo{
//	//     {Table: "users", Column: "id"},
//	//     {Table: "users", Column: "created_by_id"},
//	//     {Table: "issues", Column: "id"},
//	//     ...
//	// }
func GetUUIDColumnsFromModels(models []any) []ColumnInfo {
	seen := make(map[string]bool)
	var result []ColumnInfo

	for _, model := range models {
		tableName := getTableName(model)

		modelType := reflect.TypeOf(model)
		if modelType.Kind() == reflect.Pointer {
			modelType = modelType.Elem()
		}

		columns := extractUUIDFieldsRecursive(modelType, tableName)

		for _, col := range columns {
			key := col.Table + ":" + col.Column
			if !seen[key] {
				seen[key] = true
				result = append(result, col)
			}
		}
	}

	return result
}

// extractUUIDFieldsRecursive рекурсивно обходит структуру и извлекает все UUID поля.
// Поддерживает embedded структуры через проверку field.Anonymous.
//
// Параметры:
//   - t: тип структуры для обхода
//   - tableName: имя таблицы для этой структуры
//
// Возвращает:
//   - []ColumnInfo: список UUID полей в этой структуре
func extractUUIDFieldsRecursive(t reflect.Type, tableName string) []ColumnInfo {
	var result []ColumnInfo

	if t.Kind() != reflect.Struct {
		return result
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// Пропускаем unexported поля
		if !field.IsExported() {
			continue
		}

		// Проверяем gorm тег на "-"
		gormTag := field.Tag.Get("gorm")
		if gormTag == "-" {
			continue
		}

		// Проверяем тип поля
		fieldTypeName := field.Type.String()
		isUUID := fieldTypeName == "uuid.UUID" || fieldTypeName == "uuid.NullUUID"

		// Обработка embedded структур (Anonymous fields)
		if field.Anonymous {
			embeddedType := field.Type
			if embeddedType.Kind() == reflect.Pointer {
				embeddedType = embeddedType.Elem()
			}
			// Рекурсивно обрабатываем embedded структуру
			result = append(result, extractUUIDFieldsRecursive(embeddedType, tableName)...)
			continue
		}

		// Если это UUID поле, добавляем в результат
		if isUUID {
			columnName := getColumnName(field, gormTag)
			result = append(result, ColumnInfo{
				Table:  tableName,
				Column: columnName,
			})
		}
	}

	return result
}

// getTableName получает имя таблицы из модели через вызов метода TableName().
// Если модель не реализует TableName(), использует GORM NamingStrategy для плюрализации.
// Это аналогично тому, как GORM определяет имена таблиц.
//
// Параметры:
//   - model: экземпляр модели DAO
//
// Возвращает:
//   - string: имя таблицы для этой модели
func getTableName(model any) string {
	// Интерфейс Tabler - стандартный интерфейс GORM для получения имени таблицы
	type Tabler interface {
		TableName() string
	}

	// Пробуем type assertion на интерфейс Tabler
	if tabler, ok := model.(Tabler); ok {
		return tabler.TableName()
	}

	// Fallback: используем GORM NamingStrategy для плюрализации (как делает GORM)
	modelType := reflect.TypeOf(model)
	if modelType.Kind() == reflect.Pointer {
		modelType = modelType.Elem()
	}

	// Используем default NamingStrategy от GORM для правильной плюрализации
	namer := schema.NamingStrategy{}
	return namer.TableName(modelType.Name())
}

// getColumnName получает имя колонки из GORM тега или через конвертацию имени поля.
//
// Параметры:
//   - field: reflect.StructField с информацией о поле
//   - gormTag: значение тега `gorm:"..."` для этого поля
//
// Возвращает:
//   - string: имя колонки в базе данных
//
// Примеры:
//   - gorm:"column:id;primaryKey" → "id"
//   - gorm:"type:uuid" → snake_case от имени поля
//   - CreatedByID → "created_by_id"
func getColumnName(field reflect.StructField, gormTag string) string {
	// Парсим gorm тег для поиска "column:имя"
	if gormTag != "" {
		for part := range strings.SplitSeq(gormTag, ";") {
			part = strings.TrimSpace(part)
			if columnName, found := strings.CutPrefix(part, "column:"); found {
				return columnName
			}
		}
	}

	// Fallback: конвертируем имя поля в snake_case
	return toSnakeCase(field.Name)
}

// toSnakeCase конвертирует CamelCase строку в snake_case.
//
// Параметры:
//   - s: строка в CamelCase формате
//
// Возвращает:
//   - string: строка в snake_case формате
//
// Примеры:
//   - "ID" → "id"
//   - "CreatedByID" → "created_by_id"
//   - "AvatarId" → "avatar_id"
//   - "SelfUpdatedByUserId" → "self_updated_by_user_id"
func toSnakeCase(s string) string {
	var result strings.Builder
	runes := []rune(s)

	for i, r := range runes {
		if i > 0 && unicode.IsUpper(r) {
			// Проверяем нужно ли добавлять underscore
			needUnderscore := false

			// Если предыдущая буква строчная - это начало нового слова
			if unicode.IsLower(runes[i-1]) {
				needUnderscore = true
			}

			// Если следующая буква строчная - это начало нового слова (например HTTPServer → http_server)
			if i+1 < len(runes) && unicode.IsLower(runes[i+1]) {
				// Но только если предыдущая буква не была уже заглавной
				if i > 0 && unicode.IsLower(runes[i-1]) {
					needUnderscore = true
				}
			}

			if needUnderscore {
				result.WriteRune('_')
			}
		}
		result.WriteRune(unicode.ToLower(r))
	}

	return result.String()
}

// FilterMigratableColumns фильтрует список колонок, оставляя только те, которые нужно мигрировать.
// Исключает колонки, которые:
//   - Уже имеют тип uuid
//   - Не существуют в базе данных
//
// Сохраняет текущий тип колонки (text, bytea, etc.) в поле CurrentType для правильной конвертации.
//
// Параметры:
//   - db: подключение к базе данных GORM
//   - columns: список колонок для проверки
//
// Возвращает:
//   - []ColumnInfo: отфильтрованный список колонок с заполненным CurrentType
//   - error: ошибка при проверке базы данных
func FilterMigratableColumns(db *gorm.DB, columns []ColumnInfo) ([]ColumnInfo, error) {
	var result []ColumnInfo

	for _, col := range columns {
		var typeInfo ColumnTypeInfo

		// Проверяем текущий тип колонки в базе данных
		err := db.Raw(`
			SELECT data_type
			FROM information_schema.columns
			WHERE table_name = ? AND column_name = ?
		`, col.Table, col.Column).Scan(&typeInfo).Error

		if err != nil {
			return nil, err
		}

		// Если колонка не найдена в базе данных, пропускаем
		if typeInfo.DataType == "" {
			continue
		}

		// Если колонка уже имеет тип uuid, пропускаем
		if typeInfo.DataType == "uuid" {
			continue
		}

		// Сохраняем текущий тип и добавляем в список для миграции
		col.CurrentType = typeInfo.DataType
		result = append(result, col)
	}

	return result, nil
}

// ForeignKey содержит информацию о внешнем ключе
type ForeignKey struct {
	ForeignTableName     string
	ForeignColumnName    string
	ReferencedTableName  string
	ReferencedColumnName string
	ConstraintName       string
}

const getForeignKeysSQL = `SELECT
    kcu.table_name AS foreign_table_name,
    kcu.column_name AS foreign_column_name,
    ccu.table_name AS referenced_table_name,
    ccu.column_name AS referenced_column_name,
    tc.constraint_name
FROM information_schema.table_constraints AS tc
JOIN information_schema.key_column_usage AS kcu
    ON tc.constraint_name = kcu.constraint_name
JOIN information_schema.constraint_column_usage AS ccu
    ON ccu.constraint_name = tc.constraint_name
WHERE tc.constraint_type = 'FOREIGN KEY'
    AND ccu.table_name = ?
    AND ccu.column_name = ?;`

// ReplaceColumnTypeWithCast заменяет тип колонки с учетом текущего типа.
// Использует разные стратегии конвертации для text и bytea типов.
// Также обрабатывает foreign keys, которые ссылаются на эту колонку.
//
// Параметры:
//   - db: подключение к базе данных GORM
//   - col: информация о колонке (включая CurrentType)
//   - newType: целевой тип (обычно "uuid")
//
// Возвращает:
//   - error: ошибка при замене типа
func ReplaceColumnTypeWithCast(db *gorm.DB, col ColumnInfo, newType string) error {
	// Получаем список FK, которые ссылаются на эту колонку
	var fks []ForeignKey
	if err := db.Raw(getForeignKeysSQL, col.Table, col.Column).Find(&fks).Error; err != nil {
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		// Шаг 1: Удаляем FK constraints (IF EXISTS т.к. они могли быть удалены ранее в DropAllForeignKeys)
		for _, fk := range fks {
			if err := tx.Exec(fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT IF EXISTS %s;", fk.ForeignTableName, fk.ConstraintName)).Error; err != nil {
				return err
			}
		}

		// Шаг 2: Меняем тип основной колонки с правильным USING clause
		var usingClause string
		switch col.CurrentType {
		case "bytea":
			// Для bytea используем encode для конвертации в hex, затем в uuid
			usingClause = fmt.Sprintf("encode(%s, 'hex')::%s", col.Column, newType)
		default:
			// Для text, character varying и других - прямая конвертация
			usingClause = fmt.Sprintf("%s::%s", col.Column, newType)
		}

		if err := tx.Exec(fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE %s USING %s;",
			col.Table, col.Column, newType, usingClause)).Error; err != nil {
			return err
		}

		// Шаг 3: Меняем тип в FK колонках (они всегда text → uuid)
		for _, fk := range fks {
			if err := tx.Exec(fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE %s USING %s::%s;",
				fk.ForeignTableName, fk.ForeignColumnName, newType, fk.ForeignColumnName, newType)).Error; err != nil {
				return err
			}
		}

		// Шаг 4: Восстанавливаем FK constraints
		for _, fk := range fks {
			if err := tx.Exec(fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s(%s);",
				fk.ForeignTableName, fk.ConstraintName, fk.ForeignColumnName, fk.ReferencedTableName, fk.ReferencedColumnName)).Error; err != nil {
				return err
			}
		}

		return nil
	})
}
