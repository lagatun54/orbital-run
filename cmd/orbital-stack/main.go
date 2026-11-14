package main

import (
	"context"
	"errors"
	"os"

	"orbitalrun/internal/command"
	"orbitalrun/internal/stack"
)

func main() {
	command.Main(run)
}

func run(ctx context.Context) error {
	if os.Getenv("KAFKA_BROKERS") == "" && os.Getenv("NATS_URL") == "" {
		return errors.New("KAFKA_BROKERS or NATS_URL is required")
	}
	if os.Getenv("DATABASE_URL") == "" {
		return errors.New("DATABASE_URL is required")
	}

	return stack.Run(ctx, "scores", "game")
}
