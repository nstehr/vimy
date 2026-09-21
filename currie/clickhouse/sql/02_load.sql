-- Ingest. Re-runnable: it truncates the event tables and reloads from the
-- archive, so the archive stays the source of truth and this stays disposable.
--
-- No ETL script, deliberately -- ClickHouse reads the .json.gz and the SQLite
-- file directly, so the whole pipeline is four INSERT ... SELECTs.

-- All four, not just the event tables. games and doctrines are
-- ReplacingMergeTree, and Replacing only collapses duplicates when parts merge
-- -- which is whenever the server feels like it, or never. Re-running this
-- without truncating them leaves two rows per game in currie.games, and since
-- the loads below INNER JOIN against it to resolve game_id, every rule_eval
-- lands twice. Ask for `FINAL` or dedupe in the query when reading Replacing
-- tables; do not assume a count() is deduplicated.
TRUNCATE TABLE IF EXISTS currie.rule_evals;
TRUNCATE TABLE IF EXISTS currie.game_states;
TRUNCATE TABLE IF EXISTS currie.games;
TRUNCATE TABLE IF EXISTS currie.doctrines;

-- Entities first: the export loads join against games to resolve game_id.
INSERT INTO currie.games
SELECT
    id,
    toDateTime(played_at),
    our_faction,
    ifNull(opponent_faction, ''),
    won != 0,
    duration_ticks,
    ifNull(directive, ''),
    basename(export_path)
FROM sqlite('vimy.db', 'games')
WHERE notEmpty(ifNull(export_path, ''));

INSERT INTO currie.doctrines
SELECT
    game_id,
    id,
    tick,
    JSONExtractString(doctrine_json, 'name'),
    ifNull(rating, ''),
    doctrine_json
FROM sqlite('vimy.db', 'archived_doctrines');

-- The evaluations. JSONAsString reads each export as one row; JSONExtract
-- with a named-tuple type turns .cases into a typed array in one pass.
INSERT INTO currie.rule_evals
SELECT
    g.game_id,
    x.c.tick,
    x.c.rule,
    x.c.rule_set,
    x.c.state,
    x.c.fired,
    x.c.skipped
FROM
(
    SELECT
        _file AS f,
        arrayJoin(JSONExtract(json, 'cases',
            'Array(Tuple(tick UInt32, rule String, rule_set String, state UInt32, fired Bool, skipped Bool))')) AS c
    FROM file('exports/*.json.gz', JSONAsString)
) AS x
INNER JOIN currie.games AS g ON g.export_file = x.f;

-- The states. arrayEnumerate recovers state_idx, which is what `cases.state`
-- points at -- the index is the join key, so it has to survive the flattening.
INSERT INTO currie.game_states
SELECT
    g.game_id,
    x.i - 1,
    x.s.scalars,
    x.s.flags,
    x.s.present,
    x.s.collections,
    x.s.calls_bool,
    x.s.calls_int,
    x.s.calls_float,
    x.s.type_counts
FROM
(
    SELECT
        f,
        z.1 AS i,
        z.2 AS s
    FROM
    (
        SELECT
            _file AS f,
            JSONExtract(json, 'states',
                'Array(Tuple(scalars Map(String, Float64), flags Array(String), present Array(String), collections Map(String, Int64), calls_bool Array(String), calls_int Map(String, Int64), calls_float Map(String, Float64), type_counts Map(String, Int64)))') AS arr
        FROM file('exports/*.json.gz', JSONAsString)
    )
    ARRAY JOIN arrayZip(arrayEnumerate(arr), arr) AS z
) AS x
INNER JOIN currie.games AS g ON g.export_file = x.f;
