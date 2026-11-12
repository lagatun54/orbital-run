package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"orbitalrun/internal/arena"
	"orbitalrun/internal/broker"
	"orbitalrun/internal/env"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	startup, cancel := context.WithTimeout(ctx, 8*time.Second)
	conn, js, _, err := broker.Open(startup, env.Get("NATS_URL", "nats://127.0.0.1:4222"))
	cancel()
	if err != nil {
		return err
	}
	defer conn.Close()

	bot, err := net.ResolveUDPAddr("udp", env.Get("BOT_ADDRESS", "127.0.0.1:9001"))
	if err != nil {
		return err
	}
	target, err := url.Parse(env.Get("SCORES_URL", "http://127.0.0.1:8081"))
	if err != nil {
		return err
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 3 * time.Second
	proxy.Transport = transport

	frontendOrigin := os.Getenv("FRONTEND_ORIGIN")
	server := matchServer{bot: bot, slots: make(chan struct{}, 64), frontendOrigin: frontendOrigin}
	server.publish = func(ctx context.Context, result arena.Result) error {
		payload, err := json.Marshal(result)
		if err != nil {
			return err
		}
		_, err = js.Publish(ctx, broker.Subject, payload, jetstream.WithMsgID(result.ID))
		return err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ws", server.play)
	api := allowFrontend(frontendOrigin, proxy)
	mux.Handle("GET /api/leaderboard", api)
	mux.Handle("GET /api/results/{id}", api)
	mux.Handle("POST /api/local-results", api)
	mux.HandleFunc("OPTIONS /api/local-results", func(w http.ResponseWriter, r *http.Request) {
		if frontendOrigin == "" || r.Header.Get("Origin") != frontendOrigin ||
			r.Header.Get("Access-Control-Request-Method") != http.MethodPost {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", frontendOrigin)
		w.Header().Set("Access-Control-Allow-Methods", http.MethodPost)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Add("Vary", "Origin")
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		if !conn.IsConnected() {
			http.Error(w, "broker disconnected", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	if webDir := os.Getenv("WEB_DIR"); webDir != "" {
		mux.Handle("GET /", http.FileServer(http.Dir(webDir)))
	}

	httpServer := &http.Server{
		Addr: env.Get("GAME_LISTEN", ":8080"), Handler: mux,
		ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second,
		BaseContext: func(net.Listener) context.Context { return ctx },
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdown)
	}()

	log.Printf("game listening on %s", httpServer.Addr)
	err = httpServer.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Allow the configured frontend to read API responses.
func allowFrontend(origin string, next http.Handler) http.Handler {
	if origin == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") == origin {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		w.Header().Add("Vary", "Origin")
		next.ServeHTTP(w, r)
	})
}
