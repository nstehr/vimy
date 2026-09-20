-- +goose Up
-- What Vimy actually faced, by type, cumulative over the game.
--
-- The opponent faction predicts survival better than anything Vimy does:
-- against ukraine, 9 of 10 games ended inside 50000 ticks, averaging 40k;
-- against russia, 17 games averaged 80k. And Vimy does not adapt — air
-- defence priority 0.47 against ukraine versus 0.52 against russia, ground
-- defence 0.80 either way, AA built at the same rate per tick.
--
-- Why ukraine wins is unproven. The faction has its own airfield and the
-- engine's AI weights it heavily, which points at air, but that is inference
-- from config files rather than measurement, and inference has been wrong
-- repeatedly. The counts are already accumulated in memory by
-- GetEnemyUnitsSeen and GetEnemyBuildingsSeen; they have simply never been
-- archived, so "what beats us" cannot be asked across games.
ALTER TABLE games ADD COLUMN enemy_units_seen_json TEXT;
ALTER TABLE games ADD COLUMN enemy_buildings_seen_json TEXT;

-- +goose Down
ALTER TABLE games DROP COLUMN enemy_units_seen_json;
ALTER TABLE games DROP COLUMN enemy_buildings_seen_json;
