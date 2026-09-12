-- +goose Up
-- +goose StatementBegin
-- What the game cost the other side.
--
-- Losses were recorded from the first migration; kills were not recorded at
-- all, so a hundred and ten games say how much Vimy spent and nothing about
-- what it bought. The ~5:1 attrition that shaped a whole session of work could
-- not be read either way: trading evenly and being slaughtered leave identical
-- rows.
--
-- Confident and presumed are separate columns because they are different
-- claims. A confident kill was watched — one of our actors was still beside the
-- place it happened. A presumed one vanished into fog and may have walked away.
-- Summing them would produce a number that flatters every game.
--
-- Nullable: the hundred and ten games archived before this ran have no answer,
-- and zero would be a lie about them.
ALTER TABLE games ADD COLUMN enemy_units_killed INTEGER;
ALTER TABLE games ADD COLUMN enemy_buildings_killed INTEGER;
ALTER TABLE games ADD COLUMN enemy_units_presumed INTEGER;
ALTER TABLE games ADD COLUMN infantry_lost INTEGER;
ALTER TABLE games ADD COLUMN vehicles_lost INTEGER;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE games DROP COLUMN enemy_units_killed;
ALTER TABLE games DROP COLUMN enemy_buildings_killed;
ALTER TABLE games DROP COLUMN enemy_units_presumed;
ALTER TABLE games DROP COLUMN infantry_lost;
ALTER TABLE games DROP COLUMN vehicles_lost;
-- +goose StatementEnd
