-- +goose Up
-- +goose StatementBegin
-- The engine's own account of the game, beside the sidecar's inference of it.
--
-- Kills were being deduced from enemies vanishing from view, which cannot tell
-- a death from a walk into fog, and everything concluded from that — that Vimy
-- wins its fights three to one and loses the production race — rests on the
-- deduction. These columns are the engine's figures, so the two can be held
-- against each other and the inference calibrated or abandoned.
--
-- kills_cost and deaths_cost matter most. The trade has been read as a count of
-- units, and a medium tank and a rifleman are one unit each and 850 credits
-- apart: three-to-one by count can be losing badly by value.
--
-- Nullable: games archived before the mod reported any of this cannot answer.
ALTER TABLE games ADD COLUMN engine_units_killed INTEGER;
ALTER TABLE games ADD COLUMN engine_units_dead INTEGER;
ALTER TABLE games ADD COLUMN engine_buildings_killed INTEGER;
ALTER TABLE games ADD COLUMN engine_buildings_dead INTEGER;
ALTER TABLE games ADD COLUMN engine_kills_cost INTEGER;
ALTER TABLE games ADD COLUMN engine_deaths_cost INTEGER;
ALTER TABLE games ADD COLUMN engine_army_value INTEGER;
ALTER TABLE games ADD COLUMN engine_earned INTEGER;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE games DROP COLUMN engine_units_killed;
ALTER TABLE games DROP COLUMN engine_units_dead;
ALTER TABLE games DROP COLUMN engine_buildings_killed;
ALTER TABLE games DROP COLUMN engine_buildings_dead;
ALTER TABLE games DROP COLUMN engine_kills_cost;
ALTER TABLE games DROP COLUMN engine_deaths_cost;
ALTER TABLE games DROP COLUMN engine_army_value;
ALTER TABLE games DROP COLUMN engine_earned;
-- +goose StatementEnd
