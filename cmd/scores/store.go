package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"orbitalrun/internal/arena"
)

//go:embed schema.sql
var schema string

var errResultNotFound = errors.New("match result not found")

type rankedResult struct {
	arena.Result
	Rank int64 `json:"rank"`
}

type store struct {
	db *pgxpool.Pool
}

func openStore(ctx context.Context, url string) (*store, error) {
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(ctx, schema, pgx.QueryExecModeSimpleProtocol); err != nil {
		db.Close()
		return nil, err
	}
	return &store{db: db}, nil
}

func (s *store) Close() {
	s.db.Close()
}

func (s *store) Top(ctx context.Context) ([]arena.Result, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	rows, err := s.db.Query(ctx, `SELECT id, name, score, bot_score, finished_at, local
		FROM match_results ORDER BY score DESC, finished_at ASC, id ASC LIMIT 10`)
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

func (s *store) FindRanked(ctx context.Context, id string) (rankedResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	var result rankedResult
	// Count disjoint ranges separately so PostgreSQL can use an index-only scan for each.
	err := s.db.QueryRow(ctx, `SELECT target.id, target.name, target.score, target.bot_score, target.finished_at, target.local,
		1 + (SELECT count(*) FROM match_results AS ahead
			WHERE ahead.score > target.score)
		  + (SELECT count(*) FROM match_results AS ahead
			WHERE ahead.score = target.score AND (ahead.finished_at, ahead.id) < (target.finished_at, target.id))
		FROM match_results AS target WHERE target.id = $1`, id).
		Scan(&result.ID, &result.Name, &result.Score, &result.BotScore, &result.FinishedAt, &result.Local, &result.Rank)
	if errors.Is(err, pgx.ErrNoRows) {
		return rankedResult{}, errResultNotFound
	}
	return result, err
}

func (s *store) Save(ctx context.Context, result arena.Result) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	// Local matches use the database's receipt time; online matches carry their finish time.
	_, err := s.db.Exec(ctx, `INSERT INTO match_results (id, name, score, bot_score, finished_at, local)
		VALUES ($1, $2, $3, $4, CASE WHEN $6 THEN now() ELSE $5 END, $6)
		ON CONFLICT (id) DO NOTHING`,
		result.ID, result.Name, result.Score, result.BotScore, result.FinishedAt, result.Local)
	if err != nil {
		return fmt.Errorf("store match %s: %w", result.ID, err)
	}
	return nil
}
