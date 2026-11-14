package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"orbitalrun/api"
	"orbitalrun/config"
	"orbitalrun/db"
	"orbitalrun/internal/broker"
	"orbitalrun/internal/command"
	"orbitalrun/internal/httpserver"
	"orbitalrun/internal/scores"
)

func main() {
	command.Main(run)
}

func run(ctx context.Context) error {
	startup, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	store, err := db.OpenPostgres(startup, config.Env("DATABASE_URL", "postgres://orbital:orbital@127.0.0.1:5432/orbital?sslmode=disable"))
	if err != nil {
		return err
	}
	defer store.Close()

	consumer, err := broker.OpenConsumer(startup)
	cancel()
	if err != nil {
		return err
	}
	defer consumer.Close()

	server := &http.Server{
		Addr:              config.Env("SCORES_LISTEN", ":8081"),
		Handler:           api.NewScoresHandler(store),
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      5 * time.Second,
	}
	ctx, stop := context.WithCancel(ctx)
	defer stop()
	consumed := make(chan error, 1)
	go func() {
		err := consumer.Run(ctx, scores.NewResultConsumer(store))
		if err == nil && ctx.Err() == nil {
			err = errors.New("result consumer stopped unexpectedly")
		} else if err != nil {
			err = fmt.Errorf("consume match results: %w", err)
		}
		consumed <- err
		stop()
	}()

	log.Printf("scores listening on %s", server.Addr)
	serveErr := httpserver.Run(ctx, server)
	stop()
	return errors.Join(serveErr, <-consumed)
}
