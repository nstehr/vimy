-- +goose Up
-- How squads looked when told to re-gather.
--
-- Games 132 and 133 put 110 of 165 failed strikes on `unclumped` and zero on
-- every other cause, so squads never assemble. These three numbers separate
-- "the 8-cell radius is too tight" from "the rally order cannot reach most of
-- the squad", which have different fixes: squadIdleActorIDs commands only
-- IDLE members while SquadClumped judges ALL of them, and Idle means
-- actor.IsIdle, "has no current order".
ALTER TABLE games ADD COLUMN rally_count INTEGER;
ALTER TABLE games ADD COLUMN rally_members_sum INTEGER;
ALTER TABLE games ADD COLUMN rally_idle_sum INTEGER;
ALTER TABLE games ADD COLUMN rally_spread_sum INTEGER;

-- +goose Down
ALTER TABLE games DROP COLUMN rally_count;
ALTER TABLE games DROP COLUMN rally_members_sum;
ALTER TABLE games DROP COLUMN rally_idle_sum;
ALTER TABLE games DROP COLUMN rally_spread_sum;
