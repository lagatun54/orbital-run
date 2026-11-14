package api

import (
	"context"
	"net/http"
	"time"
)

func brokerHealth(broker BrokerReadiness) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		probe, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := broker.Ready(probe); err != nil {
			http.Error(w, "broker disconnected", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
