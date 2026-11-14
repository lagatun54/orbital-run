package api

import "context"

// BrokerReadiness is the broker operation the health endpoint needs.
type BrokerReadiness interface {
	Ready(ctx context.Context) error
}
