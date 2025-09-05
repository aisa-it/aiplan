// DAO (Data Access Object) пакет для работы с данными о командах и их членах.  Предоставляет структуры данных и методы для взаимодействия с базой данных, обеспечивая инкапсуляцию логики доступа к данным.
//
// Основные возможности:
//   - Работа с командами: создание, чтение, обновление и удаление.
//   - Работа с членами команд: добавление, удаление и получение информации о членах команд.
//   - Связывание команд и членов команд через таблицу team_members.
package dao

import "time"

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
	CreatedById *string `json:"created_by_id,omitempty" extensions:"x-nullable"`
	// updated_by_id uuid IS_NULL:YES
	UpdatedById *string `json:"updated_by_id,omitempty" extensions:"x-nullable"`
	// workspace_id uuid IS_NULL:NO
	WorkspaceId string `json:"workspace_id"`

	Workspace *Workspace `json:"workspace" gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
}

func (Team) TableName() string { return "teams" }

// Члены команды
type TeamMembers struct {
	// created_at timestamp with time zone IS_NULL:NO
	CreatedAt time.Time `json:"created_at"`
	// updated_at timestamp with time zone IS_NULL:NO
	UpdatedAt time.Time `json:"updated_at"`
	// id uuid IS_NULL:NO
	Id string `json:"id"`
	// created_by_id uuid IS_NULL:YES
	CreatedById *string `json:"created_by_id,omitempty" extensions:"x-nullable"`
	// member_id uuid IS_NULL:NO
	MemberId string `json:"member_id"`
	// team_id uuid IS_NULL:NO
	TeamId string `json:"team_id"`
	// updated_by_id uuid IS_NULL:YES
	UpdatedById *string `json:"updated_by_id,omitempty" extensions:"x-nullable"`
	// workspace_id uuid IS_NULL:NO
	WorkspaceId string `json:"workspace_id"`

	Workspace *Workspace `json:"workspace" gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	Member    *User      `json:"member" gorm:"foreignKey:MemberId" extensions:"x-nullable"`
	Team      *Team      `json:"team" gorm:"foreignKey:TeamId" extensions:"x-nullable"`
}

func (TeamMembers) TableName() string { return "team_members" }
