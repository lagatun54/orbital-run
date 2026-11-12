package main

import (
	"context"
	_ "embed"
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

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go/jetstream"
	"orbitalrun/internal/arena"
	"orbitalrun/internal/broker"
	"orbitalrun/internal/env"
)

//go:embed schema.sql
var schema string

var matchID = regexp.MustCompile(`^[A-Z2-7]{26}$`)

type rankedResult struct {
	arena.Result
	Rank int64 `json:"rank"`
}

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
	db, err := pgxpool.New(startup, env.Get("DATABASE_URL", "postgres://orbital:orbital@127.0.0.1:5432/orbital?sslmode=disable"))
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err := db.Exec(startup, schema, pgx.QueryExecModeSimpleProtocol); err != nil {
		return err
	}

	conn, _, stream, err := broker.Open(startup, env.Get("NATS_URL", "nats://127.0.0.1:4222"))
	if err != nil {
		return err
	}
	defer conn.Close()
	consumer, err := stream.CreateOrUpdateConsumer(startup, jetstream.ConsumerConfig{
		Durable: "scores", AckPolicy: jetstream.AckExplicitPolicy, AckWait: 15 * time.Second,
		FilterSubject: broker.Subject, MaxAckPending: 64,
	})
	if err != nil {
		return err
	}
	messages, err := consumer.Consume(func(message jetstream.Msg) {
		save(ctx, db, message)
	}, jetstream.PullMaxMessages(16), jetstream.ConsumeErrHandler(func(_ jetstream.ConsumeContext, err error) {
		log.Printf("consume match results: %v", err)
	}))
	if err != nil {
		return err
	}
	defer func() {
		messages.Stop()
		<-messages.Closed()
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/leaderboard", func(w http.ResponseWriter, r *http.Request) {
		query, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		rows, err := db.Query(query, `SELECT id, name, score, bot_score, finished_at
			FROM match_results ORDER BY score DESC, finished_at ASC, id ASC LIMIT 10`)
		if err != nil {
			unavailable(w, err)
			return
		}
		results := make([]arena.Result, 0, 10)
		defer rows.Close()
		for rows.Next() {
			var result arena.Result
			if err := rows.Scan(&result.ID, &result.Name, &result.Score, &result.BotScore, &result.FinishedAt); err != nil {
				unavailable(w, err)
				return
			}
			results = append(results, result)
		}
		if err := rows.Err(); err != nil {
			unavailable(w, err)
			return
		}
		respond(w, results)
	})
	mux.HandleFunc("GET /api/results/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !matchID.MatchString(id) {
			http.NotFound(w, r)
			return
		}
		query, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		var result rankedResult
		err := db.QueryRow(query, `SELECT target.id, target.name, target.score, target.bot_score, target.finished_at,
			1 + (SELECT count(*) FROM match_results AS ahead
				WHERE ahead.score > target.score
					OR (ahead.score = target.score AND (ahead.finished_at, ahead.id) < (target.finished_at, target.id)))
			FROM match_results AS target WHERE target.id = $1`, id).
			Scan(&result.ID, &result.Name, &result.Score, &result.BotScore, &result.FinishedAt, &result.Rank)
		if errors.Is(err, pgx.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			unavailable(w, err)
			return
		}
		respond(w, result)
	})

	server := &http.Server{
		Addr: env.Get("SCORES_LISTEN", ":8081"), Handler: mux,
		ReadHeaderTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second,
		BaseContext: func(net.Listener) context.Context { return ctx },
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	log.Printf("scores listening on %s", server.Addr)
	err = server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func save(ctx context.Context, db *pgxpool.Pool, message jetstream.Msg) {
	var result arena.Result
	if err := json.Unmarshal(message.Data(), &result); err != nil || !matchID.MatchString(result.ID) ||
		utf8.RuneCountInString(result.Name) < 1 || utf8.RuneCountInString(result.Name) > 20 ||
		result.Score < 0 || result.BotScore < 0 || result.FinishedAt.IsZero() {
		log.Print("discard invalid match result")
		if err := message.Term(); err != nil {
			log.Printf("terminate delivery: %v", err)
		}
		return
	}
	write, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, err := db.Exec(write, `INSERT INTO match_results (id, name, score, bot_score, finished_at)
		VALUES ($1, $2, $3, $4, $5) ON CONFLICT (id) DO NOTHING`,
		result.ID, result.Name, result.Score, result.BotScore, result.FinishedAt)
	if err != nil {
		log.Printf("store match %s: %v", result.ID, err)
		if err := message.NakWithDelay(2 * time.Second); err != nil {
			log.Printf("reschedule match %s: %v", result.ID, err)
		}
		return
	}
	if err := message.Ack(); err != nil {
		log.Printf("ack match %s: %v", result.ID, err)
	}
}

func respond(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(data)
}

func unavailable(w http.ResponseWriter, err error) {
	log.Printf("read scores: %v", err)
	http.Error(w, "scores unavailable", http.StatusServiceUnavailable)
}
