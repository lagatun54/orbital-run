package api

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"orbitalrun/internal/arena"
	"orbitalrun/internal/game"
)

// MatchHandler admits matches and tracks capacity for one game instance.
type MatchHandler struct {
	frontendOrigin string
	slots          chan struct{}
	results        game.ResultPublisher
	rejected       atomic.Uint64
}

// NewMatchHandler creates the WebSocket entry point for online matches.
func NewMatchHandler(frontendOrigin string, results game.ResultPublisher, maxMatches int) *MatchHandler {
	return &MatchHandler{
		frontendOrigin: frontendOrigin,
		slots:          make(chan struct{}, maxMatches),
		results:        results,
	}
}

func (h *MatchHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	select {
	case h.slots <- struct{}{}:
		defer func() { <-h.slots }()
	default:
		h.rejected.Add(1)
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
	batches := make(chan []pickupClaim, 4)
	go func() {
		defer cancel()
		for {
			var input clientMessage
			if err := wsjson.Read(ctx, conn, &input); err != nil {
				return
			}
			playerInput.Store(input.Direction)
			if len(input.Pickups) > 0 && len(input.Pickups) <= arena.CrystalCount {
				select {
				case batches <- input.Pickups:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	playerDirection := func() arena.Point {
		return decodeDirection(playerInput.Load())
	}
	pickups := pickupTracker{
		resolved: make(map[pickupClaim]bool),
		pending:  make(map[pickupClaim]time.Time),
	}
	sendFrame := func(frame arena.Frame) error {
		return writeFrame(ctx, conn, struct {
			arena.Frame
			PickupAcks []pickupAck `json:"pickupAcks,omitempty"`
		}{frame, pickups.update(frame, batches, time.Now())})
	}
	end := game.RunSession(ctx, arena.PilotName(first.Name), h.results, playerDirection, sendFrame)
	if end != nil {
		_ = writeFrame(ctx, conn, struct {
			*game.RoundEnd
			PickupAcks []pickupAck `json:"pickupAcks,omitempty"`
		}{end, pickups.update(end.Frame, batches, time.Now())})
		_ = conn.Close(websocket.StatusNormalClosure, "round complete")
	}
}

func (h *MatchHandler) serveMetrics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = fmt.Fprintf(w, `# TYPE game_match_slots_used gauge
game_match_slots_used %d
# TYPE game_match_slots_limit gauge
game_match_slots_limit %d
# TYPE game_match_rejections_total counter
game_match_rejections_total %d
`, len(h.slots), cap(h.slots), h.rejected.Load())
}

func writeFrame(ctx context.Context, conn *websocket.Conn, frame any) error {
	write, cancel := context.WithTimeout(ctx, 750*time.Millisecond)
	defer cancel()
	return wsjson.Write(write, conn, frame)
}
