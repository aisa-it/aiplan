// Пакет aiplan предоставляет функциональность для работы с SSH ключами пользователей
// через файловую систему без использования базы данных.
//
// Архитектурный принцип: SSH ключи должны храниться в файловой системе для
// простоты управления и независимости от структуры БД. Все ключи хранятся в директории
// {GitRepositoriesPath}/.ssh-keys/ в виде JSON файлов с метаданными и одного общего
// файла authorized_keys для SSH сервера.
package aiplan

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gofrs/uuid"
	"golang.org/x/crypto/ssh"
)

// SSHKeyMetadata представляет метаданные одного SSH ключа
type SSHKeyMetadata struct {
	// ID - уникальный идентификатор ключа (UUID)
	ID string `json:"id"`

	// Name - название ключа, заданное пользователем (например "Work Laptop")
	Name string `json:"name"`

	// PublicKey - SSH публичный ключ в OpenSSH формате
	PublicKey string `json:"public_key"`

	// Fingerprint - SHA256 отпечаток ключа для идентификации
	Fingerprint string `json:"fingerprint"`

	// KeyType - тип ключа (ssh-ed25519, ssh-rsa, ecdsa-sha2-nistp256, и т.д.)
	KeyType string `json:"key_type"`

	// CreatedAt - время добавления ключа
	CreatedAt time.Time `json:"created_at"`

	// LastUsedAt - время последнего использования ключа (может быть nil)
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`

	// Comment - комментарий из SSH ключа (обычно user@host)
	Comment string `json:"comment"`
}

// UserSSHKeys представляет все SSH ключи пользователя
type UserSSHKeys struct {
	// UserId - UUID пользователя
	UserId string `json:"user_id"`

	// UserEmail - email пользователя для удобства идентификации
	UserEmail string `json:"user_email"`

	// Keys - массив SSH ключей пользователя
	Keys []SSHKeyMetadata `json:"keys"`

	// CreatedAt - время создания файла ключей
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt - время последнего обновления файла
	UpdatedAt time.Time `json:"updated_at"`
}

// sshKeysMutex используется для обеспечения thread-safety при работе с файлами SSH ключей
var sshKeysMutex sync.Mutex

// GetSSHKeysPath возвращает путь к директории SSH ключей
// Директория: {gitReposPath}/.ssh-keys/
func GetSSHKeysPath(gitReposPath string) string {
	return filepath.Join(gitReposPath, ".ssh-keys")
}

// GetUserSSHKeysFilePath возвращает путь к файлу SSH ключей пользователя
// Формат: {gitReposPath}/.ssh-keys/{userId}.json
func GetUserSSHKeysFilePath(userId, gitReposPath string) string {
	return filepath.Join(GetSSHKeysPath(gitReposPath), userId+".json")
}

// GetAuthorizedKeysPath возвращает путь к файлу authorized_keys
// Файл: {gitReposPath}/.ssh-keys/authorized_keys
func GetAuthorizedKeysPath(gitReposPath string) string {
	return filepath.Join(GetSSHKeysPath(gitReposPath), "authorized_keys")
}

// LoadUserSSHKeys загружает SSH ключи пользователя из файла
// Если файл не существует, возвращается ошибка, которую можно проверить через os.IsNotExist
func LoadUserSSHKeys(userId, gitReposPath string) (*UserSSHKeys, error) {
	filePath := GetUserSSHKeysFilePath(userId, gitReposPath)

	data, err := os.ReadFile(filePath)
	if err != nil {
		// Возвращаем оригинальную ошибку для корректной работы os.IsNotExist
		return nil, err
	}

	var keys UserSSHKeys
	if err := json.Unmarshal(data, &keys); err != nil {
		return nil, fmt.Errorf("failed to unmarshal SSH keys: %w", err)
	}

	return &keys, nil
}

// SaveUserSSHKeys сохраняет SSH ключи пользователя в файл
// Использует атомарную запись (temp file + rename) для безопасности
func SaveUserSSHKeys(keys *UserSSHKeys, gitReposPath string) error {
	// Обновляем timestamp
	keys.UpdatedAt = time.Now()

	// Создаем директорию, если не существует
	sshKeysPath := GetSSHKeysPath(gitReposPath)
	if err := os.MkdirAll(sshKeysPath, 0755); err != nil {
		return fmt.Errorf("failed to create SSH keys directory: %w", err)
	}

	// Сериализуем данные
	data, err := json.MarshalIndent(keys, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal SSH keys: %w", err)
	}

	// Атомарная запись: temp file + rename
	filePath := GetUserSSHKeysFilePath(keys.UserId, gitReposPath)
	tempPath := filePath + ".tmp"

	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp SSH keys file: %w", err)
	}

	if err := os.Rename(tempPath, filePath); err != nil {
		os.Remove(tempPath) // Cleanup temp file
		return fmt.Errorf("failed to rename temp SSH keys file: %w", err)
	}

	return nil
}

// ParseSSHPublicKey парсит и валидирует SSH публичный ключ
// Возвращает: keyType, fingerprint (SHA256), comment, error
func ParseSSHPublicKey(publicKey string) (keyType, fingerprint, comment string, err error) {
	// Убираем лишние пробелы и переносы строк
	publicKey = strings.TrimSpace(publicKey)

	// Парсим ключ с помощью golang.org/x/crypto/ssh
	parsedKey, commentFromKey, _, _, err := ssh.ParseAuthorizedKey([]byte(publicKey))
	if err != nil {
		return "", "", "", fmt.Errorf("invalid SSH public key format: %w", err)
	}

	// Получаем тип ключа
	keyType = parsedKey.Type()

	// Вычисляем SHA256 fingerprint
	hash := sha256.Sum256(parsedKey.Marshal())
	fingerprint = "SHA256:" + base64.RawStdEncoding.EncodeToString(hash[:])

	// Комментарий
	comment = commentFromKey

	return keyType, fingerprint, comment, nil
}

// ValidateSSHKeyName валидирует имя SSH ключа
// Имя должно быть от 1 до 255 символов
func ValidateSSHKeyName(name string) bool {
	return len(name) >= 1 && len(name) <= 255
}

// AddSSHKey добавляет новый SSH ключ пользователю
// Выполняет парсинг, валидацию, проверку на дубликаты и регенерацию authorized_keys
func AddSSHKey(userId, userEmail, name, publicKey, gitReposPath string) (*SSHKeyMetadata, error) {
	// Защищаем операцию mutex'ом для thread-safety
	sshKeysMutex.Lock()
	defer sshKeysMutex.Unlock()

	// Валидация имени ключа
	if !ValidateSSHKeyName(name) {
		return nil, fmt.Errorf("invalid SSH key name: must be 1-255 characters")
	}

	// Парсим и валидируем публичный ключ
	keyType, fingerprint, comment, err := ParseSSHPublicKey(publicKey)
	if err != nil {
		return nil, err
	}

	// Загружаем существующие ключи пользователя или создаем новый файл
	userKeys, err := LoadUserSSHKeys(userId, gitReposPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Создаем новую структуру для пользователя
			userKeys = &UserSSHKeys{
				UserId:    userId,
				UserEmail: userEmail,
				Keys:      []SSHKeyMetadata{},
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
		} else {
			return nil, fmt.Errorf("failed to load user SSH keys: %w", err)
		}
	}

	// Проверяем, не существует ли уже ключ с таким fingerprint у ЭТОГО пользователя
	for _, key := range userKeys.Keys {
		if key.Fingerprint == fingerprint {
			return nil, fmt.Errorf("SSH key with fingerprint %s already exists", fingerprint)
		}
	}

	// Генерируем UUID для нового ключа
	keyId, err := uuid.NewV4()
	if err != nil {
		return nil, fmt.Errorf("failed to generate key ID: %w", err)
	}

	// Создаем новый ключ
	newKey := SSHKeyMetadata{
		ID:          keyId.String(),
		Name:        name,
		PublicKey:   strings.TrimSpace(publicKey),
		Fingerprint: fingerprint,
		KeyType:     keyType,
		CreatedAt:   time.Now(),
		LastUsedAt:  nil,
		Comment:     comment,
	}

	// Добавляем ключ в массив
	userKeys.Keys = append(userKeys.Keys, newKey)

	// Сохраняем файл пользователя
	if err := SaveUserSSHKeys(userKeys, gitReposPath); err != nil {
		return nil, fmt.Errorf("failed to save user SSH keys: %w", err)
	}

	// Регенерируем authorized_keys файл
	if err := regenerateAuthorizedKeysUnsafe(gitReposPath); err != nil {
		return nil, fmt.Errorf("failed to regenerate authorized_keys: %w", err)
	}

	return &newKey, nil
}

// DeleteSSHKey удаляет SSH ключ пользователя по ID
func DeleteSSHKey(userId, keyId, gitReposPath string) error {
	// Защищаем операцию mutex'ом
	sshKeysMutex.Lock()
	defer sshKeysMutex.Unlock()

	// Загружаем ключи пользователя
	userKeys, err := LoadUserSSHKeys(userId, gitReposPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("user has no SSH keys")
		}
		return fmt.Errorf("failed to load user SSH keys: %w", err)
	}

	// Ищем и удаляем ключ
	keyFound := false
	newKeys := []SSHKeyMetadata{}
	for _, key := range userKeys.Keys {
		if key.ID == keyId {
			keyFound = true
			continue
		}
		newKeys = append(newKeys, key)
	}

	if !keyFound {
		return fmt.Errorf("SSH key not found")
	}

	// Обновляем массив ключей
	userKeys.Keys = newKeys

	// Если ключей больше нет, удаляем файл пользователя
	if len(userKeys.Keys) == 0 {
		filePath := GetUserSSHKeysFilePath(userId, gitReposPath)
		if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove user SSH keys file: %w", err)
		}
	} else {
		// Сохраняем обновленный файл
		if err := SaveUserSSHKeys(userKeys, gitReposPath); err != nil {
			return fmt.Errorf("failed to save user SSH keys: %w", err)
		}
	}

	// Регенерируем authorized_keys файл
	if err := regenerateAuthorizedKeysUnsafe(gitReposPath); err != nil {
		return fmt.Errorf("failed to regenerate authorized_keys: %w", err)
	}

	return nil
}

// UpdateSSHKeyLastUsed обновляет время последнего использования ключа
// НЕ вызывает RegenerateAuthorizedKeys, так как это не требуется
func UpdateSSHKeyLastUsed(userId, keyId, gitReposPath string) error {
	// Защищаем операцию mutex'ом
	sshKeysMutex.Lock()
	defer sshKeysMutex.Unlock()

	// Загружаем ключи пользователя
	userKeys, err := LoadUserSSHKeys(userId, gitReposPath)
	if err != nil {
		return fmt.Errorf("failed to load user SSH keys: %w", err)
	}

	// Ищем и обновляем ключ
	keyFound := false
	now := time.Now()
	for i := range userKeys.Keys {
		if userKeys.Keys[i].ID == keyId {
			userKeys.Keys[i].LastUsedAt = &now
			keyFound = true
			break
		}
	}

	if !keyFound {
		return fmt.Errorf("SSH key not found")
	}

	// Сохраняем файл
	if err := SaveUserSSHKeys(userKeys, gitReposPath); err != nil {
		return fmt.Errorf("failed to save user SSH keys: %w", err)
	}

	return nil
}

// FindSSHKeyByFingerprint ищет SSH ключ по fingerprint среди всех пользователей
// Возвращает: (key, userId, error)
func FindSSHKeyByFingerprint(fingerprint, gitReposPath string) (*SSHKeyMetadata, string, error) {
	// Защищаем операцию mutex'ом для чтения
	sshKeysMutex.Lock()
	defer sshKeysMutex.Unlock()

	return findSSHKeyByFingerprintUnsafe(fingerprint, gitReposPath)
}

// findSSHKeyByFingerprintUnsafe - внутренняя версия без mutex (для использования внутри других функций)
func findSSHKeyByFingerprintUnsafe(fingerprint, gitReposPath string) (*SSHKeyMetadata, string, error) {
	sshKeysPath := GetSSHKeysPath(gitReposPath)

	// Читаем все файлы в директории .ssh-keys
	entries, err := os.ReadDir(sshKeysPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", fmt.Errorf("SSH keys directory not found")
		}
		return nil, "", fmt.Errorf("failed to read SSH keys directory: %w", err)
	}

	// Ищем среди всех файлов пользователей
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		// Извлекаем userId из имени файла
		userId := strings.TrimSuffix(entry.Name(), ".json")

		// Загружаем ключи пользователя
		userKeys, err := LoadUserSSHKeys(userId, gitReposPath)
		if err != nil {
			// Пропускаем поврежденные файлы
			continue
		}

		// Ищем ключ с совпадающим fingerprint
		for i := range userKeys.Keys {
			if userKeys.Keys[i].Fingerprint == fingerprint {
				return &userKeys.Keys[i], userId, nil
			}
		}
	}

	return nil, "", fmt.Errorf("SSH key not found")
}

// RegenerateAuthorizedKeys пересоздает файл authorized_keys из всех пользовательских ключей
// Формат файла:
// # User: email (uuid)
// # Key: name (keyId)
// ssh-xxx AAAA... comment
func RegenerateAuthorizedKeys(gitReposPath string) error {
	// Защищаем операцию mutex'ом
	sshKeysMutex.Lock()
	defer sshKeysMutex.Unlock()

	return regenerateAuthorizedKeysUnsafe(gitReposPath)
}

// regenerateAuthorizedKeysUnsafe - внутренняя версия без mutex
func regenerateAuthorizedKeysUnsafe(gitReposPath string) error {
	sshKeysPath := GetSSHKeysPath(gitReposPath)

	// Создаем директорию, если не существует
	if err := os.MkdirAll(sshKeysPath, 0755); err != nil {
		return fmt.Errorf("failed to create SSH keys directory: %w", err)
	}

	// Читаем все файлы пользователей
	entries, err := os.ReadDir(sshKeysPath)
	if err != nil {
		return fmt.Errorf("failed to read SSH keys directory: %w", err)
	}

	// Собираем все ключи
	var authorizedKeysContent strings.Builder

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		// Извлекаем userId из имени файла
		userId := strings.TrimSuffix(entry.Name(), ".json")

		// Загружаем ключи пользователя
		userKeys, err := LoadUserSSHKeys(userId, gitReposPath)
		if err != nil {
			// Пропускаем поврежденные файлы
			continue
		}

		// Добавляем каждый ключ в authorized_keys
		for _, key := range userKeys.Keys {
			authorizedKeysContent.WriteString(fmt.Sprintf("# User: %s (%s)\n", userKeys.UserEmail, userKeys.UserId))
			authorizedKeysContent.WriteString(fmt.Sprintf("# Key: %s (%s)\n", key.Name, key.ID))
			authorizedKeysContent.WriteString(key.PublicKey)
			authorizedKeysContent.WriteString("\n\n")
		}
	}

	// Атомарная запись: temp file + rename
	authorizedKeysPath := GetAuthorizedKeysPath(gitReposPath)
	tempPath := authorizedKeysPath + ".tmp"

	if err := os.WriteFile(tempPath, []byte(authorizedKeysContent.String()), 0644); err != nil {
		return fmt.Errorf("failed to write temp authorized_keys file: %w", err)
	}

	if err := os.Rename(tempPath, authorizedKeysPath); err != nil {
		os.Remove(tempPath) // Cleanup temp file
		return fmt.Errorf("failed to rename temp authorized_keys file: %w", err)
	}

	return nil
}
