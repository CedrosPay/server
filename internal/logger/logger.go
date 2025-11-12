package logger

import (
	"context"
	"io"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

// contextKey is the key for logger in context.
type contextKey string

const (
	loggerKey    contextKey = "logger"
	requestIDKey contextKey = "request_id"
)

// Config holds logger configuration.
type Config struct {
	Level       string // debug, info, warn, error
	Format      string // json, console
	Service     string
	Version     string
	Environment string
}

// New creates a new global logger with default configuration.
func New(cfg Config) zerolog.Logger {
	// Parse log level
	level := parseLevel(cfg.Level)
	zerolog.SetGlobalLevel(level)

	// Configure output
	var output io.Writer = os.Stdout
	if cfg.Format == "console" {
		output = zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
			NoColor:    false,
		}
	}

	// Create base logger with global fields
	logger := zerolog.New(output).With().
		Timestamp().
		Str("service", cfg.Service).
		Str("version", cfg.Version).
		Str("environment", cfg.Environment).
		Logger()

	return logger
}

// WithContext adds logger to context for retrieval in handlers.
func WithContext(ctx context.Context, logger zerolog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

// FromContext retrieves logger from context or returns global logger.
func FromContext(ctx context.Context) zerolog.Logger {
	if ctx == nil {
		return zerolog.Nop()
	}

	if logger, ok := ctx.Value(loggerKey).(zerolog.Logger); ok {
		return logger
	}

	// Fallback to disabled logger if context has no logger
	return zerolog.Nop()
}

// WithRequestID adds request ID to context for tracing.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey, requestID)
}

// GetRequestID retrieves request ID from context.
func GetRequestID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if requestID, ok := ctx.Value(requestIDKey).(string); ok {
		return requestID
	}
	return ""
}

// parseLevel converts string level to zerolog.Level.
func parseLevel(level string) zerolog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return zerolog.DebugLevel
	case "info":
		return zerolog.InfoLevel
	case "warn", "warning":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	case "fatal":
		return zerolog.FatalLevel
	case "panic":
		return zerolog.PanicLevel
	default:
		return zerolog.InfoLevel
	}
}

// TruncateAddress truncates wallet/signature for privacy (show first 8 + last 4 chars).
func TruncateAddress(addr string) string {
	if len(addr) <= 12 {
		return addr
	}
	return addr[:8] + "..." + addr[len(addr)-4:]
}

// RedactEmail masks email for PII compliance (keep domain for analytics).
func RedactEmail(email string) string {
	if email == "" {
		return ""
	}
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return "[redacted]"
	}
	// Show first 2 chars + domain
	username := parts[0]
	if len(username) > 2 {
		username = username[:2] + "***"
	} else {
		username = "***"
	}
	return username + "@" + parts[1]
}
