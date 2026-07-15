// Package logging routes all logs (ours and the Databricks SDK's) to a file.
// Nothing may ever write to stdout/stderr while the TUI owns the terminal.
package logging

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
	sdklogger "github.com/databricks/databricks-sdk-go/logger"
)

// Setup opens the log file under the XDG state directory, installs it as the
// slog default, and bridges the Databricks SDK logger into slog. It returns
// the log file path and a close function.
func Setup(level string) (string, func() error, error) {
	path, err := xdg.StateFile(filepath.Join("lazydbx", "lazydbx.log"))
	if err != nil {
		return "", nil, fmt.Errorf("resolving state dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return "", nil, fmt.Errorf("opening log file: %w", err)
	}

	lvl := ParseLevel(level)
	handler := slog.NewTextHandler(f, &slog.HandlerOptions{Level: lvl})
	slog.SetDefault(slog.New(handler))
	sdklogger.DefaultLogger = &slogBridge{level: lvl}

	return path, f.Close, nil
}

// ParseLevel maps a config string to a slog level, defaulting to info.
func ParseLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// slogBridge adapts the Databricks SDK logger interface onto slog so SDK
// traffic lands in the same file instead of stderr.
type slogBridge struct {
	level slog.Level
}

func sdkToSlog(level sdklogger.Level) slog.Level {
	switch {
	case level <= sdklogger.LevelDebug:
		return slog.LevelDebug
	case level <= sdklogger.LevelInfo:
		return slog.LevelInfo
	case level <= sdklogger.LevelWarn:
		return slog.LevelWarn
	default:
		return slog.LevelError
	}
}

func (b *slogBridge) Enabled(_ context.Context, level sdklogger.Level) bool {
	return sdkToSlog(level) >= b.level
}

func (b *slogBridge) logf(ctx context.Context, level slog.Level, format string, v ...any) {
	slog.Default().Log(ctx, level, fmt.Sprintf(format, v...), slog.String("source", "databricks-sdk"))
}

func (b *slogBridge) Tracef(ctx context.Context, format string, v ...any) {
	b.logf(ctx, slog.LevelDebug, format, v...)
}

func (b *slogBridge) Debugf(ctx context.Context, format string, v ...any) {
	b.logf(ctx, slog.LevelDebug, format, v...)
}

func (b *slogBridge) Infof(ctx context.Context, format string, v ...any) {
	b.logf(ctx, slog.LevelInfo, format, v...)
}

func (b *slogBridge) Warnf(ctx context.Context, format string, v ...any) {
	b.logf(ctx, slog.LevelWarn, format, v...)
}

func (b *slogBridge) Errorf(ctx context.Context, format string, v ...any) {
	b.logf(ctx, slog.LevelError, format, v...)
}
