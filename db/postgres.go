package db

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"orbitalrun/internal/arena"
	"orbitalrun/internal/scores"
)

//go:embed migration/schema.sql
var schema string

//go:embed query/top_results.sql
var topResultsSQL string

//go:embed query/find_ranked_result.sql
var findRankedResultSQL string

//go:embed query/save_result.sql
var saveResultSQL string

// PostgresStore persists results and queries their leaderboard positions.
type PostgresStore struct {
	db *pgxpool.Pool
}

// OpenPostgres connects to PostgreSQL and prepares the results table.
func OpenPostgres(ctx context.Context, url string) (*PostgresStore, error) {
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(ctx, schema, pgx.QueryExecModeSimpleProtocol); err != nil {
		db.Close()
		return nil, err
	}
	return &PostgresStore{db: db}, nil
}

func (s *PostgresStore) Close() {
	s.db.Close()
}

// Ready checks that PostgreSQL is reachable without reading from the cache.
func (s *PostgresStore) Ready(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return s.db.Ping(ctx)
}

func (s *PostgresStore) Top(ctx context.Context) ([]arena.Result, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	rows, err := s.db.Query(ctx, topResultsSQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make([]arena.Result, 0, 10)
	for rows.Next() {
		var result arena.Result
		if err := rows.Scan(&result.ID, &result.Name, &result.Score, &result.BotScore, &result.FinishedAt, &result.Local); err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, rows.Err()
}

func (s *PostgresStore) FindRanked(ctx context.Context, id string) (scores.RankedResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	var result scores.RankedResult
	err := s.db.QueryRow(ctx, findRankedResultSQL, id).
		Scan(&result.ID, &result.Name, &result.Score, &result.BotScore, &result.FinishedAt, &result.Local, &result.Rank)
	if errors.Is(err, pgx.ErrNoRows) {
		return scores.RankedResult{}, scores.ErrResultNotFound
	}
	return result, err
}

// Insert returns the result with the timestamp assigned by PostgreSQL.
func (s *PostgresStore) Insert(ctx context.Context, result arena.Result) (arena.Result, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	err := s.db.QueryRow(ctx, saveResultSQL,
		result.ID, result.Name, result.Score, result.BotScore, result.FinishedAt, result.Local).Scan(&result.FinishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return arena.Result{}, false, nil
	}
	if err != nil {
		return arena.Result{}, false, fmt.Errorf("store match %s: %w", result.ID, err)
	}
	return result, true, nil
}
