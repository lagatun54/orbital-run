package broker

import "context"

// Consumer delivers messages to a handler. A handler error leaves the message eligible for retry.
type Consumer interface {
	Run(ctx context.Context, handle func(context.Context, []byte) error) error
	Close()
}
