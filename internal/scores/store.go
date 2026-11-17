package scores

import (
	"context"
	"log/slog"

	"orbitalrun/internal/arena"
)

type resultRepository interface {
	resultReader
	// Insert returns the committed result. A duplicate returns false and no error.
	Insert(ctx context.Context, result arena.Result) (arena.Result, bool, error)
}

// LeaderboardCache stores snapshots of the leaderboard and applies committed results.
type LeaderboardCache interface {
	// Lookup returns a non-nil slice on a hit, including for an empty leaderboard.
	// On a miss, a non-empty token grants this caller permission to fill the cache.
	Lookup(ctx context.Context) (entries []arena.Result, token string, err error)
	// Fill publishes the snapshot only while the token is still valid.
	Fill(ctx context.Context, token string, entries []arena.Result) error
	// Update applies a committed result or invalidates a snapshot that can no longer be used.
	Update(ctx context.Context, result arena.Result) error
}

// Store coordinates result persistence and an optional leaderboard cache.
type Store struct {
	repository resultRepository
	cache      LeaderboardCache
}

func NewStore(repository resultRepository, cache LeaderboardCache) *Store {
	return &Store{repository: repository, cache: cache}
}

func (s *Store) Save(ctx context.Context, result arena.Result) error {
	saved, inserted, err := s.repository.Insert(ctx, result)
	if err != nil || !inserted || s.cache == nil {
		return err
	}

	// The result is committed even if the caller has since canceled the request.
	if err := s.cache.Update(context.WithoutCancel(ctx), saved); err != nil {
		slog.Debug("update leaderboard cache", "error", err)
	}
	return nil
}

func (s *Store) Top(ctx context.Context) ([]arena.Result, error) {
	if s.cache == nil {
		return s.repository.Top(ctx)
	}
	cached, token, err := s.cache.Lookup(ctx)
	if err == nil && cached != nil {
		return cached, nil
	}

	entries, err := s.repository.Top(ctx)
	if err == nil && token != "" {
		if err := s.cache.Fill(ctx, token, entries); err != nil {
			slog.Debug("fill leaderboard cache", "error", err)
		}
	}
	return entries, err
}

func (s *Store) FindRanked(ctx context.Context, id string) (RankedResult, error) {
	return s.repository.FindRanked(ctx, id)
}
