package game

import (
	"context"
	"time"

	"orbitalrun/internal/arena"
	"orbitalrun/internal/bot"
)

const tickInterval = 50 * time.Millisecond

// RunSession returns the final frame after publication, or nil if interrupted.
func RunSession(
	ctx context.Context,
	name string,
	results ResultPublisher,
	playerDirection func() arena.Point,
	sendFrame func(arena.Frame) error,
) *RoundEnd {
	match := arena.New(time.Now())
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case now := <-ticker.C:
			if ctx.Err() != nil {
				return nil
			}
			botDirection := bot.Direction(match.Bot.Point, match.Crystals)
			match.Step(now, playerDirection(), botDirection)
			if match.Phase == "finished" {
				return finishRound(now, name, match.Frame, results)
			}
			if err := sendFrame(match.Frame); err != nil {
				return nil
			}
		}
	}
}
