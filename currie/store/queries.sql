-- name: GetInsight :one
SELECT insight_json FROM insights
WHERE game_id = ? AND rules = ?;

-- name: PutInsight :exec
INSERT INTO insights (game_id, rules, created_at, summary, suggestion, insight_json)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (game_id, rules) DO UPDATE SET
    created_at   = excluded.created_at,
    summary      = excluded.summary,
    suggestion   = excluded.suggestion,
    insight_json = excluded.insight_json;

-- name: DropInsight :exec
DELETE FROM insights WHERE game_id = ? AND rules = ?;

-- name: ListInsights :many
-- Every reading made against one rule set, newest first. What the cross-game
-- work reads.
SELECT game_id, summary, suggestion, created_at FROM insights
WHERE rules = ?
ORDER BY created_at DESC;
