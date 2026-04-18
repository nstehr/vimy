-- +goose Up
-- +goose StatementBegin
CREATE TABLE games (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    played_at        INTEGER NOT NULL,
    our_faction      TEXT NOT NULL,
    opponent_faction TEXT,
    map_width        INTEGER NOT NULL,
    map_height       INTEGER NOT NULL,
    duration_ticks   INTEGER NOT NULL,
    won              INTEGER NOT NULL,
    quality_tag      TEXT,
    review_json      TEXT
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX idx_games_filter ON games(our_faction, opponent_faction, won);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE archived_doctrines (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    game_id        INTEGER NOT NULL REFERENCES games(id),
    tick           INTEGER NOT NULL,
    doctrine_json  TEXT NOT NULL,
    rating         TEXT,
    rating_reason  TEXT
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX idx_doctrines_game ON archived_doctrines(game_id);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE lessons (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    game_id         INTEGER NOT NULL REFERENCES games(id),
    created_at      INTEGER NOT NULL,
    trigger_text    TEXT NOT NULL,
    guidance_text   TEXT NOT NULL,
    confidence      REAL NOT NULL,
    applies_faction TEXT NOT NULL,
    applies_vs      TEXT
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX idx_lessons_filter ON lessons(applies_faction, applies_vs);
-- +goose StatementEnd

-- +goose Down
DROP TABLE IF EXISTS lessons;
DROP TABLE IF EXISTS archived_doctrines;
DROP TABLE IF EXISTS games;
