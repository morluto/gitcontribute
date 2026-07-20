package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/morluto/gitcontribute/internal/cli"
)

func TestReportCommandErrorPrintsHandledErrorWithoutErrorLog(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn}))
	var stderr bytes.Buffer

	code := reportCommandError(
		context.Background(),
		logger,
		&stderr,
		"trace-test",
		cli.NewCLIError(cli.ExitUsage, errors.New("choose a command")),
	)

	if code != cli.ExitUsage {
		t.Fatalf("exit code = %d, want %d", code, cli.ExitUsage)
	}
	if got := stderr.String(); got != "choose a command\n" {
		t.Fatalf("stderr = %q", got)
	}
	if got := logs.String(); got != "" {
		t.Fatalf("handled error produced default logs: %q", got)
	}
}

func TestReportCommandErrorLogsUnexpectedError(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn}))
	var stderr bytes.Buffer

	code := reportCommandError(context.Background(), logger, &stderr, "trace-test", errors.New("database unavailable"))

	if code != ExitGeneral {
		t.Fatalf("exit code = %d, want %d", code, ExitGeneral)
	}
	if got := stderr.String(); got != "database unavailable\n" {
		t.Fatalf("stderr = %q", got)
	}
	logOutput := logs.String()
	if !strings.Contains(logOutput, "level=ERROR") || !strings.Contains(logOutput, `msg="command failed"`) || !strings.Contains(logOutput, `error="database unavailable"`) {
		t.Fatalf("unexpected error log = %q", logOutput)
	}
}

func TestLogInvocationStartDoesNotLogArguments(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelInfo}))
	fixturePassword := strings.Join([]string{"fixture", "password"}, "-")
	remote := "https://fixture-user:" + fixturePassword + "@github.com/owner/repo.git"

	logInvocationStart(context.Background(), logger, "trace-test", []string{"workspace", "create", "--remote", remote})

	got := logs.String()
	if strings.Contains(got, fixturePassword) || strings.Contains(got, remote) || strings.Contains(got, "--remote") {
		t.Fatalf("invocation log exposed argument values: %q", got)
	}
	if !strings.Contains(got, "arg_count=4") {
		t.Fatalf("invocation log omitted safe argument count: %q", got)
	}
}
