-- name: InsertGame :one
INSERT INTO games (
    played_at, our_faction, opponent_faction,
    map_width, map_height, duration_ticks,
    won, quality_tag, review_json
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id;

-- name: InsertDoctrine :exec
INSERT INTO archived_doctrines (
    game_id, tick, doctrine_json, rating, rating_reason
) VALUES (?, ?, ?, ?, ?);

-- name: InsertLesson :exec
INSERT INTO lessons (
    game_id, created_at, trigger_text, guidance_text,
    confidence, applies_faction, applies_vs
) VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: CountWins :one
SELECT COUNT(*) FROM games WHERE won = 1;

-- name: CountLosses :one
SELECT COUNT(*) FROM games WHERE won = 0;

-- name: ListGames :many
SELECT our_faction, won FROM games ORDER BY played_at ASC;

-- name: ListGamesDetailed :many
SELECT id, played_at, our_faction, opponent_faction,
       map_width, map_height, duration_ticks, won,
       quality_tag, review_json
FROM games
ORDER BY played_at DESC
LIMIT ? OFFSET ?;

-- name: GetGame :one
SELECT id, played_at, our_faction, opponent_faction,
       map_width, map_height, duration_ticks, won,
       quality_tag, review_json
FROM games WHERE id = ?;

-- name: QueryExemplarDoctrines :many
SELECT d.doctrine_json, d.rating, d.rating_reason,
       g.won, g.quality_tag, g.opponent_faction, g.played_at
FROM archived_doctrines d
JOIN games g ON d.game_id = g.id
WHERE g.our_faction = @our_faction
  AND (g.opponent_faction = @opponent_faction
       OR g.opponent_faction IS NULL
       OR @opponent_faction = 'unknown')
  AND g.quality_tag = 'exemplary'
  AND d.rating IN ('strong', 'adequate')
ORDER BY g.played_at DESC
LIMIT @lim;

-- name: QueryCautionaryDoctrines :many
SELECT d.doctrine_json, d.rating, d.rating_reason,
       g.won, g.quality_tag, g.opponent_faction, g.played_at
FROM archived_doctrines d
JOIN games g ON d.game_id = g.id
WHERE g.our_faction = @our_faction
  AND (g.opponent_faction = @opponent_faction
       OR g.opponent_faction IS NULL
       OR @opponent_faction = 'unknown')
  AND d.rating = 'weak'
ORDER BY g.played_at DESC
LIMIT @lim;

-- name: QueryLessons :many
SELECT trigger_text, guidance_text, confidence
FROM lessons
WHERE applies_faction = @our_faction
  AND (applies_vs = @opponent_faction
       OR applies_vs IS NULL
       OR @opponent_faction = 'unknown')
ORDER BY confidence DESC, created_at DESC
LIMIT @lim;

-- name: ListTopLessons :many
SELECT trigger_text, guidance_text, confidence,
       applies_faction, applies_vs
FROM lessons
ORDER BY confidence DESC, created_at DESC
LIMIT ?;
