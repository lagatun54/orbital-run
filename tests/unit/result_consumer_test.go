package unit

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"testing"
	"time"

	"orbitalrun/internal/arena"
	"orbitalrun/internal/scores"
)

type resultWriterFunc func(context.Context, arena.Result) error

func (write resultWriterFunc) Save(ctx context.Context, result arena.Result) error {
	return write(ctx, result)
}

func TestResultConsumerSkipsUnstorableEvents(t *testing.T) {
	valid := arena.Result{
		ID:         "ABCDEFGHIJKLMNOPQRSTUVWXYZ",
		Name:       "Пилот",
		Score:      3,
		BotScore:   2,
		FinishedAt: time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC),
	}
	for _, tc := range []struct {
		name  string
		field string
		value any
	}{
		{name: "null character in name", field: "name", value: "A\x00B"},
		{name: "player score exceeds integer range", field: "score", value: int64(math.MaxInt32) + 1},
		{name: "bot score exceeds integer range", field: "botScore", value: int64(math.MaxInt32) + 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := map[string]any{
				"id": valid.ID, "name": valid.Name, "score": valid.Score,
				"botScore": valid.BotScore, "finishedAt": valid.FinishedAt,
			}
			result[tc.field] = tc.value
			payload, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			consume := scores.NewResultConsumer(resultWriterFunc(func(context.Context, arena.Result) error {
				t.Fatal("attempted to store an invalid result")
				return nil
			}))
			if err := consume(t.Context(), payload); err != nil {
				t.Fatalf("invalid event should be acknowledged, got %v", err)
			}
		})
	}

	t.Run("storage failure remains eligible for retry", func(t *testing.T) {
		payload, err := json.Marshal(valid)
		if err != nil {
			t.Fatal(err)
		}
		storeErr := errors.New("database unavailable")
		consume := scores.NewResultConsumer(resultWriterFunc(func(_ context.Context, result arena.Result) error {
			if result != valid {
				t.Fatalf("stored result = %+v, want %+v", result, valid)
			}
			return storeErr
		}))
		if err := consume(t.Context(), payload); !errors.Is(err, storeErr) {
			t.Fatalf("consume error = %v, want %v", err, storeErr)
		}
	})
}
