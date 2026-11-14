package unit

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"orbitalrun/internal/arena"
	"orbitalrun/internal/game"
)

const tickInterval = 50 * time.Millisecond

type publishFunc func(context.Context, string, []byte) error

func (p publishFunc) Publish(ctx context.Context, key string, payload []byte) error {
	return p(ctx, key, payload)
}

func TestSessionPublishesFinalResult(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "queued"},
		{name: "publish failed", err: errors.New("broker unavailable")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				start := time.Now()
				var published arena.Result
				var calls, frames int
				results := publishFunc(func(_ context.Context, key string, payload []byte) error {
					calls++
					if err := json.Unmarshal(payload, &published); err != nil {
						t.Fatal(err)
					}
					if key == "" || key != published.ID {
						t.Fatalf("published key = %q, result ID = %q", key, published.ID)
					}
					return tc.err
				})
				end := game.RunSession(t.Context(), "Pilot", results, idleInput, func(frame arena.Frame) error {
					frames++
					if frame.Phase == "finished" {
						t.Fatal("finished frame sent as an ordinary frame")
					}
					return nil
				})
				if end == nil || end.Phase != "finished" || end.Remaining != 0 {
					t.Fatalf("unexpected final frame: %+v", end)
				}
				wantFrames := int((arena.Countdown+arena.Duration)/tickInterval) - 1
				if frames != wantFrames || calls != 1 || end.Queued != (tc.err == nil) {
					t.Fatalf("frames = %d (want %d), publishes = %d, queued = %v", frames, wantFrames, calls, end.Queued)
				}
				want := arena.Result{
					ID:         end.Result.ID,
					Name:       "Pilot",
					Score:      end.Player.Score,
					BotScore:   end.Bot.Score,
					FinishedAt: start.Add(arena.Countdown + arena.Duration).UTC(),
				}
				if end.Result != want || published != want {
					t.Fatalf("final result = %+v, published = %+v, want %+v", end.Result, published, want)
				}
			})
		})
	}
}

func TestSessionDoesNotPublishAfterClientDisconnects(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		results := publishFunc(func(context.Context, string, []byte) error {
			t.Fatal("session published a result after the client disconnected")
			return nil
		})
		attempts := 0
		end := game.RunSession(t.Context(), "Pilot", results, idleInput, func(arena.Frame) error {
			attempts++
			return errors.New("connection closed")
		})
		if end != nil || attempts != 1 {
			t.Fatalf("session returned %+v after %d send attempts, want no result and one attempt", end, attempts)
		}
	})
}

func idleInput() arena.Point { return arena.Point{} }
