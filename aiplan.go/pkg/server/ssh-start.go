// Запуск встроенного SSH-сервера для Git.
package server

import (
	"context"
	"log/slog"
)

// startSSHServer поднимает SSH-сервер, если git и ssh включены в конфиге.
// Ошибка запуска не фатальна: сервер продолжает работу без SSH.
func (srv *Server) startSSHServer(ctx context.Context) {
	db := srv.services.db
	var sshServer *SSHServer

	if cfg.GitEnabled && cfg.SSHEnabled {
		sshServerConfig := SSHServerConfig{
			DB:               db,
			GitReposPath:     cfg.GitRepositoriesPath,
			Host:             cfg.SSHHost,
			Port:             cfg.SSHPort,
			HostKeyPath:      cfg.SSHHostKeyPath,
			RateLimitEnabled: cfg.SSHRateLimitEnabled,
		}

		var err error
		sshServer, err = NewSSHServer(sshServerConfig)
		if err != nil {
			slog.Error("Failed to create SSH server", "err", err)
		} else {
			// Запускаем SSH сервер в отдельной горутине
			go func() {
				if err := sshServer.Start(ctx); err != nil {
					slog.Error("SSH server error", "err", err)
				}
			}()

			slog.Info("SSH server started successfully",
				"host", cfg.SSHHost,
				"port", cfg.SSHPort)
		}
	}

	srv.sshServer = sshServer
}
