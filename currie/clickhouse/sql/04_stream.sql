-- The streamed side of the archive.
--
-- Everything in 01_schema.sql is loaded from files after a game ends, sampled
-- 1-in-15, and cannot be counted. These tables are fed by the shipper from the
-- sidecar's write-ahead log while the game is still running, unsampled -- so
-- `countIf(fired)` here means what it says, and build-war-factory's one firing
-- per game is one row, not a coin flip against the sampler.

CREATE DATABASE IF NOT EXISTS currie;

CREATE TABLE IF NOT EXISTS currie.stream_rule_evals
(
    session_id  LowCardinality(String),
    tick        UInt32,
    rule        LowCardinality(String),
    rule_set    LowCardinality(String),
    -- -1 when the evaluation was streamed without a projection, which is most
    -- of them: projecting costs ~60x evaluating, so state stays sampled even
    -- though the evaluations no longer are. Join to game_states only where
    -- this is >= 0, and never read it as "no state existed".
    state_idx   Int32,
    fired       Bool,
    skipped     Bool
)
ENGINE = MergeTree
ORDER BY (session_id, rule, tick)
-- The shipper retries a segment whose INSERT landed but whose ledger row did
-- not, carrying the same insert_deduplication_token. Without a window here
-- that retry doubles the segment; with one ClickHouse drops the repeated
-- block. 1000 is roughly a game's worth of segments at any sane segment size.
SETTINGS non_replicated_deduplication_window = 1000;

-- One row per game the sidecar streamed. game_id is 0 until the retrospective
-- archives the game: SQLite hands out the id at archive time, so nothing
-- during the game can know it, and every cross-game query joins through here.
CREATE TABLE IF NOT EXISTS currie.stream_sessions
(
    session_id LowCardinality(String),
    started_at DateTime,
    game_id    UInt32,

    -- WHAT PLAYED THE GAME. This is the dimension the archive never had, and
    -- the reason a rule change could not be evaluated: 147 games and no way to
    -- group them by the code that produced them. Squad spread sat near 20 cells
    -- against a required 8 through four separate fixes, each judged by eye
    -- against the next game or two.
    --
    -- rules_digest fingerprints the compiled rule set for a FIXED doctrine, so
    -- it moves when the .vy sources or vimyc's codegen move and stays put when
    -- the LLM merely picks different weights. revision is the sidecar build.
    -- A refactor that changes no behaviour shows a new revision and the same
    -- digest, which is the answer you want.
    rules_digest LowCardinality(String),
    revision     LowCardinality(String),
    -- Built from a dirty tree. A before/after that spans one of these is not
    -- an answer, so it is a column rather than a footnote.
    modified     Bool,


    -- The log drops rows rather than stall the game. Non-zero means any count
    -- from this session is a floor.
    rows_written UInt64,
    rows_dropped UInt64
)
ENGINE = ReplacingMergeTree
ORDER BY session_id;

-- What the shipper has moved. Not load-bearing -- deduplication is what makes
-- the pipeline correct -- but it is how the shipper avoids re-reading files it
-- has already sent, and how a human answers "did the tail of that game land".
CREATE TABLE IF NOT EXISTS currie.stream_segments
(
    session_id  LowCardinality(String),
    segment     String,
    rows        UInt64,
    bytes       UInt64,
    ingested_at DateTime
)
ENGINE = ReplacingMergeTree
ORDER BY (session_id, segment);

-- One row per occurrence of the sparse things worth analysing: a rally, a
-- strike that did not happen.
--
-- This replaces nothing -- the per-game counters stay, and a disagreement
-- between a counter and a count over these rows is a bug worth finding. What
-- the counters cannot do is carry the unit of analysis. rally_count and
-- rally_spread_sum reduce a game to two integers, so a game with 763 rallies
-- and a game with ONE produce means of equal apparent weight. Game 146's mean
-- spread of 17.0 is a single rally. Pooling at the rally, across games, is the
-- only honest way to ask whether a fix moved anything.
CREATE TABLE IF NOT EXISTS currie.stream_events
(
    session_id LowCardinality(String),
    tick       UInt32,
    kind       LowCardinality(String),   -- 'rally' | 'strike-blocked' | 'transit'
    squad      LowCardinality(String),
    reason     LowCardinality(String),   -- strike-blocked: which blocker

    -- members and spread mean the same thing for every kind that sets them.
    -- idle and near DO NOT: idle is the subset an ORDER CAN REACH (rally) --
    -- the roster minus whoever is retreating or held, which is exactly who the
    -- rally order is sent to. It is NOT a count of idle units and never was:
    -- squadAssaultActorIDs passes onlyIdle=false. The name is historical, kept
    -- because renaming it would orphan every row already shipped. Read it as
    -- `commandable`, which is what every query below calls it,
    -- near is the subset WITHIN squadRallyRadius OF THE CENTRE (transit).
    -- Two different measurements, so two columns -- sharing one is what forced
    -- strike_blocked_no_target to be split, and that could not be applied
    -- backwards. Each kind fills a subset; filter on kind before reading these.
    members    Int32,
    idle       Int32,                    -- rally
    near       Int32,                    -- transit
    spread     Int32,

    -- The next thing worth recording, without a migration -- and where the
    -- transit sampler's target_fraction lives: the squad's distance from its
    -- target as a fraction of the map diagonal. The in-memory sampler buckets
    -- that into three bands and sums them, so game 148's 118920 ticks reduce
    -- to 76 far / 5 mid / 0 near. Keeping the number means the bands are a
    -- choice made at query time, which is the only way to ask whether moving a
    -- threshold moved anything.
    attrs      Map(LowCardinality(String), Float64)
)
ENGINE = MergeTree
ORDER BY (session_id, kind, tick)
SETTINGS non_replicated_deduplication_window = 1000;

-- stream_units: where everything on the field was, ours and theirs.
--
-- Every diagnosis this telemetry has supported was made from scalars - spread,
-- members, a distance ratio - and more than one was read wrong. A squad that
-- looked like it was "arriving" was two stragglers. A sawtooth was blamed on
-- three different rules before the right one. An approach that looked like bad
-- routing turned out to be missing intel. Positions are what those scalars are
-- summaries of, and unlike the scalars they cannot be misread into a story.
--
-- Rows repeat per sample rather than being diffed. A unit that has not moved
-- costs almost nothing here: sorted by tick, x and y delta-encode away, and
-- stream_rule_evals already demonstrates the ratio - 10.8M rows in 21 MiB,
-- about two bytes each. A diff format would trade that for the ability to lose
-- a base state and desync an entire replay.
--
-- side, not owner, so the table reads without knowing which faction was which.
CREATE TABLE IF NOT EXISTS stream_units
(
    session_id LowCardinality(String),
    tick       UInt32,
    unit_id    UInt32,
    type       LowCardinality(String),
    side       LowCardinality(String),   -- 'ours' | 'enemy'
    x          UInt16,
    y          UInt16,
    hp         UInt16,
    idle       Bool,
    -- What holds ground versus what moves over it. A map without bases on it
    -- cannot be read.
    is_building Bool,
    -- Intel rather than sight. GameState.Enemies carries only what is visible
    -- this instant - a small minority of samples - while targeting and the
    -- approach router run off what the AI remembers. Drawing only the visible
    -- gives an empty enemy half of the map during a battle.
    remembered  Bool
)
ENGINE = MergeTree
-- tick first after the session: every question here is "what did the field look
-- like at time T", and a replay scans consecutive ticks.
ORDER BY (session_id, tick, unit_id)
SETTINGS non_replicated_deduplication_window = 1000;

-- stream_threat: the AI's own danger map, sparse.
--
-- This is the field BestApproachAxis scores corridors against, so it decides
-- whether an approach detours around the defences or drives straight in. It is
-- built from REMEMBERED defences, which means an unscouted base scores zero and
-- reads as safe: game 173 had observed one flame tower, every corridor scored
-- 0.00 against a threshold of 1.0, the detour switched itself off and the squad
-- walked into towers it had never seen. Establishing that took an evening and a
-- new counter. Drawn on the map it is a glance.
--
-- Sparse because most zones are empty and an empty field is the finding, not a
-- gap. Sampled far more coarsely than units: threat only moves when intel does.
CREATE TABLE IF NOT EXISTS stream_threat
(
    session_id LowCardinality(String),
    tick       UInt32,
    col        UInt16,
    row        UInt16,
    value      Float32
)
ENGINE = MergeTree
ORDER BY (session_id, tick, row, col)
SETTINGS non_replicated_deduplication_window = 1000;
