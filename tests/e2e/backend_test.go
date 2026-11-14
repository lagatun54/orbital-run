//go:build e2e

package e2e

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"orbitalrun/internal/arena"
	"orbitalrun/internal/game"
)

type backend struct {
	url, origin string
	client      *http.Client
}

type rankedResult struct {
	arena.Result
	Rank int64 `json:"rank"`
}

func TestBackend(t *testing.T) {
	api := backend{
		url:    strings.TrimRight(os.Getenv("E2E_BASE_URL"), "/"),
		origin: os.Getenv("FRONTEND_ORIGIN"),
		client: &http.Client{Timeout: 3 * time.Second},
	}
	if api.url == "" || api.origin == "" {
		t.Fatal("set E2E_BASE_URL and FRONTEND_ORIGIN, or run bash tests/e2e/run.sh")
	}

	t.Run("HTTP contract", func(t *testing.T) {
		res := api.request(t, t.Context(), http.MethodGet, "/healthz", "", http.StatusNoContent, nil)
		res.Body.Close()
		res = api.request(t, t.Context(), http.MethodGet, "/", "", http.StatusNotFound, nil)
		res.Body.Close()
		res = api.request(t, t.Context(), http.MethodHead, "/api/leaderboard", "", http.StatusOK, nil)
		res.Body.Close()
		assertCORS(t, res, api.origin)
		res = api.request(t, t.Context(), http.MethodDelete, "/api/leaderboard", "", http.StatusMethodNotAllowed, nil)
		res.Body.Close()
		if res.Header.Get("Allow") != "GET, HEAD" {
			t.Fatalf("unexpected allowed methods: %q", res.Header.Get("Allow"))
		}

		var leaderboard []arena.Result
		res = api.request(t, t.Context(), http.MethodGet, "/api/leaderboard", "", http.StatusOK, nil)
		defer res.Body.Close()
		if err := json.NewDecoder(res.Body).Decode(&leaderboard); err != nil || leaderboard == nil {
			t.Fatalf("expected a JSON leaderboard array, got %v (%v)", leaderboard, err)
		}
		assertCORS(t, res, api.origin)

		res = api.request(t, t.Context(), http.MethodOptions, "/api/local-results", "", http.StatusNoContent, http.Header{
			"Access-Control-Request-Method":  {http.MethodPost},
			"Access-Control-Request-Headers": {"content-type"},
		})
		defer res.Body.Close()
		assertCORS(t, res, api.origin)
		if res.Header.Get("Access-Control-Allow-Methods") != http.MethodPost ||
			!strings.EqualFold(res.Header.Get("Access-Control-Allow-Headers"), "content-type") {
			t.Fatalf("preflight did not allow a JSON POST: %v", res.Header)
		}
	})

	t.Run("online match reaches leaderboard", func(t *testing.T) {
		project := os.Getenv("E2E_COMPOSE_PROJECT")
		var outage *postgresOutage
		if project != "" {
			outage = stopPostgres(t, project)
		}
		t.Log("playing a full 32-second round")
		end := api.playMatch(t)
		t.Logf("round %s queued; waiting for scores", end.Result.ID)
		if outage != nil {
			outage.recover(t, api, end.Result.ID)
		}
		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
		defer cancel()
		ranked := api.waitForResult(t, ctx, end.Result.ID)
		if ranked.Rank < 1 || !sameResult(ranked.Result, end.Result) {
			t.Fatalf("stored result = %+v; want %+v with a positive rank", ranked, end.Result)
		}

		res := api.request(t, ctx, http.MethodGet, "/api/leaderboard", "", http.StatusOK, nil)
		defer res.Body.Close()
		var leaderboard []arena.Result
		if err := json.NewDecoder(res.Body).Decode(&leaderboard); err != nil {
			t.Fatal(err)
		}
		// The Compose runner starts with an empty database.
		if len(leaderboard) != 1 || !sameResult(leaderboard[0], end.Result) || ranked.Rank != 1 {
			t.Fatalf("first match should rank first: leaderboard = %+v, rank = %d", leaderboard, ranked.Rank)
		}
	})

	t.Run("local result is marked and duplicate cannot change it", func(t *testing.T) {
		var uuid [16]byte
		rand.Read(uuid[:])
		uuid[6] = uuid[6]&0x0f | 0x40 // UUID v4
		uuid[8] = uuid[8]&0x3f | 0x80 // RFC 9562 variant
		id := fmt.Sprintf("local-%x-%x-%x-%x-%x", uuid[:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:])
		post := func(name string, score int, wantStatus int) {
			t.Helper()
			body := fmt.Sprintf(`{"id":%q,"name":%q,"score":%d,"botScore":1}`, id, name, score)
			res := api.request(t, t.Context(), http.MethodPost, "/api/local-results", body, wantStatus, nil)
			res.Body.Close()
		}
		post("Offline", 301, http.StatusBadRequest)
		get := api.request(t, t.Context(), http.MethodGet, "/api/results/"+id, "", http.StatusNotFound, nil)
		get.Body.Close()
		post("Offline!", 12, http.StatusNoContent)
		post("Duplicate", 99, http.StatusNoContent)

		var result rankedResult
		res := api.request(t, t.Context(), http.MethodGet, "/api/results/"+id, "", http.StatusOK, nil)
		defer res.Body.Close()
		if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}
		if result.ID != id || result.Name != "Offline" || result.Score != 12 || result.BotScore != 1 ||
			!result.Local || result.FinishedAt.IsZero() || result.Rank < 1 {
			t.Fatalf("stored local result changed or is incomplete: %+v", result)
		}
	})

	t.Run("exact ranks follow score and finish time", func(t *testing.T) {
		const (
			earlier = "local-00000000-0000-4000-8000-000000000003"
			later   = "local-00000000-0000-4000-8000-000000000002"
			higher  = "local-00000000-0000-4000-8000-000000000001"
		)
		for _, entry := range []struct {
			id    string
			score int
		}{{earlier, 100}, {later, 100}, {higher, 101}} {
			body := fmt.Sprintf(`{"id":%q,"name":"Pilot","score":%d,"botScore":0}`, entry.id, entry.score)
			res := api.request(t, t.Context(), http.MethodPost, "/api/local-results", body, http.StatusNoContent, nil)
			res.Body.Close()
		}

		var leaderboard []arena.Result
		res := api.request(t, t.Context(), http.MethodGet, "/api/leaderboard", "", http.StatusOK, nil)
		defer res.Body.Close()
		if err := json.NewDecoder(res.Body).Decode(&leaderboard); err != nil {
			t.Fatal(err)
		}
		wantOrder := []string{higher, earlier, later}
		for i, entry := range leaderboard {
			switch entry.ID {
			case higher, earlier, later:
				if len(wantOrder) == 0 || entry.ID != wantOrder[0] {
					t.Fatalf("unexpected result %s in leaderboard; remaining order %v", entry.ID, wantOrder)
				}
				wantOrder = wantOrder[1:]
			}
			result := api.waitForResult(t, t.Context(), entry.ID)
			if result.Rank != int64(i+1) {
				t.Errorf("rank of %s = %d, want %d", entry.ID, result.Rank, i+1)
			}
		}
		if len(wantOrder) != 0 {
			t.Fatalf("missing leaderboard entries: %v", wantOrder)
		}
	})

	t.Run("online match completes while scores is down", func(t *testing.T) {
		project := os.Getenv("E2E_COMPOSE_PROJECT")
		if project == "" {
			t.Skip("requires a disposable Compose project from tests/e2e/run.sh")
		}
		composeOutput(t, project, "stop", "scores")

		res := api.request(t, t.Context(), http.MethodGet, "/api/leaderboard", "", http.StatusServiceUnavailable, nil)
		res.Body.Close()
		assertCORS(t, res, api.origin)
		res = api.request(t, t.Context(), http.MethodGet, "/healthz", "", http.StatusNoContent, nil)
		res.Body.Close()

		t.Log("playing a full round with scores stopped")
		end := api.playMatch(t)
		t.Logf("round %s queued; starting scores", end.Result.ID)
		composeOutput(t, project, "start", "--wait", "--wait-timeout", "25", "scores")
		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
		defer cancel()
		ranked := api.waitForResult(t, ctx, end.Result.ID)
		if ranked.Rank < 1 || !sameResult(ranked.Result, end.Result) {
			t.Fatalf("stored result = %+v; want %+v with a positive rank", ranked, end.Result)
		}
	})
}

func (api backend) playMatch(t *testing.T) game.RoundEnd {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, api.url+"/ws", &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": {api.origin}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	if err := wsjson.Write(ctx, conn, map[string]string{"name": " E2E Pilot! "}); err != nil {
		t.Fatal(err)
	}
	var first arena.Frame
	if err := wsjson.Read(ctx, conn, &first); err != nil {
		t.Fatal(err)
	}
	if first.Phase != "countdown" {
		t.Fatalf("initial phase = %q, want countdown", first.Phase)
	}
	if err := wsjson.Write(ctx, conn, map[string]int{"direction": 8}); err != nil {
		t.Fatal(err)
	}

	var playerMoved, botMoved bool
	for {
		var frame game.RoundEnd
		if err := wsjson.Read(ctx, conn, &frame); err != nil {
			t.Fatalf("read match frame: %v", err)
		}
		switch frame.Phase {
		case "countdown":
		case "playing":
			if !playerMoved && frame.Player.X > first.Player.X {
				playerMoved = true
				if err := wsjson.Write(ctx, conn, map[string]int{"direction": 0}); err != nil {
					t.Fatal(err)
				}
			}
			botMoved = botMoved || frame.Bot.Point != first.Bot.Point
		case "finished":
			if !playerMoved || !botMoved || !frame.Queued || frame.Remaining != 0 ||
				frame.Result.ID == "" || frame.Result.Name != "E2E Pilot" || frame.Result.FinishedAt.IsZero() ||
				frame.Result.Score != frame.Player.Score || frame.Result.BotScore != frame.Bot.Score || frame.Result.Local {
				t.Fatalf("invalid match outcome: player moved=%v, bot moved=%v, final frame=%+v", playerMoved, botMoved, frame)
			}
			if err := wsjson.Read(ctx, conn, &arena.Frame{}); websocket.CloseStatus(err) != websocket.StatusNormalClosure {
				t.Fatalf("expected a normal close after the final frame, got %v", err)
			}
			return frame
		default:
			t.Fatalf("unexpected match phase %q", frame.Phase)
		}
	}
}

func (api backend) waitForResult(t *testing.T, ctx context.Context, id string) rankedResult {
	t.Helper()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		res := api.send(t, ctx, http.MethodGet, "/api/results/"+id, "", nil)
		var result rankedResult
		switch res.StatusCode {
		case http.StatusOK:
			err := json.NewDecoder(res.Body).Decode(&result)
			res.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
			return result
		case http.StatusNotFound:
			res.Body.Close()
		default:
			res.Body.Close()
			t.Fatalf("GET /api/results/%s: unexpected status %v", id, res.Status)
		}
		select {
		case <-ctx.Done():
			t.Fatalf("result %s was queued but not persisted: %v", id, ctx.Err())
		case <-ticker.C:
		}
	}
}

func (api backend) request(t *testing.T, ctx context.Context, method, path, body string, status int, headers http.Header) *http.Response {
	t.Helper()
	res := api.send(t, ctx, method, path, body, headers)
	if res.StatusCode != status {
		res.Body.Close()
		t.Fatalf("%s %s: got status %s, want %d", method, path, res.Status, status)
	}
	return res
}

func (api backend) send(t *testing.T, ctx context.Context, method, path, body string, headers http.Header) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, method, api.url+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	for name, values := range headers {
		req.Header[name] = values
	}
	req.Header.Set("Origin", api.origin)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := api.client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return res
}

func assertCORS(t *testing.T, res *http.Response, origin string) {
	t.Helper()
	if got := res.Header.Get("Access-Control-Allow-Origin"); got != origin {
		t.Fatalf("Access-Control-Allow-Origin = %q, want %q", got, origin)
	}
}

func sameResult(got, want arena.Result) bool {
	// PostgreSQL stores timestamps with microsecond precision.
	return got.ID == want.ID && got.Name == want.Name && got.Score == want.Score &&
		got.BotScore == want.BotScore && got.Local == want.Local && got.FinishedAt.Sub(want.FinishedAt).Abs() < time.Microsecond
}
