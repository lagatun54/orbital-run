-- Count disjoint ranges separately so PostgreSQL can use an index-only scan for each.
SELECT target.id, target.name, target.score, target.bot_score, target.finished_at, target.local,
       1 + (SELECT count(*) FROM match_results AS ahead
            WHERE ahead.score > target.score)
         + (SELECT count(*) FROM match_results AS ahead
            WHERE ahead.score = target.score
              AND (ahead.finished_at, ahead.id) < (target.finished_at, target.id))
FROM match_results AS target
WHERE target.id = $1
