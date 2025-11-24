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

// ListGitRepositoriesResponse - структура ответа со списком репозиториев
type ListGitRepositoriesResponse struct {
	Repositories []GitRepositoryLight `json:"repositories"`
	Total        int                  `json:"total"`
}

// DeleteGitRepositoryRequest - структура запроса на удаление Git репозитория
type DeleteGitRepositoryRequest struct {
	// Name - название репозитория (обязательное поле)
	Name string `json:"name" validate:"required"`
}

// ========================================
// SSH Keys DTOs
// ========================================

// SSHKeyDTO - SSH ключ для ответов API (без публичного ключа для безопасности)
type SSHKeyDTO struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	KeyType     string     `json:"key_type"`
	Fingerprint string     `json:"fingerprint"`
	CreatedAt   time.Time  `json:"created_at"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	Comment     string     `json:"comment,omitempty"`
}

// AddSSHKeyRequest - запрос на добавление SSH ключа
type AddSSHKeyRequest struct {
	Name      string `json:"name" validate:"required,min=1,max=255"`
	PublicKey string `json:"public_key" validate:"required"`
}

// AddSSHKeyResponse - ответ при добавлении SSH ключа
type AddSSHKeyResponse struct {
	SSHKeyDTO
}

// ListSSHKeysResponse - список SSH ключей пользователя
type ListSSHKeysResponse struct {
	Keys  []SSHKeyDTO `json:"keys"`
	Total int         `json:"total"`
}

// DeleteSSHKeyRequest - запрос на удаление SSH ключа (не используется, keyId в URL)
type DeleteSSHKeyRequest struct {
	KeyId string `json:"key_id" validate:"required"`
}

// SSHConfigResponse - конфигурация SSH сервера
type SSHConfigResponse struct {
	SSHEnabled bool   `json:"ssh_enabled"`
	SSHHost    string `json:"ssh_host"`
	SSHPort    int    `json:"ssh_port"`
}
