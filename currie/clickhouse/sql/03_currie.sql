-- The questions Currie asks, as queries instead of as columns.
--
--   docker compose exec -T clickhouse \
--     clickhouse-client --user currie --password currie --queries-file /sql/03_currie.sql

SELECT '--- 1. the pairing replayGame exists to do, across every game at once ---';
-- ASOF picks the doctrine in force at each evaluation's tick. The equi-join on
-- game_id is required by ASOF and is also what makes it correct across games.
-- 8938 doctrines against 1.2M evaluations, no replay loop.

SELECT
    g.our_faction      AS us,
    g.opponent_faction AS them,
    d.name             AS doctrine,
    count()            AS evaluations,
    countIf(e.fired)   AS fired
FROM currie.rule_evals AS e
ASOF INNER JOIN currie.doctrines AS d
    ON e.game_id = d.game_id AND e.tick >= d.tick
INNER JOIN currie.games AS g ON g.game_id = e.game_id
GROUP BY us, them, doctrine
ORDER BY evaluations DESC
LIMIT 10;

SELECT '--- 2. conjunct-level blame, without a blame table ---';
-- What the world looked like when these rules declined. The state is already
-- joined to the evaluation by state_idx, so "why not" is a question about a
-- distribution rather than a counter someone had to add.
--
-- This is the one question the sampled corpus answers honestly: it is about
-- the SHAPE of the declining states, not how many there were. `skipped` rows
-- are excluded -- a rule that lost its exclusive slot was never evaluated, so
-- the state it did not look at says nothing about why.
--
-- Currie's own BLAME for game 147 reaches the same place by a different road:
--   build-extra-war-factory  sole blocker 268 x (61%): require cash >= 2000
--   build-refinery           sole blocker  12 x  (2%): require cash >= lerp(...)

SELECT
    e.rule                                   AS rule,
    count()                                  AS declined,
    round(quantile(0.5)(s.scalars['cash']))  AS median_cash,
    round(quantile(0.9)(s.scalars['cash']))  AS p90_cash,
    countIf(has(s.flags, 'base-under-attack')) AS while_under_attack,
    countIf(s.collections['idle-harvesters'] > 0) AS while_harvesters_idle
FROM currie.rule_evals AS e
INNER JOIN currie.game_states AS s
    ON s.game_id = e.game_id AND s.state_idx = e.state_idx
WHERE e.fired = false AND e.skipped = false
  AND e.rule IN ('build-extra-war-factory', 'build-refinery', 'produce-vehicle')
GROUP BY rule
ORDER BY declined DESC;

SELECT '--- 3. what beats us, per opponent, over every game ---';
-- migration 0015 added enemy_units_seen_json to answer this, and could only
-- answer it for games played after it. This answers it for everything loaded.

SELECT
    g.opponent_faction                  AS them,
    count(DISTINCT g.game_id)           AS games,
    round(100 * countIf(g.won) / count(DISTINCT g.game_id), 1) AS pct_won_rows,
    round(avg(g.duration_ticks))        AS mean_ticks
FROM currie.games AS g
GROUP BY them
ORDER BY games DESC;

SELECT '--- 4. one rule, one game, at tick resolution ---';
-- Shape over the game, not totals: see the sampling note in 01_schema.sql.

SELECT
    intDiv(tick, 5000) * 5000 AS tick_bucket,
    countIf(fired)            AS fired,
    count()                   AS evaluated
FROM currie.rule_evals
WHERE game_id = 147 AND rule = 'form-ground-attack'
GROUP BY tick_bucket
ORDER BY tick_bucket;
