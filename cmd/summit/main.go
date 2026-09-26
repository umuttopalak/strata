package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/umuttopalak/summit/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := cli.Execute(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "summit:", err)
		os.Exit(1)
	}
}
