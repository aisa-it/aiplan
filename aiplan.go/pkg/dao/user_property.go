package dao

import (
	"strings"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/apierrors"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/dto"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

// Поля типов "user" и "users": значение - id пользователя (users - JSON-массив id),
// допустимы только участники проекта задачи. Отображаемое значение - имя пользователя.

// IsUserPropertyType: тип поля, значение которого - пользователь или список пользователей
func IsUserPropertyType(propType string) bool {
	return propType == "user" || propType == "users"
}

// UserPropertyIds извлекает id пользователей из хранимого значения поля: user - один,
// users - список. Пустое значение - пустой список. Не-UUID -
// ErrPropertyValueValidationFailed, повторы в списке - ErrPropertyValuesNotUnique
func UserPropertyIds(propType, value string) ([]uuid.UUID, error) {
	if value == "" {
		return nil, nil
	}
	var raw []string
	if propType == "users" {
		raw = ParseMultiselectValue(value)
	} else {
		raw = []string{value}
	}
	ids := make([]uuid.UUID, 0, len(raw))
	seen := make(map[uuid.UUID]struct{}, len(raw))
	for _, s := range raw {
		id, err := uuid.FromString(s)
		if err != nil {
			return nil, apierrors.ErrPropertyValueValidationFailed
		}
		if _, dup := seen[id]; dup {
			return nil, apierrors.ErrPropertyValuesNotUnique
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, nil
}

// CheckUserPropertyValue валидирует значение user/users-поля: все пользователи
// существуют и состоят в проекте задачи, иначе ErrPropertyUserNotMember. Возвращает
// пользователей в порядке значения. Для пустого значения (сброс) - (nil, nil)
func CheckUserPropertyValue(db *gorm.DB, projectId uuid.UUID, template ProjectPropertyTemplate, valueStr string) ([]User, error) {
	ids, err := UserPropertyIds(template.Type, valueStr)
	if err != nil || len(ids) == 0 {
		return nil, err
	}

	var found []User
	if err := db.Joins("JOIN project_members pm ON pm.member_id = users.id AND pm.project_id = ?", projectId).
		Where("users.id IN (?)", ids).
		Find(&found).Error; err != nil {
		return nil, err
	}
	byId := make(map[uuid.UUID]User, len(found))
	for _, u := range found {
		byId[u.ID] = u
	}

	users := make([]User, 0, len(ids))
	for _, id := range ids {
		u, ok := byId[id]
		if !ok {
			return nil, apierrors.ErrPropertyUserNotMember
		}
		users = append(users, u)
	}
	return users, nil
}

// UserPropertyLabel - отображаемое значение user/users-поля: имена через запятую
func UserPropertyLabel(users []User) string {
	names := make([]string, 0, len(users))
	for i := range users {
		names = append(names, users[i].GetName())
	}
	return strings.Join(names, ", ")
}

// ResolveUserNames возвращает имена пользователей по id одним запросом
func ResolveUserNames(db *gorm.DB, userIds []uuid.UUID) (map[uuid.UUID]string, error) {
	result := make(map[uuid.UUID]string, len(userIds))
	if len(userIds) == 0 {
		return result, nil
	}
	var users []User
	if err := db.Select("id, first_name, last_name, email").Where("id IN (?)", userIds).Find(&users).Error; err != nil {
		return nil, err
	}
	for i := range users {
		result[users[i].ID] = users[i].GetName()
	}
	return result, nil
}

// userPropertyValueIds - id пользователей из DTO-значения user/users-поля
// (строка для user, список строк для users); не-UUID пропускаются
func userPropertyValueIds(prop dto.IssueProperty) []uuid.UUID {
	if !IsUserPropertyType(prop.Type) {
		return nil
	}
	var raw []string
	switch v := prop.Value.(type) {
	case string:
		raw = []string{v}
	case []string:
		raw = v
	default:
		return nil
	}
	ids := make([]uuid.UUID, 0, len(raw))
	for _, s := range raw {
		if id, err := uuid.FromString(s); err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

// FillUserValueLabels батчем проставляет отображаемые значения (value_label)
// user/users-полям - имена пользователей через запятую
func FillUserValueLabels(db *gorm.DB, props []dto.IssueProperty) error {
	userIds := make([]uuid.UUID, 0, len(props))
	for _, prop := range props {
		userIds = append(userIds, userPropertyValueIds(prop)...)
	}
	if len(userIds) == 0 {
		return nil
	}

	names, err := ResolveUserNames(db, userIds)
	if err != nil {
		return err
	}

	for i := range props {
		ids := userPropertyValueIds(props[i])
		if len(ids) == 0 {
			continue
		}
		label := JoinUserNames(ids, names)
		props[i].ValueLabel = &label
	}
	return nil
}

// JoinUserNames собирает подпись из имён по id в порядке ids; пользователь без
// найденного имени остаётся id (данные не теряем)
func JoinUserNames(ids []uuid.UUID, names map[uuid.UUID]string) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		if name, ok := names[id]; ok {
			parts = append(parts, name)
		} else {
			parts = append(parts, id.String())
		}
	}
	return strings.Join(parts, ", ")
}
