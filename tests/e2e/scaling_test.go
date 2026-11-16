//go:build e2e

package e2e

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"orbitalrun/internal/arena"
)

func TestGameScaling(t *testing.T) {
	project := os.Getenv("E2E_COMPOSE_PROJECT")
	if project == "" {
		t.Skip("requires a disposable Compose project from tests/e2e/run.sh")
	}
	baseURL := os.Getenv("E2E_BASE_URL")
	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Second)
	defer cancel()
	options := &websocket.DialOptions{HTTPHeader: http.Header{"Origin": {os.Getenv("FRONTEND_ORIGIN")}}}

	connect := func(wantStatus int) *websocket.Conn {
		t.Helper()
		conn, res, err := websocket.Dial(ctx, baseURL+"/ws", options)
		if conn != nil {
			t.Cleanup(func() { _ = conn.CloseNow() })
		}
		if res == nil || res.StatusCode != wantStatus {
			t.Fatalf("GET /ws: response %v, error %v; want status %d", res, err, wantStatus)
		}
		return conn
	}
	start := func(conn *websocket.Conn) arena.Frame {
		t.Helper()
		if err := wsjson.Write(ctx, conn, map[string]string{"name": "Scaling Pilot"}); err != nil {
			t.Fatal(err)
		}
		return requireGameFrame(t, ctx, conn)
	}

	first := connect(http.StatusSwitchingProtocols)
	initial := start(first)
	connect(http.StatusServiceUnavailable)

	composeOutput(t, project, "up", "--no-deps", "--no-build", "--no-recreate", "--scale", "game=2", "--wait", "--wait-timeout", "15", "game")
	second := connect(http.StatusSwitchingProtocols)
	start(second)
	connect(http.StatusServiceUnavailable)
	if err := wsjson.Write(ctx, first, map[string]uint32{"direction": 8}); err != nil {
		t.Fatal(err)
	}
	// Wait for the first match to act on input sent after the scale-up.
	for requireGameFrame(t, ctx, first).Player.X <= initial.Player.X {
	}
	requireGameFrame(t, ctx, second)

	api := backend{url: baseURL, client: &http.Client{Timeout: 3 * time.Second}}
	_, err := api.getLeaderboard(ctx)
	assertRPCCode(t, err, 0)
	api.request(t, ctx, http.MethodGet, "/metrics", http.StatusNotFound, nil).Body.Close()
	if err := first.Close(websocket.StatusNormalClosure, "leaving match"); err != nil {
		t.Fatal(err)
	}
	start(connect(http.StatusSwitchingProtocols))
}

func requireGameFrame(t *testing.T, ctx context.Context, conn *websocket.Conn) arena.Frame {
	t.Helper()
	var frame arena.Frame
	if err := wsjson.Read(ctx, conn, &frame); err != nil {
		t.Fatal(err)
	}
	if frame.Phase != "countdown" && frame.Phase != "playing" {
		t.Fatalf("unexpected game phase %q", frame.Phase)
	}
	return frame
}
