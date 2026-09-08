-- +goose Up
-- +goose StatementBegin
-- fire_count records conditions that matched. act_count records the subset that
-- actually did something — sent an order, or moved something in memory.
--
-- Nullable with no default, deliberately: rows written before this migration
-- cannot know, and giving them 0 would make every one of them look like a rule
-- that matched and never acted. NULL says "not measured", which is true, and
-- keeps `act_count = 0` meaning what it says.
ALTER TABLE rule_firings ADD COLUMN act_count INTEGER;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE rule_firings DROP COLUMN act_count;
-- +goose StatementEnd
