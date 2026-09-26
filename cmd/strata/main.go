package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/umuttopalak/strata/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	err := cli.Execute(ctx)
	switch {
	case errors.Is(err, context.Canceled):
		os.Exit(130) // interrupted: the player already restored the terminal
	case err != nil:
		fmt.Fprintln(os.Stderr, "strata:", err)
		os.Exit(1)
	}
}
