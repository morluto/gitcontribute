package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/google/uuid"
	"github.com/morluto/gitcontribute/internal/app"
	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/config"
	gitlog "github.com/morluto/gitcontribute/internal/log"
	"github.com/morluto/gitcontribute/internal/tui"
)

// version is replaced from the release tag with -ldflags.
var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger := gitlog.New("main")

	// Generate a trace ID for this invocation so all log lines from the
	// same command run can be correlated.
	traceID := uuid.NewString()
	logInvocationStart(ctx, logger, traceID, os.Args[1:])
	ctx = gitlog.WithTrace(ctx, traceID)

	paths := config.NewPaths(nil)
	svc, err := app.NewWithContext(ctx, paths, version, logger.With("component", "app"))
	if err != nil {
		logger.ErrorContext(ctx, "failed to initialize application", "error", err)
		fmt.Fprintln(os.Stderr, err)
		os.Exit(ExitGeneral)
	}
	defer func() {
		if err := svc.Close(); err != nil {
			logger.ErrorContext(ctx, "error during shutdown", "error", err)
		}
	}()

	c := cli.New(svc, svc.NewMCPRunner(), os.Stdout, os.Stderr)
	c.SetLogger(logger.With("component", "cli"))
	c.SetTUIRunner(tui.NewRunner(svc, os.Stdin, os.Stdout))
	if err := c.Run(ctx, os.Args[1:]); err != nil {
		os.Exit(reportCommandError(ctx, logger, os.Stderr, traceID, err))
	}

	logger.InfoContext(ctx, "command completed", "trace_id", traceID)
}

const ExitGeneral = 1

func logInvocationStart(ctx context.Context, logger *slog.Logger, traceID string, args []string) {
	logger.InfoContext(ctx, "starting",
		"version", version,
		"trace_id", traceID,
		"arg_count", len(args),
	)
}

func reportCommandError(ctx context.Context, logger *slog.Logger, stderr io.Writer, traceID string, err error) int {
	var cliErr *cli.CLIError
	if errors.As(err, &cliErr) {
		logger.DebugContext(ctx, "command failed",
			"error", cliErr.Error(),
			"code", cliErr.Code,
			"trace_id", traceID,
		)
		if _, writeErr := fmt.Fprintln(stderr, cliErr.Error()); writeErr != nil {
			logger.ErrorContext(ctx, "failed to write command error", "error", writeErr, "trace_id", traceID)
		}
		return cliErr.Code
	}

	logger.ErrorContext(ctx, "command failed",
		"error", err,
		"trace_id", traceID,
	)
	if _, writeErr := fmt.Fprintln(stderr, err); writeErr != nil {
		logger.ErrorContext(ctx, "failed to write command error", "error", writeErr, "trace_id", traceID)
	}
	return ExitGeneral
}
