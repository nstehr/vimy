-- +goose Up
-- +goose StatementBegin
-- What the two armies were worth while the game was live.
--
-- engine_army_value is sampled at the end, when our army is already dead — it
-- reads 0 or 100 and describes nothing. These are the peak and the mean across
-- the game, beside the same for the enemy.
--
-- Theirs is summed from what we could SEE, priced by the engine's own Valued
-- trait, so it undercounts whatever sat in fog. The gap it measures is a floor
-- on the real one, which is the safe direction: the case being tested is that
-- Vimy fields a far cheaper army than it fights.
ALTER TABLE games ADD COLUMN our_army_peak INTEGER;
ALTER TABLE games ADD COLUMN our_army_mean INTEGER;
ALTER TABLE games ADD COLUMN enemy_army_seen_peak INTEGER;
ALTER TABLE games ADD COLUMN enemy_army_seen_mean INTEGER;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE games DROP COLUMN our_army_peak;
ALTER TABLE games DROP COLUMN our_army_mean;
ALTER TABLE games DROP COLUMN enemy_army_seen_peak;
ALTER TABLE games DROP COLUMN enemy_army_seen_mean;
-- +goose StatementEnd
