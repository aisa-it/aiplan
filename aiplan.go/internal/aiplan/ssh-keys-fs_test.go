package aiplan

import (
	"os"
	"strings"
	"testing"
	"time"
)

// TestGetSSHKeysPath проверяет генерацию пути к директории SSH ключей
func TestGetSSHKeysPath(t *testing.T) {
	gitReposPath := "/var/lib/aiplan/git-repositories"
	expected := "/var/lib/aiplan/git-repositories/.ssh-keys"
	result := GetSSHKeysPath(gitReposPath)

	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

// TestGetUserSSHKeysFilePath проверяет генерацию пути к файлу ключей пользователя
func TestGetUserSSHKeysFilePath(t *testing.T) {
	gitReposPath := "/var/lib/aiplan/git-repositories"
	userId := "6bad34e1-382f-4ec3-ad3c-0f3bb48f4696"
	expected := "/var/lib/aiplan/git-repositories/.ssh-keys/6bad34e1-382f-4ec3-ad3c-0f3bb48f4696.json"
	result := GetUserSSHKeysFilePath(userId, gitReposPath)

	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

// TestGetAuthorizedKeysPath проверяет генерацию пути к authorized_keys
func TestGetAuthorizedKeysPath(t *testing.T) {
	gitReposPath := "/var/lib/aiplan/git-repositories"
	expected := "/var/lib/aiplan/git-repositories/.ssh-keys/authorized_keys"
	result := GetAuthorizedKeysPath(gitReposPath)

	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

// TestValidateSSHKeyName проверяет валидацию имени SSH ключа
func TestValidateSSHKeyName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"Valid name", "Work Laptop", true},
		{"Empty name", "", false},
		{"Very long name", strings.Repeat("a", 256), false},
		{"Max length name", strings.Repeat("a", 255), true},
		{"Single char", "A", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateSSHKeyName(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %v for '%s', got %v", tt.expected, tt.input, result)
			}
		})
	}
}

// TestParseSSHPublicKey проверяет парсинг SSH публичных ключей
func TestParseSSHPublicKey(t *testing.T) {
	tests := []struct {
		name           string
		publicKey      string
		expectError    bool
		expectedType   string
		expectedPrefix string // Префикс fingerprint (SHA256:)
	}{
		{
			name:           "Valid ED25519 key",
			publicKey:      "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl test@example.com",
			expectError:    false,
			expectedType:   "ssh-ed25519",
			expectedPrefix: "SHA256:",
		},
		{
			name:        "Invalid key",
			publicKey:   "invalid-key-data",
			expectError: true,
		},
		{
			name:        "Empty key",
			publicKey:   "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keyType, fingerprint, comment, err := ParseSSHPublicKey(tt.publicKey)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if keyType != tt.expectedType {
				t.Errorf("Expected keyType %s, got %s", tt.expectedType, keyType)
			}

			if !strings.HasPrefix(fingerprint, tt.expectedPrefix) {
				t.Errorf("Expected fingerprint to start with %s, got %s", tt.expectedPrefix, fingerprint)
			}

			if comment == "" {
				t.Logf("Warning: comment is empty for test '%s'", tt.name)
			}
		})
	}
}

// TestAddSSHKey проверяет добавление SSH ключа
func TestAddSSHKey(t *testing.T) {
	// Создаем временную директорию для тестов
	tempDir, err := os.MkdirTemp("", "ssh-keys-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	userId := "test-user-123"
	userEmail := "test@example.com"
	keyName := "Test Laptop"
	publicKey := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl test@example.com"

	// Добавляем ключ
	key, err := AddSSHKey(userId, userEmail, keyName, publicKey, tempDir)
	if err != nil {
		t.Fatalf("Failed to add SSH key: %v", err)
	}

	// Проверяем результат
	if key.Name != keyName {
		t.Errorf("Expected name %s, got %s", keyName, key.Name)
	}

	if key.KeyType != "ssh-ed25519" {
		t.Errorf("Expected keyType ssh-ed25519, got %s", key.KeyType)
	}

	if !strings.HasPrefix(key.Fingerprint, "SHA256:") {
		t.Errorf("Expected fingerprint to start with SHA256:, got %s", key.Fingerprint)
	}

	// Проверяем, что файл создан
	userKeysPath := GetUserSSHKeysFilePath(userId, tempDir)
	if _, err := os.Stat(userKeysPath); os.IsNotExist(err) {
		t.Errorf("User SSH keys file was not created")
	}

	// Проверяем, что authorized_keys создан
	authorizedKeysPath := GetAuthorizedKeysPath(tempDir)
	if _, err := os.Stat(authorizedKeysPath); os.IsNotExist(err) {
		t.Errorf("Authorized keys file was not created")
	}

	// Проверяем содержимое authorized_keys
	content, err := os.ReadFile(authorizedKeysPath)
	if err != nil {
		t.Fatalf("Failed to read authorized_keys: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, userEmail) {
		t.Errorf("Authorized keys does not contain user email")
	}
	if !strings.Contains(contentStr, keyName) {
		t.Errorf("Authorized keys does not contain key name")
	}
	if !strings.Contains(contentStr, publicKey) {
		t.Errorf("Authorized keys does not contain public key")
	}
}

// TestAddSSHKeyDuplicate проверяет защиту от дубликатов
func TestAddSSHKeyDuplicate(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "ssh-keys-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	userId := "test-user-456"
	userEmail := "test@example.com"
	keyName := "Test Key"
	publicKey := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl test@example.com"

	// Добавляем ключ первый раз
	_, err = AddSSHKey(userId, userEmail, keyName, publicKey, tempDir)
	if err != nil {
		t.Fatalf("Failed to add SSH key first time: %v", err)
	}

	// Пытаемся добавить тот же ключ снова
	_, err = AddSSHKey(userId, userEmail, "Different Name", publicKey, tempDir)
	if err == nil {
		t.Errorf("Expected error when adding duplicate key, got nil")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("Expected 'already exists' error, got: %v", err)
	}
}

// TestDeleteSSHKey проверяет удаление SSH ключа
func TestDeleteSSHKey(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "ssh-keys-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	userId := "test-user-789"
	userEmail := "test@example.com"
	keyName := "Test Key"
	publicKey := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl test@example.com"

	// Добавляем ключ
	key, err := AddSSHKey(userId, userEmail, keyName, publicKey, tempDir)
	if err != nil {
		t.Fatalf("Failed to add SSH key: %v", err)
	}

	// Удаляем ключ
	err = DeleteSSHKey(userId, key.ID, tempDir)
	if err != nil {
		t.Errorf("Failed to delete SSH key: %v", err)
	}

	// Проверяем, что файл пользователя удален (так как это был единственный ключ)
	userKeysPath := GetUserSSHKeysFilePath(userId, tempDir)
	if _, err := os.Stat(userKeysPath); !os.IsNotExist(err) {
		t.Errorf("User SSH keys file should be deleted when no keys remain")
	}

	// Попытка удалить несуществующий ключ
	err = DeleteSSHKey(userId, "non-existent-key-id", tempDir)
	if err == nil {
		t.Errorf("Expected error when deleting non-existent key, got nil")
	}
}

// TestUpdateSSHKeyLastUsed проверяет обновление времени последнего использования
func TestUpdateSSHKeyLastUsed(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "ssh-keys-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	userId := "test-user-update"
	userEmail := "test@example.com"
	keyName := "Test Key"
	publicKey := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl test@example.com"

	// Добавляем ключ
	key, err := AddSSHKey(userId, userEmail, keyName, publicKey, tempDir)
	if err != nil {
		t.Fatalf("Failed to add SSH key: %v", err)
	}

	// Проверяем, что LastUsedAt изначально nil
	if key.LastUsedAt != nil {
		t.Errorf("Expected LastUsedAt to be nil initially, got %v", key.LastUsedAt)
	}

	// Ждем немного, чтобы время изменилось
	time.Sleep(10 * time.Millisecond)

	// Обновляем LastUsedAt
	err = UpdateSSHKeyLastUsed(userId, key.ID, tempDir)
	if err != nil {
		t.Errorf("Failed to update LastUsedAt: %v", err)
	}

	// Загружаем ключи и проверяем обновление
	userKeys, err := LoadUserSSHKeys(userId, tempDir)
	if err != nil {
		t.Fatalf("Failed to load user keys: %v", err)
	}

	if len(userKeys.Keys) != 1 {
		t.Fatalf("Expected 1 key, got %d", len(userKeys.Keys))
	}

	updatedKey := userKeys.Keys[0]
	if updatedKey.LastUsedAt == nil {
		t.Errorf("Expected LastUsedAt to be set, got nil")
	} else {
		// Проверяем, что LastUsedAt после CreatedAt
		if !updatedKey.LastUsedAt.After(updatedKey.CreatedAt) {
			t.Errorf("Expected LastUsedAt to be after CreatedAt")
		}
	}
}

// TestFindSSHKeyByFingerprint проверяет поиск ключа по fingerprint
func TestFindSSHKeyByFingerprint(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "ssh-keys-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	userId := "test-user-find"
	userEmail := "test@example.com"
	keyName := "Test Key"
	publicKey := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl test@example.com"

	// Добавляем ключ
	key, err := AddSSHKey(userId, userEmail, keyName, publicKey, tempDir)
	if err != nil {
		t.Fatalf("Failed to add SSH key: %v", err)
	}

	// Ищем ключ по fingerprint
	foundKey, foundUserId, err := FindSSHKeyByFingerprint(key.Fingerprint, tempDir)
	if err != nil {
		t.Errorf("Failed to find SSH key by fingerprint: %v", err)
	}

	if foundUserId != userId {
		t.Errorf("Expected userId %s, got %s", userId, foundUserId)
	}

	if foundKey.ID != key.ID {
		t.Errorf("Expected key ID %s, got %s", key.ID, foundKey.ID)
	}

	// Ищем несуществующий ключ
	_, _, err = FindSSHKeyByFingerprint("SHA256:nonexistent", tempDir)
	if err == nil {
		t.Errorf("Expected error when finding non-existent key, got nil")
	}
}

// TestRegenerateAuthorizedKeys проверяет генерацию authorized_keys
func TestRegenerateAuthorizedKeys(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "ssh-keys-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Добавляем несколько ключей для разных пользователей
	users := []struct {
		userId string
		email  string
		keys   []string
	}{
		{
			userId: "user-1",
			email:  "user1@example.com",
			keys: []string{
				"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl user1@laptop",
			},
		},
		{
			userId: "user-2",
			email:  "user2@example.com",
			keys: []string{
				"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAILq8kbZPW5JqB5nDQXKLs6Y9pE3rA9lB0hZ8mWx4qXyz user2@desktop",
			},
		},
	}

	for _, user := range users {
		for i, publicKey := range user.keys {
			keyName := user.email + " key " + string(rune('1'+i))
			_, err := AddSSHKey(user.userId, user.email, keyName, publicKey, tempDir)
			if err != nil {
				t.Fatalf("Failed to add SSH key for %s: %v", user.email, err)
			}
		}
	}

	// Проверяем содержимое authorized_keys
	authorizedKeysPath := GetAuthorizedKeysPath(tempDir)
	content, err := os.ReadFile(authorizedKeysPath)
	if err != nil {
		t.Fatalf("Failed to read authorized_keys: %v", err)
	}

	contentStr := string(content)

	// Проверяем, что все пользователи присутствуют
	for _, user := range users {
		if !strings.Contains(contentStr, user.email) {
			t.Errorf("Authorized keys does not contain %s", user.email)
		}
	}

	// Проверяем формат комментариев
	if !strings.Contains(contentStr, "# User:") {
		t.Errorf("Authorized keys does not contain user comments")
	}
	if !strings.Contains(contentStr, "# Key:") {
		t.Errorf("Authorized keys does not contain key comments")
	}
}

// TestLoadUserSSHKeysNonExistent проверяет загрузку несуществующего файла
func TestLoadUserSSHKeysNonExistent(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "ssh-keys-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	_, err = LoadUserSSHKeys("non-existent-user", tempDir)
	if err == nil {
		t.Errorf("Expected error when loading non-existent user keys, got nil")
	}
	if !os.IsNotExist(err) && !strings.Contains(err.Error(), "not found") {
		t.Errorf("Expected 'not found' error, got: %v", err)
	}
}
