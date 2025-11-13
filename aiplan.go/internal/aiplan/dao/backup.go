// DAO для работы с бэкапами рабочих пространств.  Предоставляет методы для создания, чтения, обновления и удаления бэкапов, связанных с конкретным рабочим пространством и аккаунтом пользователя.  Также обеспечивает связь с моделями User и Workspace для хранения метаданных.
//
// Основные возможности:
//   - Создание новых бэкапов рабочих пространств.
//   - Получение информации о бэкапах по различным критериям (например, по ID, рабочему пространству).
//   - Обновление информации о бэкапах.
//   - Удаление бэкапов.
//   - Получение бэкапов, связанных с конкретным рабочим пространством и пользователем.
package dao

import (
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"gorm.io/gorm"

	"github.com/gofrs/uuid"
)

type WorkspaceBackup struct {
	ID          uuid.UUID            `gorm:"primaryKey;type:uuid"`
	CreatedAt   time.Time            `json:"created_at"`
	CreatedBy   string               `json:"created_by_id"`
	WorkspaceId *string              `json:"workspace_id"`
	Asset       uuid.UUID            `json:"asset"`
	MetaData    types.BackupMetadata `json:"meta_data" gorm:"type:jsonb"`
	InProgress  bool                 `json:"in_progress" gorm:"default:true"`

	Author    *User      `gorm:"foreignKey:CreatedBy" json:"created_by" extensions:"x-nullable"`
	Workspace *Workspace `gorm:"-" json:"workspace_detail" extensions:"x-nullable"`
}

func (wb *WorkspaceBackup) BeforeDelete(tx *gorm.DB) error {
	var file FileAsset
	if err := tx.Where("id = ?", wb.Asset).First(&file).Error; err != nil {
		return err
	}

	if err := tx.Delete(&file).Error; err != nil {
		return err
	}
	return nil
}
