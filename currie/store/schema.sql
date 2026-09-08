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
