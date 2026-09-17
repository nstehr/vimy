-- +goose Up
-- Splits the old no-target counter, which could not distinguish a squad
-- standing on the enemy base seeing nothing from a squad that simply had not
-- arrived. Game 132 recorded 23 no-targets out of 27 attempts while its squad
-- was disengaging under a 5x army deficit, so most may have been the second —
-- and the two have entirely different fixes.
ALTER TABLE games ADD COLUMN strike_blocked_blind_at_base INTEGER;
ALTER TABLE games ADD COLUMN strike_blocked_en_route INTEGER;

-- +goose Down
ALTER TABLE games DROP COLUMN strike_blocked_blind_at_base;
ALTER TABLE games DROP COLUMN strike_blocked_en_route;
