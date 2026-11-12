CREATE TABLE IF NOT EXISTS match_results (
    id text PRIMARY KEY,
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 20),
    score integer NOT NULL CHECK (score >= 0),
    bot_score integer NOT NULL CHECK (bot_score >= 0),
    finished_at timestamptz NOT NULL
);

CREATE INDEX IF NOT EXISTS match_results_leaderboard_idx
    ON match_results (score DESC, finished_at ASC, id ASC);
