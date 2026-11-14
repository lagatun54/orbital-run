package integration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"orbitalrun/api"
	"orbitalrun/internal/arena"
)

func TestWebSocketFrontendOrigin(t *testing.T) {
	const frontend = "https://owner.github.io"
	server := httptest.NewServer(api.GameRoutes(frontend, api.NewMatchHandler(frontend, nil), nil, nil))
	defer server.Close()

	for _, tc := range []struct {
		name, origin string
		wantStatus   int
	}{
		{"Pages", frontend, http.StatusSwitchingProtocols},
		{"same origin", server.URL, http.StatusSwitchingProtocols},
		{"another site", "https://other.example", http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			conn, resp, err := websocket.Dial(ctx, server.URL+"/ws", &websocket.DialOptions{
				HTTPHeader: http.Header{"Origin": {tc.origin}},
			})
			if conn != nil {
				defer conn.CloseNow()
			}
			if resp == nil || resp.StatusCode != tc.wantStatus {
				t.Fatalf("WebSocket upgrade from %s: response %v, error %v; want status %d", tc.origin, resp, err, tc.wantStatus)
			}
		})
	}
}

func TestMatchMovesPlayerAndLocalBot(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	server := httptest.NewServer(api.GameRoutes("", api.NewMatchHandler("", nil), nil, nil))
	defer server.Close()
	conn, _, err := websocket.Dial(ctx, server.URL+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	if err := wsjson.Write(ctx, conn, map[string]string{"name": "Pilot"}); err != nil {
		t.Fatal(err)
	}

	var first arena.Frame
	if err := wsjson.Read(ctx, conn, &first); err != nil {
		t.Fatal(err)
	}
	if first.Phase != "countdown" {
		t.Fatalf("initial phase = %q, want countdown", first.Phase)
	}

	if err := wsjson.Write(ctx, conn, map[string]uint32{"direction": 8}); err != nil {
		t.Fatal(err)
	}
	for {
		var frame arena.Frame
		if err := wsjson.Read(ctx, conn, &frame); err != nil {
			t.Fatalf("read match frame: %v", err)
		}
		if frame.Phase == "playing" && frame.Player.X > first.Player.X && frame.Bot.Point != first.Bot.Point {
			return
		}
	}
}
