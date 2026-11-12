package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	if os.Getenv("KAFKA_BROKERS") == "" && os.Getenv("NATS_URL") == "" {
		log.Fatal("KAFKA_BROKERS or NATS_URL is required")
	}
	if os.Getenv("DATABASE_URL") == "" {
		log.Fatal("DATABASE_URL is required")
	}
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

// Run the existing services together for hosts that limit the number of containers.
func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	names := []string{"bot", "scores", "game"}
	exits := make(chan error, len(names))
	started := 0
	var result error
	for _, name := range names {
		cmd := exec.CommandContext(ctx, name)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
		cmd.WaitDelay = 5 * time.Second
		if err := cmd.Start(); err != nil {
			result = fmt.Errorf("start %s: %w", name, err)
			break
		}
		started++
		go func() {
			if err := cmd.Wait(); err != nil {
				exits <- fmt.Errorf("%s exited: %w", name, err)
			} else {
				exits <- fmt.Errorf("%s exited unexpectedly", name)
			}
		}()
	}

	if result == nil {
		select {
		case <-ctx.Done():
		case result = <-exits:
			started--
		}
	}
	cancel()
	for range started {
		<-exits
	}
	return result
}
