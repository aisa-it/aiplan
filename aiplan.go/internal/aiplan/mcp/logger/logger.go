package logger

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"runtime"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/apierrors"
	"github.com/mark3labs/mcp-go/mcp"
)

func Error(err error, hints ...string) *mcp.CallToolResult {
	if customErr, ok := err.(apierrors.DefinedError); ok {
		return customErr.MCPError(hints...)
	}
	slog.Error("MCP internal error", "file", getCallerFile(), "err", err)
	return mcp.NewToolResultError("internal error")
}

func getCallerFile() slog.Attr {
	_, path, no, ok := runtime.Caller(2)
	if !ok {
		return slog.Attr{}
	}
	_, file := filepath.Split(path)
	return slog.String("caller", fmt.Sprintf("%s:%d", file, no))
}
