-- Local matches use the database receipt time. Online matches use their finish time.
INSERT INTO match_results (id, name, score, bot_score, finished_at, local)
VALUES ($1, $2, $3, $4, CASE WHEN $6 THEN now() ELSE $5 END, $6)
ON CONFLICT (id) DO NOTHING
RETURNING finished_at
