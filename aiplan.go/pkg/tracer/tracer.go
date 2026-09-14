package tracer

import (
	"context"
	"log/slog"
	"os"
	"time"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"gorm.io/gorm"
	"gorm.io/plugin/opentelemetry/tracing"
)

type Config struct {
	Version     string
	Endpoint    string
	Token       string
	SampleRatio float64
}

type StopFn func(context.Context) error

func Init(ctx context.Context, cfg *Config, db *gorm.DB) (StopFn, error) {
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		slog.Default().Warn("OTEL export error", "err", err)
	}))

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName("aiplan"),
			semconv.ServiceVersion(cfg.Version),
		),
		resource.WithProcess(), resource.WithHost(), resource.WithFromEnv(),
	)
	if err != nil {
		return nil, err
	}

	closeTracer, err := InitTracer(ctx, cfg, res)
	if err != nil {
		return nil, err
	}
	closeLogs, err := InitLogs(ctx, cfg, res)
	if err != nil {
		return nil, err
	}

	db.Use(tracing.NewPlugin(
		tracing.WithoutMetrics(),
		tracing.WithDBSystem("aiplan-db"),
		tracing.WithoutQueryVariables(),
	))

	return func(ctx context.Context) error {
		err := closeTracer(ctx)
		err2 := closeLogs(ctx)
		if err != nil {
			return err
		}
		return err2
	}, nil
}

func InitTracer(ctx context.Context, cfg *Config, res *resource.Resource) (StopFn, error) {
	exporter, err := otlptracehttp.New(
		ctx,
		otlptracehttp.WithEndpoint(cfg.Endpoint),
		otlptracehttp.WithHeaders(map[string]string{
			"Authorization": "Bearer " + cfg.Token,
		}),
		otlptracehttp.WithCompression(otlptracehttp.GzipCompression),
		otlptracehttp.WithURLPath("/api/otel/v1/traces"),
	)
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter,
			sdktrace.WithMaxQueueSize(4096),
			sdktrace.WithMaxExportBatchSize(512),
			sdktrace.WithBatchTimeout(5*time.Second)),
		sdktrace.WithSampler(sdktrace.ParentBased(
			sdktrace.TraceIDRatioBased(cfg.SampleRatio),
		)),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))

	return tp.Shutdown, nil
}

func InitLogs(ctx context.Context, cfg *Config, res *resource.Resource) (StopFn, error) {
	exporter, err := otlploghttp.New(ctx,
		otlploghttp.WithEndpoint(cfg.Endpoint),
		otlploghttp.WithHeaders(map[string]string{"Authorization": "Bearer " + cfg.Token}),
		otlploghttp.WithCompression(otlploghttp.GzipCompression),
		otlploghttp.WithURLPath("/api/otel/v1/logs"),
	)
	if err != nil {
		return nil, err
	}

	lp := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exporter,
			sdklog.WithMaxQueueSize(4096),
			sdklog.WithExportInterval(5*time.Second),
		)),
		sdklog.WithResource(res),
	)
	global.SetLoggerProvider(lp)

	stdoutHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := otelslog.NewLogger("aiplan",
		otelslog.WithLoggerProvider(lp),
	)

	slog.SetDefault(slog.New(slog.NewMultiHandler(logger.Handler(), stdoutHandler)))

	return lp.Shutdown, nil
}
