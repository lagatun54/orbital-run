package broker

import "context"

// Producer publishes match results and checks whether the broker is reachable.
type Producer interface {
	Publish(ctx context.Context, key string, payload []byte) error
	Ready(ctx context.Context) error
	Close()
}
