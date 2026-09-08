-- +goose Up
-- +goose StatementBegin
-- The directive in force when this game was played.
--
-- The strategist turns a line of prose into doctrine weights, and those weights
-- decide which rules can fire. Analysis that stops at "lower infantry-weight"
-- is addressing a symptom: the weight came from somewhere. Recording the
-- directive is what lets a post mortem reach the sentence that caused it.
--
-- Nullable: games archived before this migration ran under a directive nobody
-- wrote down.
ALTER TABLE games ADD COLUMN directive TEXT;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE games DROP COLUMN directive;
-- +goose StatementEnd
