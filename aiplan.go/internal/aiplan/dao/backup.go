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

	"github.com/gofrs/uuid"
)

type WorkspaceBackup struct {
	ID          string    `gorm:"primaryKey" json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	CreatedBy   uuid.UUID `json:"created_by_id"`
	WorkspaceId uuid.UUID `json:"workspace_id" gorm:"type:uuid"`
	Asset       uuid.UUID `json:"asset"`

	Author    *User      `gorm:"foreignKey:CreatedBy" json:"created_by" extensions:"x-nullable"`
	Workspace *Workspace `gorm:"foreignKey:WorkspaceId" json:"workspace_detail" extensions:"x-nullable"`
}

func (wb *WorkspaceBackup) GetWorkspaceId() uuid.UUID {
	return wb.WorkspaceId
}
