-- +goose Up
-- +goose StatementBegin
-- Where the state export for this game was written.
--
-- Currie replays a recorded game against the rule set that ran, which means
-- pairing an export file to the doctrines archived beside it. That pairing used
-- to be a human typing EXPORT= and GAME= into a test; this makes it a fact the
-- archive knows.
--
-- Nullable: games recorded without --export-states have no export, and games
-- archived before this migration have one nobody wrote down.
ALTER TABLE games ADD COLUMN export_path TEXT;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE games DROP COLUMN export_path;
-- +goose StatementEnd
