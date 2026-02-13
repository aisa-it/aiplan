// Package config загружает конфигурацию приложения из переменных окружения.
//
// Использует рефлексию для автоматического маппинга env-переменных на поля
// структуры Config по тегу `env:"VAR_NAME"`. Поддерживает типы:
// string, int, bool, *url.URL.
//
// Особенности:
//   - Обязательные переменные (WEB_URL) — приложение не запустится без них
//   - Значения по умолчанию для опциональных параметров
//   - Автоматическая маскировка секретов в логах (password, secret, token)
//   - Валидация значений (например, NotificationsSleep: 1-59 минут)
//
// Все параметры документированы в .env.example и README проекта.
package config

import (
	"log/slog"
	"net/url"
	"os"
	"reflect"
	"strings"
)

type Config struct {
	SecretKey string `env:"SECRET_KEY"`

	AWSRegion     string `env:"AWS_REGION"`
	AWSAccessKey  string `env:"AWS_ACCESS_KEY_ID"`
	AWSSecretKey  string `env:"AWS_SECRET_ACCESS_KEY"`
	AWSEndpoint   string `env:"AWS_S3_ENDPOINT_URL"`
	AWSBucketName string `env:"AWS_S3_BUCKET_NAME"`

	AssetsPath string `env:"ASSETS_PATH"`

	DatabaseDSN string `env:"DATABASE_URL"`

	DefaultUserEmail string `env:"DEFAULT_EMAIL"`

	EmailActivityDisabled bool   `env:"EMAIL_ACTIVITY_DISABLED"`
	EmailHost             string `env:"EMAIL_HOST"`
	EmailUser             string `env:"EMAIL_HOST_USER"`
	EmailPassword         string `env:"EMAIL_HOST_PASSWORD"`
	EmailPort             int    `env:"EMAIL_PORT"`
	EmailFrom             string `env:"EMAIL_FROM"`
	EmailWorkers          int    `env:"EMAIL_WORKERS"`

	WebURL *url.URL `env:"WEB_URL"`

	JitsiDisabled  bool     `env:"JITSI_DISABLED"`
	JitsiURL       *url.URL `env:"JITSI_URL"`
	JitsiJWTSecret string   `env:"JITSI_JWT_SECRET"`
	JitsiAppID     string   `env:"JITSI_APP_ID"`

	FrontFilesPath string `env:"FRONT_PATH"`

	NotificationsSleep int `env:"NOTIFICATIONS_PERIOD"`

	TelegramBotToken        string `env:"TELEGRAM_BOT_TOKEN"`
	TelegramCommandsDisable bool   `env:"TELEGRAM_COMMANDS_DISABLED"`

	SessionsDBPath string `env:"SESSIONS_DB_PATH"`

	SignUpEnable  bool `env:"SIGN_UP_ENABLE"`
	Demo          bool `env:"DEMO"`
	SwaggerEnable bool `env:"SWAGGER"`
	NYEnable      bool `env:"NY_ENABLE"`

	CaptchaDisabled bool `env:"CAPTCHA_DISABLED"`

	ExternalLimiter *url.URL `env:"EXTERNAL_LIMITER_URL"`
	ExternalMemDB   *url.URL `env:"EXTERNAL_MEMDB"`

	GitEnabled          bool   `env:"GIT_ENABLED"`
	GitRepositoriesPath string `env:"GIT_REPOSITORIES_PATH"`

	// SSH Git server configuration
	SSHEnabled          bool   `env:"SSH_ENABLED"`
	SSHHost             string `env:"SSH_HOST"`
	SSHPort             int    `env:"SSH_PORT"`
	SSHHostKeyPath      string `env:"SSH_HOST_KEY_PATH"`
	SSHRateLimitEnabled bool   `env:"SSH_RATE_LIMIT_ENABLED"`

	// LDAP configuration
	LDAPServerURL    *url.URL `env:"LDAP_URL"`
	LDAPBaseDN       string   `env:"LDAP_BASE_DN"`
	LDAPBindUser     string   `env:"LDAP_BIND_DN"`
	LDAPBindPassword string   `env:"LDAP_BIND_PASSWORD"`
	LDAPFilter       string   `env:"LDAP_FILTER"`
	LDAPForce        bool     `env:"LDAP_FORCE"`

	MCPEnabled bool `env:"MCP_ENABLED"`
}

// ReadConfig загружает конфигурацию приложения из переменных окружения и выполняет валидацию. Возвращает структуру Config с загруженными параметрами. Если WebURL не задан, приложение завершает работу с ошибкой.  Обязательные переменные валидируются, типы данных преобразуются из строк, а секретные значения маскируются в логах.  Также обрабатываются ошибки при парсинге URL и предоставляются значения по умолчанию для некоторых параметров. Ограничение значений для некоторых параметров (например, NotificationsSleep, EmailWorkers) также выполняется в этой функции.  Возвращает указатель на структуру Config, заполненную данными из переменных окружения и обработанную в соответствии с заданными правилами.
func ReadConfig() *Config {
	config := &Config{}

	envConfig("env", config)

	// Check required envs
	if config.WebURL == nil {
		slog.Error("WEB_URL is required")
		os.Exit(1)
	}

	if config.NotificationsSleep <= 0 || config.NotificationsSleep > 59 {
		config.NotificationsSleep = 5
	}

	if config.EmailWorkers <= 0 {
		config.EmailWorkers = 5
	}
	if config.LDAPServerURL != nil && config.LDAPFilter == "" {
		config.LDAPFilter = "(&(uniqueIdentifier={email}))"
	}

	return config
}

// Присваивает полям в переданной структуре значения переменных. Название переменной для каждого поля лежит в теге этого поля.
func envConfig(key string, s any) {
	v := reflect.ValueOf(s).Elem()
	typeParam := v.Type()
	for i := 0; i < v.NumField(); i++ {
		fName := typeParam.Field(i).Name
		fEnvTag := typeParam.Field(i).Tag.Get(key)

		if !Exist(fEnvTag) {
			continue
		}

		logValue := GetEnv(fEnvTag)
		if logValue == "" {
			continue
		}

		// Secure passwords in log
		if strings.Contains(strings.ToLower(fName), "pass") || strings.Contains(strings.ToLower(fName), "secret") || strings.Contains(strings.ToLower(fName), "token") {
			pass := strings.Split(GetEnv(fEnvTag), "")
			logValue = pass[0]
			for i := 1; i < len(pass)-1; i++ {
				logValue += "*"
			}
			logValue += pass[len(pass)-1]

		} else if u, err := url.Parse(GetEnv(fEnvTag)); err == nil {
			if _, ok := u.User.Password(); ok {
				u.User = url.UserPassword(u.User.Username(), "SECRET")
			}
			logValue = u.String()
		}

		slog.Info("Set config value",
			slog.String("key", typeParam.Name()+"."+fName),
			slog.String("value", logValue),
			slog.String("source", "ENVIRONMENT"),
		)

		switch v.Field(i).Interface().(type) {
		case string:
			v.Field(i).SetString(GetEnv(fEnvTag))
		case int:
			v.Field(i).SetInt(int64(GetIntEnv(fEnvTag)))
		case bool:
			v.Field(i).SetBool(GetBoolEnv(fEnvTag))
		case *url.URL:
			v.Field(i).Set(reflect.ValueOf(GetURLEnv(fEnvTag)))
		}
	}
}
