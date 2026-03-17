// Управление конфигурацией приложения через переменные окружения.
package config

import (
	"net/url"
	"os"
	"strconv"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
)

// Exist - возвращает true, если глобальная переменная key существует, иначе false
func Exist(key string) bool {
	_, exist := os.LookupEnv(key)
	return exist
}

// GetEnv - возвращает содержимое глобальной строковой переменной.
func GetEnv(key string) string {
	val, _ := os.LookupEnv(key)
	return val
}

// GetIntEnv - возвращает содержимое глобальной числовой переменной. Если возникла ошибка при обработке, возвращается 0
func GetIntEnv(key string) int {
	val, _ := os.LookupEnv(key)
	v, err := strconv.Atoi(val)
	if err != nil {
		return 0
	}
	return v
}

// GetBoolEnv - возвращает содержимое глобальной логической переменной. Если возникла ошибка при обработке, возвращается false
func GetBoolEnv(key string) bool {
	val, _ := os.LookupEnv(key)
	v, err := strconv.ParseBool(val)
	if err != nil {
		return false
	}
	return v
}

func GetURLEnv(key string) types.JsonURL {
	val, _ := os.LookupEnv(key)
	u, err := url.Parse(val)
	if err != nil {
		return types.JsonURL{}
	}
	return types.JsonURL{URL: u}
}
