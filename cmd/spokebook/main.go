package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spokebook/spokebook/internal/app/spokebook"
)

func main() {
	log, err := spokebook.NewLogger(os.Stderr)
	if err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := spokebook.Run(ctx, log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}
