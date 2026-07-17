package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/morluto/gitcontribute/internal/app"
	"github.com/morluto/gitcontribute/internal/cli"
	"github.com/morluto/gitcontribute/internal/config"
	"github.com/morluto/gitcontribute/internal/tui"
)

const version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	paths := config.NewPaths(nil)
	svc, err := app.New(paths, version)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(ExitGeneral)
	}
	defer func() { _ = svc.Close() }()

	c := cli.New(svc, svc.NewMCPRunner(), os.Stdout, os.Stderr)
	c.SetTUIRunner(tui.NewRunner(svc, os.Stdin, os.Stdout))
	if err := c.Run(ctx, os.Args[1:]); err != nil {
		var ce *cli.CLIError
		if errors.As(err, &ce) {
			fmt.Fprintln(os.Stderr, ce.Error())
			os.Exit(ce.Code)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(ExitGeneral)
	}
}

const ExitGeneral = 1
