// Файл enrich.go содержит подготовку задачи к вызову Lua-хуков.
package rules

import (
	"github.com/aisa-it/aiplan/aiplan.go/pkg/dao"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

// EnrichIssue догружает в задачу счётчик вложений и значения кастомных полей
// с шаблонами — данные, недоступные после стандартной загрузки, но нужные Lua-правилам.
// Для lookup-, file- и user/users-полей дополнительно резолвит отображаемые значения
// (строка справочника, имя файла вложения, имена пользователей; Value хранит id)
// в IssueProperty.ResolvedValue
func EnrichIssue(db *gorm.DB, issue *dao.Issue) error {
	if err := db.Model(&dao.IssueAttachment{}).
		Where("issue_id = ?", issue.ID).
		Count(&issue.AttachmentCount).Error; err != nil {
		return err
	}

	if err := db.Where("issue_id = ?", issue.ID).
		Preload("Template").
		Find(&issue.Properties).Error; err != nil {
		return err
	}

	if err := resolveLookupValues(db, issue.Properties); err != nil {
		return err
	}
	if err := resolveFileValues(db, issue.Properties); err != nil {
		return err
	}
	return resolveUserValues(db, issue.Properties)
}

// resolveUserValues батчем проставляет ResolvedValue для user/users-полей (имена
// пользователей через запятую); значение с не-UUID остаётся без подписи
func resolveUserValues(db *gorm.DB, props []dao.IssueProperty) error {
	idsByProp := make(map[int][]uuid.UUID)
	var all []uuid.UUID
	for i, prop := range props {
		if prop.Template == nil || !dao.IsUserPropertyType(prop.Template.Type) {
			continue
		}
		ids, err := dao.UserPropertyIds(prop.Template.Type, prop.Value)
		if err != nil || len(ids) == 0 {
			continue
		}
		idsByProp[i] = ids
		all = append(all, ids...)
	}
	if len(all) == 0 {
		return nil
	}

	names, err := dao.ResolveUserNames(db, all)
	if err != nil {
		return err
	}
	for i, ids := range idsByProp {
		props[i].ResolvedValue = dao.JoinUserNames(ids, names)
	}
	return nil
}

// resolveLookupValues батчем проставляет ResolvedValue для lookup-полей
// (отображаемое значение строки справочника)
func resolveLookupValues(db *gorm.DB, props []dao.IssueProperty) error {
	return resolveReferenceValues(db, props, "lookup", dao.ResolveDictionaryRowValues)
}

// resolveFileValues батчем проставляет ResolvedValue для file-полей (имя файла вложения)
func resolveFileValues(db *gorm.DB, props []dao.IssueProperty) error {
	return resolveReferenceValues(db, props, "file", dao.ResolveAttachmentFileNames)
}

// resolveReferenceValues батчем проставляет ResolvedValue полям типа propType, значение
// которых - UUID-ссылка: подписи берутся из resolve одним запросом
func resolveReferenceValues(db *gorm.DB, props []dao.IssueProperty, propType string,
	resolve func(*gorm.DB, []uuid.UUID) (map[uuid.UUID]string, error)) error {
	var ids []uuid.UUID
	for _, prop := range props {
		if id, ok := referenceValueId(prop, propType); ok {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil
	}

	labels, err := resolve(db, ids)
	if err != nil {
		return err
	}

	for i := range props {
		if id, ok := referenceValueId(props[i], propType); ok {
			props[i].ResolvedValue = labels[id]
		}
	}
	return nil
}

// referenceValueId - UUID из значения поля типа propType; поле другого типа, без
// шаблона, пустое или не-UUID - false
func referenceValueId(prop dao.IssueProperty, propType string) (uuid.UUID, bool) {
	if prop.Template == nil || prop.Template.Type != propType || prop.Value == "" {
		return uuid.Nil, false
	}
	id, err := uuid.FromString(prop.Value)
	return id, err == nil
}
