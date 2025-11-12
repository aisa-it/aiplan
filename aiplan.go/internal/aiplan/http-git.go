// Пакет aiplan предоставляет функциональность для управления GIT.
//
// Архитектурный принцип: Git репозитории НЕ хранятся в базе данных.
// Вся информация находится в файловой системе в файлах aiplan.json.
// База данных используется только для workspace, users и audit log (activity).
package aiplan

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
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
	// 1. Проверка конфигурации Git
	if !cfg.GitEnabled {
		return EErrorDefined(c, apierrors.ErrGitDisabled)
	}

	// 2. Получение параметра workspace из URL
	workspaceSlug := c.Param("workspaceSlug")
	if workspaceSlug == "" {
		return EErrorDefined(c, apierrors.ErrGeneric)
	}

	// 3. Получение пользователя
	user := c.(AuthContext).User

	// 4. Проверка существования workspace в БД
	var workspace dao.Workspace
	if err := s.db.Where("slug = ?", workspaceSlug).First(&workspace).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return EErrorDefined(c, apierrors.ErrWorkspaceNotFound)
		}
		slog.Error("Failed to load workspace", "slug", workspaceSlug, "err", err)
		return EError(c, err)
	}

	// 5. Проверка прав доступа к workspace (любая роль - достаточно быть участником)
	var member dao.WorkspaceMember
	err := s.db.Where("member_id = ? AND workspace_id = ?", user.ID, workspace.ID).First(&member).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return EErrorDefined(c, apierrors.ErrWorkspaceForbidden)
		}
		slog.Error("Failed to check workspace permissions", "user", user.ID, "workspace", workspace.ID, "err", err)
		return EError(c, err)
	}

	// 6. Получение списка репозиториев из файловой системы
	repos, err := ListGitRepositories(workspace.Slug, cfg.GitRepositoriesPath)
	if err != nil {
		slog.Error("Failed to list git repositories",
			"workspace", workspace.Slug,
			"path", cfg.GitRepositoriesPath,
			"err", err)
		return EError(c, err)
	}

	// 7. Преобразование в DTO
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

	// 8. Формирование ответа
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

	// Проверяем, что Git функциональность включена
	if !cfg.GitEnabled {
		return EErrorDefined(c, apierrors.ErrGitDisabled)
	}

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

	// Проверяем, что Git функциональность включена
	if !cfg.GitEnabled {
		return EErrorDefined(c, apierrors.ErrGitDisabled)
	}

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

	// Проверяем, что Git функциональность включена
	if !cfg.GitEnabled {
		return EErrorDefined(c, apierrors.ErrGitDisabled)
	}

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

// AddGitServices регистрирует все эндпоинты для работы с GIT
func (s *Services) AddGitServices(g *echo.Group) {
	// Existing Git endpoints
	g.GET("git/config/", s.getGitConfig)
	g.GET("git/:workspaceSlug/repositories/", s.listGitRepositories)
	g.POST("git/:workspaceSlug/repositories/", s.createGitRepository)
	g.DELETE("git/:workspaceSlug/repositories/", s.deleteGitRepository)

	// NEW SSH endpoints
	g.GET("git/ssh-config/", s.getGitSSHConfig)
	g.POST("git/ssh-keys/", s.addGitSSHKey)
	g.GET("git/ssh-keys/", s.listGitSSHKeys)
	g.DELETE("git/ssh-keys/:keyId", s.deleteGitSSHKey)
}
