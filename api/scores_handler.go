package api

import (
	"context"
	"errors"
	"log"
	"net/http"
	"regexp"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	scoresv1 "orbitalrun/api/scores/v1"
	"orbitalrun/internal/arena"
	"orbitalrun/internal/scores"
)

var localMatchID = regexp.MustCompile(`^local-[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

type scoresHandler struct {
	store scores.ResultStore
}

// NewScoresHandler serves the internal RPCs and a readiness probe.
func NewScoresHandler(store scores.ResultStore, ready func(context.Context) error) http.Handler {
	service := scoresHandler{store: store}
	router := scoresRPCs(service, service)
	router.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := ready(r.Context()); err != nil {
			http.Error(w, "scores unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	return router
}

func (h scoresHandler) GetLeaderboard(ctx context.Context, _ *connect.Request[scoresv1.GetLeaderboardRequest]) (*connect.Response[scoresv1.GetLeaderboardResponse], error) {
	results, err := h.store.Top(ctx)
	if err != nil {
		return nil, scoresUnavailable(err)
	}
	entries := make([]*scoresv1.MatchResult, 0, len(results))
	for _, result := range results {
		entries = append(entries, matchResultMessage(result))
	}
	return connect.NewResponse(&scoresv1.GetLeaderboardResponse{Entries: entries}), nil
}

func (h scoresHandler) GetResult(ctx context.Context, req *connect.Request[scoresv1.GetResultRequest]) (*connect.Response[scoresv1.GetResultResponse], error) {
	id := req.Msg.GetId()
	if !scores.ValidOnlineResultID(id) && !localMatchID.MatchString(id) {
		return nil, resultNotFound()
	}
	result, err := h.store.FindRanked(ctx, id)
	if errors.Is(err, scores.ErrResultNotFound) {
		return nil, resultNotFound()
	}
	if err != nil {
		return nil, scoresUnavailable(err)
	}
	return connect.NewResponse(&scoresv1.GetResultResponse{
		Result: matchResultMessage(result.Result),
		Rank:   result.Rank,
	}), nil
}

func (h scoresHandler) SaveLocalResult(ctx context.Context, req *connect.Request[scoresv1.SaveLocalResultRequest]) (*connect.Response[scoresv1.SaveLocalResultResponse], error) {
	input := req.Msg
	if !localMatchID.MatchString(input.GetId()) || !validLocalScore(input.Score) || !validLocalScore(input.BotScore) {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid local result"))
	}

	// The client reports this result, so mark it as local on the leaderboard.
	result := arena.Result{
		ID:       input.GetId(),
		Name:     arena.PilotName(input.GetName()),
		Score:    int(input.GetScore()),
		BotScore: int(input.GetBotScore()),
		Local:    true,
	}
	if err := h.store.Save(ctx, result); err != nil {
		return nil, scoresUnavailable(err)
	}
	return connect.NewResponse(&scoresv1.SaveLocalResultResponse{}), nil
}

func validLocalScore(score *int32) bool {
	return score != nil && *score >= 0 && *score <= 300
}

func matchResultMessage(result arena.Result) *scoresv1.MatchResult {
	return &scoresv1.MatchResult{
		Id:         result.ID,
		Name:       result.Name,
		Score:      int32(result.Score),
		BotScore:   int32(result.BotScore),
		FinishedAt: timestamppb.New(result.FinishedAt),
		Local:      result.Local,
	}
}

func resultNotFound() error {
	return connect.NewError(connect.CodeNotFound, scores.ErrResultNotFound)
}

func scoresUnavailable(err error) error {
	log.Printf("scores: %v", err)
	return connect.NewError(connect.CodeUnavailable, errors.New("scores unavailable"))
}
