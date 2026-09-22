// Типы задач проекта: справочник, задающий вид задачи (заявка, инцидент и т.п.).
// Тип у задачи необязателен: задачи, заведённые до появления справочника,
// остаются без типа.
package dao

import (
	"errors"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/dto"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

// IssueType - тип задачи уровня проекта
type IssueType struct {
	Id          uuid.UUID `gorm:"primaryKey;type:uuid"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	CreatedById uuid.NullUUID `gorm:"type:uuid" extensions:"x-nullable"`
	UpdatedById uuid.NullUUID `gorm:"type:uuid" extensions:"x-nullable"`

	WorkspaceId uuid.UUID `gorm:"type:uuid"`
	// Имя уникально в пределах проекта: тип выбирают по названию.
	// Частичный индекс по признаку default: тип по умолчанию в проекте один
	ProjectId uuid.UUID `gorm:"type:uuid;uniqueIndex:unique_issue_type_idx,priority:1;uniqueIndex:unique_default_issue_type_idx,where:\"default\" = true"`

	Name        string `gorm:"not null;uniqueIndex:unique_issue_type_idx,priority:2"`
	Description string
	Color       string
	// Default - тип, подставляемый в новую задачу
	Default bool

	Workspace *Workspace `gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	Project   *Project   `gorm:"foreignKey:ProjectId" extensions:"x-nullable"`
	CreatedBy *User      `gorm:"foreignKey:CreatedById;references:ID;belongsTo" extensions:"x-nullable"`
	UpdatedBy *User      `gorm:"foreignKey:UpdatedById;references:ID;belongsTo" extensions:"x-nullable"`
}

func (IssueType) TableName() string { return "project_issue_types" }

// ToLightDTO преобразует тип задачи в облегченное DTO
func (it *IssueType) ToLightDTO() *dto.IssueTypeLight {
	if it == nil {
		return nil
	}
	return &dto.IssueTypeLight{
		Id:          it.Id,
		Name:        it.Name,
		Description: it.Description,
		Color:       it.Color,
		Default:     it.Default,
	}
}

// IsIssueTypeUsed сообщает, есть ли задачи с этим типом
func IsIssueTypeUsed(db *gorm.DB, issueTypeId uuid.UUID) (bool, error) {
	var used bool
	err := db.Model(&Issue{}).
		Select("count(*) > 0").
		Where("issue_type_id = ?", issueTypeId).
		Find(&used).Error
	return used, err
}

// ResolveIssueTypeForProject возвращает тип для новой задачи: заданный клиентом
// (с проверкой принадлежности проекту) либо тип проекта по умолчанию.
// Второе значение - false, если заданный тип в проекте не найден.
func ResolveIssueTypeForProject(db *gorm.DB, projectId uuid.UUID, requested uuid.NullUUID) (uuid.NullUUID, bool, error) {
	var issueType IssueType
	query := db.Model(&IssueType{}).Select("id").Where("project_id = ?", projectId)
	if requested.Valid {
		query = query.Where("id = ?", requested.UUID)
	} else {
		query = query.Where("\"default\" = true")
	}

	if err := query.First(&issueType).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Типа по умолчанию может не быть - задача останется без типа
			return uuid.NullUUID{}, !requested.Valid, nil
		}
		return uuid.NullUUID{}, false, err
	}
	return uuid.NullUUID{UUID: issueType.Id, Valid: true}, true, nil
}
