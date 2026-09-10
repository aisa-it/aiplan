// Файл enrich.go содержит подготовку задачи к вызову Lua-хуков.
package rules

import (
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

// EnrichIssue догружает в задачу счётчик вложений и значения кастомных полей
// с шаблонами — данные, недоступные после стандартной загрузки, но нужные Lua-правилам.
// Для lookup-полей дополнительно резолвит отображаемые значения строк справочников
// (Value хранит id строки) в IssueProperty.ResolvedValue
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

	return resolveLookupValues(db, issue.Properties)
}

// resolveLookupValues батчем проставляет ResolvedValue для lookup-полей
func resolveLookupValues(db *gorm.DB, props []dao.IssueProperty) error {
	var rowIds []uuid.UUID
	for _, prop := range props {
		if prop.Template == nil || prop.Template.Type != "lookup" || prop.Value == "" {
			continue
		}
		if rowId, err := uuid.FromString(prop.Value); err == nil {
			rowIds = append(rowIds, rowId)
		}
	}
	if len(rowIds) == 0 {
		return nil
	}

	labels, err := dao.ResolveDictionaryRowValues(db, rowIds)
	if err != nil {
		return err
	}

	for i := range props {
		prop := &props[i]
		if prop.Template == nil || prop.Template.Type != "lookup" || prop.Value == "" {
			continue
		}
		if rowId, err := uuid.FromString(prop.Value); err == nil {
			prop.ResolvedValue = labels[rowId]
		}
	}
	return nil
}
