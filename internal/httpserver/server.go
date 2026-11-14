package httpserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

const shutdownTimeout = 4 * time.Second

// Run starts a new server and waits for its handlers to return before releasing
// control to the caller. Handlers must stop when their request context is canceled
// or their connection is closed.
func Run(ctx context.Context, server *http.Server) error {
	if ctx.Err() != nil {
		return nil
	}
	var config net.ListenConfig
	listener, err := config.Listen(ctx, "tcp", server.Addr)
	if err != nil {
		if ctx.Err() != nil && errors.Is(err, ctx.Err()) {
			return nil
		}
		return fmt.Errorf("listen for HTTP: %w", err)
	}
	return serve(ctx, server, listener)
}

func serve(ctx context.Context, server *http.Server, listener net.Listener) error {
	requestCtx, cancelRequests := context.WithCancel(ctx)
	server.BaseContext = func(net.Listener) context.Context { return requestCtx }

	// Track handlers as well as connections because Shutdown skips upgraded WebSockets.
	var mu sync.Mutex
	var active sync.WaitGroup
	stopping := false
	handler := server.Handler
	if handler == nil {
		handler = http.DefaultServeMux
	}
	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		if stopping {
			mu.Unlock()
			http.Error(w, "server is shutting down", http.StatusServiceUnavailable)
			return
		}
		active.Add(1)
		mu.Unlock()
		defer active.Done()
		handler.ServeHTTP(w, r)
	})

	served := make(chan error, 1)
	go func() { served <- server.Serve(listener) }()

	var serveErr error
	select {
	case <-ctx.Done():
	case serveErr = <-served:
		served = nil
	}
	mu.Lock()
	stopping = true
	mu.Unlock()
	cancelRequests()

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), shutdownTimeout)
	shutdownErr := server.Shutdown(shutdownCtx)
	cancelShutdown()
	if shutdownErr != nil {
		shutdownErr = errors.Join(fmt.Errorf("shut down HTTP: %w", shutdownErr), server.Close())
	}
	if served != nil {
		serveErr = <-served
	}
	active.Wait()
	if errors.Is(serveErr, http.ErrServerClosed) {
		serveErr = nil
	}
	if serveErr != nil {
		serveErr = fmt.Errorf("serve HTTP: %w", serveErr)
	}
	return errors.Join(serveErr, shutdownErr)
}
