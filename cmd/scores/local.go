package main

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"orbitalrun/internal/arena"
)

func localResults(db *pgxpool.Pool) http.HandlerFunc {
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
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		_, err = db.Exec(ctx, `INSERT INTO match_results (id, name, score, bot_score, finished_at, local)
			VALUES ($1, $2, $3, $4, now(), true) ON CONFLICT (id) DO NOTHING`,
			input.ID, arena.PilotName(input.Name), *input.Score, *input.BotScore)
		if err != nil {
			unavailable(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func validLocalScore(score *int) bool {
	return score != nil && *score >= 0 && *score <= 300
}
