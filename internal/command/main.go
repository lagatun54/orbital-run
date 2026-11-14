package command

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
)

// Main runs a command until it returns or receives an interrupt or SIGTERM.
func Main(run func(context.Context) error) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx)
	stop()
	if err != nil {
		log.Fatal(err)
	}
}
