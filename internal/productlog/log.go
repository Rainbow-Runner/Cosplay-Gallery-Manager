// Package productlog provides privacy-preserving structured logging for the
// Cosplay Gallery Manager product runtime.
package productlog

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
)

type requestIDKey struct{}

// ParseLevel validates a configured product log level.
func ParseLevel(value string) (slog.Level, error) {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "DEBUG":
		return slog.LevelDebug, nil
	case "INFO":
		return slog.LevelInfo, nil
	case "WARN":
		return slog.LevelWarn, nil
	case "ERROR":
		return slog.LevelError, nil
	default:
		return 0, errors.New("log_level must be DEBUG, INFO, WARN, or ERROR")
	}
}

// Configure installs the process-wide structured logger used by the product
// server. Log attributes must remain limited to technical identifiers and
// stable event/error codes.
func Configure(value string, output io.Writer) error {
	level, err := ParseLevel(value)
	if err != nil {
		return err
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(output, &slog.HandlerOptions{Level: level})))
	return nil
}

// WithRequestID attaches the server-generated request identifier to a context.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, requestID)
}

// RequestID returns the server-generated request identifier, when available.
func RequestID(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey{}).(string)
	return value
}
