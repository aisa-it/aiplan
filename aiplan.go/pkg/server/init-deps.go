// Инициализация внешних зависимостей сервера: трассировка, memDB, LDAP.
package server

import (
	"context"
	"fmt"
	"log/slog"

	authprovider "github.com/aisa-it/aiplan/aiplan.go/pkg/auth-provider"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/config"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/tracer"

	mem "github.com/aisa-it/aiplan-mem/api"
	"gorm.io/gorm"
)

// initTracer поднимает OTEL-трассировку, если задан эндпоинт.
// Возвращает nil-функцию остановки, когда трассировка выключена.
func initTracer(ctx context.Context, c *config.Config, db *gorm.DB, version string) (tracer.StopFn, error) {
	if c.OTELEndpoint == "" {
		return nil, nil
	}
	slog.Info("Start OTEL tracer")
	stop, err := tracer.Init(ctx, &tracer.Config{
		Endpoint:    c.OTELEndpoint,
		Token:       c.OTELToken,
		Version:     version,
		SampleRatio: c.OTELSampleRate,
	}, db)
	if err != nil {
		return nil, fmt.Errorf("init OTEL tracer: %w", err)
	}
	return stop, nil
}

// initMemDB подключается к AIPlan MemDB: встроенный модуль или внешний сервис.
func initMemDB(c *config.Config) (*mem.AIPlanMemAPI, error) {
	memDBConn := "session.db"
	if c.ExternalMemDB.URL != nil {
		memDBConn = c.ExternalMemDB.URL.String()
	}
	memDB, err := mem.NewClient(c.ExternalMemDB.URL == nil, memDBConn)
	if err != nil {
		return nil, fmt.Errorf("connect to AIPlan MemDB: %w", err)
	}
	return memDB, nil
}

// initLDAP создаёт LDAP-провайдер аутентификации, если он настроен.
func initLDAP(c *config.Config) (*authprovider.LdapProvider, error) {
	if c.LDAPServerURL.URL == nil {
		return nil, nil
	}
	provider, err := authprovider.InitLDAP(
		c.LDAPServerURL.URL,
		c.LDAPBindUser,
		c.LDAPBindPassword,
		c.LDAPBaseDN,
		c.LDAPFilter,
	)
	if err != nil {
		return nil, fmt.Errorf("connect to LDAP server: %w", err)
	}
	return provider, nil
}
