package stack_error

import (
	"errors"
	"fmt"
	"github.com/labstack/echo/v4"
	"log/slog"
	"path/filepath"
	"runtime"
)

type TrackerError struct {
	Context  map[string]any
	ErrStack []slog.Attr
	cause    error
}

func TrackErrorStack(err error) *TrackerError {
	var te *TrackerError
	if errors.As(err, &te) {
		te.ErrStack = append(te.ErrStack, getCallerFile(err))
		return te
	}

	newTe := newTrackError(err)
	newTe.ErrStack = append(newTe.ErrStack, getCallerFile(err))
	return newTe
}

func newTrackError(err error) *TrackerError {
	return &TrackerError{
		Context:  make(map[string]any),
		ErrStack: make([]slog.Attr, 0),
		cause:    err,
	}
}

func (te *TrackerError) AddContext(k string, v any) *TrackerError {
	if _, ok := te.Context[k]; !ok {
		te.Context[k] = v
	}
	return te
}

func GetError(c echo.Context, err error) {
	var trackerError *TrackerError
	var attrs []any

	if errors.As(err, &trackerError) {
		trackerError.traceOut()
		attrs = trackerError.getAttrs()
	} else {
		attrs = []any{slog.String("raw_error", err.Error())}
	}

	if c != nil {
		attrs = append(attrs,
			slog.String("method", c.Request().Method),
			slog.String("url", c.Request().URL.String()))
	}

	slog.With(attrs...).Error("stack error")
}

func (te *TrackerError) Error() string {
	if te.cause != nil {
		return te.cause.Error()
	}
	return "TrackerError"
}

func (te *TrackerError) Unwrap() error {
	return te.cause
}

func (te *TrackerError) getAttrs() []any {
	res := make([]any, 0, len(te.Context))
	for k, v := range te.Context {
		res = append(res, slog.Any(k, v))
	}
	return res
}

func (te *TrackerError) traceOut() {
	for _, attr := range te.ErrStack {
		slog.Info("trace:", attr)
	}
}

func getCallerFile(err error) slog.Attr {
	_, path, no, ok := runtime.Caller(2)
	if !ok {
		return slog.String("trace", "unknown")
	}
	_, file := filepath.Split(path)
	return slog.String("trace", fmt.Sprintf("%s:%d %s", file, no, err.Error()))
}

func (te *TrackerError) AddErr(err error) *TrackerError {
	te.ErrStack = append(te.ErrStack, getCallerFile(err))
	return te
}
