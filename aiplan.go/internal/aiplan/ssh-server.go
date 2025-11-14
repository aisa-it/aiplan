// Пакет aiplan предоставляет функциональность SSH сервера для Git операций.
//
// SSH сервер позволяет пользователям выполнять Git операции (clone, push, pull)
// через SSH протокол, используя аутентификацию по публичным ключам.
//
// Архитектурные принципы:
// - SSH ключи хранятся в файловой системе (не в БД)
// - Аутентификация через authorized_keys файл
// - Права доступа основаны на workspace membership
// - Публичные репозитории: read для всех, write для членов
// - Приватные репозитории: read/write только для членов
package aiplan

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/gliderlabs/ssh"
	gossh "golang.org/x/crypto/ssh"
	"gorm.io/gorm"
)

// SSHServer представляет SSH сервер для Git операций
type SSHServer struct {
	// db - подключение к базе данных для проверки пользователей и прав
	db *gorm.DB

	// gitReposPath - путь к директории Git репозиториев
	gitReposPath string

	// host - адрес для прослушивания (например, "0.0.0.0")
	host string

	// port - порт для прослушивания (например, 22222)
	port int

	// hostKeyPath - путь к файлу SSH host key
	hostKeyPath string

	// server - экземпляр gliderlabs/ssh сервера
	server *ssh.Server

	// rateLimiter - ограничитель частоты запросов (будет реализован в Phase 4)
	rateLimiter *SSHRateLimiter
}

// SSHServerConfig конфигурация SSH сервера
type SSHServerConfig struct {
	// DB - подключение к базе данных
	DB *gorm.DB

	// GitReposPath - путь к директории Git репозиториев
	GitReposPath string

	// Host - адрес для прослушивания
	Host string

	// Port - порт для прослушивания
	Port int

	// HostKeyPath - путь к файлу SSH host key
	HostKeyPath string

	// RateLimitEnabled - включить ограничение частоты запросов
	RateLimitEnabled bool
}

// NewSSHServer создает новый SSH сервер для Git операций
func NewSSHServer(config SSHServerConfig) (*SSHServer, error) {
	sshServer := &SSHServer{
		db:           config.DB,
		gitReposPath: config.GitReposPath,
		host:         config.Host,
		port:         config.Port,
		hostKeyPath:  config.HostKeyPath,
	}

	// Создаем gliderlabs/ssh сервер
	addr := fmt.Sprintf("%s:%d", config.Host, config.Port)
	sshServer.server = &ssh.Server{
		Addr:             addr,
		Handler:          sshServer.handleSSHSession,
		PublicKeyHandler: sshServer.handlePublicKeyAuth,
	}

	// Загружаем или генерируем SSH host key
	if err := sshServer.loadOrGenerateHostKey(sshServer.server); err != nil {
		return nil, fmt.Errorf("failed to load or generate host key: %w", err)
	}

	// Инициализируем rate limiter, если включен (Phase 4)
	if config.RateLimitEnabled {
		// Rate limiter: максимум 5 попыток в минуту
		sshServer.rateLimiter = NewSSHRateLimiter(5, 1*time.Minute)
	}

	slog.Info("SSH server created",
		"addr", addr,
		"git_repos_path", config.GitReposPath,
		"host_key_path", config.HostKeyPath,
		"rate_limit_enabled", config.RateLimitEnabled)

	return sshServer, nil
}

// loadOrGenerateHostKey загружает или генерирует SSH host key
func (s *SSHServer) loadOrGenerateHostKey(server *ssh.Server) error {
	// Проверяем существование файла host key
	if _, err := os.Stat(s.hostKeyPath); os.IsNotExist(err) {
		// Файл не существует - генерируем новый ключ
		slog.Info("SSH host key not found, generating new key", "path", s.hostKeyPath)

		// Создаем директорию, если не существует
		hostKeyDir := filepath.Dir(s.hostKeyPath)
		if err := os.MkdirAll(hostKeyDir, 0755); err != nil {
			return fmt.Errorf("failed to create host key directory: %w", err)
		}

		// Генерируем ED25519 ключ с помощью ssh-keygen
		cmd := exec.Command("ssh-keygen", "-t", "ed25519", "-f", s.hostKeyPath, "-N", "")
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to generate host key: %w (output: %s)", err, string(output))
		}

		slog.Info("SSH host key generated successfully", "path", s.hostKeyPath)
	}

	// Читаем приватный ключ из файла
	keyData, err := os.ReadFile(s.hostKeyPath)
	if err != nil {
		return fmt.Errorf("failed to read host key file: %w", err)
	}

	// Парсим приватный ключ
	hostKey, err := gossh.ParsePrivateKey(keyData)
	if err != nil {
		return fmt.Errorf("failed to parse host key: %w", err)
	}

	// Добавляем host key в сервер
	server.AddHostKey(hostKey)

	slog.Info("SSH host key loaded successfully",
		"path", s.hostKeyPath,
		"key_type", hostKey.PublicKey().Type())

	return nil
}

// handlePublicKeyAuth обрабатывает аутентификацию по публичному ключу
// Возвращает true, если аутентификация успешна
func (s *SSHServer) handlePublicKeyAuth(ctx ssh.Context, key ssh.PublicKey) bool {
	// Получаем fingerprint входящего ключа
	fingerprint := gossh.FingerprintSHA256(key)
	remoteAddr := ctx.RemoteAddr().String()

	slog.Debug("SSH authentication attempt",
		"fingerprint", fingerprint,
		"remote_addr", remoteAddr,
		"user", ctx.User())

	// Проверяем rate limit, если включен
	if s.rateLimiter != nil {
		// Извлекаем IP из remote address (формат: "ip:port")
		ip := remoteAddr
		if idx := strings.LastIndex(remoteAddr, ":"); idx != -1 {
			ip = remoteAddr[:idx]
		}

		if !s.rateLimiter.CheckAndRecord(ip) {
			slog.Warn("SSH rate limit exceeded",
				"ip", ip,
				"fingerprint", fingerprint)
			return false
		}
	}

	// Ищем SSH ключ по fingerprint в файловой системе
	sshKey, userId, err := FindSSHKeyByFingerprint(fingerprint, s.gitReposPath)
	if err != nil {
		slog.Warn("SSH key not found",
			"fingerprint", fingerprint,
			"remote_addr", remoteAddr,
			"err", err)
		return false
	}

	// Загружаем пользователя из базы данных
	var user dao.User
	if err := s.db.Where("id = ?", userId).First(&user).Error; err != nil {
		slog.Error("Failed to load user from database",
			"user_id", userId,
			"fingerprint", fingerprint,
			"err", err)
		return false
	}

	// Проверяем, что пользователь активен
	if !user.IsActive {
		slog.Warn("SSH authentication failed: user is not active",
			"user_id", userId,
			"user_email", user.Email,
			"fingerprint", fingerprint)
		return false
	}

	// Сохраняем пользователя в SSH context для использования в обработчике сессии
	ctx.SetValue("user", &user)
	ctx.SetValue("ssh_key_id", sshKey.ID)

	slog.Info("SSH authentication successful",
		"user_id", userId,
		"user_email", user.Email,
		"key_id", sshKey.ID,
		"key_name", sshKey.Name,
		"fingerprint", fingerprint,
		"remote_addr", remoteAddr)

	// Асинхронно обновляем last_used_at для ключа
	go func() {
		if err := UpdateSSHKeyLastUsed(userId, sshKey.ID, s.gitReposPath); err != nil {
			slog.Warn("Failed to update SSH key last_used_at",
				"user_id", userId,
				"key_id", sshKey.ID,
				"err", err)
		}
	}()

	return true
}

// handleSSHSession обрабатывает SSH сессию
func (s *SSHServer) handleSSHSession(sess ssh.Session) {
	// Получаем пользователя из context (установлен в handlePublicKeyAuth)
	userValue := sess.Context().Value("user")
	if userValue == nil {
		slog.Error("SSH session without user in context")
		io.WriteString(sess.Stderr(), "Internal error: user not found\n")
		sess.Exit(1)
		return
	}

	user := userValue.(*dao.User)

	// Получаем команду
	command := sess.Command()

	// Если команда пустая - это интерактивная сессия
	if len(command) == 0 {
		// Выводим приветственное сообщение
		welcomeMsg := fmt.Sprintf("Hi there, %s! You've successfully authenticated, but AIPlan does not provide shell access.\n", user.Email)
		io.WriteString(sess, welcomeMsg)
		sess.Exit(0)
		return
	}

	// Обрабатываем Git команду
	if err := s.handleGitCommand(sess, user, command); err != nil {
		slog.Error("Git command failed",
			"user", user.Email,
			"command", strings.Join(command, " "),
			"err", err)
		io.WriteString(sess.Stderr(), fmt.Sprintf("Error: %v\n", err))
		sess.Exit(1)
		return
	}

	sess.Exit(0)
}

// handleGitCommand обрабатывает Git команду
func (s *SSHServer) handleGitCommand(sess ssh.Session, user *dao.User, command []string) error {
	startTime := time.Now()

	// Парсим Git команду
	gitCmd, repoPath, err := parseGitCommand(command)
	if err != nil {
		return fmt.Errorf("invalid git command: %w", err)
	}

	remoteAddr := sess.RemoteAddr().String()
	slog.Info("Git command received",
		"user", user.Email,
		"git_cmd", gitCmd,
		"repo_path", repoPath,
		"remote_addr", remoteAddr)

	// Парсим путь репозитория: workspace-slug/repo-name
	workspaceSlug, repoName, err := parseRepositoryPath(repoPath)
	if err != nil {
		return fmt.Errorf("invalid repository path: %w", err)
	}

	// Проверяем права доступа к репозиторию
	canRead, canWrite, err := s.checkGitPermissions(user, workspaceSlug, repoName, gitCmd)
	if err != nil {
		return fmt.Errorf("permission check failed: %w", err)
	}

	// Проверяем права для конкретной операции
	if gitCmd == "git-receive-pack" && !canWrite {
		return fmt.Errorf("no write access to repository %s/%s", workspaceSlug, repoName)
	}

	if gitCmd == "git-upload-pack" && !canRead {
		return fmt.Errorf("no read access to repository %s/%s", workspaceSlug, repoName)
	}

	// Строим полный путь к репозиторию
	fullRepoPath := GetRepositoryPath(workspaceSlug, repoName, s.gitReposPath)

	// Проверяем существование репозитория
	if !GitRepositoryExists(workspaceSlug, repoName, s.gitReposPath) {
		return fmt.Errorf("repository not found: %s/%s", workspaceSlug, repoName)
	}

	// Создаем команду Git
	cmd := exec.Command(gitCmd, fullRepoPath)

	// Настраиваем IO: stdin, stdout, stderr
	cmd.Stdin = sess
	cmd.Stdout = sess
	cmd.Stderr = sess.Stderr()

	// Настраиваем переменные окружения для Git
	// Это позволит Git знать, кто выполняет операцию
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("GIT_COMMITTER_NAME=%s %s", user.FirstName, user.LastName),
		fmt.Sprintf("GIT_COMMITTER_EMAIL=%s", user.Email),
	)

	// Выполняем команду
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git command execution failed: %w", err)
	}

	duration := time.Since(startTime)
	slog.Info("Git command completed successfully",
		"user", user.Email,
		"git_cmd", gitCmd,
		"repo", fmt.Sprintf("%s/%s", workspaceSlug, repoName),
		"duration_ms", duration.Milliseconds())

	return nil
}

// checkGitPermissions проверяет права доступа к репозиторию
// Возвращает: (canRead, canWrite, error)
func (s *SSHServer) checkGitPermissions(user *dao.User, workspaceSlug, repoName, gitCmd string) (bool, bool, error) {
	// Загружаем workspace по slug
	var workspace dao.Workspace
	if err := s.db.Where("slug = ?", workspaceSlug).First(&workspace).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return false, false, fmt.Errorf("workspace not found: %s", workspaceSlug)
		}
		return false, false, fmt.Errorf("failed to load workspace: %w", err)
	}

	// Загружаем метаданные репозитория из файловой системы
	repoPath := GetRepositoryPath(workspaceSlug, repoName, s.gitReposPath)
	repo, err := LoadGitRepository(repoPath)
	if err != nil {
		return false, false, fmt.Errorf("failed to load repository metadata: %w", err)
	}

	// Проверяем membership в workspace
	var workspaceMember dao.WorkspaceMember
	err = s.db.Where("workspace_id = ? AND member_id = ?", workspace.ID, user.ID).First(&workspaceMember).Error
	isMember := (err == nil)

	// Логика прав доступа:
	// - Приватный репозиторий: только для членов workspace (read + write)
	// - Публичный репозиторий: read для всех, write только для членов
	if repo.Private {
		// Приватный репозиторий - только для членов
		if !isMember {
			slog.Warn("Access denied to private repository",
				"user", user.Email,
				"workspace", workspaceSlug,
				"repo", repoName,
				"git_cmd", gitCmd)
			return false, false, nil
		}
		// Член workspace имеет полный доступ
		return true, true, nil
	} else {
		// Публичный репозиторий
		canRead := true
		canWrite := isMember

		if !canWrite {
			slog.Debug("Public repository access",
				"user", user.Email,
				"workspace", workspaceSlug,
				"repo", repoName,
				"git_cmd", gitCmd,
				"can_read", canRead,
				"can_write", canWrite,
				"is_member", isMember)
		}

		return canRead, canWrite, nil
	}
}

// parseGitCommand парсит Git команду из SSH command
// Ожидается формат: ["git-upload-pack", "/workspace/repo.git"]
// Возвращает: (gitCmd, repoPath, error)
func parseGitCommand(command []string) (string, string, error) {
	if len(command) < 2 {
		return "", "", fmt.Errorf("invalid command format: expected at least 2 arguments, got %d", len(command))
	}

	gitCmd := command[0]
	repoPath := command[1]

	// Валидируем команду Git
	allowedCommands := map[string]bool{
		"git-upload-pack":    true, // git fetch, git clone
		"git-receive-pack":   true, // git push
		"git-upload-archive": true, // git archive
	}

	if !allowedCommands[gitCmd] {
		return "", "", fmt.Errorf("unsupported git command: %s", gitCmd)
	}

	// Убираем кавычки из пути (SSH может передавать путь в кавычках)
	repoPath = strings.Trim(repoPath, "'\"")

	return gitCmd, repoPath, nil
}

// parseRepositoryPath парсит путь репозитория
// Ожидается формат: "workspace-slug/repo-name.git" или "/workspace-slug/repo-name.git"
// Возвращает: (workspaceSlug, repoName, error)
func parseRepositoryPath(path string) (string, string, error) {
	// Убираем начальный слэш
	path = strings.TrimPrefix(path, "/")

	// Убираем .git суффикс
	path = strings.TrimSuffix(path, ".git")

	// Разбиваем на части
	parts := strings.Split(path, "/")

	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid repository path format: expected 'workspace/repo', got '%s'", path)
	}

	workspaceSlug := parts[0]
	repoName := parts[1]

	// Валидируем имя репозитория
	if !ValidateRepositoryName(repoName) {
		return "", "", fmt.Errorf("invalid repository name: %s", repoName)
	}

	return workspaceSlug, repoName, nil
}

// Start запускает SSH сервер
// Блокирует выполнение до получения сигнала остановки через context
func (s *SSHServer) Start(ctx context.Context) error {
	slog.Info("Starting SSH server",
		"addr", s.server.Addr,
		"git_repos_path", s.gitReposPath)

	// Создаем канал для ошибок запуска
	errChan := make(chan error, 1)

	// Запускаем сервер в горутине
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != ssh.ErrServerClosed {
			errChan <- err
		}
	}()

	// Ждем shutdown signal или ошибку запуска
	select {
	case <-ctx.Done():
		slog.Info("SSH server received shutdown signal")
		return s.Shutdown(context.Background())
	case err := <-errChan:
		slog.Error("SSH server failed to start", "err", err)
		return err
	}
}

// Shutdown останавливает SSH сервер с graceful shutdown
func (s *SSHServer) Shutdown(ctx context.Context) error {
	slog.Info("Shutting down SSH server")

	// Создаем context с таймаутом для graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Останавливаем сервер
	if err := s.server.Shutdown(shutdownCtx); err != nil {
		slog.Error("SSH server shutdown error", "err", err)
		return err
	}

	slog.Info("SSH server stopped successfully")
	return nil
}
