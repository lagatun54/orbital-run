package game

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"log"
	"time"

	"orbitalrun/internal/arena"
)

// RoundEnd is the last WebSocket frame. Queued reports whether the broker accepted the result.
type RoundEnd struct {
	arena.Frame
	Result arena.Result `json:"result"`
	Queued bool         `json:"queued"`
}

func finishRound(now time.Time, name string, frame arena.Frame, results ResultPublisher) *RoundEnd {
	result := arena.Result{
		ID:         rand.Text(),
		Name:       name,
		Score:      frame.Player.Score,
		BotScore:   frame.Bot.Score,
		FinishedAt: now.UTC(),
	}
	err := publishResult(results, result)
	if err != nil {
		log.Printf("publish match %s: %v", result.ID, err)
	}
	return &RoundEnd{Frame: frame, Result: result, Queued: err == nil}
}

func publishResult(results ResultPublisher, result arena.Result) error {
	// A completed round can still be published if the browser disconnects.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	payload, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return results.Publish(ctx, result.ID, payload)
}
