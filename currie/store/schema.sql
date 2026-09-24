-- Currie's cache.
--
-- Its own database, not a table in the archive. `vimy.db` runs in
-- journal_mode=delete with no busy timeout, so writing to it from here while a
-- game is being recorded would take a lock the sidecar needs. This is also
-- derived data — deletable and rebuildable — which does not belong in the
-- system of record.
CREATE TABLE IF NOT EXISTS insights (
    game_id      INTEGER NOT NULL,
    -- Fingerprint of the .vy sources the reading was made against. An archived
    -- game never changes, but a reading is about numbers the rules produced, so
    -- edited rules must miss rather than serve prose describing nothing.
    rules        TEXT NOT NULL,
    created_at   INTEGER NOT NULL,
    -- Lifted out of the JSON so a reading is queryable, which is the point of a
    -- table rather than a directory of files.
    summary      TEXT NOT NULL,
    suggestion   TEXT NOT NULL,
    insight_json TEXT NOT NULL,
    PRIMARY KEY (game_id, rules)
);

-- Investigations: the analyzer's tool loop, and what it looked at.
--
-- Separate from insights because they are not the same artefact. An insight is
-- one call over a fixed projection, so it is the same answer every time and
-- caching it is free. An investigation chooses its own path through the
-- telemetry, and is only stable once the telemetry is.
--
-- Hence `settled`: a row is written ONLY when the game is finished and its
-- stream has stopped arriving. A live game re-runs, because a second look at
-- more data is a different and probably better answer; a finished one is
-- answered once. Caching a partial reading forever is the outcome to avoid.
CREATE TABLE IF NOT EXISTS investigations (
    game_id    INTEGER NOT NULL,
    rules      TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    summary    TEXT NOT NULL,
    steps      INTEGER NOT NULL,
    body_json  TEXT NOT NULL,
    PRIMARY KEY (game_id, rules)
);
