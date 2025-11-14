// Пакет aiplan предоставляет функциональность для работы с Git репозиториями
// через файловую систему без использования базы данных.
//
// Архитектурный принцип: Git репозитории должны быть полностью самодостаточными
// и хранить всю информацию в файловой системе. База данных НЕ должна
// использоваться для хранения метаданных Git репозиториев.
package aiplan

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/uuid"
)

// GitRepository представляет Git репозиторий, хранящийся в файловой системе
type GitRepository struct {
	// Name - название репозитория
	Name string `json:"name"`

	// Workspace - slug рабочего пространства
	Workspace string `json:"workspace"`

	// Private - флаг приватности репозитория
	Private bool `json:"private"`

	// Description - описание репозитория
	Description string `json:"description"`

	// CreatedAt - время создания репозитория
	CreatedAt time.Time `json:"created_at"`

	// CreatedBy - UUID пользователя, создавшего репозиторий
	CreatedBy uuid.UUID `json:"created_by"`

	// Branch - ветка по умолчанию
	Branch string `json:"branch"`

	// Path - полный путь к репозиторию на файловой системе (не сохраняется в JSON)
	Path string `json:"-"`
}

// Save сохраняет метаданные репозитория в файл aiplan.json
func (r *GitRepository) Save() error {
	metadataPath := filepath.Join(r.Path, "aiplan.json")
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal repository metadata: %w", err)
	}

	if err := os.WriteFile(metadataPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write metadata file: %w", err)
	}

	return nil
}

// LoadGitRepository загружает метаданные репозитория из файла aiplan.json
func LoadGitRepository(path string) (*GitRepository, error) {
	metadataPath := filepath.Join(path, "aiplan.json")
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata file: %w", err)
	}

	var repo GitRepository
	if err := json.Unmarshal(data, &repo); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	repo.Path = path
	return &repo, nil
}

// GitRepositoryExists проверяет существование репозитория по workspace slug и имени
func GitRepositoryExists(workspaceSlug, repoName, gitReposPath string) bool {
	repoPath := filepath.Join(gitReposPath, workspaceSlug, repoName+".git")
	metadataPath := filepath.Join(repoPath, "aiplan.json")

	_, err := os.Stat(metadataPath)
	return err == nil
}

// ListGitRepositories возвращает список всех репозиториев в workspace
func ListGitRepositories(workspaceSlug, gitReposPath string) ([]*GitRepository, error) {
	workspacePath := filepath.Join(gitReposPath, workspaceSlug)

	entries, err := os.ReadDir(workspacePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []*GitRepository{}, nil
		}
		return nil, fmt.Errorf("failed to read workspace directory: %w", err)
	}

	var repos []*GitRepository
	for _, entry := range entries {
		if entry.IsDir() && strings.HasSuffix(entry.Name(), ".git") {
			repoPath := filepath.Join(workspacePath, entry.Name())
			repo, err := LoadGitRepository(repoPath)
			if err != nil {
				// Пропускаем поврежденные репозитории
				continue
			}
			repos = append(repos, repo)
		}
	}

	return repos, nil
}

// DeleteGitRepository удаляет репозиторий из файловой системы
func DeleteGitRepository(workspaceSlug, repoName, gitReposPath string) error {
	repoPath := filepath.Join(gitReposPath, workspaceSlug, repoName+".git")

	// Проверяем существование репозитория
	if !GitRepositoryExists(workspaceSlug, repoName, gitReposPath) {
		return fmt.Errorf("repository does not exist")
	}

	// Удаляем директорию репозитория
	if err := os.RemoveAll(repoPath); err != nil {
		return fmt.Errorf("failed to delete repository directory: %w", err)
	}

	return nil
}

// ValidateRepositoryName проверяет корректность названия репозитория
// Допустимые символы: a-z, A-Z, 0-9, дефис (-), подчеркивание (_), точка (.)
func ValidateRepositoryName(name string) bool {
	if len(name) == 0 || len(name) > 100 {
		return false
	}

	for _, char := range name {
		if !((char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '-' || char == '_' || char == '.') {
			return false
		}
	}

	// Не должно начинаться или заканчиваться точкой
	if name[0] == '.' || name[len(name)-1] == '.' {
		return false
	}

	return true
}

// GetRepositoryPath возвращает полный путь к репозиторию
func GetRepositoryPath(workspaceSlug, repoName, gitReposPath string) string {
	return filepath.Join(gitReposPath, workspaceSlug, repoName+".git")
}
