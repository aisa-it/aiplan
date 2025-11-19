// Пакет aiplan предоставляет функциональность для управления GIT.
//
// Архитектурный принцип: Git репозитории НЕ хранятся в базе данных.
// Вся информация находится в файловой системе в файлах aiplan.json.
// База данных используется только для workspace, users и audit log (activity).
package aiplan

import (
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/apierrors"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dto"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/gofrs/uuid"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

// ========================================
// Git Browse API DTO Structures
// ========================================

// TreeEntryDTO представляет файл или директорию в Git tree
type TreeEntryDTO struct {
	Name string `json:"name"`         // Имя файла/директории
	Type string `json:"type"`         // "file" или "dir"
	Mode string `json:"mode"`         // Режим файла (100644, 040000, etc.)
	Size int64  `json:"size,omitempty"` // Размер файла (только для файлов)
	SHA  string `json:"sha"`          // SHA объекта
}

// TreeResponseDTO представляет ответ на запрос дерева репозитория
type TreeResponseDTO struct {
	Ref     string         `json:"ref"`     // Ветка/тег/коммит
	Path    string         `json:"path"`    // Путь в репозитории
	Entries []TreeEntryDTO `json:"entries"` // Список файлов и директорий
}

// BlobResponseDTO представляет ответ на запрос содержимого файла
type BlobResponseDTO struct {
	Path     string `json:"path"`     // Путь к файлу
	Ref      string `json:"ref"`      // Ветка/тег/коммит
	Size     int64  `json:"size"`     // Размер файла
	SHA      string `json:"sha"`      // SHA объекта
	Content  string `json:"content"`  // Содержимое файла (base64 encoded)
	Encoding string `json:"encoding"` // "base64"
	IsBinary bool   `json:"is_binary"` // Является ли файл бинарным
}

// PersonDTO представляет информацию об авторе/коммиттере
type PersonDTO struct {
	Name  string    `json:"name"`  // Имя
	Email string    `json:"email"` // Email
	Date  time.Time `json:"date"`  // Дата
}

// CommitDTO представляет информацию о коммите
type CommitDTO struct {
	SHA        string     `json:"sha"`         // SHA коммита
	Author     PersonDTO  `json:"author"`      // Автор коммита
	Committer  PersonDTO  `json:"committer"`   // Коммиттер
	Message    string     `json:"message"`     // Сообщение коммита
	ParentSHAs []string   `json:"parent_shas"` // SHA родительских коммитов
}

// CommitsResponseDTO представляет ответ на запрос истории коммитов
type CommitsResponseDTO struct {
	Commits []CommitDTO `json:"commits"` // Список коммитов
	Total   int         `json:"total"`   // Общее количество коммитов
	Limit   int         `json:"limit"`   // Лимит на страницу
	Offset  int         `json:"offset"`  // Смещение
}

// BranchDTO представляет информацию о ветке
type BranchDTO struct {
	Name      string `json:"name"`       // Имя ветки
	SHA       string `json:"sha"`        // SHA последнего коммита
	IsDefault bool   `json:"is_default"` // Является ли веткой по умолчанию
}

// BranchesResponseDTO представляет ответ на запрос списка веток
type BranchesResponseDTO struct {
	Branches []BranchDTO `json:"branches"` // Список веток
}

// RepoInfoDTO представляет информацию о репозитории
type RepoInfoDTO struct {
	Name          string     `json:"name"`            // Имя репозитория
	Workspace     string     `json:"workspace"`       // Slug workspace
	DefaultBranch string     `json:"default_branch"`  // Ветка по умолчанию
	BranchesCount int        `json:"branches_count"`  // Количество веток
	CommitsCount  int        `json:"commits_count"`   // Количество коммитов
	Size          int64      `json:"size"`            // Размер репозитория (байты)
	LastCommit    *CommitDTO `json:"last_commit,omitempty"` // Последний коммит
}

// @Summary Конфигурация: получение Git настроек
// @Description Возвращает информацию о состоянии Git конфигурации системы
// @Tags GIT
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} dto.GitConfigInfo "Информация о Git конфигурации"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/git/config/ [get]
func (s *Services) getGitConfig(c echo.Context) error {
	// Проверяем, что пользователь авторизован (middleware уже проверил это)
	_ = c.(AuthContext).User

	gitConfig := dto.GitConfigInfo{
		GitEnabled:          cfg.GitEnabled,
		GitRepositoriesPath: cfg.GitRepositoriesPath,
	}

	return c.JSON(http.StatusOK, gitConfig)
}

// @Summary Репозиторий: создание Git репозитория
// @Description Создает новый bare Git репозиторий в указанном рабочем пространстве. Метаданные хранятся в файловой системе.
// @Tags GIT
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug workspace"
// @Param request body dto.CreateGitRepositoryRequest true "Параметры создания репозитория"
// @Success 201 {object} dto.CreateGitRepositoryResponse "Созданный репозиторий"
// @Failure 400 {object} apierrors.DefinedError "Некорректный запрос"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Git отключен или недостаточно прав"
// @Failure 409 {object} apierrors.DefinedError "Репозиторий с таким именем уже существует"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/git/{workspaceSlug}/repositories/ [post]
func (s *Services) createGitRepository(c echo.Context) error {
	user := c.(AuthContext).User

	// Проверяем, что Git функциональность включена
	if !cfg.GitEnabled {
		return EErrorDefined(c, apierrors.ErrGitDisabled)
	}

	// Проверяем наличие пути к репозиториям
	if cfg.GitRepositoriesPath == "" {
		slog.Error("GIT_REPOSITORIES_PATH is not configured")
		return EErrorDefined(c, apierrors.ErrGitDisabled)
	}

	// Получаем workspace slug из URL
	workspaceSlug := c.Param("workspaceSlug")
	if workspaceSlug == "" {
		return EErrorDefined(c, apierrors.ErrGeneric)
	}

	// Парсим входные данные
	var req dto.CreateGitRepositoryRequest
	if err := c.Bind(&req); err != nil {
		slog.Error("Failed to bind request", "err", err)
		return EErrorDefined(c, apierrors.ErrGeneric)
	}

	// Валидация обязательных полей
	if req.Name == "" {
		return EErrorDefined(c, apierrors.ErrGeneric)
	}

	// Валидация имени репозитория
	if !ValidateRepositoryName(req.Name) {
		return EErrorDefined(c, apierrors.ErrGitInvalidRepositoryName)
	}

	// Получаем workspace по slug
	var workspace dao.Workspace
	if err := s.db.
		Where("slug = ?", workspaceSlug).
		First(&workspace).Error; err != nil {
		return EErrorDefined(c, apierrors.ErrWorkspaceNotFound)
	}

	// Проверяем права пользователя на workspace (должен быть как минимум членом)
	var workspaceMember dao.WorkspaceMember
	if err := s.db.
		Where("workspace_id = ? AND member_id = ?", workspace.ID, user.ID).
		First(&workspaceMember).Error; err != nil {
		return EErrorDefined(c, apierrors.ErrWorkspaceForbidden)
	}

	// Проверяем, что репозиторий с таким именем еще не существует (через ФС!)
	if GitRepositoryExists(workspace.Slug, req.Name, cfg.GitRepositoriesPath) {
		return EErrorDefined(c, apierrors.ErrGitRepositoryExists)
	}

	// Устанавливаем значение по умолчанию для ветки
	branch := req.Branch
	if branch == "" {
		branch = "main"
	}

	// Валидация имени ветки
	if !validateBranchName(branch) {
		return EErrorDefined(c, apierrors.ErrGitInvalidBranch)
	}

	// Создаем путь к репозиторию: {GitRepositoriesPath}/{workspace-slug}/{repo-name}.git
	repoPath := GetRepositoryPath(workspace.Slug, req.Name, cfg.GitRepositoriesPath)

	// Создаем директорию для репозитория
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		slog.Error("Failed to create repository directory", "path", repoPath, "err", err)
		return EErrorDefined(c, apierrors.ErrGitPathCreationFailed)
	}

	// Инициализируем bare репозиторий
	cmd := exec.Command("git", "init", "--bare", "--initial-branch="+branch, repoPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		slog.Error("Failed to init git repository",
			"path", repoPath,
			"err", err,
			"output", string(output))

		// Пытаемся очистить созданную директорию
		os.RemoveAll(repoPath)

		return EErrorDefined(c, apierrors.ErrGitCommandFailed.WithFormattedMessage(string(output)))
	}

	// Парсим UUID пользователя
	userUUID, err := uuid.FromString(user.ID)
	if err != nil {
		slog.Error("Failed to parse user UUID", "user_id", user.ID, "err", err)
		os.RemoveAll(repoPath) // Cleanup
		return EErrorDefined(c, apierrors.ErrGeneric)
	}

	// Создаем структуру Git репозитория
	gitRepo := &GitRepository{
		Name:        req.Name,
		Workspace:   workspace.Slug,
		Private:     req.Private,
		Description: req.Description,
		CreatedAt:   time.Now(),
		CreatedBy:   userUUID,
		Branch:      branch,
		Path:        repoPath,
	}

	// Сохраняем метаданные в файл aiplan.json
	if err := gitRepo.Save(); err != nil {
		slog.Error("Failed to save repository metadata", "err", err)
		os.RemoveAll(repoPath) // Cleanup
		return EErrorDefined(c, apierrors.ErrGitCommandFailed.WithFormattedMessage("Failed to save metadata"))
	}

	// Если указано описание, создаем стандартный файл description
	if req.Description != "" {
		descFile := filepath.Join(repoPath, "description")
		if err := os.WriteFile(descFile, []byte(req.Description), 0644); err != nil {
			slog.Warn("Failed to write description file", "path", descFile, "err", err)
			// Не критичная ошибка, продолжаем
		}
	}

	// Генерируем clone URL
	cloneURL := fmt.Sprintf("git@%s:%s/%s.git", cfg.WebURL.Host, workspace.Slug, req.Name)

	slog.Info("Git repository created",
		"workspace", workspace.Slug,
		"repo", req.Name,
		"path", repoPath,
		"user", user.Email)

	// Загружаем пользователя для ответа
	var creator dao.User
	if err := s.db.Where("id = ?", user.ID).First(&creator).Error; err != nil {
		slog.Warn("Failed to load creator user", "err", err)
	}

	// Формируем ответ
	response := dto.CreateGitRepositoryResponse{
		Workspace:   workspace.Slug,
		Name:        gitRepo.Name,
		Path:        gitRepo.Path,
		Branch:      gitRepo.Branch,
		Private:     gitRepo.Private,
		Description: gitRepo.Description,
		CloneURL:    cloneURL,
		CreatedAt:   gitRepo.CreatedAt,
		CreatedBy:   creator.ToLightDTO(),
	}

	return c.JSON(http.StatusCreated, response)
}

// validateBranchName проверяет корректность имени ветки
func validateBranchName(branch string) bool {
	if len(branch) == 0 || len(branch) > 100 {
		return false
	}

	// Простая проверка: не должно содержать пробелов и специальных символов
	for _, char := range branch {
		if !((char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '-' || char == '_' || char == '/' || char == '.') {
			return false
		}
	}

	// Не должно начинаться с точки или слэша
	if branch[0] == '.' || branch[0] == '/' {
		return false
	}

	return true
}

// @Summary Репозиторий: список Git репозиториев
// @Description Возвращает список всех Git репозиториев в указанном workspace
// @Tags GIT
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug workspace"
// @Success 200 {object} dto.ListGitRepositoriesResponse "Список репозиториев"
// @Failure 400 {object} apierrors.DefinedError "Некорректный запрос"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Git отключен или недостаточно прав"
// @Failure 404 {object} apierrors.DefinedError "Workspace не найден"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/git/{workspaceSlug}/repositories/ [get]
func (s *Services) listGitRepositories(c echo.Context) error {
	// 1. Получение параметра workspace из URL
	workspaceSlug := c.Param("workspaceSlug")
	if workspaceSlug == "" {
		return EErrorDefined(c, apierrors.ErrGeneric)
	}

	// 2. Получение пользователя
	user := c.(AuthContext).User

	// 3. Проверка существования workspace в БД
	var workspace dao.Workspace
	if err := s.db.Where("slug = ?", workspaceSlug).First(&workspace).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return EErrorDefined(c, apierrors.ErrWorkspaceNotFound)
		}
		slog.Error("Failed to load workspace", "slug", workspaceSlug, "err", err)
		return EError(c, err)
	}

	// 4. Проверка прав доступа к workspace (любая роль - достаточно быть участником)
	var member dao.WorkspaceMember
	err := s.db.Where("member_id = ? AND workspace_id = ?", user.ID, workspace.ID).First(&member).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return EErrorDefined(c, apierrors.ErrWorkspaceForbidden)
		}
		slog.Error("Failed to check workspace permissions", "user", user.ID, "workspace", workspace.ID, "err", err)
		return EError(c, err)
	}

	// 5. Получение списка репозиториев из файловой системы
	repos, err := ListGitRepositories(workspace.Slug, cfg.GitRepositoriesPath)
	if err != nil {
		slog.Error("Failed to list git repositories",
			"workspace", workspace.Slug,
			"path", cfg.GitRepositoriesPath,
			"err", err)
		return EError(c, err)
	}

	// 6. Преобразование в DTO
	reposList := make([]dto.GitRepositoryLight, 0, len(repos))
	for _, repo := range repos {
		// Генерация clone URL
		// Используем host из WebURL для формирования SSH clone URL
		host := cfg.WebURL.Host
		if host == "" {
			host = "localhost"
		}

		cloneURL := fmt.Sprintf("git@%s:%s/%s.git", host, workspace.Slug, repo.Name)

		reposList = append(reposList, dto.GitRepositoryLight{
			Name:        repo.Name,
			Workspace:   repo.Workspace,
			Path:        repo.Path,
			Branch:      repo.Branch,
			Private:     repo.Private,
			Description: repo.Description,
			CloneURL:    cloneURL,
			CreatedAt:   repo.CreatedAt,
			CreatedBy:   repo.CreatedBy.String(),
		})
	}

	// 7. Формирование ответа
	response := dto.ListGitRepositoriesResponse{
		Repositories: reposList,
		Total:        len(reposList),
	}

	slog.Info("Listed git repositories",
		"workspace", workspace.Slug,
		"count", len(repos),
		"user", user.Email)

	return c.JSON(http.StatusOK, response)
}

// @Summary Репозиторий: удаление Git репозитория
// @Description Удаляет Git репозиторий из файловой системы. Требуется роль администратора workspace.
// @Tags GIT
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug workspace"
// @Param request body dto.DeleteGitRepositoryRequest true "Параметры удаления репозитория"
// @Success 204 "Репозиторий успешно удален"
// @Failure 400 {object} apierrors.DefinedError "Некорректный запрос"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Git отключен или недостаточно прав (требуется роль администратора)"
// @Failure 404 {object} apierrors.DefinedError "Workspace или репозиторий не найден"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/git/{workspaceSlug}/repositories/ [delete]
func (s *Services) deleteGitRepository(c echo.Context) error {
	user := c.(AuthContext).User

	// Проверяем наличие пути к репозиториям
	if cfg.GitRepositoriesPath == "" {
		slog.Error("GIT_REPOSITORIES_PATH is not configured")
		return EErrorDefined(c, apierrors.ErrGitDisabled)
	}

	// Получаем workspace slug из URL
	workspaceSlug := c.Param("workspaceSlug")
	if workspaceSlug == "" {
		return EErrorDefined(c, apierrors.ErrGeneric)
	}

	// Парсим входные данные
	var req dto.DeleteGitRepositoryRequest
	if err := c.Bind(&req); err != nil {
		slog.Error("Failed to bind request", "err", err)
		return EErrorDefined(c, apierrors.ErrGeneric)
	}

	// Валидация обязательных полей
	if req.Name == "" {
		return EErrorDefined(c, apierrors.ErrGeneric)
	}

	// Валидация имени репозитория (защита от path traversal)
	if !ValidateRepositoryName(req.Name) {
		return EErrorDefined(c, apierrors.ErrGitInvalidRepositoryName)
	}

	// Получаем workspace по slug
	var workspace dao.Workspace
	if err := s.db.
		Where("slug = ?", workspaceSlug).
		First(&workspace).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return EErrorDefined(c, apierrors.ErrWorkspaceNotFound)
		}
		slog.Error("Failed to load workspace", "slug", workspaceSlug, "err", err)
		return EError(c, err)
	}

	// Проверяем права пользователя на workspace
	// Для удаления репозитория требуется роль администратора workspace
	var workspaceMember dao.WorkspaceMember
	if err := s.db.
		Where("workspace_id = ? AND member_id = ?", workspace.ID, user.ID).
		First(&workspaceMember).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return EErrorDefined(c, apierrors.ErrWorkspaceForbidden)
		}
		slog.Error("Failed to check workspace membership", "user", user.ID, "workspace", workspace.ID, "err", err)
		return EError(c, err)
	}

	// Проверяем, что пользователь является администратором workspace
	if workspaceMember.Role != types.AdminRole && !user.IsSuperuser {
		slog.Warn("User attempted to delete repository without admin rights",
			"user", user.Email,
			"workspace", workspace.Slug,
			"repo", req.Name,
			"role", workspaceMember.Role)
		return EErrorDefined(c, apierrors.ErrWorkspaceForbidden)
	}

	// Проверяем существование репозитория в файловой системе
	if !GitRepositoryExists(workspace.Slug, req.Name, cfg.GitRepositoriesPath) {
		return EErrorDefined(c, apierrors.ErrGitRepositoryNotFound)
	}

	// Удаляем репозиторий из файловой системы
	if err := DeleteGitRepository(workspace.Slug, req.Name, cfg.GitRepositoriesPath); err != nil {
		slog.Error("Failed to delete git repository",
			"workspace", workspace.Slug,
			"repo", req.Name,
			"err", err)
		return EErrorDefined(c, apierrors.ErrGitCommandFailed.WithFormattedMessage("Failed to delete repository"))
	}

	slog.Info("Git repository deleted",
		"workspace", workspace.Slug,
		"repo", req.Name,
		"user", user.Email,
		"role", workspaceMember.Role)

	return c.NoContent(http.StatusNoContent)
}

// ========================================
// SSH Keys Management Endpoints
// ========================================

// @Summary SSH Config: получить конфигурацию SSH
// @Description Возвращает конфигурацию SSH сервера (host, port, enabled)
// @Tags GIT-SSH
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} dto.SSHConfigResponse "Конфигурация SSH сервера"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Router /api/auth/git/ssh-config/ [get]
func (s *Services) getGitSSHConfig(c echo.Context) error {
	// Проверяем авторизацию (middleware уже проверил это)
	_ = c.(AuthContext).User

	// Получаем hostname из WebURL
	sshHost := cfg.WebURL.Host
	if sshHost == "" {
		sshHost = "localhost"
	}

	response := dto.SSHConfigResponse{
		SSHEnabled: cfg.SSHEnabled && cfg.GitEnabled,
		SSHHost:    sshHost,
		SSHPort:    cfg.SSHPort,
	}

	return c.JSON(http.StatusOK, response)
}

// @Summary SSH Keys: добавить SSH ключ
// @Description Добавляет новый SSH публичный ключ для текущего пользователя
// @Tags GIT-SSH
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param request body dto.AddSSHKeyRequest true "SSH ключ"
// @Success 201 {object} dto.AddSSHKeyResponse "Добавленный SSH ключ"
// @Failure 400 {object} apierrors.DefinedError "Некорректные данные запроса"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Git или SSH отключены"
// @Failure 409 {object} apierrors.DefinedError "SSH ключ с таким fingerprint уже существует"
// @Router /api/auth/git/ssh-keys/ [post]
func (s *Services) addGitSSHKey(c echo.Context) error {
	user := c.(AuthContext).User

	// Проверяем, что SSH функциональность включена
	if !cfg.SSHEnabled {
		return EErrorDefined(c, apierrors.ErrSSHDisabled)
	}

	// Парсим входные данные
	var req dto.AddSSHKeyRequest
	if err := c.Bind(&req); err != nil {
		slog.Error("Failed to bind AddSSHKeyRequest", "err", err)
		return EErrorDefined(c, apierrors.ErrGeneric)
	}

	// Валидация полей
	if req.Name == "" || req.PublicKey == "" {
		return EErrorDefined(c, apierrors.ErrSSHKeyInvalidData)
	}

	// Добавляем SSH ключ через файловую систему
	keyMetadata, err := AddSSHKey(user.ID, user.Email, req.Name, req.PublicKey, cfg.GitRepositoriesPath)
	if err != nil {
		// Проверяем тип ошибки
		errMsg := err.Error()
		if strings.Contains(errMsg, "invalid SSH key name") {
			return EErrorDefined(c, apierrors.ErrSSHKeyInvalidData)
		}
		if strings.Contains(errMsg, "invalid SSH public key format") {
			return EErrorDefined(c, apierrors.ErrSSHInvalidPublicKey)
		}
		if strings.Contains(errMsg, "already exists") {
			return EErrorDefined(c, apierrors.ErrSSHKeyAlreadyExists)
		}

		slog.Error("Failed to add SSH key", "user", user.Email, "err", err)
		return EError(c, err)
	}

	// Логируем успех
	slog.Info("SSH key added",
		"user", user.Email,
		"key_id", keyMetadata.ID,
		"key_name", keyMetadata.Name,
		"key_type", keyMetadata.KeyType,
		"fingerprint", keyMetadata.Fingerprint)

	// Конвертируем в DTO (без публичного ключа!)
	response := dto.AddSSHKeyResponse{
		SSHKeyDTO: dto.SSHKeyDTO{
			ID:          keyMetadata.ID,
			Name:        keyMetadata.Name,
			KeyType:     keyMetadata.KeyType,
			Fingerprint: keyMetadata.Fingerprint,
			CreatedAt:   keyMetadata.CreatedAt,
			LastUsedAt:  keyMetadata.LastUsedAt,
			Comment:     keyMetadata.Comment,
		},
	}

	return c.JSON(http.StatusCreated, response)
}

// @Summary SSH Keys: список SSH ключей
// @Description Возвращает список всех SSH ключей текущего пользователя
// @Tags GIT-SSH
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} dto.ListSSHKeysResponse "Список SSH ключей"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Git или SSH отключены"
// @Router /api/auth/git/ssh-keys/ [get]
func (s *Services) listGitSSHKeys(c echo.Context) error {
	user := c.(AuthContext).User

	// Проверяем, что SSH функциональность включена
	if !cfg.SSHEnabled {
		return EErrorDefined(c, apierrors.ErrSSHDisabled)
	}

	// Загружаем SSH ключи пользователя
	userKeys, err := LoadUserSSHKeys(user.ID, cfg.GitRepositoriesPath)
	if err != nil {
		// Если файл не найден, возвращаем пустой список
		if os.IsNotExist(err) {
			return c.JSON(http.StatusOK, dto.ListSSHKeysResponse{
				Keys:  []dto.SSHKeyDTO{},
				Total: 0,
			})
		}

		slog.Error("Failed to load user SSH keys", "user", user.Email, "err", err)
		return EError(c, err)
	}

	// Конвертируем в DTO (без публичных ключей!)
	keysDTO := make([]dto.SSHKeyDTO, 0, len(userKeys.Keys))
	for _, key := range userKeys.Keys {
		keysDTO = append(keysDTO, dto.SSHKeyDTO{
			ID:          key.ID,
			Name:        key.Name,
			KeyType:     key.KeyType,
			Fingerprint: key.Fingerprint,
			CreatedAt:   key.CreatedAt,
			LastUsedAt:  key.LastUsedAt,
			Comment:     key.Comment,
		})
	}

	response := dto.ListSSHKeysResponse{
		Keys:  keysDTO,
		Total: len(keysDTO),
	}

	return c.JSON(http.StatusOK, response)
}

// @Summary SSH Keys: удалить SSH ключ
// @Description Удаляет SSH ключ по ID
// @Tags GIT-SSH
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param keyId path string true "ID SSH ключа (UUID)"
// @Success 204 "SSH ключ успешно удален"
// @Failure 400 {object} apierrors.DefinedError "Некорректный запрос"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Git или SSH отключены"
// @Failure 404 {object} apierrors.DefinedError "SSH ключ не найден"
// @Router /api/auth/git/ssh-keys/{keyId} [delete]
func (s *Services) deleteGitSSHKey(c echo.Context) error {
	user := c.(AuthContext).User

	// Проверяем, что SSH функциональность включена
	if !cfg.SSHEnabled {
		return EErrorDefined(c, apierrors.ErrSSHDisabled)
	}

	// Получаем keyId из URL параметра
	keyId := c.Param("keyId")
	if keyId == "" {
		return EErrorDefined(c, apierrors.ErrGeneric)
	}

	// Валидация UUID формата (простая проверка)
	if len(keyId) < 10 {
		return EErrorDefined(c, apierrors.ErrSSHKeyInvalidData)
	}

	// Удаляем SSH ключ
	err := DeleteSSHKey(user.ID, keyId, cfg.GitRepositoriesPath)
	if err != nil {
		// Проверяем тип ошибки
		errMsg := err.Error()
		if strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "has no SSH keys") {
			return EErrorDefined(c, apierrors.ErrSSHKeyNotFound)
		}

		slog.Error("Failed to delete SSH key", "user", user.Email, "key_id", keyId, "err", err)
		return EError(c, err)
	}

	slog.Info("SSH key deleted",
		"user", user.Email,
		"key_id", keyId)

	return c.NoContent(http.StatusNoContent)
}

// ========================================
// Git Browse API Helper Functions
// ========================================

// executeGitCommand выполняет Git команду в bare репозитории
// ВАЖНО: Для bare репозитория используем --git-dir вместо cmd.Dir
func executeGitCommand(repoPath string, args ...string) (string, error) {
	slog.Debug("executeGitCommand called",
		"repoPath", repoPath,
		"args", args)

	// Для bare репозитория добавляем --git-dir в начало аргументов
	gitArgs := append([]string{"--git-dir=" + repoPath}, args...)

	cmd := exec.Command("git", gitArgs...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		slog.Debug("Git command failed",
			"repoPath", repoPath,
			"args", args,
			"err", err,
			"output", string(output))
		return "", fmt.Errorf("git command failed: %w, output: %s", err, string(output))
	}

	result := strings.TrimSpace(string(output))
	slog.Debug("Git command succeeded",
		"repoPath", repoPath,
		"args", args,
		"output_length", len(result))

	return result, nil
}

// getDefaultBranch определяет ветку по умолчанию для репозитория
// Приоритет определения:
// 1. git symbolic-ref HEAD (официальная дефолтная ветка) - с валидацией существования
// 2. master или main (если существуют)
// 3. Первая ветка в алфавитном порядке
// Возвращает пустую строку если репозиторий пустой (нет веток)
func getDefaultBranch(repoPath string) (string, error) {
	slog.Debug("getDefaultBranch called",
		"repoPath", repoPath)

	// Получаем список всех веток ОДИН РАЗ в начале функции
	// Используем for-each-ref для более надежного парсинга
	slog.Debug("Fetching all branches via for-each-ref")
	branchesOutput, err := executeGitCommand(repoPath, "for-each-ref", "refs/heads/", "--format=%(refname:short)")
	if err != nil || branchesOutput == "" {
		// Репозиторий пустой (нет веток)
		slog.Debug("Repository has no branches",
			"repo", repoPath,
			"err", err,
			"output", branchesOutput)
		return "", fmt.Errorf("repository is empty (no branches)")
	}

	// Парсим список веток
	branchNames := strings.Split(strings.TrimSpace(branchesOutput), "\n")
	if len(branchNames) == 0 || (len(branchNames) == 1 && branchNames[0] == "") {
		slog.Debug("No valid branches found after parsing",
			"repo", repoPath)
		return "", fmt.Errorf("repository is empty (no branches)")
	}

	slog.Debug("Total branches found",
		"repo", repoPath,
		"count", len(branchNames),
		"branches", branchNames)

	// Приоритет 1: Пытаемся получить HEAD reference через symbolic-ref
	slog.Debug("Trying method 1: git symbolic-ref HEAD")
	output, err := executeGitCommand(repoPath, "symbolic-ref", "HEAD")
	if err == nil && output != "" {
		// Парсим refs/heads/main -> main
		ref := strings.TrimSpace(output)
		branchName := strings.TrimPrefix(ref, "refs/heads/")

		// ВАЛИДАЦИЯ: проверяем что ветка действительно существует
		if branchExists(branchName, branchNames) {
			slog.Debug("Default branch determined via symbolic-ref",
				"repo", repoPath,
				"branch", branchName,
				"method", "symbolic-ref",
				"raw_output", output,
				"validated", true)

			return branchName, nil
		} else {
			// symbolic-ref указывает на несуществующую ветку - это проблема
			slog.Warn("symbolic-ref points to non-existent branch",
				"repo", repoPath,
				"branch", branchName,
				"symbolic_ref", output,
				"existing_branches", branchNames)
			// Продолжаем к методу 2 (fallback)
		}
	} else {
		// symbolic-ref не сработал - логируем на уровне DEBUG (это нормально для bare репозиториев)
		slog.Debug("Method 1 failed, falling back to branch detection",
			"repo", repoPath,
			"err", err)
	}

	// Приоритет 2: Проверяем наличие master
	slog.Debug("Checking for 'master' branch")
	for _, branch := range branchNames {
		if branch == "master" {
			slog.Debug("Default branch determined",
				"repo", repoPath,
				"branch", "master",
				"method", "fallback-master")
			return "master", nil
		}
	}

	// Приоритет 2: Проверяем наличие main
	slog.Debug("Checking for 'main' branch")
	for _, branch := range branchNames {
		if branch == "main" {
			slog.Debug("Default branch determined",
				"repo", repoPath,
				"branch", "main",
				"method", "fallback-main")
			return "main", nil
		}
	}

	// Приоритет 3: Берем первую ветку в алфавитном порядке
	slog.Debug("Using alphabetical order (no master/main found)")
	firstBranch := branchNames[0]
	for _, branch := range branchNames {
		if branch < firstBranch {
			firstBranch = branch
		}
	}

	slog.Debug("Default branch determined",
		"repo", repoPath,
		"branch", firstBranch,
		"method", "alphabetical",
		"total_branches", len(branchNames))

	return firstBranch, nil
}

// branchExists проверяет существование ветки в списке
func branchExists(branch string, branches []string) bool {
	for _, b := range branches {
		if b == branch {
			return true
		}
	}
	return false
}

// parseLsTree парсит вывод команды git ls-tree
func parseLsTree(output string) ([]TreeEntryDTO, error) {
	if output == "" {
		return []TreeEntryDTO{}, nil
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	entries := make([]TreeEntryDTO, 0, len(lines))

	for _, line := range lines {
		if line == "" {
			continue
		}

		// Формат: <mode> SP <type> SP <object> TAB <file>
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}

		metaParts := strings.Fields(parts[0])
		if len(metaParts) != 3 {
			continue
		}

		mode := metaParts[0]
		objType := metaParts[1]
		sha := metaParts[2]
		name := parts[1]

		entryType := "file"
		if objType == "tree" {
			entryType = "dir"
		}

		entries = append(entries, TreeEntryDTO{
			Name: name,
			Type: entryType,
			Mode: mode,
			SHA:  sha,
		})
	}

	return entries, nil
}

// parseCommitLog парсит вывод команды git log --pretty=format:...
func parseCommitLog(output string) ([]CommitDTO, error) {
	if output == "" {
		return []CommitDTO{}, nil
	}

	commits := []CommitDTO{}
	lines := strings.Split(strings.TrimSpace(output), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}

		// Формат: sha|author_name|author_email|author_date|committer_name|committer_email|committer_date|parents|message
		parts := strings.SplitN(line, "|", 9)
		if len(parts) < 9 {
			continue
		}

		sha := parts[0]
		authorName := parts[1]
		authorEmail := parts[2]
		authorDate := parts[3]
		committerName := parts[4]
		committerEmail := parts[5]
		committerDate := parts[6]
		parents := parts[7]
		message := parts[8]

		// Парсим даты
		authorTime, _ := time.Parse(time.RFC3339, authorDate)
		committerTime, _ := time.Parse(time.RFC3339, committerDate)

		// Парсим родительские коммиты
		parentSHAs := []string{}
		if parents != "" {
			parentSHAs = strings.Split(parents, " ")
		}

		commit := CommitDTO{
			SHA: sha,
			Author: PersonDTO{
				Name:  authorName,
				Email: authorEmail,
				Date:  authorTime,
			},
			Committer: PersonDTO{
				Name:  committerName,
				Email: committerEmail,
				Date:  committerTime,
			},
			Message:    message,
			ParentSHAs: parentSHAs,
		}

		commits = append(commits, commit)
	}

	return commits, nil
}

// isBinaryFile проверяет является ли файл бинарным
func isBinaryFile(content []byte) bool {
	// Простая эвристика: если в первых 8KB есть нулевой байт - это бинарный файл
	sampleSize := 8192
	if len(content) < sampleSize {
		sampleSize = len(content)
	}

	for i := 0; i < sampleSize; i++ {
		if content[i] == 0 {
			return true
		}
	}

	return false
}

// checkRepositoryAccess проверяет права доступа пользователя к репозиторию
func (s *Services) checkRepositoryAccess(user *dao.User, workspaceSlug, repoName string) (bool, error) {
	// Superuser имеет доступ ко всему
	if user.IsSuperuser {
		return true, nil
	}

	// Загружаем метаданные репозитория
	repoPath := GetRepositoryPath(workspaceSlug, repoName, cfg.GitRepositoriesPath)
	repo, err := LoadGitRepository(repoPath)
	if err != nil {
		return false, fmt.Errorf("failed to load repository metadata: %w", err)
	}

	// Публичный репозиторий - все могут читать
	if !repo.Private {
		return true, nil
	}

	// Приватный репозиторий - проверяем membership в workspace
	var workspace dao.Workspace
	if err := s.db.Where("slug = ?", workspaceSlug).First(&workspace).Error; err != nil {
		return false, fmt.Errorf("workspace not found: %w", err)
	}

	var membership dao.WorkspaceMember
	err = s.db.Where("workspace_id = ? AND member_id = ?", workspace.ID, user.ID).First(&membership).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return false, nil // Не является членом workspace
		}
		return false, fmt.Errorf("failed to check workspace membership: %w", err)
	}

	return true, nil
}

// ========================================
// Git Browse API Endpoints
// ========================================

// @Summary Browse: просмотр дерева файлов
// @Description Возвращает список файлов и директорий в указанном пути репозитория
// @Tags GIT-BROWSE
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug workspace"
// @Param repoName path string true "Имя репозитория"
// @Param ref query string false "Ветка/тег/коммит (по умолчанию: main/master)"
// @Param path query string false "Путь в репозитории (по умолчанию: корень)"
// @Success 200 {object} TreeResponseDTO "Дерево файлов"
// @Failure 400 {object} apierrors.DefinedError "Некорректный запрос"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Репозиторий или путь не найден"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/git/{workspaceSlug}/repositories/{repoName}/tree [get]
func (s *Services) getRepositoryTree(c echo.Context) error {
	user := c.(AuthContext).User
	workspaceSlug := c.Param("workspaceSlug")
	repoName := c.Param("repoName")
	ref := c.QueryParam("ref")
	path := c.QueryParam("path")

	// Валидация параметров
	if workspaceSlug == "" || repoName == "" {
		return EErrorDefined(c, apierrors.ErrGeneric)
	}

	// Проверка прав доступа
	hasAccess, err := s.checkRepositoryAccess(user, workspaceSlug, repoName)
	if err != nil {
		slog.Error("Failed to check repository access", "err", err)
		return EError(c, err)
	}
	if !hasAccess {
		return EErrorDefined(c, apierrors.ErrWorkspaceForbidden)
	}

	// Получаем путь к репозиторию
	repoPath := GetRepositoryPath(workspaceSlug, repoName, cfg.GitRepositoriesPath)

	// Проверяем существование репозитория
	if !GitRepositoryExists(workspaceSlug, repoName, cfg.GitRepositoriesPath) {
		return EErrorDefined(c, apierrors.ErrGitRepositoryNotFound)
	}

	// Получаем ветку по умолчанию, если ref не указан
	if ref == "" {
		defaultBranch, err := getDefaultBranch(repoPath)
		if err != nil {
			slog.Error("Failed to get default branch",
				"repo", fmt.Sprintf("%s/%s", workspaceSlug, repoName),
				"err", err)
			return EErrorDefined(c, apierrors.ErrGitCommandFailed.WithFormattedMessage("Repository is empty or has no branches"))
		}
		ref = defaultBranch
	}

	// Нормализуем path
	if path == "" || path == "/" {
		path = ""
	} else {
		// Убираем начальный слэш
		path = strings.TrimPrefix(path, "/")
	}

	// Выполняем git ls-tree
	gitPath := ref
	if path != "" {
		gitPath = ref + ":" + path
	}

	output, err := executeGitCommand(repoPath, "ls-tree", gitPath)
	if err != nil {
		errMsg := err.Error()
		slog.Error("Failed to execute git ls-tree",
			"repo", fmt.Sprintf("%s/%s", workspaceSlug, repoName),
			"ref", ref,
			"path", path,
			"err", err)

		// Более детальные ошибки
		if strings.Contains(errMsg, "Not a valid object name") {
			return EErrorDefined(c, apierrors.ErrGitCommandFailed.WithFormattedMessage("Branch or path not found"))
		}
		if strings.Contains(errMsg, "does not exist") {
			return EErrorDefined(c, apierrors.ErrGitCommandFailed.WithFormattedMessage("Path not found"))
		}

		return EErrorDefined(c, apierrors.ErrGitCommandFailed.WithFormattedMessage("Failed to read repository tree"))
	}

	// Парсим результат
	entries, err := parseLsTree(output)
	if err != nil {
		slog.Error("Failed to parse ls-tree output", "err", err)
		return EError(c, err)
	}

	// Для файлов получаем размер
	for i := range entries {
		if entries[i].Type == "file" {
			sizeOutput, err := executeGitCommand(repoPath, "cat-file", "-s", entries[i].SHA)
			if err == nil {
				size, _ := strconv.ParseInt(sizeOutput, 10, 64)
				entries[i].Size = size
			}
		}
	}

	displayPath := "/" + path
	if path == "" {
		displayPath = "/"
	}

	response := TreeResponseDTO{
		Ref:     ref,
		Path:    displayPath,
		Entries: entries,
	}

	slog.Info("Git tree browsed",
		"workspace", workspaceSlug,
		"repo", repoName,
		"ref", ref,
		"path", path,
		"entries_count", len(entries),
		"user", user.Email)

	return c.JSON(http.StatusOK, response)
}

// @Summary Browse: получение содержимого файла
// @Description Возвращает содержимое файла из репозитория (base64 encoded)
// @Tags GIT-BROWSE
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug workspace"
// @Param repoName path string true "Имя репозитория"
// @Param ref query string false "Ветка/тег/коммит (по умолчанию: main/master)"
// @Param path query string true "Путь к файлу"
// @Success 200 {object} BlobResponseDTO "Содержимое файла"
// @Failure 400 {object} apierrors.DefinedError "Некорректный запрос"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Файл не найден"
// @Failure 413 {object} apierrors.DefinedError "Файл слишком большой"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/git/{workspaceSlug}/repositories/{repoName}/blob [get]
func (s *Services) getRepositoryBlob(c echo.Context) error {
	user := c.(AuthContext).User
	workspaceSlug := c.Param("workspaceSlug")
	repoName := c.Param("repoName")
	ref := c.QueryParam("ref")
	path := c.QueryParam("path")

	// Валидация параметров
	if workspaceSlug == "" || repoName == "" || path == "" {
		return EErrorDefined(c, apierrors.ErrGeneric)
	}

	// Проверка прав доступа
	hasAccess, err := s.checkRepositoryAccess(user, workspaceSlug, repoName)
	if err != nil {
		slog.Error("Failed to check repository access", "err", err)
		return EError(c, err)
	}
	if !hasAccess {
		return EErrorDefined(c, apierrors.ErrWorkspaceForbidden)
	}

	// Получаем путь к репозиторию
	repoPath := GetRepositoryPath(workspaceSlug, repoName, cfg.GitRepositoriesPath)

	// Проверяем существование репозитория
	if !GitRepositoryExists(workspaceSlug, repoName, cfg.GitRepositoriesPath) {
		return EErrorDefined(c, apierrors.ErrGitRepositoryNotFound)
	}

	// Получаем ветку по умолчанию, если ref не указан
	if ref == "" {
		defaultBranch, err := getDefaultBranch(repoPath)
		if err != nil {
			slog.Error("Failed to get default branch",
				"repo", fmt.Sprintf("%s/%s", workspaceSlug, repoName),
				"err", err)
			return EErrorDefined(c, apierrors.ErrGitCommandFailed.WithFormattedMessage("Repository is empty or has no branches"))
		}
		ref = defaultBranch
	}

	// Нормализуем path
	path = strings.TrimPrefix(path, "/")

	// Получаем размер файла
	sizeOutput, err := executeGitCommand(repoPath, "cat-file", "-s", ref+":"+path)
	if err != nil {
		errMsg := err.Error()
		slog.Error("Failed to get file size",
			"repo", fmt.Sprintf("%s/%s", workspaceSlug, repoName),
			"ref", ref,
			"path", path,
			"err", err)

		// Более детальные ошибки
		if strings.Contains(errMsg, "Not a valid object name") {
			return EErrorDefined(c, apierrors.ErrGitCommandFailed.WithFormattedMessage("Branch or file not found"))
		}
		if strings.Contains(errMsg, "does not exist") || strings.Contains(errMsg, "Path") {
			return EErrorDefined(c, apierrors.ErrGitCommandFailed.WithFormattedMessage("File not found"))
		}

		return EErrorDefined(c, apierrors.ErrGitCommandFailed.WithFormattedMessage("Failed to read file"))
	}

	size, _ := strconv.ParseInt(sizeOutput, 10, 64)

	// Ограничение размера файла (10 MB)
	const maxFileSize = 10 * 1024 * 1024
	if size > maxFileSize {
		return EErrorDefined(c, apierrors.ErrGeneric.WithFormattedMessage("File too large (max 10MB)"))
	}

	// Получаем SHA файла
	shaOutput, err := executeGitCommand(repoPath, "rev-parse", ref+":"+path)
	if err != nil {
		slog.Error("Failed to get file SHA", "err", err)
		return EError(c, err)
	}
	sha := strings.TrimSpace(shaOutput)

	// Получаем содержимое файла
	contentOutput, err := executeGitCommand(repoPath, "show", ref+":"+path)
	if err != nil {
		slog.Error("Failed to get file content", "err", err)
		return EError(c, err)
	}

	contentBytes := []byte(contentOutput)

	// Проверяем, является ли файл бинарным
	isBinary := isBinaryFile(contentBytes)

	// Кодируем содержимое в base64
	content := base64.StdEncoding.EncodeToString(contentBytes)

	response := BlobResponseDTO{
		Path:     "/" + path,
		Ref:      ref,
		Size:     size,
		SHA:      sha,
		Content:  content,
		Encoding: "base64",
		IsBinary: isBinary,
	}

	slog.Info("Git file blob retrieved",
		"workspace", workspaceSlug,
		"repo", repoName,
		"ref", ref,
		"path", path,
		"size", size,
		"is_binary", isBinary,
		"user", user.Email)

	return c.JSON(http.StatusOK, response)
}

// @Summary Browse: история коммитов
// @Description Возвращает список коммитов в указанной ветке
// @Tags GIT-BROWSE
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug workspace"
// @Param repoName path string true "Имя репозитория"
// @Param ref query string false "Ветка/тег (по умолчанию: main/master)"
// @Param limit query int false "Лимит коммитов (по умолчанию: 50, макс: 100)"
// @Param offset query int false "Смещение (по умолчанию: 0)"
// @Success 200 {object} CommitsResponseDTO "История коммитов"
// @Failure 400 {object} apierrors.DefinedError "Некорректный запрос"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Репозиторий не найден"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/git/{workspaceSlug}/repositories/{repoName}/commits [get]
func (s *Services) getRepositoryCommits(c echo.Context) error {
	user := c.(AuthContext).User
	workspaceSlug := c.Param("workspaceSlug")
	repoName := c.Param("repoName")
	ref := c.QueryParam("ref")
	limitStr := c.QueryParam("limit")
	offsetStr := c.QueryParam("offset")

	// Валидация параметров
	if workspaceSlug == "" || repoName == "" {
		return EErrorDefined(c, apierrors.ErrGeneric)
	}

	// Парсим limit и offset
	limit := 50
	if limitStr != "" {
		parsedLimit, err := strconv.Atoi(limitStr)
		if err == nil && parsedLimit > 0 {
			limit = parsedLimit
			if limit > 100 {
				limit = 100
			}
		}
	}

	offset := 0
	if offsetStr != "" {
		parsedOffset, err := strconv.Atoi(offsetStr)
		if err == nil && parsedOffset >= 0 {
			offset = parsedOffset
		}
	}

	// Проверка прав доступа
	hasAccess, err := s.checkRepositoryAccess(user, workspaceSlug, repoName)
	if err != nil {
		slog.Error("Failed to check repository access", "err", err)
		return EError(c, err)
	}
	if !hasAccess {
		return EErrorDefined(c, apierrors.ErrWorkspaceForbidden)
	}

	// Получаем путь к репозиторию
	repoPath := GetRepositoryPath(workspaceSlug, repoName, cfg.GitRepositoriesPath)

	// Проверяем существование репозитория
	if !GitRepositoryExists(workspaceSlug, repoName, cfg.GitRepositoriesPath) {
		return EErrorDefined(c, apierrors.ErrGitRepositoryNotFound)
	}

	// Получаем ветку по умолчанию, если ref не указан
	if ref == "" {
		defaultBranch, err := getDefaultBranch(repoPath)
		if err != nil {
			slog.Error("Failed to get default branch",
				"repo", fmt.Sprintf("%s/%s", workspaceSlug, repoName),
				"err", err)
			return EErrorDefined(c, apierrors.ErrGitCommandFailed.WithFormattedMessage("Repository is empty or has no branches"))
		}
		ref = defaultBranch
	}

	// Получаем общее количество коммитов
	countOutput, err := executeGitCommand(repoPath, "rev-list", "--count", ref)
	if err != nil {
		slog.Error("Failed to count commits",
			"repo", fmt.Sprintf("%s/%s", workspaceSlug, repoName),
			"ref", ref,
			"err", err)
		return EErrorDefined(c, apierrors.ErrGitCommandFailed.WithFormattedMessage("Branch not found"))
	}
	total, _ := strconv.Atoi(strings.TrimSpace(countOutput))

	// Формат вывода для git log
	format := "%H|%an|%ae|%aI|%cn|%ce|%cI|%P|%s"

	// Выполняем git log с pagination
	args := []string{"log", "--pretty=format:" + format, fmt.Sprintf("--skip=%d", offset), fmt.Sprintf("--max-count=%d", limit), ref}
	output, err := executeGitCommand(repoPath, args...)
	if err != nil {
		slog.Error("Failed to execute git log", "err", err)
		return EError(c, err)
	}

	// Парсим коммиты
	commits, err := parseCommitLog(output)
	if err != nil {
		slog.Error("Failed to parse commit log", "err", err)
		return EError(c, err)
	}

	response := CommitsResponseDTO{
		Commits: commits,
		Total:   total,
		Limit:   limit,
		Offset:  offset,
	}

	slog.Info("Git commits retrieved",
		"workspace", workspaceSlug,
		"repo", repoName,
		"ref", ref,
		"total", total,
		"limit", limit,
		"offset", offset,
		"user", user.Email)

	return c.JSON(http.StatusOK, response)
}

// @Summary Browse: список веток
// @Description Возвращает список всех веток в репозитории
// @Tags GIT-BROWSE
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug workspace"
// @Param repoName path string true "Имя репозитория"
// @Success 200 {object} BranchesResponseDTO "Список веток"
// @Failure 400 {object} apierrors.DefinedError "Некорректный запрос"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Репозиторий не найден"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/git/{workspaceSlug}/repositories/{repoName}/branches [get]
func (s *Services) getRepositoryBranches(c echo.Context) error {
	user := c.(AuthContext).User
	workspaceSlug := c.Param("workspaceSlug")
	repoName := c.Param("repoName")

	// Валидация параметров
	if workspaceSlug == "" || repoName == "" {
		return EErrorDefined(c, apierrors.ErrGeneric)
	}

	// Проверка прав доступа
	hasAccess, err := s.checkRepositoryAccess(user, workspaceSlug, repoName)
	if err != nil {
		slog.Error("Failed to check repository access", "err", err)
		return EError(c, err)
	}
	if !hasAccess {
		return EErrorDefined(c, apierrors.ErrWorkspaceForbidden)
	}

	// Получаем путь к репозиторию
	repoPath := GetRepositoryPath(workspaceSlug, repoName, cfg.GitRepositoriesPath)

	// Проверяем существование репозитория
	if !GitRepositoryExists(workspaceSlug, repoName, cfg.GitRepositoriesPath) {
		return EErrorDefined(c, apierrors.ErrGitRepositoryNotFound)
	}

	// Получаем ветку по умолчанию
	defaultBranch, err := getDefaultBranch(repoPath)
	if err != nil {
		slog.Warn("Failed to get default branch (repository may be empty)",
			"repo", fmt.Sprintf("%s/%s", workspaceSlug, repoName),
			"err", err)
		defaultBranch = "" // Пустая строка для пустого репозитория
	}

	// Получаем список веток с их SHA
	// Формат: <sha> refs/heads/<branch>
	output, err := executeGitCommand(repoPath, "show-ref", "--heads")
	if err != nil {
		slog.Warn("Failed to execute git show-ref",
			"repo", fmt.Sprintf("%s/%s", workspaceSlug, repoName),
			"err", err)
		// Репозиторий может быть пустым (нет веток)
		return c.JSON(http.StatusOK, BranchesResponseDTO{
			Branches: []BranchDTO{},
		})
	}

	// Парсим вывод
	branches := []BranchDTO{}
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) != 2 {
			continue
		}

		sha := parts[0]
		ref := parts[1] // refs/heads/main

		// Извлекаем имя ветки
		branchName := strings.TrimPrefix(ref, "refs/heads/")

		branch := BranchDTO{
			Name:      branchName,
			SHA:       sha,
			IsDefault: (branchName == defaultBranch),
		}

		branches = append(branches, branch)
	}

	response := BranchesResponseDTO{
		Branches: branches,
	}

	slog.Info("Git branches retrieved",
		"workspace", workspaceSlug,
		"repo", repoName,
		"branches_count", len(branches),
		"user", user.Email)

	return c.JSON(http.StatusOK, response)
}

// @Summary Browse: информация о репозитории
// @Description Возвращает метаданные репозитория (размер, количество веток, последний коммит)
// @Tags GIT-BROWSE
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workspaceSlug path string true "Slug workspace"
// @Param repoName path string true "Имя репозитория"
// @Success 200 {object} RepoInfoDTO "Информация о репозитории"
// @Failure 400 {object} apierrors.DefinedError "Некорректный запрос"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Доступ запрещен"
// @Failure 404 {object} apierrors.DefinedError "Репозиторий не найден"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/git/{workspaceSlug}/repositories/{repoName}/info [get]
func (s *Services) getRepositoryInfo(c echo.Context) error {
	user := c.(AuthContext).User
	workspaceSlug := c.Param("workspaceSlug")
	repoName := c.Param("repoName")

	// Валидация параметров
	if workspaceSlug == "" || repoName == "" {
		return EErrorDefined(c, apierrors.ErrGeneric)
	}

	// Проверка прав доступа
	hasAccess, err := s.checkRepositoryAccess(user, workspaceSlug, repoName)
	if err != nil {
		slog.Error("Failed to check repository access", "err", err)
		return EError(c, err)
	}
	if !hasAccess {
		return EErrorDefined(c, apierrors.ErrWorkspaceForbidden)
	}

	// Получаем путь к репозиторию
	repoPath := GetRepositoryPath(workspaceSlug, repoName, cfg.GitRepositoriesPath)

	// Проверяем существование репозитория
	if !GitRepositoryExists(workspaceSlug, repoName, cfg.GitRepositoriesPath) {
		return EErrorDefined(c, apierrors.ErrGitRepositoryNotFound)
	}

	// Получаем ветку по умолчанию
	slog.Debug("getRepositoryInfo: calling getDefaultBranch",
		"repoPath", repoPath,
		"workspace", workspaceSlug,
		"repo", repoName)

	defaultBranch, err := getDefaultBranch(repoPath)
	if err != nil {
		slog.Warn("Failed to get default branch (repository may be empty)",
			"repo", fmt.Sprintf("%s/%s", workspaceSlug, repoName),
			"repoPath", repoPath,
			"err", err)
		defaultBranch = "" // Пустая строка для пустого репозитория
	}

	slog.Debug("getRepositoryInfo: got default branch",
		"defaultBranch", defaultBranch,
		"is_empty", defaultBranch == "",
		"workspace", workspaceSlug,
		"repo", repoName)

	// Получаем количество веток
	branchesOutput, err := executeGitCommand(repoPath, "branch", "-a")
	branchesCount := 0
	if err == nil {
		lines := strings.Split(strings.TrimSpace(branchesOutput), "\n")
		branchesCount = len(lines)
	}

	// Получаем количество коммитов
	commitsCount := 0
	countOutput, err := executeGitCommand(repoPath, "rev-list", "--count", "--all")
	if err == nil {
		commitsCount, _ = strconv.Atoi(strings.TrimSpace(countOutput))
	}

	// Получаем размер репозитория (в байтах)
	var repoSize int64
	err = filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			repoSize += info.Size()
		}
		return nil
	})
	if err != nil {
		slog.Warn("Failed to calculate repository size", "err", err)
	}

	// Получаем последний коммит
	var lastCommit *CommitDTO
	format := "%H|%an|%ae|%aI|%cn|%ce|%cI|%P|%s"
	lastCommitOutput, err := executeGitCommand(repoPath, "log", "--pretty=format:"+format, "-1", defaultBranch)
	if err == nil && lastCommitOutput != "" {
		commits, err := parseCommitLog(lastCommitOutput)
		if err == nil && len(commits) > 0 {
			lastCommit = &commits[0]
		}
	}

	response := RepoInfoDTO{
		Name:          repoName,
		Workspace:     workspaceSlug,
		DefaultBranch: defaultBranch,
		BranchesCount: branchesCount,
		CommitsCount:  commitsCount,
		Size:          repoSize,
		LastCommit:    lastCommit,
	}

	slog.Info("Git repository info retrieved",
		"workspace", workspaceSlug,
		"repo", repoName,
		"default_branch", defaultBranch,
		"branches_count", branchesCount,
		"commits_count", commitsCount,
		"size", repoSize,
		"has_last_commit", lastCommit != nil,
		"user", user.Email)

	slog.Debug("getRepositoryInfo: response prepared",
		"response", response)

	return c.JSON(http.StatusOK, response)
}

// requireGitEnabled middleware проверяет, что Git функциональность включена
// Возвращает 503 Service Unavailable если Git отключен
func (s *Services) requireGitEnabled(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if !cfg.GitEnabled {
			return c.JSON(http.StatusServiceUnavailable, map[string]string{
				"error": "Git functionality is disabled",
			})
		}
		return next(c)
	}
}

// AddGitServices регистрирует все эндпоинты для работы с GIT
func (s *Services) AddGitServices(g *echo.Group) {
	// Config endpoints (no path params) - ВСЕГДА доступны для проверки статуса
	g.GET("git/config/", s.getGitConfig)
	g.GET("git/ssh-config/", s.getGitSSHConfig)

	// Создаем подгруппу с middleware для проверки GIT_ENABLED
	gitEnabledGroup := g.Group("git/", s.requireGitEnabled)

	// SSH Keys endpoints (no workspace/repo params)
	gitEnabledGroup.POST("ssh-keys/", s.addGitSSHKey)
	gitEnabledGroup.GET("ssh-keys/", s.listGitSSHKeys)
	gitEnabledGroup.DELETE("ssh-keys/:keyId/", s.deleteGitSSHKey)

	// Git Browse API endpoints (specific routes with :repoName - MUST be before general routes)
	gitEnabledGroup.GET(":workspaceSlug/repositories/:repoName/tree/", s.getRepositoryTree)
	gitEnabledGroup.GET(":workspaceSlug/repositories/:repoName/blob/", s.getRepositoryBlob)
	gitEnabledGroup.GET(":workspaceSlug/repositories/:repoName/commits/", s.getRepositoryCommits)
	gitEnabledGroup.GET(":workspaceSlug/repositories/:repoName/branches/", s.getRepositoryBranches)
	gitEnabledGroup.GET(":workspaceSlug/repositories/:repoName/info/", s.getRepositoryInfo)

	// Repository CRUD endpoints (general routes - MUST be after specific routes)
	gitEnabledGroup.GET(":workspaceSlug/repositories/", s.listGitRepositories)
	gitEnabledGroup.POST(":workspaceSlug/repositories/", s.createGitRepository)
	gitEnabledGroup.DELETE(":workspaceSlug/repositories/", s.deleteGitRepository)
}
