// Пакет aiplan предоставляет функциональность ограничения частоты SSH запросов (rate limiting)
// для защиты от brute-force атак на SSH сервер.
//
// Rate limiter отслеживает количество попыток аутентификации с каждого IP адреса
// и блокирует дальнейшие попытки при превышении лимита.
package aiplan

import (
	"sync"
	"time"
)

// SSHRateLimiter ограничивает частоту SSH попыток аутентификации по IP адресу
type SSHRateLimiter struct {
	// attempts - map IP адреса → список timestamp попыток
	attempts map[string][]time.Time

	// mu - мьютекс для thread-safe доступа к map
	mu sync.RWMutex

	// maxAttempts - максимальное количество попыток в течение window
	maxAttempts int

	// window - временное окно для подсчета попыток
	window time.Duration

	// stopCleanup - канал для остановки фоновой горутины очистки
	stopCleanup chan struct{}
}

// NewSSHRateLimiter создает новый rate limiter для SSH
// maxAttempts - максимальное количество попыток (например, 5)
// window - временное окно (например, 1 минута)
func NewSSHRateLimiter(maxAttempts int, window time.Duration) *SSHRateLimiter {
	limiter := &SSHRateLimiter{
		attempts:    make(map[string][]time.Time),
		maxAttempts: maxAttempts,
		window:      window,
		stopCleanup: make(chan struct{}),
	}

	// Запускаем фоновую горутину для периодической очистки старых записей
	go limiter.startCleanup()

	return limiter
}

// CheckAndRecord проверяет, не превышен ли лимит попыток для указанного IP,
// и записывает новую попытку.
// Возвращает true, если попытка разрешена, false - если лимит превышен.
func (rl *SSHRateLimiter) CheckAndRecord(ip string) bool {
	now := time.Now()

	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Получаем список попыток для данного IP
	attempts, exists := rl.attempts[ip]

	// Фильтруем старые попытки (за пределами временного окна)
	validAttempts := []time.Time{}
	cutoffTime := now.Add(-rl.window)

	if exists {
		for _, attemptTime := range attempts {
			if attemptTime.After(cutoffTime) {
				validAttempts = append(validAttempts, attemptTime)
			}
		}
	}

	// Проверяем, не превышен ли лимит
	if len(validAttempts) >= rl.maxAttempts {
		// Лимит превышен - не добавляем попытку и возвращаем false
		return false
	}

	// Добавляем текущую попытку
	validAttempts = append(validAttempts, now)
	rl.attempts[ip] = validAttempts

	return true
}

// startCleanup запускает фоновую горутину для периодической очистки старых записей
func (rl *SSHRateLimiter) startCleanup() {
	// Очищаем каждые 1 минуту
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.cleanup()
		case <-rl.stopCleanup:
			return
		}
	}
}

// cleanup удаляет старые записи из map для освобождения памяти
func (rl *SSHRateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	// Удаляем записи старше 2*window (с запасом)
	cutoffTime := now.Add(-2 * rl.window)

	// Проходим по всем IP и удаляем старые записи
	for ip, attempts := range rl.attempts {
		validAttempts := []time.Time{}
		for _, attemptTime := range attempts {
			if attemptTime.After(cutoffTime) {
				validAttempts = append(validAttempts, attemptTime)
			}
		}

		if len(validAttempts) == 0 {
			// Нет валидных попыток - удаляем запись для этого IP
			delete(rl.attempts, ip)
		} else {
			// Обновляем список попыток
			rl.attempts[ip] = validAttempts
		}
	}
}

// Stop останавливает фоновую горутину очистки
func (rl *SSHRateLimiter) Stop() {
	close(rl.stopCleanup)
}
