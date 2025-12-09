// DAO (Data Access Object) пакет для работы с данными о командах и их членах.  Предоставляет структуры данных и методы для взаимодействия с базой данных, обеспечивая инкапсуляцию логики доступа к данным.
//
// Основные возможности:
//   - Работа с командами: создание, чтение, обновление и удаление.
//   - Работа с членами команд: добавление, удаление и получение информации о членах команд.
//   - Связывание команд и членов команд через таблицу team_members.
package dao

import (
	"time"

	"github.com/gofrs/uuid"
)

// Команды
type Team struct {
	// created_at timestamp with time zone IS_NULL:NO
	CreatedAt time.Time `json:"created_at"`
	// updated_at timestamp with time zone IS_NULL:NO
	UpdatedAt time.Time `json:"updated_at"`
	// id uuid IS_NULL:NO
	Id string `json:"id"`
	// name character varying IS_NULL:NO
	Name string `json:"name"`
	// description text IS_NULL:NO
	Description string `json:"description"`
	// created_by_id uuid IS_NULL:YES
	// Note: type:text используется потому что в существующей БД это поле имеет тип text, а не uuid
	CreatedById uuid.NullUUID `json:"created_by_id,omitempty" gorm:"type:uuid" extensions:"x-nullable"`
	// updated_by_id uuid IS_NULL:YES
	// Note: type:text используется потому что в существующей БД это поле имеет тип text, а не uuid
	UpdatedById uuid.NullUUID `json:"updated_by_id,omitempty" gorm:"type:uuid" extensions:"x-nullable"`
	// workspace_id uuid IS_NULL:NO
	WorkspaceId uuid.UUID `json:"workspace_id" gorm:"type:uuid"`

	Workspace *Workspace `json:"workspace" gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	CreatedBy *User      `json:"created_by_detail" gorm:"foreignKey:CreatedById;references:ID" extensions:"x-nullable"`
	UpdatedBy *User      `json:"updated_by_detail" gorm:"foreignKey:UpdatedById;references:ID" extensions:"x-nullable"`
}

func (Team) TableName() string { return "teams" }

func (t Team) GetWorkspaceId() uuid.UUID {
	return t.WorkspaceId
}

// Члены команды
type TeamMembers struct {
	// created_at timestamp with time zone IS_NULL:NO
	CreatedAt time.Time `json:"created_at"`
	// updated_at timestamp with time zone IS_NULL:NO
	UpdatedAt time.Time `json:"updated_at"`
	// id uuid IS_NULL:NO
	Id string `json:"id"`
	// created_by_id uuid IS_NULL:YES
	// Note: type:text используется потому что в существующей БД это поле имеет тип text, а не uuid
	CreatedById uuid.NullUUID `json:"created_by_id,omitempty" gorm:"type:uuid" extensions:"x-nullable"`
	// member_id uuid IS_NULL:NO
	// Note: type:text используется потому что в существующей БД это поле имеет тип text, а не uuid
	MemberId uuid.UUID `json:"member_id" gorm:"type:uuid"`
	// team_id uuid IS_NULL:NO
	TeamId string `json:"team_id"`
	// updated_by_id uuid IS_NULL:YES
	// Note: type:text используется потому что в существующей БД это поле имеет тип text, а не uuid
	UpdatedById uuid.NullUUID `json:"updated_by_id,omitempty" gorm:"type:uuid" extensions:"x-nullable"`
	// workspace_id uuid IS_NULL:NO
	WorkspaceId uuid.UUID `json:"workspace_id" gorm:"type:uuid"`

	Workspace *Workspace `json:"workspace" gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	Member    *User      `json:"member" gorm:"foreignKey:MemberId" extensions:"x-nullable"`
	Team      *Team      `json:"team" gorm:"foreignKey:TeamId" extensions:"x-nullable"`
	CreatedBy *User      `json:"created_by_detail" gorm:"foreignKey:CreatedById;references:ID" extensions:"x-nullable"`
	UpdatedBy *User      `json:"updated_by_detail" gorm:"foreignKey:UpdatedById;references:ID" extensions:"x-nullable"`
}

func (TeamMembers) TableName() string { return "team_members" }

func (tm TeamMembers) GetWorkspaceId() uuid.UUID {
	return tm.WorkspaceId
}
