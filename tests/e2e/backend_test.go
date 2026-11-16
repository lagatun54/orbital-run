//go:build e2e

package e2e

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	scoresv1 "orbitalrun/api/scores/v1"
	"orbitalrun/api/scores/v1/scoresv1connect"
	"orbitalrun/internal/arena"
	"orbitalrun/internal/game"
)

type backend struct {
	url, origin string
	client      *http.Client
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

	t.Run("gRPC-Web contract", func(t *testing.T) {
		api.request(t, t.Context(), http.MethodGet, "/healthz", http.StatusNoContent, nil).Body.Close()
		api.request(t, t.Context(), http.MethodGet, "/", http.StatusNotFound, nil).Body.Close()
		api.request(t, t.Context(), http.MethodGet, "/api/leaderboard", http.StatusNotFound, nil).Body.Close()

		leaderboard, err := api.getLeaderboard(t.Context())
		assertRPCCode(t, err, 0)
		if got := leaderboard.Header().Get("Access-Control-Allow-Origin"); got != api.origin {
			t.Fatalf("Access-Control-Allow-Origin = %q, want %q", got, api.origin)
		}

		for _, procedure := range []string{
			scoresv1connect.LeaderboardServiceGetLeaderboardProcedure,
			scoresv1connect.ResultsServiceGetResultProcedure,
			scoresv1connect.ResultsServiceSaveLocalResultProcedure,
		} {
			res := api.request(t, t.Context(), http.MethodOptions, procedure, http.StatusNoContent, http.Header{
				"Access-Control-Request-Method":  {http.MethodPost},
				"Access-Control-Request-Headers": {"content-type,grpc-timeout,x-grpc-web,x-user-agent"},
			})
			res.Body.Close()
			if res.Header.Get("Access-Control-Allow-Origin") != api.origin ||
				res.Header.Get("Access-Control-Allow-Methods") != http.MethodPost {
				t.Fatalf("%s preflight did not allow a gRPC-Web POST: %v", procedure, res.Header)
			}
			allowed := strings.ToLower(res.Header.Get("Access-Control-Allow-Headers"))
			for _, header := range []string{"content-type", "grpc-timeout", "x-grpc-web", "x-user-agent"} {
				if !strings.Contains(allowed, header) {
					t.Errorf("%s preflight did not allow %s", procedure, header)
				}
			}
			api.request(t, t.Context(), http.MethodOptions, procedure, http.StatusForbidden, http.Header{
				"Origin": {"https://untrusted.example"}, "Access-Control-Request-Method": {http.MethodPost},
			}).Body.Close()
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
		if ranked.GetRank() < 1 || !sameResult(ranked.GetResult(), end.Result) {
			t.Fatalf("stored result = %+v; want %+v with a positive rank", ranked, end.Result)
		}

		response, err := api.getLeaderboard(ctx)
		assertRPCCode(t, err, 0)
		leaderboard := response.Msg.GetEntries()
		// The Compose runner starts with an empty database.
		if len(leaderboard) != 1 || !sameResult(leaderboard[0], end.Result) || ranked.GetRank() != 1 {
			t.Fatalf("first match should rank first: leaderboard = %+v, rank = %d", leaderboard, ranked.GetRank())
		}
	})

	t.Run("local result is marked and duplicate cannot change it", func(t *testing.T) {
		var uuid [16]byte
		rand.Read(uuid[:])
		uuid[6] = uuid[6]&0x0f | 0x40 // UUID v4
		uuid[8] = uuid[8]&0x3f | 0x80 // RFC 9562 variant
		id := fmt.Sprintf("local-%x-%x-%x-%x-%x", uuid[:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:])
		post := func(name string, score int32, wantCode connect.Code) {
			t.Helper()
			botScore := int32(1)
			assertRPCCode(t, api.saveLocalResult(t.Context(), &scoresv1.SaveLocalResultRequest{
				Id: id, Name: name, Score: &score, BotScore: &botScore,
			}), wantCode)
		}
		post("Offline", 301, connect.CodeInvalidArgument)
		_, err := api.getResult(t.Context(), id)
		assertRPCCode(t, err, connect.CodeNotFound)
		post("Offline!", 12, 0)
		post("Duplicate", 99, 0)

		ranked, err := api.getResult(t.Context(), id)
		assertRPCCode(t, err, 0)
		result := ranked.Msg.GetResult()
		if result.GetId() != id || result.GetName() != "Offline" || result.GetScore() != 12 ||
			result.GetBotScore() != 1 || !result.GetLocal() || result.GetFinishedAt() == nil || ranked.Msg.GetRank() < 1 {
			t.Fatalf("stored local result changed or is incomplete: %v", ranked.Msg)
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
			score int32
		}{{earlier, 100}, {later, 100}, {higher, 101}} {
			botScore := int32(0)
			assertRPCCode(t, api.saveLocalResult(t.Context(), &scoresv1.SaveLocalResultRequest{
				Id: entry.id, Name: "Pilot", Score: &entry.score, BotScore: &botScore,
			}), 0)
		}

		response, err := api.getLeaderboard(t.Context())
		assertRPCCode(t, err, 0)
		leaderboard := response.Msg.GetEntries()
		wantOrder := []string{higher, earlier, later}
		for i, entry := range leaderboard {
			switch entry.GetId() {
			case higher, earlier, later:
				if len(wantOrder) == 0 || entry.GetId() != wantOrder[0] {
					t.Fatalf("unexpected result %s in leaderboard; remaining order %v", entry.GetId(), wantOrder)
				}
				wantOrder = wantOrder[1:]
			}
			result := api.waitForResult(t, t.Context(), entry.GetId())
			if result.GetRank() != int64(i+1) {
				t.Errorf("rank of %s = %d, want %d", entry.GetId(), result.GetRank(), i+1)
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

		_, err := api.getLeaderboard(t.Context())
		assertRPCCode(t, err, connect.CodeUnavailable)
		api.request(t, t.Context(), http.MethodGet, "/healthz", http.StatusNoContent, nil).Body.Close()

		t.Log("playing a full round with scores stopped")
		end := api.playMatch(t)
		t.Logf("round %s queued; starting scores", end.Result.ID)
		composeOutput(t, project, "start", "--wait", "--wait-timeout", "25", "scores")
		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
		defer cancel()
		ranked := api.waitForResult(t, ctx, end.Result.ID)
		if ranked.GetRank() < 1 || !sameResult(ranked.GetResult(), end.Result) {
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

func (api backend) getLeaderboard(ctx context.Context) (*connect.Response[scoresv1.GetLeaderboardResponse], error) {
	client := scoresv1connect.NewLeaderboardServiceClient(api.client, api.url, connect.WithGRPCWeb())
	req := connect.NewRequest(&scoresv1.GetLeaderboardRequest{})
	req.Header().Set("Origin", api.origin)
	return client.GetLeaderboard(ctx, req)
}

func (api backend) getResult(ctx context.Context, id string) (*connect.Response[scoresv1.GetResultResponse], error) {
	client := scoresv1connect.NewResultsServiceClient(api.client, api.url, connect.WithGRPCWeb())
	req := connect.NewRequest(&scoresv1.GetResultRequest{Id: id})
	req.Header().Set("Origin", api.origin)
	return client.GetResult(ctx, req)
}

func (api backend) saveLocalResult(ctx context.Context, result *scoresv1.SaveLocalResultRequest) error {
	client := scoresv1connect.NewResultsServiceClient(api.client, api.url, connect.WithGRPCWeb())
	req := connect.NewRequest(result)
	req.Header().Set("Origin", api.origin)
	_, err := client.SaveLocalResult(ctx, req)
	return err
}

func assertRPCCode(t *testing.T, err error, want connect.Code) {
	t.Helper()
	if (want == 0 && err != nil) || (want != 0 && connect.CodeOf(err) != want) {
		t.Fatalf("RPC returned %v, want %v", err, want)
	}
}

func (api backend) waitForResult(t *testing.T, ctx context.Context, id string) *scoresv1.GetResultResponse {
	t.Helper()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		res, err := api.getResult(ctx, id)
		if err == nil {
			return res.Msg
		}
		if connect.CodeOf(err) != connect.CodeNotFound {
			t.Fatalf("GetResult(%s): %v", id, err)
		}
		select {
		case <-ctx.Done():
			t.Fatalf("result %s was queued but not persisted: %v", id, ctx.Err())
		case <-ticker.C:
		}
	}
}

func (api backend) request(t *testing.T, ctx context.Context, method, path string, status int, headers http.Header) *http.Response {
	t.Helper()
	res := api.send(t, ctx, method, path, headers)
	if res.StatusCode != status {
		res.Body.Close()
		t.Fatalf("%s %s: got status %s, want %d", method, path, res.Status, status)
	}
	return res
}

func (api backend) send(t *testing.T, ctx context.Context, method, path string, headers http.Header) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, method, api.url+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", api.origin)
	for name, values := range headers {
		req.Header[name] = values
	}
	res, err := api.client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return res
}

func sameResult(got *scoresv1.MatchResult, want arena.Result) bool {
	// PostgreSQL stores timestamps with microsecond precision.
	return got.GetId() == want.ID && got.GetName() == want.Name &&
		int(got.GetScore()) == want.Score && int(got.GetBotScore()) == want.BotScore &&
		got.GetLocal() == want.Local && got.GetFinishedAt() != nil &&
		got.GetFinishedAt().AsTime().Sub(want.FinishedAt).Abs() < time.Microsecond
}
