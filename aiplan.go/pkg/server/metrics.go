// Сервер метрик Prometheus.
package server

import (
	"errors"
	"net/http"
	"time"

	"github.com/labstack/echo-contrib/echoprometheus"
	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus"
)

// startMetricsServer поднимает отдельный echo с /metrics. Блокирует до остановки.
func startMetricsServer(addr string) error {
	bootTimeGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "aiplan",
		Name:      "boot_time",
		Help:      "Server startup time",
	})
	bootTimeGauge.Set(float64(time.Now().UnixMilli()))

	if err := prometheus.Register(bootTimeGauge); err != nil {
		return err
	}

	metrics := echo.New()
	metrics.HideBanner = true
	metrics.GET("/metrics", echoprometheus.NewHandler())
	if err := metrics.Start(addr); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
