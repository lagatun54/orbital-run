package api

import (
	"context"
	"net/http"

	"connectrpc.com/connect"

	scoresv1 "orbitalrun/api/scores/v1"
	"orbitalrun/api/scores/v1/scoresv1connect"
)

type scoresGateway struct {
	leaderboard scoresv1connect.LeaderboardServiceClient
	results     scoresv1connect.ResultsServiceClient
}

// NewScoresGateway accepts browser RPCs and calls the private scores service via native gRPC.
func NewScoresGateway(client *http.Client, url string) http.Handler {
	gateway := scoresGateway{
		leaderboard: scoresv1connect.NewLeaderboardServiceClient(client, url, connect.WithGRPC()),
		results:     scoresv1connect.NewResultsServiceClient(client, url, connect.WithGRPC()),
	}
	return scoresRPCs(gateway, gateway)
}

func scoresRPCs(leaderboard scoresv1connect.LeaderboardServiceHandler, results scoresv1connect.ResultsServiceHandler) *http.ServeMux {
	mux := http.NewServeMux()
	limit := connect.WithReadMaxBytes(2048)
	path, handler := scoresv1connect.NewLeaderboardServiceHandler(leaderboard, limit)
	mux.Handle(path, handler)
	path, handler = scoresv1connect.NewResultsServiceHandler(results, limit)
	mux.Handle(path, handler)
	return mux
}

func (g scoresGateway) GetLeaderboard(ctx context.Context, req *connect.Request[scoresv1.GetLeaderboardRequest]) (*connect.Response[scoresv1.GetLeaderboardResponse], error) {
	return forwardRPC(ctx, req, g.leaderboard.GetLeaderboard)
}

func (g scoresGateway) GetResult(ctx context.Context, req *connect.Request[scoresv1.GetResultRequest]) (*connect.Response[scoresv1.GetResultResponse], error) {
	return forwardRPC(ctx, req, g.results.GetResult)
}

func (g scoresGateway) SaveLocalResult(ctx context.Context, req *connect.Request[scoresv1.SaveLocalResultRequest]) (*connect.Response[scoresv1.SaveLocalResultResponse], error) {
	return forwardRPC(ctx, req, g.results.SaveLocalResult)
}

func forwardRPC[Req, Res any](ctx context.Context, req *connect.Request[Req], call func(context.Context, *connect.Request[Req]) (*connect.Response[Res], error)) (*connect.Response[Res], error) {
	// Recreate the request and response so protocol headers stay on their own connection.
	resp, err := call(ctx, connect.NewRequest(req.Msg))
	if err != nil {
		if connect.IsWireError(err) {
			return nil, err
		}
		return nil, scoresUnavailable(err)
	}
	return connect.NewResponse(resp.Msg), nil
}
