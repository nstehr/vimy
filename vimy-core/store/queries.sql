-- name: InsertGame :one
INSERT INTO games (
    played_at, our_faction, opponent_faction,
    map_width, map_height, duration_ticks,
    won, quality_tag, review_json, export_path, directive
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id;

-- name: InsertDoctrine :one
INSERT INTO archived_doctrines (
    game_id, tick, doctrine_json, rating, rating_reason, rule_set_json
) VALUES (?, ?, ?, ?, ?, ?)
RETURNING id;

-- name: InsertRuleFiring :exec
INSERT INTO rule_firings (
    doctrine_id, rule_name, fire_count, act_count, first_tick, last_tick
) VALUES (?, ?, ?, ?, ?, ?);

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

-- name: ListReplayableGames :many
-- Games with a recorded state export, newest first: the ones Currie can replay.
SELECT id, played_at, our_faction, opponent_faction,
       duration_ticks, won, quality_tag, export_path, directive
FROM games
WHERE export_path IS NOT NULL AND export_path != ''
ORDER BY played_at DESC;

-- name: ListDoctrinesForGame :many
-- Every doctrine window of one game, in the order they took effect.
SELECT id, tick, doctrine_json, rating, rating_reason
FROM archived_doctrines
WHERE game_id = ?
ORDER BY tick;

-- name: SetGameExportPath :exec
-- Backfill: associate an export with a game recorded before provenance existed.
UPDATE games SET export_path = ? WHERE id = ?;

-- name: ListAllGames :many
SELECT id, played_at, our_faction, opponent_faction,
       duration_ticks, won, quality_tag, export_path, directive
FROM games ORDER BY played_at DESC;

-- name: ListFiringsForGame :many
-- What each rule actually did during the game, summed over its doctrine
-- windows. act_count is NULL for games recorded before it was measured.
SELECT f.rule_name, SUM(f.fire_count) AS matched, SUM(f.act_count) AS acted
FROM rule_firings f
JOIN archived_doctrines d ON d.id = f.doctrine_id
WHERE d.game_id = ?
GROUP BY f.rule_name;
