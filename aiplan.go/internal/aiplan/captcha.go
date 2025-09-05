// Обработка и проверка CAPTCHA-подписей для защиты от повторных атак.
// Содержит логику декодирования, проверки подписи и хранения подписей для предотвращения повторного использования.
//
// Основные возможности:
//   - Декодирование base64 CAPTCHA-подписей.
//   - Проверка подлинности CAPTCHA с использованием ключа Altcha HMAC.
//   - Хранение проверенных CAPTCHA-подписей для предотвращения повторного использования.
//   - Подсчет неудачных попыток использования CAPTCHA.
package aiplan

import (
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/altcha-org/altcha-lib-go"
	"github.com/prometheus/client_golang/prometheus"
)

var CaptchaService = NewCaptchaService()

const (
	AltchaHMACKey = "qwdlelqwedmkdejmi931jnfk8wedweffwe23nefdqd0-q"
)

var AltchaExpires time.Duration = time.Hour

type CaptchaSignatures struct {
	signatures map[string]struct{}
	mu         sync.Mutex

	badSignaturesCounter prometheus.Counter
}

// NewCaptchaService создает экземпляр сервиса для обработки CAPTCHA-подписей.  Сервис хранит проверенные подписи, чтобы предотвратить повторное использование.  Также включает в себя периодическую очистку хранилища подписей и подсчет неудачных попыток использования CAPTCHA для обнаружения атак.
//
// Возвращает:
//   - *CaptchaSignatures: экземпляр структуры CaptchaSignatures, который содержит логику проверки и хранения CAPTCHA-подписей.
//
// Нет параметров.
func NewCaptchaService() *CaptchaSignatures {
	ticker := time.NewTicker(time.Hour * 24)

	s := CaptchaSignatures{signatures: make(map[string]struct{})}
	go func() {
		for range ticker.C {
			slog.Info("Clear captchas signatures")
			s.mu.Lock()
			clear(s.signatures)
			s.mu.Unlock()
		}
	}()

	s.badSignaturesCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "captcha_replica_attacks_total",
		Help: "Total count of duplicated signatures in requests with captcha",
	})

	return &s
}

// Validate проверяет подлинность CAPTCHA-подписи, декодирует payload, проверяет с помощью Altcha HMAC и сохраняет подпись для предотвращения повторного использования.
//
// Параметры:
//   - payload: base64-encoded CAPTCHA-подпись для проверки.
//
// Возвращает:
//   - bool: true, если подпись валидна, false в противном случае.
func (c *CaptchaSignatures) Validate(payload string) bool {
	if cfg.CaptchaDisabled {
		return true
	}

	decodedPayload, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		slog.Error("Decode altcha payload", "err", err)
		return false
	}

	var m altcha.Payload
	if err := json.Unmarshal(decodedPayload, &m); err != nil {
		slog.Error("Unmarshal altcha payload", "err", err)
		return false
	}

	verified, err := altcha.VerifySolution(m, AltchaHMACKey, true)
	if err != nil || !verified {
		return false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.signatures[m.Signature]; ok {
		c.badSignaturesCounter.Inc()
		return false
	}

	c.signatures[m.Signature] = struct{}{}

	return true
}
