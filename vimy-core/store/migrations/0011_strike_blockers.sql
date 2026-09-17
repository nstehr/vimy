-- +goose Up
-- Why a squad told to attack never shot a building.
--
-- Game 131 was the first where the army went out: 76 squads formed, 121 base
-- attacks issued, deaths finally forward rather than at home. It also made 34
-- assault-phase transitions and reached `strike` ZERO times, for the fifth
-- consecutive game with no buildings destroyed. The phase log records where a
-- squad got to, not what turned it back, and the four things that can turn it
-- back have four different fixes.
ALTER TABLE games ADD COLUMN strike_blocked_unclumped INTEGER;
ALTER TABLE games ADD COLUMN strike_blocked_no_target INTEGER;
ALTER TABLE games ADD COLUMN strike_blocked_not_building INTEGER;
ALTER TABLE games ADD COLUMN strike_blocked_out_of_reach INTEGER;

-- +goose Down
ALTER TABLE games DROP COLUMN strike_blocked_unclumped;
ALTER TABLE games DROP COLUMN strike_blocked_no_target;
ALTER TABLE games DROP COLUMN strike_blocked_not_building;
ALTER TABLE games DROP COLUMN strike_blocked_out_of_reach;
