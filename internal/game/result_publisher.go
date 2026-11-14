package game

import "context"

// ResultPublisher sends a completed match to the scores service through the broker.
type ResultPublisher interface {
	Publish(ctx context.Context, key string, payload []byte) error
}
