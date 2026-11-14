SELECT id, name, score, bot_score, finished_at, local
FROM match_results
ORDER BY score DESC, finished_at ASC, id ASC
LIMIT 10
