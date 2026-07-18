// Package log provides structured logging for gitcontribute using log/slog.
// Each package receives a pre-configured *slog.Logger at construction time.
// Logs are written to stderr; stdout is reserved for CLI program output.
package log

import (
	"context"
	"log/slog"
	"os"
)

// New creates a structured logger for the given component name.
// The component name is attached to every log line as an attribute.
// In production (non-tty), output is JSON. In development (tty), a human-readable
// text handler is used.
func New(component string) *slog.Logger {
	handler := handlerForEnv()
	return slog.New(handler).With("component", component)
}

func handlerForEnv() slog.Handler {
	opts := &slog.HandlerOptions{
		// Normal CLI output is rendered by the command adapters. Operational
		// INFO events are opt-in so interactive prompts are not preceded by
		// timestamped implementation logs.
		Level: slog.LevelWarn,
	}

	if level := os.Getenv("GITCONTRIBUTE_LOG_LEVEL"); level != "" {
		switch level {
		case "debug":
			opts.Level = slog.LevelDebug
		case "info":
			opts.Level = slog.LevelInfo
		case "warn":
			opts.Level = slog.LevelWarn
		case "error":
			opts.Level = slog.LevelError
		}
	}

	format := os.Getenv("GITCONTRIBUTE_LOG_FORMAT")
	if format == "json" {
		return slog.NewJSONHandler(os.Stderr, opts)
	}

	// Default to text handler which is more readable in terminal output.
	return slog.NewTextHandler(os.Stderr, opts)
}

// RedactedString wraps a potentially sensitive string so that it is redacted
// when logged. The LogValue method ensures only a safe prefix is emitted.
type RedactedString string

// LogValue implements slog.LogValuer to emit a redacted representation.
func (s RedactedString) LogValue() slog.Value {
	if len(s) <= 8 {
		return slog.StringValue("[redacted]")
	}
	return slog.StringValue(string(s)[:4] + "...[redacted]")
}

// WithTrace adds a trace ID to the context for correlation across log lines.
// Use log/slog's built-in context propagation via *Context methods.
func WithTrace(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceContextKey{}, traceID)
}

type traceContextKey struct{}

// TraceFromContext extracts a trace ID from the context for logging.
func TraceFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(traceContextKey{}).(string); ok {
		return v
	}
	return ""
}
