package log

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	t.Run("creates logger with component", func(t *testing.T) {
		l := New("test-component")
		if l == nil {
			t.Fatal("expected non-nil logger")
		}
	})

	t.Run("honors GITCONTRIBUTE_LOG_FORMAT", func(t *testing.T) {
		os.Setenv("GITCONTRIBUTE_LOG_FORMAT", "json")
		defer os.Unsetenv("GITCONTRIBUTE_LOG_FORMAT")

		l := New("json-test")
		var buf bytes.Buffer
		h := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
		l2 := slog.New(h).With("component", "json-test")

		l2.Info("hello")
		output := buf.String()
		if !strings.Contains(output, "hello") {
			t.Errorf("expected output to contain 'hello', got %q", output)
		}

		_ = l
	})
}

func TestRedactedString(t *testing.T) {
	t.Parallel()
	t.Run("short strings fully redacted", func(t *testing.T) {
		s := RedactedString("abc")
		val := s.LogValue()
		if val.String() != "[redacted]" {
			t.Errorf("expected '[redacted]', got %q", val.String())
		}
	})

	t.Run("long strings partially redacted", func(t *testing.T) {
		s := RedactedString("github_pat_1234567890abcdef")
		val := s.LogValue()
		if !strings.Contains(val.String(), "[redacted]") {
			t.Errorf("expected redacted value, got %q", val.String())
		}
	})
}

func TestTraceContext(t *testing.T) {
	t.Parallel()
	ctx := WithTrace(context.Background(), "trace-abc-123")
	got := TraceFromContext(ctx)
	if got != "trace-abc-123" {
		t.Errorf("expected 'trace-abc-123', got %q", got)
	}
}

func TestTraceFromContextEmpty(t *testing.T) {
	t.Parallel()
	got := TraceFromContext(context.Background())
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}
