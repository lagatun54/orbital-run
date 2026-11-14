package httpserver

import (
	"context"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestServeWaitsForWebSocketHandler(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	serverCtx, stop := context.WithCancel(ctx)
	defer stop()

	requestCanceled := make(chan struct{})
	finishHandler := make(chan struct{})
	finish := sync.OnceFunc(func() { close(finishHandler) })
	defer finish()
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		<-r.Context().Done()
		close(requestCanceled)
		<-finishHandler
	})}
	done := make(chan error, 1)
	go func() { done <- serve(serverCtx, server, listener) }()

	conn, _, err := websocket.Dial(ctx, "ws://"+listener.Addr().String(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	stop()

	select {
	case <-requestCanceled:
	case <-ctx.Done():
		t.Fatal("WebSocket request was not canceled")
	}
	select {
	case err := <-done:
		t.Fatalf("server returned before the WebSocket handler finished: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	finish()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("server did not stop after the WebSocket handler finished")
	}
}
