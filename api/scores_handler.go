package api

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime"
	"net/http"
	"regexp"

	"github.com/gin-gonic/gin"

	"orbitalrun/internal/arena"
	"orbitalrun/internal/scores"
)

var localMatchID = regexp.MustCompile(`^local-[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

type httpHandler struct {
	store scores.ResultStore
}

// NewScoresHandler registers the leaderboard, result lookup, and local result endpoints.
func NewScoresHandler(store scores.ResultStore) http.Handler {
	handler := httpHandler{store: store}
	router := newRouter()
	routes := router.Group("/api")
	routes.GET("/leaderboard", handler.leaderboard)
	routes.HEAD("/leaderboard", handler.leaderboard)
	routes.GET("/results/:id", handler.result)
	routes.HEAD("/results/:id", handler.result)
	routes.POST("/local-results", handler.saveLocalResult)
	return router
}

func (h httpHandler) leaderboard(c *gin.Context) {
	results, err := h.store.Top(c.Request.Context())
	if err != nil {
		unavailable(c.Writer, err)
		return
	}
	respond(c.Writer, results)
}

func (h httpHandler) result(c *gin.Context) {
	w, r := c.Writer, c.Request
	id := c.Param("id")
	if !scores.ValidOnlineResultID(id) && !localMatchID.MatchString(id) {
		http.NotFound(w, r)
		return
	}
	result, err := h.store.FindRanked(r.Context(), id)
	if errors.Is(err, scores.ErrResultNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		unavailable(w, err)
		return
	}
	respond(w, result)
}

func (h httpHandler) saveLocalResult(c *gin.Context) {
	w, r := c.Writer, c.Request
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
		ID:       input.ID,
		Name:     arena.PilotName(input.Name),
		Score:    *input.Score,
		BotScore: *input.BotScore,
		Local:    true,
	}
	if err := h.store.Save(r.Context(), result); err != nil {
		unavailable(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
