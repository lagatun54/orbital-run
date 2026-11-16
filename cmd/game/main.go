package main

import (
	"context"
	"log"
	"net/http"
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
	maxMatches, err := config.PositiveInt("GAME_MAX_MATCHES", 64)
	if err != nil {
		return err
	}

	startup, cancel := context.WithTimeout(ctx, 8*time.Second)
	producer, err := broker.OpenProducer(startup)
	cancel()
	if err != nil {
		return err
	}
	defer producer.Close()

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Protocols = new(http.Protocols)
	transport.Protocols.SetHTTP2(true)
	transport.Protocols.SetUnencryptedHTTP2(true)
	defer transport.CloseIdleConnections()
	scoresClient := &http.Client{Transport: transport, Timeout: 4 * time.Second}
	scoresRPC := api.NewScoresGateway(scoresClient, config.Env("SCORES_URL", "http://127.0.0.1:8081"))

	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)

	frontendOrigin := os.Getenv("FRONTEND_ORIGIN")
	match := api.NewMatchHandler(frontendOrigin, producer, maxMatches)
	server := &http.Server{
		Addr:              config.Env("GAME_LISTEN", ":8080"),
		Handler:           api.GameRoutes(frontendOrigin, match, scoresRPC, producer),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
		Protocols:         protocols,
	}

	log.Printf("game listening on %s (match slots: %d)", server.Addr, maxMatches)
	return httpserver.Run(ctx, server)
}
