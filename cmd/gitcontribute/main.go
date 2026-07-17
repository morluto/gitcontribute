package main

import (
	"context"
	"errors"
	"fmt"
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

const version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger := gitlog.New("main")

	// Generate a trace ID for this invocation so all log lines from the
	// same command run can be correlated.
	traceID := uuid.NewString()
	logger.InfoContext(ctx, "starting",
		"version", version,
		"trace_id", traceID,
		"args", os.Args[1:],
	)
	ctx = gitlog.WithTrace(ctx, traceID)

	paths := config.NewPaths(nil)
	svc, err := app.New(paths, version, logger.With("component", "app"))
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
		var ce *cli.CLIError
		if errors.As(err, &ce) {
			logger.ErrorContext(ctx, "command failed",
				"error", ce.Error(),
				"code", ce.Code,
				"trace_id", traceID,
			)
			fmt.Fprintln(os.Stderr, ce.Error())
			os.Exit(ce.Code)
		}
		logger.ErrorContext(ctx, "command failed",
			"error", err,
			"trace_id", traceID,
		)
		fmt.Fprintln(os.Stderr, err)
		os.Exit(ExitGeneral)
	}

	logger.InfoContext(ctx, "command completed", "trace_id", traceID)
}

const ExitGeneral = 1
