package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/morluto/gitcontribute/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	c := cli.New(cli.NewBootstrapService(), cli.NewBootstrapMCPRunner(), os.Stdout, os.Stderr)
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
