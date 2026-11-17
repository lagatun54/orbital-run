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
	postgres, err := db.OpenPostgres(startup, config.Env("DATABASE_URL", "postgres://orbital:orbital@127.0.0.1:5432/orbital?sslmode=disable"))
	if err != nil {
		return err
	}
	defer postgres.Close()

	var cache scores.LeaderboardCache
	if url := config.Env("REDIS_URL", ""); url != "" {
		redis, err := db.NewRedisLeaderboardCache(url)
		if err != nil {
			return err
		}
		defer redis.Close()
		cache = redis
	}
	store := scores.NewStore(postgres, cache)

	consumer, err := broker.OpenConsumer(startup)
	cancel()
	if err != nil {
		return err
	}
	defer consumer.Close()

	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)
	server := &http.Server{
		Addr:              config.Env("SCORES_LISTEN", ":8081"),
		Handler:           api.NewScoresHandler(store, postgres.Ready),
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      5 * time.Second,
		Protocols:         protocols,
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
