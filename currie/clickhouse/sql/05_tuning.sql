-- Did that change move the number?
--
-- The question the archive could not answer. 147 games, no column saying which
-- rule sources played any of them, so squad spread sat near 20 cells against a
-- required 8 through four separate fixes -- each judged by eye against the next
-- game or two, against a metric whose game-to-game noise is larger than the
-- effect being chased.

SELECT '--- 1. rally spread by rule sources, pooled at the rally ---';
-- Pooled across games, not averaged over per-game means. Game 146's mean
-- spread of 17.0 was ONE rally and a per-game mean gives it the same weight as
-- game 135's 763. `rallies` is the denominator that was missing.

SELECT
    s.rules_digest                  AS rules,
    count()                         AS rallies,
    uniq(e.session_id)              AS games,
    round(avg(e.spread), 1)         AS mean_spread,
    round(quantile(0.5)(e.spread))  AS median,
    round(quantile(0.9)(e.spread))  AS p90,
    countIf(e.spread <= 8)          AS clumped,
    round(100 * countIf(e.spread <= 8) / count(), 1) AS pct_clumped,
    -- The standard error of that mean. An effect smaller than a couple of
    -- these is the noise four fixes were read against.
    round(stddevSamp(e.spread) / sqrt(count()), 2) AS stderr
FROM currie.stream_events AS e
INNER JOIN currie.stream_sessions AS s FINAL USING (session_id)
WHERE e.kind = 'rally'
GROUP BY rules
ORDER BY rules;

SELECT '--- 2. the rally diagnosis: radius too tight, or order cannot reach? ---';
-- Three numbers per rally settle it, and only per rally.
--   members ~= idle   the rally reaches everyone; the radius is the fault
--   idle much lower   the rally cannot reach the squad; the predicate is
-- See rules/assault_phase.go.

SELECT
    s.rules_digest            AS rules,
    round(avg(e.members), 2)  AS mean_members,
    round(avg(e.idle), 2)     AS mean_commandable,
    round(avg(e.idle) / avg(e.members), 2) AS reachable_fraction
FROM currie.stream_events AS e
INNER JOIN currie.stream_sessions AS s FINAL USING (session_id)
WHERE e.kind = 'rally'
GROUP BY rules
ORDER BY rules;

SELECT '--- 3. the strike funnel, per blocker, per rule sources ---';
-- strike_blocked_no_target had to be SPLIT in two because one number hid two
-- causes with opposite fixes, and the split could not be applied backwards.
-- A row per occurrence cannot hide anything, and the next cause is a new
-- `reason` value, not a migration.

SELECT
    s.rules_digest AS rules,
    e.reason       AS blocker,
    count()        AS blocked,
    round(count() / uniq(e.session_id), 1) AS per_game
FROM currie.stream_events AS e
INNER JOIN currie.stream_sessions AS s FINAL USING (session_id)
WHERE e.kind = 'strike-blocked'
GROUP BY rules, blocker
ORDER BY rules, blocked DESC;

SELECT '--- 4. sessions whose numbers are a floor, not a number ---';
-- The log drops rather than stall the game. Anything non-zero here means the
-- counts above undercount, and by how much is knowable.

SELECT session_id, game_id, rows_written, rows_dropped, modified
FROM currie.stream_sessions FINAL
WHERE rows_dropped > 0 OR modified
ORDER BY session_id;

SELECT '--- 5. transit: WHEN did the squad stop closing? ---';
-- The question the three bands cannot answer. Game 148 reduces 118920 ticks to
-- 76 far / 5 mid / 0 near; this is the same data as a trajectory. A squad that
-- closes and stops shows a floor in min_fraction that never falls further.

SELECT
    s.rules_digest                             AS rules,
    intDiv(e.tick, 10000) * 10000              AS tick_bucket,
    count()                                    AS samples,
    round(min(e.attrs['target_fraction']), 3)  AS closest,
    round(avg(e.attrs['target_fraction']), 3)  AS mean_fraction,
    round(avg(e.spread), 1)                    AS mean_spread,
    round(avg(e.near) / nullIf(avg(e.members), 0), 2) AS cohesion
FROM currie.stream_events AS e
INNER JOIN currie.stream_sessions AS s FINAL USING (session_id)
WHERE e.kind = 'transit'
GROUP BY rules, tick_bucket
ORDER BY rules, tick_bucket;

SELECT '--- 6. the bands, chosen at query time ---';
-- The same three numbers the archive stores, recomputed from the samples. The
-- boundaries are in the query now, not in storage: change 0.20 here and every
-- game already recorded re-bands with it. That is the test the 0.32 threshold
-- change needs, and the archive cannot run it on anything played before it.

SELECT
    s.rules_digest AS rules,
    multiIf(e.attrs['target_fraction'] > 0.35, 'far',
            e.attrs['target_fraction'] > 0.20, 'mid',
                                               'near') AS band,
    count()                 AS samples,
    round(avg(e.members), 1) AS mean_members,
    round(avg(e.near), 1)    AS mean_within_radius,
    round(avg(e.spread), 1)  AS mean_spread
FROM currie.stream_events AS e
INNER JOIN currie.stream_sessions AS s FINAL USING (session_id)
WHERE e.kind = 'transit'
GROUP BY rules, band
ORDER BY rules, band;
