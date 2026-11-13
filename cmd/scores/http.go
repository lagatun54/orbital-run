package main

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime"
	"net/http"
	"regexp"

	"orbitalrun/internal/arena"
)

var localMatchID = regexp.MustCompile(`^local-[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func routes(scores *store) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/leaderboard", func(w http.ResponseWriter, r *http.Request) {
		results, err := scores.Top(r.Context())
		if err != nil {
			unavailable(w, err)
			return
		}
		respond(w, results)
	})
	mux.HandleFunc("GET /api/results/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !matchID.MatchString(id) && !localMatchID.MatchString(id) {
			http.NotFound(w, r)
			return
		}
		result, err := scores.FindRanked(r.Context(), id)
		if errors.Is(err, errResultNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			unavailable(w, err)
			return
		}
		respond(w, result)
	})
	mux.HandleFunc("POST /api/local-results", localResults(scores))
	return mux
}

func localResults(scores *store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || contentType != "application/json" {
			http.Error(w, "expected a JSON result", http.StatusUnsupportedMediaType)
			return
		}
		var input struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Score    *int   `json:"score"`
			BotScore *int   `json:"botScore"`
		}
		body := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048))
		if err := body.Decode(&input); err != nil || body.Decode(new(any)) != io.EOF ||
			!localMatchID.MatchString(input.ID) || !validLocalScore(input.Score) || !validLocalScore(input.BotScore) {
			http.Error(w, "invalid local result", http.StatusBadRequest)
			return
		}

		// A local result is reported by the client. Keep its origin visible in the leaderboard.
		result := arena.Result{
			ID: input.ID, Name: arena.PilotName(input.Name),
			Score: *input.Score, BotScore: *input.BotScore, Local: true,
		}
		if err := scores.Save(r.Context(), result); err != nil {
			unavailable(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func validLocalScore(score *int) bool {
	return score != nil && *score >= 0 && *score <= 300
}

func respond(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(data)
}

func unavailable(w http.ResponseWriter, err error) {
	log.Printf("scores: %v", err)
	http.Error(w, "scores unavailable", http.StatusServiceUnavailable)
}
