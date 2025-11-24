package aiplan

import (
	"testing"
	"time"
)

// TestSSHRateLimiter проверяет работу rate limiter
func TestSSHRateLimiter(t *testing.T) {
	// Создаем rate limiter: максимум 3 попытки за 100ms
	limiter := NewSSHRateLimiter(3, 100*time.Millisecond)
	defer limiter.Stop()

	testIP := "192.168.1.1"

	// Первые 3 попытки должны быть разрешены
	for i := 0; i < 3; i++ {
		if !limiter.CheckAndRecord(testIP) {
			t.Errorf("Attempt %d should be allowed", i+1)
		}
	}

	// 4-я попытка должна быть заблокирована
	if limiter.CheckAndRecord(testIP) {
		t.Error("4th attempt should be blocked")
	}

	// Ждем истечения временного окна
	time.Sleep(110 * time.Millisecond)

	// После истечения окна попытка снова должна быть разрешена
	if !limiter.CheckAndRecord(testIP) {
		t.Error("Attempt after window expiration should be allowed")
	}
}

// TestSSHRateLimiterMultipleIPs проверяет изоляцию между разными IP
func TestSSHRateLimiterMultipleIPs(t *testing.T) {
	limiter := NewSSHRateLimiter(2, 100*time.Millisecond)
	defer limiter.Stop()

	ip1 := "192.168.1.1"
	ip2 := "192.168.1.2"

	// IP1: 2 попытки разрешены
	if !limiter.CheckAndRecord(ip1) {
		t.Error("IP1: 1st attempt should be allowed")
	}
	if !limiter.CheckAndRecord(ip1) {
		t.Error("IP1: 2nd attempt should be allowed")
	}
	if limiter.CheckAndRecord(ip1) {
		t.Error("IP1: 3rd attempt should be blocked")
	}

	// IP2: должен иметь свой независимый лимит
	if !limiter.CheckAndRecord(ip2) {
		t.Error("IP2: 1st attempt should be allowed")
	}
	if !limiter.CheckAndRecord(ip2) {
		t.Error("IP2: 2nd attempt should be allowed")
	}
	if limiter.CheckAndRecord(ip2) {
		t.Error("IP2: 3rd attempt should be blocked")
	}
}

// TestSSHRateLimiterCleanup проверяет очистку старых записей
func TestSSHRateLimiterCleanup(t *testing.T) {
	limiter := NewSSHRateLimiter(5, 50*time.Millisecond)
	defer limiter.Stop()

	testIP := "192.168.1.1"

	// Делаем несколько попыток
	for i := 0; i < 3; i++ {
		limiter.CheckAndRecord(testIP)
	}

	// Проверяем, что запись есть
	limiter.mu.RLock()
	if len(limiter.attempts[testIP]) != 3 {
		t.Errorf("Expected 3 attempts, got %d", len(limiter.attempts[testIP]))
	}
	limiter.mu.RUnlock()

	// Ждем больше 2*window для cleanup
	time.Sleep(120 * time.Millisecond)

	// Вызываем cleanup вручную
	limiter.cleanup()

	// Проверяем, что старые записи удалены
	limiter.mu.RLock()
	attempts, exists := limiter.attempts[testIP]
	limiter.mu.RUnlock()

	if exists && len(attempts) > 0 {
		t.Errorf("Old attempts should be cleaned up, but got %d attempts", len(attempts))
	}
}

// TestSSHRateLimiterZeroAttempts проверяет поведение с нулевым лимитом
func TestSSHRateLimiterZeroAttempts(t *testing.T) {
	limiter := NewSSHRateLimiter(0, 100*time.Millisecond)
	defer limiter.Stop()

	testIP := "192.168.1.1"

	// С нулевым лимитом все попытки должны быть заблокированы
	if limiter.CheckAndRecord(testIP) {
		t.Error("With zero max attempts, all attempts should be blocked")
	}
}
