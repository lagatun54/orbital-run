package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"syscall"
	"time"
	"unicode/utf8"

	"orbitalrun/internal/arena"
	"orbitalrun/internal/broker"
	"orbitalrun/internal/env"
)

var matchID = regexp.MustCompile(`^[A-Z2-7]{26}$`)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	startup, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	scores, err := openStore(startup, env.Get("DATABASE_URL", "postgres://orbital:orbital@127.0.0.1:5432/orbital?sslmode=disable"))
	if err != nil {
		return err
	}
	defer scores.Close()

	consumer, err := broker.OpenConsumer(startup)
	cancel()
	if err != nil {
		return err
	}
	defer consumer.Close()

	server := &http.Server{
		Addr: env.Get("SCORES_LISTEN", ":8081"), Handler: routes(scores),
		ReadHeaderTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second,
		BaseContext: func(net.Listener) context.Context { return ctx },
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	consumed := make(chan error, 1)
	go func() {
		consumed <- consumer.Run(ctx, func(ctx context.Context, payload []byte) error {
			return consumeResult(ctx, scores, payload)
		})
		stop()
	}()
	log.Printf("scores listening on %s", server.Addr)
	err = server.ListenAndServe()
	stop()
	consumeErr := <-consumed
	if errors.Is(err, http.ErrServerClosed) {
		return consumeErr
	}
	return err
}

func consumeResult(ctx context.Context, scores *store, payload []byte) error {
	var result arena.Result
	if err := json.Unmarshal(payload, &result); err != nil || !matchID.MatchString(result.ID) ||
		utf8.RuneCountInString(result.Name) < 1 || utf8.RuneCountInString(result.Name) > 20 ||
		result.Score < 0 || result.BotScore < 0 || result.FinishedAt.IsZero() {
		log.Print("discard invalid match result")
		return nil
	}
	result.Local = false
	return scores.Save(ctx, result)
}
