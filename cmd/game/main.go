package main

import (
	"context"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"time"

	"orbitalrun/api"
	"orbitalrun/config"
	"orbitalrun/internal/broker"
	"orbitalrun/internal/command"
	"orbitalrun/internal/httpserver"
)

func main() {
	command.Main(run)
}

func run(ctx context.Context) error {
	startup, cancel := context.WithTimeout(ctx, 8*time.Second)
	producer, err := broker.OpenProducer(startup)
	cancel()
	if err != nil {
		return err
	}
	defer producer.Close()

	target, err := url.Parse(config.Env("SCORES_URL", "http://127.0.0.1:8081"))
	if err != nil {
		return err
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		log.Printf("scores proxy: %v", err)
		http.Error(w, "scores unavailable", http.StatusServiceUnavailable)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 3 * time.Second
	proxy.Transport = transport
	defer transport.CloseIdleConnections()

	frontendOrigin := os.Getenv("FRONTEND_ORIGIN")
	match := api.NewMatchHandler(frontendOrigin, producer)
	server := &http.Server{
		Addr:              config.Env("GAME_LISTEN", ":8080"),
		Handler:           api.GameRoutes(frontendOrigin, match, proxy, producer),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}

	log.Printf("game listening on %s", server.Addr)
	return httpserver.Run(ctx, server)
}
