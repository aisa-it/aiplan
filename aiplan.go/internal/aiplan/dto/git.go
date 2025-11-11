// Содержит структуры данных (DTO) для работы с Git репозиториями.
// Предназначен для обеспечения структурированного обмена данными между компонентами приложения и внешними системами.
//
// Важно: Git репозитории НЕ хранятся в базе данных, вся информация находится в файловой системе.
package dto

import "time"

// GitRepositoryLight - облегченная структура для представления Git репозитория
type GitRepositoryLight struct {
	Workspace   string    `json:"workspace"`
	Name        string    `json:"name"`
	Path        string    `json:"path"`
	Branch      string    `json:"branch"`
	Private     bool      `json:"private"`
	Description string    `json:"description"`
	CloneURL    string    `json:"clone_url"`
	CreatedAt   time.Time `json:"created_at"`
	CreatedBy   string    `json:"created_by"` // UUID пользователя
}

// CreateGitRepositoryRequest - структура запроса на создание Git репозитория
type CreateGitRepositoryRequest struct {
	// Workspace - slug рабочего пространства (обязательное поле)
	Workspace string `json:"workspace" validate:"required"`

	// Name - название репозитория (обязательное поле)
	// Допустимые символы: a-z, A-Z, 0-9, дефис, подчеркивание, точка
	Name string `json:"name" validate:"required,min=1,max=100"`

	// Branch - начальная ветка репозитория (необязательное поле, по умолчанию: main)
	Branch string `json:"branch,omitempty"`

	// Private - флаг приватности репозитория (необязательное поле, по умолчанию: false)
	Private bool `json:"private"`

	// Description - описание репозитория (необязательное поле)
	Description string `json:"description,omitempty"`
}

// CreateGitRepositoryResponse - структура ответа при создании Git репозитория
type CreateGitRepositoryResponse struct {
	Workspace   string     `json:"workspace"`
	Name        string     `json:"name"`
	Path        string     `json:"path"`
	Branch      string     `json:"branch"`
	Private     bool       `json:"private"`
	Description string     `json:"description"`
	CloneURL    string     `json:"clone_url"`
	CreatedAt   time.Time  `json:"created_at"`
	CreatedBy   *UserLight `json:"created_by,omitempty"`
}
