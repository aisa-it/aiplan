// Файл enrich.go содержит подготовку задачи к вызову Lua-хуков.
package rules

import (
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"gorm.io/gorm"
)

// EnrichIssue догружает в задачу счётчик вложений и значения кастомных полей
// с шаблонами — данные, недоступные после стандартной загрузки, но нужные Lua-правилам
func EnrichIssue(db *gorm.DB, issue *dao.Issue) error {
	if err := db.Model(&dao.IssueAttachment{}).
		Where("issue_id = ?", issue.ID).
		Count(&issue.AttachmentCount).Error; err != nil {
		return err
	}

	return db.Where("issue_id = ?", issue.ID).
		Preload("Template").
		Find(&issue.Properties).Error
}
