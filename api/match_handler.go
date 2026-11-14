package api

import (
	"context"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"orbitalrun/internal/arena"
	"orbitalrun/internal/game"
)

const maxConcurrentMatches = 64

type websocketHandler struct {
	frontendOrigin string
	slots          chan struct{}
	results        game.ResultPublisher
}

// NewMatchHandler creates the WebSocket entry point for online matches.
func NewMatchHandler(frontendOrigin string, results game.ResultPublisher) http.Handler {
	handler := websocketHandler{
		frontendOrigin: frontendOrigin,
		slots:          make(chan struct{}, maxConcurrentMatches),
		results:        results,
	}
	return handler
}

func (h websocketHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	select {
	case h.slots <- struct{}{}:
		defer func() { <-h.slots }()
	default:
		http.Error(w, "all arenas are busy", http.StatusServiceUnavailable)
		return
	}
	var opts *websocket.AcceptOptions
	if h.frontendOrigin != "" && r.Header.Get("Origin") == h.frontendOrigin {
		opts = &websocket.AcceptOptions{OriginPatterns: []string{h.frontendOrigin}}
	}
	conn, err := websocket.Accept(w, r, opts)
	if err != nil {
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(512)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	var first clientMessage
	hello, cancelHello := context.WithTimeout(ctx, 4*time.Second)
	err = wsjson.Read(hello, conn, &first)
	cancelHello()
	if err != nil {
		return
	}
	// The reader stores the latest command while the session advances on its own timer.
	var playerInput atomic.Uint32
	go func() {
		defer cancel()
		for {
			var input clientMessage
			if err := wsjson.Read(ctx, conn, &input); err != nil {
				return
			}
			playerInput.Store(input.Direction)
		}
	}()

	playerDirection := func() arena.Point {
		return decodeDirection(playerInput.Load())
	}
	sendFrame := func(frame arena.Frame) error {
		return writeFrame(ctx, conn, frame)
	}
	end := game.RunSession(ctx, arena.PilotName(first.Name), h.results, playerDirection, sendFrame)
	if end != nil {
		_ = writeFrame(ctx, conn, end)
		_ = conn.Close(websocket.StatusNormalClosure, "round complete")
	}
}

func writeFrame(ctx context.Context, conn *websocket.Conn, frame any) error {
	write, cancel := context.WithTimeout(ctx, 750*time.Millisecond)
	defer cancel()
	return wsjson.Write(write, conn, frame)
}
