-- name: InsertGame :one
INSERT INTO games (
    played_at, our_faction, opponent_faction,
    map_width, map_height, duration_ticks,
    won, quality_tag, review_json
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id;

-- name: InsertDoctrine :one
INSERT INTO archived_doctrines (
    game_id, tick, doctrine_json, rating, rating_reason, rule_set_json
) VALUES (?, ?, ?, ?, ?, ?)
RETURNING id;

-- name: InsertRuleFiring :exec
INSERT INTO rule_firings (
    doctrine_id, rule_name, fire_count, first_tick, last_tick
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
-- Opponent faction is NOT filtered here; the librarian judges cross-opponent
-- relevance semantically. SQL only scopes to our faction + rating quality.
SELECT d.doctrine_json, d.rating, d.rating_reason,
       g.won, g.quality_tag, g.opponent_faction, g.played_at
FROM archived_doctrines d
JOIN games g ON d.game_id = g.id
WHERE g.our_faction = @our_faction
  AND g.quality_tag = 'exemplary'
  AND d.rating IN ('strong', 'adequate')
ORDER BY g.played_at DESC
LIMIT @lim;

-- name: QueryCautionaryDoctrines :many
-- Opponent faction is NOT filtered here; the librarian decides whether the
-- failure mode transfers across opponents.
SELECT d.doctrine_json, d.rating, d.rating_reason,
       g.won, g.quality_tag, g.opponent_faction, g.played_at
FROM archived_doctrines d
JOIN games g ON d.game_id = g.id
WHERE g.our_faction = @our_faction
  AND d.rating = 'weak'
ORDER BY g.played_at DESC
LIMIT @lim;

-- name: QueryLessons :many
-- Opponent faction is NOT filtered here; lessons are often generalizable
-- across opponents and the librarian will drop what doesn't apply.
SELECT trigger_text, guidance_text, confidence
FROM lessons
WHERE applies_faction = @our_faction
ORDER BY confidence DESC, created_at DESC
LIMIT @lim;

-- name: ListTopLessons :many
SELECT trigger_text, guidance_text, confidence,
       applies_faction, applies_vs
FROM lessons
ORDER BY confidence DESC, created_at DESC
LIMIT ?;
