-- Currie's archive, as columns.
--
-- The point of the shape: nothing here is an aggregate. Every number Currie
-- reports -- spend, blame, firings, the trade -- is a SELECT over these rows,
-- not a column someone had to add a migration for. A new question is a new
-- query, and it runs over every game already loaded.

CREATE DATABASE IF NOT EXISTS currie;

-- One row per rule per SAMPLED evaluation.
--
-- READ THIS BEFORE COUNTING ANYTHING. These rows come from -export-states,
-- which exists to build vimyc's differential corpus, not to measure games:
--
--   * 1 evaluation in 15 is recorded (-export-every 15), and begin() is called
--     again after every firing, so the sampling is not uniform in ticks;
--   * recording stops dead at -export-max 20000 cases. 37 of 69 exports sit at
--     that cap, covering a mean 85% of their game's ticks.
--
-- Measured against rule_firings in SQLite over the same 69 games, the
-- undercount is not a constant: repair-buildings 27x, produce-vehicle 37x,
-- build-power 25x. So neither counts NOR rates off this table mean anything
-- in absolute terms. build-war-factory fires exactly once per game and shows
-- 0 of 56 firings here for precisely this reason.
--
-- What it IS good for: state-conditioned questions. "What did the world look
-- like when this rule declined" is answerable, because every sampled row
-- carries its state. For counting, use SQLite's rule_firings, or give the
-- engine a real unsampled event sink.
CREATE TABLE IF NOT EXISTS currie.rule_evals
(
    game_id    UInt32,
    tick       UInt32,
    rule       LowCardinality(String),
    rule_set   LowCardinality(String),  -- hash of the compiled set in force
    state_idx  UInt32,                  -- index into currie.game_states
    fired      Bool,
    skipped    Bool
)
ENGINE = MergeTree
-- game first because almost every question is scoped to a game; rule second
-- because "what happened to THIS rule" is the second most common cut.
ORDER BY (game_id, rule, tick);

-- The sampled world the rules were evaluated against. Maps, not columns:
-- a new predicate in the sidecar shows up here without a schema change.
CREATE TABLE IF NOT EXISTS currie.game_states
(
    game_id      UInt32,
    state_idx    UInt32,
    scalars      Map(LowCardinality(String), Float64),
    flags        Array(LowCardinality(String)),
    present      Array(LowCardinality(String)),
    collections  Map(LowCardinality(String), Int64),
    calls_bool   Array(String),
    calls_int    Map(String, Int64),
    calls_float  Map(String, Float64),
    type_counts  Map(LowCardinality(String), Int64)
)
ENGINE = MergeTree
ORDER BY (game_id, state_idx);

-- Entities, copied from SQLite. SQLite stays the source of truth for these:
-- they are small, mutable and relational, which is everything ClickHouse is
-- bad at. Only the events above belong here.
CREATE TABLE IF NOT EXISTS currie.games
(
    game_id          UInt32,
    played_at        DateTime,
    our_faction      LowCardinality(String),
    opponent_faction LowCardinality(String),
    won              Bool,
    duration_ticks   UInt32,
    directive        String,
    export_file      String            -- basename, joins to file()'s _file
)
ENGINE = ReplacingMergeTree
ORDER BY game_id;

CREATE TABLE IF NOT EXISTS currie.doctrines
(
    game_id       UInt32,
    doctrine_id   UInt32,
    tick          UInt32,              -- the tick it came into force
    name          String,
    rating        LowCardinality(String),
    doctrine_json String
)
ENGINE = ReplacingMergeTree
ORDER BY (game_id, tick, doctrine_id);
