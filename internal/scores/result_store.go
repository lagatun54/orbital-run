package scores

import (
	"context"
	"errors"

	"orbitalrun/internal/arena"
)

// ErrResultNotFound is returned when the requested match ID has no stored result.
var ErrResultNotFound = errors.New("match result not found")

// RankedResult adds the result's overall leaderboard position.
type RankedResult struct {
	arena.Result
	Rank int64 `json:"rank"`
}

// ResultStore provides the storage operations used by the scores RPCs.
type ResultStore interface {
	ResultWriter
	Top(ctx context.Context) ([]arena.Result, error)
	FindRanked(ctx context.Context, id string) (RankedResult, error)
}
