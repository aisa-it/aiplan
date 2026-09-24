package server

import (
	"net/http"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/dto"
	"github.com/labstack/echo/v4"
)

// getVersion godoc
// @id getVersion
// @Summary Система: версия и флаги сборки
// @Description Версия сервера, включённые возможности и названия сущностей от подключённого движка. Без авторизации.
// @Tags System
// @Produce json
// @Success 200 {object} dto.VersionInfo "Версия и флаги"
// @Router /api/version [get]
func (s *Services) getVersion(c echo.Context) error {
	return c.JSON(http.StatusOK, dto.VersionInfo{
		Version:     s.version,
		SignUp:      s.cfg.SignUpEnable,
		Demo:        s.cfg.Demo,
		NY:          s.cfg.NYEnable,
		Captcha:     !s.cfg.CaptchaDisabled,
		Jitsi:       !s.cfg.JitsiDisabled,
		EntityNames: s.entityNames,
	})
}
