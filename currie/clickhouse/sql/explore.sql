-- For clickhouse-local. No server, no ingest, no state:
--
--   docker compose run --rm local --queries-file /sql/explore.sql
--
-- Everything below reads ~/.vimy/exports and ~/.vimy/vimy.db in place.
--
-- CAVEAT: the exports are a SAMPLED corpus (1 in 15, capped at 20000 cases).
-- Absolute counts from them are wrong, and the undercount is not uniform.
-- See the header of 01_schema.sql. Ground truth for counts is rule_firings
-- in SQLite.

SELECT '--- the archive, from outside it ---';

SELECT
    count()                               AS exports,
    formatReadableSize(sum(length(json))) AS json_decompressed
FROM file('exports/*.json.gz', JSONAsString);

SELECT '--- rules that matched most, across every recorded game ---';

WITH evals AS
(
    SELECT
        _file AS f,
        arrayJoin(JSONExtract(json, 'cases',
            'Array(Tuple(tick UInt32, rule String, rule_set String, state UInt32, fired Bool, skipped Bool))')) AS c
    FROM file('exports/*.json.gz', JSONAsString)
)
SELECT
    c.rule                                     AS rule,
    count()                                    AS evaluations,
    countIf(c.fired)                           AS fired,
    round(100 * countIf(c.fired) / count(), 2) AS pct,
    uniq(f)                                    AS games
FROM evals
GROUP BY rule
ORDER BY evaluations DESC
LIMIT 20;

SELECT '--- what a rule was DOING when it declined, not how often ---';
-- NOT "rules that never fired". The corpus is sampled 1-in-15 and truncated at
-- 20000 cases, so a rule that fires once per game is usually missed entirely --
-- build-war-factory fired in 56 of these games and appears here 0 times.
-- Counting belongs in SQLite's rule_firings. This asks the question the sample
-- can answer: of the evaluations we DID see, how did they split?

WITH evals AS
(
    SELECT arrayJoin(JSONExtract(json, 'cases',
        'Array(Tuple(tick UInt32, rule String, rule_set String, state UInt32, fired Bool, skipped Bool))')) AS c
    FROM file('exports/*.json.gz', JSONAsString)
)
SELECT
    c.rule                 AS rule,
    count()                AS sampled,
    countIf(c.skipped)     AS lost_exclusive_slot,
    countIf(c.fired)       AS matched
FROM evals
GROUP BY rule
ORDER BY lost_exclusive_slot DESC
LIMIT 20;

SELECT '--- ASOF JOIN: each evaluation against the doctrine actually running ---';
-- The pairing replayGame exists to do, and the one the Python script got wrong.
-- `e.tick >= d.tick` picks the doctrine in force per row. Note the equi-join on
-- game_id: ASOF requires at least one, and it is also what makes this correct
-- across games rather than only within one.

WITH
    files AS
    (
        SELECT id AS game_id, basename(export_path) AS f
        FROM sqlite('vimy.db', 'games')
        WHERE notEmpty(ifNull(export_path, ''))
    ),
    raw AS
    (
        SELECT
            _file AS f,
            arrayJoin(JSONExtract(json, 'cases',
                'Array(Tuple(tick UInt32, rule String, rule_set String, state UInt32, fired Bool, skipped Bool))')) AS c
        FROM file('exports/export-20260920-164429.json.gz', JSONAsString)
    ),
    e AS
    (
        SELECT
            fi.game_id AS game_id,
            r.c.tick   AS tick,
            r.c.rule   AS rule,
            r.c.fired  AS fired
        FROM raw AS r
        INNER JOIN files AS fi ON fi.f = r.f
    )
SELECT
    d.name             AS doctrine,
    d.tick             AS in_force_from,
    count()            AS evaluations,
    countIf(e.fired)   AS fired,
    topK(3)(e.rule)    AS busiest_rules
FROM e
ASOF INNER JOIN
(
    SELECT game_id, tick, JSONExtractString(doctrine_json, 'name') AS name
    FROM sqlite('vimy.db', 'archived_doctrines')
) AS d
ON e.game_id = d.game_id AND e.tick >= d.tick
GROUP BY doctrine, in_force_from
ORDER BY in_force_from
LIMIT 30;
