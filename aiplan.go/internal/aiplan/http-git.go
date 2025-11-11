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
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/apierrors"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dto"
	"github.com/gofrs/uuid"
	"github.com/labstack/echo/v4"
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
// @Param request body dto.CreateGitRepositoryRequest true "Параметры создания репозитория"
// @Success 201 {object} dto.CreateGitRepositoryResponse "Созданный репозиторий"
// @Failure 400 {object} apierrors.DefinedError "Некорректный запрос"
// @Failure 401 {object} apierrors.DefinedError "Необходима авторизация"
// @Failure 403 {object} apierrors.DefinedError "Git отключен или недостаточно прав"
// @Failure 409 {object} apierrors.DefinedError "Репозиторий с таким именем уже существует"
// @Failure 500 {object} apierrors.DefinedError "Ошибка сервера"
// @Router /api/auth/git/repositories/ [post]
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

	// Парсим входные данные
	var req dto.CreateGitRepositoryRequest
	if err := c.Bind(&req); err != nil {
		slog.Error("Failed to bind request", "err", err)
		return EErrorDefined(c, apierrors.ErrGeneric)
	}

	// Валидация обязательных полей
	if req.Workspace == "" || req.Name == "" {
		return EErrorDefined(c, apierrors.ErrGeneric)
	}

	// Валидация имени репозитория
	if !ValidateRepositoryName(req.Name) {
		return EErrorDefined(c, apierrors.ErrGitInvalidRepositoryName)
	}

	// Получаем workspace по slug
	var workspace dao.Workspace
	if err := s.db.
		Where("slug = ?", req.Workspace).
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

// AddGitServices регистрирует все эндпоинты для работы с GIT
func (s *Services) AddGitServices(g *echo.Group) {
	g.GET("git/config/", s.getGitConfig)
	g.POST("git/repositories/", s.createGitRepository)
}
