package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestWebSocketFrontendOrigin(t *testing.T) {
	const frontend = "https://owner.github.io"
	game := matchServer{slots: make(chan struct{}, 3), frontendOrigin: frontend}
	server := httptest.NewServer(http.HandlerFunc(game.play))
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
			conn, resp, err := websocket.Dial(ctx, server.URL, &websocket.DialOptions{
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
