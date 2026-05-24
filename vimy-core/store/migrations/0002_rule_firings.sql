-- +goose Up
-- +goose StatementBegin
ALTER TABLE archived_doctrines ADD COLUMN rule_set_json TEXT;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE rule_firings (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    doctrine_id INTEGER NOT NULL REFERENCES archived_doctrines(id),
    rule_name   TEXT NOT NULL,
    fire_count  INTEGER NOT NULL,
    first_tick  INTEGER NOT NULL,
    last_tick   INTEGER NOT NULL
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX idx_rule_firings_doctrine ON rule_firings(doctrine_id);
-- +goose StatementEnd

-- +goose Down
DROP INDEX IF EXISTS idx_rule_firings_doctrine;
DROP TABLE IF EXISTS rule_firings;
ALTER TABLE archived_doctrines DROP COLUMN rule_set_json;
